// Package files owns Atlas's document and evidence metadata and the pre-signed
// upload/download flow (docs/ATLAS.md §6.5).
//
// Bytes never flow through the API: a client asks Go for a pre-signed PUT URL,
// uploads straight to object storage, then tells Go the upload finished. Go
// records the metadata in core.files and, once the object is verified, emits
// core.file.uploaded.v1. Downloads are another pre-signed URL, issued only
// after the route's permission gate has run.
package files

import (
	"errors"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Sentinel errors. The HTTP layer maps these onto problem responses.
var (
	// ErrNotFound marks an unknown file id.
	ErrNotFound = errors.New("files: not found")
	// ErrInvalid marks a request that fails validation.
	ErrInvalid = errors.New("files: invalid")
	// ErrConflict marks a status transition that is not allowed (for example,
	// confirming an upload that was already archived).
	ErrConflict = errors.New("files: conflict")
	// ErrForbidden marks an action the caller may not take on this file.
	ErrForbidden = errors.New("files: forbidden")
	// ErrObjectMissing marks a confirmation whose object is not in storage.
	ErrObjectMissing = errors.New("files: object missing")
)

// MaxUploadBytes caps a single object. §6.5 leaves the exact cap to
// configuration; a constant keeps the Phase 1 value reviewable in one place.
const MaxUploadBytes int64 = 100 << 20 // 100 MiB

// Status mirrors the core.files.status check constraint.
type Status string

const (
	StatusPending     Status = "pending"
	StatusAvailable   Status = "available"
	StatusQuarantined Status = "quarantined"
	StatusDeleted     Status = "deleted"
)

// Classification mirrors the core.files.classification check constraint (§6.5).
type Classification string

const (
	Public       Classification = "public"
	Internal     Classification = "internal"
	Confidential Classification = "confidential"
	Restricted   Classification = "restricted"
)

// Valid reports whether c is one of the four levels.
func (c Classification) Valid() bool {
	switch c {
	case Public, Internal, Confidential, Restricted:
		return true
	}
	return false
}

// File is a row in core.files.
type File struct {
	ID             uuid.UUID
	Bucket         string
	ObjectKey      string
	Filename       string
	ContentType    string
	SizeBytes      int64
	ChecksumSHA256 string
	Classification Classification
	Status         Status
	UploadedBy     *uuid.UUID
	EntityType     string
	EntityID       *uuid.UUID
	CreatedAt      time.Time
	UpdatedAt      time.Time
	ArchivedAt     *time.Time
}

// UploadRequest is a client's request to start an upload.
type UploadRequest struct {
	Filename       string
	ContentType    string
	SizeBytes      int64
	Classification Classification
	EntityType     string
	EntityID       *uuid.UUID
	ChecksumSHA256 string
}

// Normalized validates the request and fills defaults, returning a copy fit to
// persist. The filename is reduced to a base name so a caller cannot smuggle
// path segments into the object key.
func (u UploadRequest) Normalized() (UploadRequest, error) {
	u.Filename = path.Base(strings.ReplaceAll(strings.TrimSpace(u.Filename), "\\", "/"))
	if u.Filename == "" || u.Filename == "." || u.Filename == ".." || u.Filename == "/" {
		return u, fmt.Errorf("%w: filename is required", ErrInvalid)
	}
	if len(u.Filename) > 255 {
		return u, fmt.Errorf("%w: filename is too long", ErrInvalid)
	}

	u.ContentType = strings.TrimSpace(u.ContentType)
	if u.ContentType == "" {
		u.ContentType = "application/octet-stream"
	}

	if u.SizeBytes <= 0 {
		return u, fmt.Errorf("%w: size_bytes must be positive", ErrInvalid)
	}
	if u.SizeBytes > MaxUploadBytes {
		return u, fmt.Errorf("%w: size_bytes exceeds the %d byte limit", ErrInvalid, MaxUploadBytes)
	}

	if u.Classification == "" {
		u.Classification = Internal
	}
	if !u.Classification.Valid() {
		return u, fmt.Errorf("%w: classification must be public|internal|confidential|restricted", ErrInvalid)
	}

	u.EntityType = strings.TrimSpace(u.EntityType)
	if (u.EntityType == "") != (u.EntityID == nil) {
		return u, fmt.Errorf("%w: entity_type and entity_id must be provided together", ErrInvalid)
	}
	u.ChecksumSHA256 = strings.ToLower(strings.TrimSpace(u.ChecksumSHA256))
	return u, nil
}

// ObjectKey builds the storage key for a file: a date prefix for operators, the
// id (never the client's name) for uniqueness, then the sanitized name for
// readability in a bucket listing.
func ObjectKey(id uuid.UUID, filename string, at time.Time) string {
	return fmt.Sprintf("%s/%s/%s", at.UTC().Format("2006/01/02"), id.String(), safeName(filename))
}

func safeName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "file"
	}
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '.' || r == '-' || r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	if out := strings.Trim(b.String(), " ."); out != "" {
		return out
	}
	return "file"
}

// UploadTicket is what the API returns so a client can PUT the object straight
// to storage: where to send it, with what method, and which headers the
// signature covers (the client must send them verbatim).
type UploadTicket struct {
	FileID    uuid.UUID
	UploadURL string
	Method    string
	Headers   map[string]string
	ExpiresAt time.Time
}

// DownloadTicket is a short-lived pre-signed GET URL.
type DownloadTicket struct {
	URL       string
	ExpiresAt time.Time
}

// ObjectInfo is what storage reports about a stored object.
type ObjectInfo struct {
	SizeBytes      int64
	ContentType    string
	ChecksumSHA256 string
	ETag           string
}

// ListQuery filters a list of files. A zero value lists every file that is not
// deleted, newest first.
type ListQuery struct {
	EntityType string
	EntityID   *uuid.UUID
	UploadedBy *uuid.UUID
	Status     Status
}

func uuidOrZero(p *uuid.UUID) uuid.UUID {
	if p == nil {
		return uuid.UUID{}
	}
	return *p
}
