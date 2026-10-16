package files

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/safaristack/atlas/apps/api/internal/outbox"
)

// Cursor is a keyset position in a file listing.
type Cursor struct {
	At time.Time
	ID uuid.UUID
}

// Store is the file-metadata persistence interface.
type Store interface {
	// Create inserts a new file row (normally in the pending state).
	Create(ctx context.Context, f File) error
	// Get returns one file, or (nil, nil) when no row exists.
	Get(ctx context.Context, id uuid.UUID) (*File, error)
	// List returns a page of files, newest first.
	List(ctx context.Context, q ListQuery, limit int, before *Cursor) ([]File, error)
	// Complete moves a pending file to status and, when ev is non-nil, writes
	// the outbox event in the same transaction.
	Complete(ctx context.Context, id uuid.UUID, in ObjectInfo, status Status, ev *outbox.Event) (*File, error)
	// Archive marks a file deleted and, when ev is non-nil, writes the outbox
	// event in the same transaction.
	Archive(ctx context.Context, id uuid.UUID, ev *outbox.Event) (*File, error)
}

// PGStore implements Store over Postgres.
type PGStore struct{ db *pgxpool.Pool }

// NewPGStore builds a store over the pool.
func NewPGStore(db *pgxpool.Pool) *PGStore { return &PGStore{db: db} }

// rowScanner is satisfied by both pgx.Row and pgx.Rows.
type rowScanner interface {
	Scan(dest ...any) error
}

const fileCols = `id, bucket, object_key, filename, content_type, size_bytes,
	COALESCE(checksum_sha256,''), classification, status, uploaded_by,
	entity_type, entity_id, created_at, updated_at, archived_at`

func scanFile(row rowScanner) (File, error) {
	var (
		f             File
		class, status string
		uploadedBy    *uuid.UUID
		entityType    *string
		entityID      *uuid.UUID
	)
	if err := row.Scan(
		&f.ID, &f.Bucket, &f.ObjectKey, &f.Filename, &f.ContentType, &f.SizeBytes,
		&f.ChecksumSHA256, &class, &status, &uploadedBy, &entityType, &entityID,
		&f.CreatedAt, &f.UpdatedAt, &f.ArchivedAt,
	); err != nil {
		return f, err
	}
	f.Classification = Classification(class)
	f.Status = Status(status)
	f.UploadedBy = uploadedBy
	f.EntityID = entityID
	if entityType != nil {
		f.EntityType = *entityType
	}
	return f, nil
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func (s *PGStore) Create(ctx context.Context, f File) error {
	_, err := s.db.Exec(ctx, `
		INSERT INTO core.files (
			id, bucket, object_key, filename, content_type, size_bytes,
			checksum_sha256, classification, status, uploaded_by,
			entity_type, entity_id, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`,
		f.ID, f.Bucket, f.ObjectKey, f.Filename, f.ContentType, f.SizeBytes,
		nullIfEmpty(f.ChecksumSHA256), string(f.Classification), string(f.Status),
		f.UploadedBy, nullIfEmpty(f.EntityType), f.EntityID, f.CreatedAt, f.UpdatedAt)
	if err != nil {
		return fmt.Errorf("files: create: %w", err)
	}
	return nil
}

func (s *PGStore) Get(ctx context.Context, id uuid.UUID) (*File, error) {
	f, err := scanFile(s.db.QueryRow(ctx, `SELECT `+fileCols+` FROM core.files WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("files: get: %w", err)
	}
	return &f, nil
}

func (s *PGStore) List(ctx context.Context, q ListQuery, limit int, before *Cursor) ([]File, error) {
	where := make([]string, 0, 5)
	args := make([]any, 0, 6)
	add := func(clause string, v any) {
		args = append(args, v)
		where = append(where, fmt.Sprintf(clause, len(args)))
	}

	if q.EntityType != "" {
		add("entity_type = $%d", q.EntityType)
	}
	if q.EntityID != nil {
		add("entity_id = $%d", *q.EntityID)
	}
	if q.UploadedBy != nil {
		add("uploaded_by = $%d", *q.UploadedBy)
	}
	if q.Status != "" {
		add("status = $%d", string(q.Status))
	} else {
		where = append(where, "status <> 'deleted'")
	}
	if before != nil {
		args = append(args, before.At.UTC(), before.ID)
		where = append(where, fmt.Sprintf("(created_at, id) < ($%d, $%d)", len(args)-1, len(args)))
	}
	args = append(args, limit)

	sql := `SELECT ` + fileCols + ` FROM core.files`
	if len(where) > 0 {
		sql += " WHERE " + strings.Join(where, " AND ")
	}
	sql += fmt.Sprintf(" ORDER BY created_at DESC, id DESC LIMIT $%d", len(args))

	rows, err := s.db.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("files: list: %w", err)
	}
	defer rows.Close()

	out := make([]File, 0, limit)
	for rows.Next() {
		f, err := scanFile(rows)
		if err != nil {
			return nil, fmt.Errorf("files: scan: %w", err)
		}
		out = append(out, f)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *PGStore) Complete(ctx context.Context, id uuid.UUID, in ObjectInfo, status Status, ev *outbox.Event) (*File, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("files: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	f, err := scanFile(tx.QueryRow(ctx, `
		UPDATE core.files
		   SET status = $2, size_bytes = $3, content_type = $4, checksum_sha256 = $5
		 WHERE id = $1 AND status = 'pending'
		RETURNING `+fileCols,
		id, string(status), in.SizeBytes, in.ContentType, nullIfEmpty(in.ChecksumSHA256)))
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		// Either the file does not exist or it has already left `pending`.
		cur, gerr := scanFile(tx.QueryRow(ctx, `SELECT `+fileCols+` FROM core.files WHERE id = $1`, id))
		if errors.Is(gerr, pgx.ErrNoRows) {
			return nil, fmt.Errorf("%w: %s", ErrNotFound, id)
		}
		if gerr != nil {
			return nil, fmt.Errorf("files: read: %w", gerr)
		}
		return nil, fmt.Errorf("%w: file is %s", ErrConflict, cur.Status)
	case err != nil:
		return nil, fmt.Errorf("files: complete: %w", err)
	}

	if ev != nil {
		if _, err := outbox.Write(ctx, tx, *ev); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("files: commit: %w", err)
	}
	return &f, nil
}

func (s *PGStore) Archive(ctx context.Context, id uuid.UUID, ev *outbox.Event) (*File, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("files: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	f, err := scanFile(tx.QueryRow(ctx, `
		UPDATE core.files SET status = 'deleted', archived_at = now()
		 WHERE id = $1 AND status <> 'deleted'
		RETURNING `+fileCols, id))
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		if _, gerr := scanFile(tx.QueryRow(ctx, `SELECT `+fileCols+` FROM core.files WHERE id = $1`, id)); errors.Is(gerr, pgx.ErrNoRows) {
			return nil, fmt.Errorf("%w: %s", ErrNotFound, id)
		}
		return nil, fmt.Errorf("%w: file is already deleted", ErrConflict)
	case err != nil:
		return nil, fmt.Errorf("files: archive: %w", err)
	}

	if ev != nil {
		if _, err := outbox.Write(ctx, tx, *ev); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("files: commit: %w", err)
	}
	return &f, nil
}
