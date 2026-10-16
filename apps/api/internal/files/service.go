package files

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/safaristack/atlas/apps/api/internal/audit"
	"github.com/safaristack/atlas/apps/api/internal/outbox"
)

// Config tunes the service. Bucket must match the object-storage adapter's.
type Config struct {
	Bucket      string
	UploadTTL   time.Duration
	DownloadTTL time.Duration
}

// Service coordinates metadata, storage and events for files.
type Service struct {
	store   Store
	objects ObjectStore
	scanner Scanner
	audit   *audit.Writer
	bucket  string
	upload  time.Duration
	downld  time.Duration
	now     func() time.Time
	logger  *slog.Logger
}

// NewService builds the file service. aw may be nil (tests); when it is,
// changes are still recorded but not written to the global audit log.
func NewService(store Store, objects ObjectStore, scanner Scanner, aw *audit.Writer, cfg Config, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	if scanner == nil {
		scanner = NoopScanner{}
	}
	if cfg.UploadTTL <= 0 {
		cfg.UploadTTL = 15 * time.Minute
	}
	if cfg.DownloadTTL <= 0 {
		cfg.DownloadTTL = 5 * time.Minute
	}
	return &Service{
		store:   store,
		objects: objects,
		scanner: scanner,
		audit:   aw,
		bucket:  cfg.Bucket,
		upload:  cfg.UploadTTL,
		downld:  cfg.DownloadTTL,
		now:     func() time.Time { return time.Now().UTC() },
		logger:  logger,
	}
}

// RequestUpload validates a request, records a pending file, and returns a
// pre-signed PUT URL the client uploads to directly.
func (s *Service) RequestUpload(ctx context.Context, actor uuid.UUID, req UploadRequest) (File, UploadTicket, error) {
	norm, err := req.Normalized()
	if err != nil {
		return File{}, UploadTicket{}, err
	}

	id, err := uuid.NewV7()
	if err != nil {
		return File{}, UploadTicket{}, fmt.Errorf("files: new id: %w", err)
	}
	now := s.now()
	f := File{
		ID:             id,
		Bucket:         s.bucket,
		ObjectKey:      ObjectKey(id, norm.Filename, now),
		Filename:       norm.Filename,
		ContentType:    norm.ContentType,
		SizeBytes:      norm.SizeBytes,
		ChecksumSHA256: norm.ChecksumSHA256,
		Classification: norm.Classification,
		Status:         StatusPending,
		UploadedBy:     &actor,
		EntityType:     norm.EntityType,
		EntityID:       norm.EntityID,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := s.store.Create(ctx, f); err != nil {
		return File{}, UploadTicket{}, err
	}

	url, headers, err := s.objects.PresignPut(ctx, f.ObjectKey, f.ContentType, s.upload)
	if err != nil {
		return File{}, UploadTicket{}, err
	}
	return f, UploadTicket{
		FileID:    f.ID,
		UploadURL: url,
		Method:    http.MethodPut,
		Headers:   headers,
		ExpiresAt: now.Add(s.upload),
	}, nil
}

// CompleteUpload verifies the object is in storage, records the verified size
// and content type, and marks the file available (or quarantined, if the
// scanner objects). Confirming an available file again is a no-op.
func (s *Service) CompleteUpload(ctx context.Context, actor uuid.UUID, id uuid.UUID) (File, error) {
	f, err := s.store.Get(ctx, id)
	if err != nil {
		return File{}, err
	}
	if f == nil {
		return File{}, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	switch f.Status {
	case StatusAvailable, StatusQuarantined:
		return *f, nil // idempotent
	case StatusPending:
		// continue
	default:
		return File{}, fmt.Errorf("%w: file is %s", ErrConflict, f.Status)
	}

	info, err := s.objects.Stat(ctx, f.ObjectKey)
	if err != nil {
		return File{}, err
	}
	if info.SizeBytes > MaxUploadBytes {
		return File{}, fmt.Errorf("%w: uploaded object exceeds the %d byte limit", ErrInvalid, MaxUploadBytes)
	}
	contentType := info.ContentType
	if info.ContentType == "" {
		contentType = f.ContentType
	}

	status := StatusAvailable
	scan, err := s.scanner.Scan(ctx, f.ObjectKey)
	if err != nil {
		return File{}, err
	}
	if !scan.Clean {
		status = StatusQuarantined
	}

	var ev *outbox.Event
	if status == StatusAvailable {
		built, err := outbox.NewFileUploaded(&actor, outbox.FileRef{
			ID:             id,
			Filename:       f.Filename,
			ContentType:    info.ContentType,
			SizeBytes:      info.SizeBytes,
			Classification: string(f.Classification),
			EntityType:     f.EntityType,
			EntityID:       uuidOrZero(f.EntityID),
			UploadedBy:     actor,
		})
		if err != nil {
			return File{}, err
		}
		ev = &built
	}

	updated, err := s.store.Complete(ctx, id, ObjectInfo{
		SizeBytes:      info.SizeBytes,
		ContentType:    contentType,
		ChecksumSHA256: f.ChecksumSHA256,
	}, status, ev)
	if err != nil {
		return File{}, err
	}

	s.writeAuditUploaded(ctx, actor, updated)
	return *updated, nil
}

// DownloadURL issues a short-lived pre-signed GET URL for an available file.
func (s *Service) DownloadURL(ctx context.Context, id uuid.UUID) (DownloadTicket, error) {
	f, err := s.store.Get(ctx, id)
	if err != nil {
		return DownloadTicket{}, err
	}
	if f == nil {
		return DownloadTicket{}, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	if f.Status != StatusAvailable {
		return DownloadTicket{}, fmt.Errorf("%w: file is %s", ErrConflict, f.Status)
	}
	url, err := s.objects.PresignGet(ctx, f.ObjectKey, f.Filename, s.downld)
	if err != nil {
		return DownloadTicket{}, err
	}
	return DownloadTicket{URL: url, ExpiresAt: s.now().Add(s.downld)}, nil
}

// Get returns a file's metadata.
func (s *Service) Get(ctx context.Context, id uuid.UUID) (File, error) {
	f, err := s.store.Get(ctx, id)
	if err != nil {
		return File{}, err
	}
	if f == nil {
		return File{}, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	return *f, nil
}

// List returns a page of files, newest first.
func (s *Service) List(ctx context.Context, q ListQuery, limit int, before *Cursor) ([]File, error) {
	if limit <= 0 {
		limit = 50
	}
	return s.store.List(ctx, q, limit, before)
}

// Archive soft-deletes a file (metadata only; the object is purged later by the
// retention job). Only the uploader may delete their own file.
func (s *Service) Archive(ctx context.Context, actor uuid.UUID, id uuid.UUID, reason string) error {
	f, err := s.store.Get(ctx, id)
	if err != nil {
		return err
	}
	if f == nil {
		return fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	if f.UploadedBy == nil || *f.UploadedBy != actor {
		return fmt.Errorf("%w: only the uploader may delete a file", ErrForbidden)
	}

	ev, err := outbox.NewFileDeleted(&actor, id, reason, actor)
	if err != nil {
		return err
	}
	if _, err := s.store.Archive(ctx, id, &ev); err != nil {
		return err
	}
	s.writeAudit(ctx, actor, "core.file.deleted", id, nil, map[string]any{
		"file_id": id.String(), "reason": reason,
	})
	return nil
}

func (s *Service) writeAuditUploaded(ctx context.Context, actor uuid.UUID, f *File) {
	s.writeAudit(ctx, actor, "core.file.uploaded", f.ID, nil, map[string]any{
		"filename":       f.Filename,
		"content_type":   f.ContentType,
		"size_bytes":     f.SizeBytes,
		"classification": string(f.Classification),
		"entity_type":    f.EntityType,
		"entity_id":      uuidOrZero(f.EntityID).String(),
	})
}

func (s *Service) writeAudit(ctx context.Context, actor uuid.UUID, action string, id uuid.UUID, before, after any) {
	if s.audit == nil {
		return
	}
	entity := id
	if err := s.audit.Write(ctx, nil, audit.Event{
		ActorType:  audit.ActorUser,
		ActorID:    actor.String(),
		Action:     action,
		EntityType: "file",
		EntityID:   &entity,
		Before:     before,
		After:      after,
	}); err != nil {
		// The change is committed; a failed audit write must not report the
		// change as failed (§6.3). Log it loudly instead.
		s.logger.ErrorContext(ctx, "files: audit write failed",
			slog.String("action", action), slog.String("error", err.Error()))
	}
}
