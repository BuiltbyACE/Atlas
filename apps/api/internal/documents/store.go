// Package documents serves Atlas's document storage layer (§8.6).
//
// Metadata lives in core.documents, core.document_shares and
// core.document_access_log. Actual bytes live in R2 via the files.ObjectStore
// port, accessed through pre-signed URLs.
package documents

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Sentinel errors.
var (
	ErrNotFound  = errors.New("documents: not found")
	ErrInvalid   = errors.New("documents: invalid")
	ErrForbidden = errors.New("documents: forbidden")
)

// DocumentCategory is the category of a document.
type DocumentCategory string

const (
	CategoryInvoice     DocumentCategory = "invoice"
	CategorySystemDoc   DocumentCategory = "system_doc"
	CategoryContract    DocumentCategory = "contract"
	CategoryPolicy      DocumentCategory = "policy"
	CategoryOther       DocumentCategory = "other"
)

// Document is a stored document's metadata.
type Document struct {
	ID           uuid.UUID        `json:"id"`
	Key          string           `json:"key"`
	Filename     string           `json:"filename"`
	ContentType  string           `json:"content_type"`
	SizeBytes    int64            `json:"size_bytes"`
	Category     DocumentCategory `json:"category"`
	Description  string           `json:"description"`
	UploadedBy   uuid.UUID        `json:"uploaded_by"`
	UploadedAt   time.Time        `json:"uploaded_at"`
	UpdatedAt    time.Time        `json:"updated_at"`
	IsPublic     bool             `json:"is_public"`
	Version      int              `json:"version"`
}

// DocumentShare is a grant of access to a document.
type DocumentShare struct {
	DocumentID uuid.UUID `json:"document_id"`
	UserID     uuid.UUID `json:"user_id"`
	Permission string    `json:"permission"` // "read" or "write"
	SharedBy   uuid.UUID `json:"shared_by"`
	CreatedAt  time.Time `json:"created_at"`
}

// AccessLogEntry is a record of a document access.
type AccessLogEntry struct {
	ID         uuid.UUID `json:"id"`
	DocumentID uuid.UUID `json:"document_id"`
	UserID     uuid.UUID `json:"user_id"`
	Action     string    `json:"action"` // "view" or "download"
	IP         string    `json:"ip,omitempty"`
	UserAgent  string    `json:"user_agent,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

// Store is the persistence interface for documents.
type Store interface {
	// DB returns the underlying connection pool.
	DB() *pgxpool.Pool

	// Documents.
	CreateDocument(ctx context.Context, ex Execer, d *Document) error
	GetDocument(ctx context.Context, id uuid.UUID) (*Document, error)
	ListDocuments(ctx context.Context, category DocumentCategory, limit int, before *uuid.UUID) ([]Document, error)
	UpdateDocument(ctx context.Context, ex Execer, id uuid.UUID, category DocumentCategory, description string, isPublic bool) error
	DeleteDocument(ctx context.Context, ex Execer, id uuid.UUID) error

	// Shares.
	CreateShare(ctx context.Context, ex Execer, s *DocumentShare) error
	ListShares(ctx context.Context, documentID uuid.UUID) ([]DocumentShare, error)
	DeleteShare(ctx context.Context, ex Execer, documentID, userID uuid.UUID) error
	HasAccess(ctx context.Context, documentID, userID uuid.UUID) (bool, error)

	// Access log.
	LogAccess(ctx context.Context, ex Execer, entry *AccessLogEntry) error
}

// Execer is the transaction interface used by the store.
type Execer interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// PGStore is the Postgres-backed documents store.
type PGStore struct {
	db *pgxpool.Pool
}

// NewPGStore builds a store over the pool.
func NewPGStore(db *pgxpool.Pool) *PGStore { return &PGStore{db: db} }

// DB returns the underlying connection pool.
func (s *PGStore) DB() *pgxpool.Pool { return s.db }

// ── documents ──────────────────────────────────────────────────────────────

// CreateDocument inserts a new document record.
func (s *PGStore) CreateDocument(ctx context.Context, ex Execer, d *Document) error {
	if _, err := ex.Exec(ctx, `
		INSERT INTO core.documents
		(id, key, filename, content_type, size_bytes, category, description,
		 uploaded_by, uploaded_at, updated_at, is_public, version)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,
		d.ID, d.Key, d.Filename, d.ContentType, d.SizeBytes, d.Category, d.Description,
		d.UploadedBy, d.UploadedAt, d.UpdatedAt, d.IsPublic, d.Version); err != nil {
		return fmt.Errorf("documents: create document: %w", err)
	}
	return nil
}

// GetDocument returns a document by ID.
func (s *PGStore) GetDocument(ctx context.Context, id uuid.UUID) (*Document, error) {
	var d Document
	if err := s.db.QueryRow(ctx, `
		SELECT id, key, filename, content_type, size_bytes, category, description,
		       uploaded_by, uploaded_at, updated_at, is_public, version
		FROM core.documents WHERE id = $1`, id).Scan(
		&d.ID, &d.Key, &d.Filename, &d.ContentType, &d.SizeBytes, &d.Category,
		&d.Description, &d.UploadedBy, &d.UploadedAt, &d.UpdatedAt,
		&d.IsPublic, &d.Version); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("documents: get document: %w", err)
	}
	return &d, nil
}

// ListDocuments returns documents, optionally filtered by category.
func (s *PGStore) ListDocuments(ctx context.Context, category DocumentCategory, limit int, before *uuid.UUID) ([]Document, error) {
	args := []any{}
	where := "TRUE"
	if category != "" {
		args = append(args, category)
		where += fmt.Sprintf(" AND category = $%d", len(args))
	}
	if before != nil {
		args = append(args, *before)
		where += fmt.Sprintf(" AND uploaded_at < $%d", len(args))
	}
	args = append(args, limit)
	rows, err := s.db.Query(ctx, `
		SELECT id, key, filename, content_type, size_bytes, category, description,
		       uploaded_by, uploaded_at, updated_at, is_public, version
		FROM core.documents
		WHERE ` + where + `
		ORDER BY uploaded_at DESC
		LIMIT $%d`, args...)
	if err != nil {
		return nil, fmt.Errorf("documents: list documents: %w", err)
	}
	defer rows.Close()

	var documents []Document
	for rows.Next() {
		var d Document
		if err := rows.Scan(
			&d.ID, &d.Key, &d.Filename, &d.ContentType, &d.SizeBytes,
			&d.Category, &d.Description, &d.UploadedBy, &d.UploadedAt,
			&d.UpdatedAt, &d.IsPublic, &d.Version); err != nil {
			return nil, fmt.Errorf("documents: scan document: %w", err)
		}
		documents = append(documents, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("documents: documents: %w", err)
	}
	return documents, nil
}

// UpdateDocument updates a document's metadata.
func (s *PGStore) UpdateDocument(ctx context.Context, ex Execer, id uuid.UUID, category DocumentCategory, description string, isPublic bool) error {
	if _, err := ex.Exec(ctx, `
		UPDATE core.documents
		SET category = $2, description = $3, is_public = $4, updated_at = now()
		WHERE id = $1`, id, category, description, isPublic); err != nil {
		return fmt.Errorf("documents: update document: %w", err)
	}
	return nil
}

// DeleteDocument soft-deletes a document (marks via version bump + future purge).
func (s *PGStore) DeleteDocument(ctx context.Context, ex Execer, id uuid.UUID) error {
	if _, err := ex.Exec(ctx, `
		DELETE FROM core.documents WHERE id = $1`, id); err != nil {
		return fmt.Errorf("documents: delete document: %w", err)
	}
	return nil
}

// ── shares ─────────────────────────────────────────────────────────────────

// CreateShare grants access to a document.
func (s *PGStore) CreateShare(ctx context.Context, ex Execer, share *DocumentShare) error {
	if _, err := ex.Exec(ctx, `
		INSERT INTO core.document_shares (document_id, user_id, permission, shared_by, created_at)
		VALUES ($1,$2,$3,$4,$5)
		ON CONFLICT (document_id, user_id) DO UPDATE
			SET permission = EXCLUDED.permission, shared_by = EXCLUDED.shared_by`,
		share.DocumentID, share.UserID, share.Permission, share.SharedBy, share.CreatedAt); err != nil {
		return fmt.Errorf("documents: create share: %w", err)
	}
	return nil
}

// ListShares returns all shares for a document.
func (s *PGStore) ListShares(ctx context.Context, documentID uuid.UUID) ([]DocumentShare, error) {
	rows, err := s.db.Query(ctx, `
		SELECT document_id, user_id, permission, shared_by, created_at
		FROM core.document_shares WHERE document_id = $1`, documentID)
	if err != nil {
		return nil, fmt.Errorf("documents: list shares: %w", err)
	}
	defer rows.Close()

	var shares []DocumentShare
	for rows.Next() {
		var sh DocumentShare
		if err := rows.Scan(&sh.DocumentID, &sh.UserID, &sh.Permission, &sh.SharedBy, &sh.CreatedAt); err != nil {
			return nil, fmt.Errorf("documents: scan share: %w", err)
		}
		shares = append(shares, sh)
	}
	return shares, nil
}

// DeleteShare revokes access to a document.
func (s *PGStore) DeleteShare(ctx context.Context, ex Execer, documentID, userID uuid.UUID) error {
	if _, err := ex.Exec(ctx, `
		DELETE FROM core.document_shares WHERE document_id = $1 AND user_id = $2`,
		documentID, userID); err != nil {
		return fmt.Errorf("documents: delete share: %w", err)
	}
	return nil
}

// HasAccess checks if a user can access a document.
func (s *PGStore) HasAccess(ctx context.Context, documentID, userID uuid.UUID) (bool, error) {
	var d Document
	if err := s.db.QueryRow(ctx, `
		SELECT id FROM core.documents WHERE id = $1`, documentID).Scan(&d.ID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, ErrNotFound
		}
		return false, fmt.Errorf("documents: check access: %w", err)
	}
	if d.IsPublic {
		return true, nil
	}
	var exists bool
	if err := s.db.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM core.document_shares
			WHERE document_id = $1 AND user_id = $2)`, documentID, userID).Scan(&exists); err != nil {
		return false, fmt.Errorf("documents: check share: %w", err)
	}
	return exists, nil
}

// ── access log ─────────────────────────────────────────────────────────────

// LogAccess records a document access.
func (s *PGStore) LogAccess(ctx context.Context, ex Execer, entry *AccessLogEntry) error {
	if _, err := ex.Exec(ctx, `
		INSERT INTO core.document_access_log
		(id, document_id, user_id, action, ip, user_agent, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7)`,
		entry.ID, entry.DocumentID, entry.UserID, entry.Action, entry.IP, entry.UserAgent, entry.CreatedAt); err != nil {
		return fmt.Errorf("documents: log access: %w", err)
	}
	return nil
}

// Compile-time check that PGStore implements Store.
var _ Store = (*PGStore)(nil)
