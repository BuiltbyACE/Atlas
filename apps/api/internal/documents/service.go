// Package documents serves Atlas's document storage layer (§8.6).
package documents

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/safaristack/atlas/apps/api/internal/outbox"
)

// ObjectStore is the subset of files.ObjectStore that documents needs:
// presigned URLs for upload and download to R2.
type ObjectStore interface {
	PresignPut(ctx context.Context, key, contentType string, ttl time.Duration) (string, map[string]string, error)
	PresignGet(ctx context.Context, key, downloadName string, ttl time.Duration) (string, error)
}

// Service coordinates document metadata and access-controlled delivery.
type Service struct {
	store    Store
	objects  ObjectStore // nil when no bucket configured (uploads disabled)
	audit  auditWriter // optional audit trail
	logger   *slog.Logger
	now      func() time.Time
	uploadTTL time.Duration
	dlTTL    time.Duration

	outbox func(ctx context.Context, e outbox.Event) error
}

// auditWriter is a minimal subset of audit.Writer to avoid importing audit.
type auditWriter interface {
	Write(ctx context.Context, actorID uuid.UUID, action string, entityType string, entityID uuid.UUID, before, after interface{}) error
}

// NewService builds the documents service. objects may be nil.
func NewService(store Store, objects ObjectStore, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{
		store:    store,
		objects:  objects,
		logger:   logger,
		now:      time.Now,
		uploadTTL: 15 * time.Minute,
		dlTTL:    5 * time.Minute,
	}
}

// WithOutbox wires the outbox event writer.
func (s *Service) WithOutbox(write func(ctx context.Context, e outbox.Event) error) *Service {
	s.outbox = write
	return s
}

// WithAuditWriter wires the audit trail.
func (s *Service) WithAuditWriter(w auditWriter) *Service {
	s.audit = w
	return s
}

// WithTTLs sets the presigned URL lifetimes.
func (s *Service) WithTTLs(upload, download time.Duration) *Service {
	s.uploadTTL = upload
	s.dlTTL = download
	return s
}

// UploadOptions is input for registering a document upload.
type UploadOptions struct {
	UploadedBy uuid.UUID
	Filename   string
	ContentType string
	SizeBytes  int64
	Category   DocumentCategory
	Description string
}

// UploadResult is returned by PrepareUpload: a presigned PUT URL and document metadata.
type UploadResult struct {
	Document   *Document `json:"document"`
	UploadURL  string    `json:"upload_url"`
	Headers    map[string]string `json:"headers,omitempty"`
}

// PrepareUpload creates a document record and returns a presigned PUT URL.
// The client PUTs the file to R2 directly using the returned URL.
func (s *Service) PrepareUpload(ctx context.Context, opts UploadOptions) (*UploadResult, error) {
	if opts.Filename == "" {
		return nil, ErrInvalid
	}
	if opts.ContentType == "" {
		return nil, ErrInvalid
	}

	id, err := uuid.NewV7()
	if err != nil {
		return nil, fmt.Errorf("documents: id: %w", err)
	}
	key := fmt.Sprintf("documents/%s/%s", id, opts.Filename)

	now := s.now()
	doc := &Document{
		ID:          id,
		Key:         key,
		Filename:    opts.Filename,
		ContentType: opts.ContentType,
		SizeBytes:   opts.SizeBytes,
		Category:    opts.Category,
		Description: opts.Description,
		UploadedBy:  opts.UploadedBy,
		UploadedAt:  now,
		UpdatedAt:   now,
		IsPublic:    false,
		Version:     1,
	}

	tx, err := s.store.DB().Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("documents: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := s.store.CreateDocument(ctx, tx, doc); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("documents: commit tx: %w", err)
	}

	var uploadURL string
	var headers map[string]string
	if s.objects != nil {
		uploadURL, headers, err = s.objects.PresignPut(ctx, key, opts.ContentType, s.uploadTTL)
		if err != nil {
			s.logger.Warn("documents: presign put failed", slog.String("error", err.Error()))
		}
	}

	if s.outbox != nil {
		ev, err := outboxEvent("core.document.created.v1", "document", id,
			map[string]any{"document_id": id, "category": opts.Category, "uploaded_by": opts.UploadedBy})
		if err == nil {
			if err := s.outbox(ctx, ev); err != nil {
				s.logger.Warn("documents: outbox event failed", slog.String("error", err.Error()))
			}
		}
	}

	return &UploadResult{
		Document: doc,
		UploadURL: uploadURL,
		Headers:   headers,
	}, nil
}

// DownloadOptions is input for downloading a document.
type DownloadOptions struct {
	DocumentID uuid.UUID
	UserID     uuid.UUID
	IP         string
	UserAgent  string
}

// DownloadResult is returned by PrepareDownload: a presigned GET URL.
type DownloadResult struct {
	Document    *Document `json:"document"`
	DownloadURL string    `json:"download_url"`
}

// PrepareDownload returns a presigned download URL for a document.
// Logs the access and checks permissions.
func (s *Service) PrepareDownload(ctx context.Context, opts DownloadOptions) (*DownloadResult, error) {
	doc, err := s.store.GetDocument(ctx, opts.DocumentID)
	if err != nil {
		return nil, err
	}

	ok, err := s.store.HasAccess(ctx, opts.DocumentID, opts.UserID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrForbidden
	}

	// Log access
	entry := &AccessLogEntry{
		ID:         uuid.Nil,
		DocumentID: opts.DocumentID,
		UserID:     opts.UserID,
		Action:     "download",
		IP:         opts.IP,
		UserAgent:  opts.UserAgent,
		CreatedAt:  s.now(),
	}
	entry.ID, _ = uuid.NewV7()
	if err := s.store.LogAccess(ctx, s.store.DB(), entry); err != nil {
		s.logger.Warn("documents: log access failed", slog.String("error", err.Error()))
	}

	var downloadURL string
	if s.objects != nil {
		downloadURL, err = s.objects.PresignGet(ctx, doc.Key, doc.Filename, s.dlTTL)
		if err != nil {
			return nil, fmt.Errorf("documents: presign get: %w", err)
		}
	}

	return &DownloadResult{
		Document:    doc,
		DownloadURL: downloadURL,
	}, nil
}

// ListOptions is input for listing documents.
type ListOptions struct {
	Category DocumentCategory
	Limit    int
	Before   *uuid.UUID
}

// ListDocuments returns documents matching the options.
func (s *Service) ListDocuments(ctx context.Context, opts ListOptions) ([]Document, error) {
	return s.store.ListDocuments(ctx, opts.Category, opts.Limit, opts.Before)
}

// GetDocument returns document metadata.
func (s *Service) GetDocument(ctx context.Context, id uuid.UUID) (*Document, error) {
	return s.store.GetDocument(ctx, id)
}

// UpdateDocumentOptions is input for updating document metadata.
type UpdateDocumentOptions struct {
	Category   DocumentCategory
	Description string
	IsPublic   bool
}

// UpdateDocument updates document metadata.
func (s *Service) UpdateDocument(ctx context.Context, id uuid.UUID, opts UpdateDocumentOptions) error {
	return s.store.UpdateDocument(ctx, s.store.DB(), id, opts.Category, opts.Description, opts.IsPublic)
}

// DeleteDocument deletes a document record.
func (s *Service) DeleteDocument(ctx context.Context, id uuid.UUID) error {
	return s.store.DeleteDocument(ctx, s.store.DB(), id)
}

// ShareDocument grants access to a document.
func (s *Service) ShareDocument(ctx context.Context, documentID, userID, sharedBy uuid.UUID, permission string) error {
	share := &DocumentShare{
		DocumentID: documentID,
		UserID:     userID,
		Permission: permission,
		SharedBy:   sharedBy,
		CreatedAt:  s.now(),
	}
	return s.store.CreateShare(ctx, s.store.DB(), share)
}

// ListShares returns all shares for a document.
func (s *Service) ListShares(ctx context.Context, documentID uuid.UUID) ([]DocumentShare, error) {
	return s.store.ListShares(ctx, documentID)
}

// outboxEvent builds an outbox event.
func outboxEvent(eventType, aggregateType string, aggregateID uuid.UUID, payload map[string]any) (outbox.Event, error) {
	b, err := jsonMarshal(payload)
	if err != nil {
		return outbox.Event{}, fmt.Errorf("documents: marshal outbox payload: %w", err)
	}
	return outbox.Event{
		Type:          eventType,
		AggregateType: aggregateType,
		AggregateID:   aggregateID,
		Payload:       b,
	}, nil
}

var jsonMarshal = func(v any) ([]byte, error) {
	return json.Marshal(v)
}
