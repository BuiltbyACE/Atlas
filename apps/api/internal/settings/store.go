package settings

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Cursor is a keyset position in a setting's history.
type Cursor struct {
	At time.Time
	ID uuid.UUID
}

// Store is the settings persistence interface.
type Store interface {
	// List returns every stored setting, ordered by key.
	List(ctx context.Context) ([]Setting, error)
	// Get returns one setting, or (nil, nil) when no row exists yet.
	Get(ctx context.Context, key string) (*Setting, error)
	// Set writes the value and its history row in one transaction, honouring
	// optimistic concurrency. It returns the new row and the change recorded.
	Set(ctx context.Context, in SetInput) (*Setting, *Change, error)
	// History returns a page of a setting's changes, newest first.
	History(ctx context.Context, key string, limit int, before *Cursor) ([]Change, error)
}

// PGStore implements Store over Postgres.
type PGStore struct{ db *pgxpool.Pool }

// NewPGStore builds a store over the pool.
func NewPGStore(db *pgxpool.Pool) *PGStore { return &PGStore{db: db} }

// rowScanner is satisfied by both pgx.Row and pgx.Rows.
type rowScanner interface {
	Scan(dest ...any) error
}

func decodeJSON(raw []byte) (any, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, fmt.Errorf("settings: decode value: %w", err)
	}
	return v, nil
}

func nullIfBlank(s string) any {
	if s == "" {
		return nil
	}
	return s
}

const storedCols = `key, value, COALESCE(description,''), version, updated_by, updated_at`

func scanStored(row rowScanner) (Setting, error) {
	var (
		st  Setting
		raw []byte
	)
	if err := row.Scan(&st.Key, &raw, &st.Description, &st.Version, &st.UpdatedBy, &st.UpdatedAt); err != nil {
		return st, err
	}
	v, err := decodeJSON(raw)
	if err != nil {
		return st, err
	}
	st.Value = v
	return st, nil
}

func (s *PGStore) List(ctx context.Context) ([]Setting, error) {
	rows, err := s.db.Query(ctx, `SELECT `+storedCols+` FROM core.settings ORDER BY key`)
	if err != nil {
		return nil, fmt.Errorf("settings: list: %w", err)
	}
	defer rows.Close()

	out := make([]Setting, 0, 8)
	for rows.Next() {
		st, err := scanStored(rows)
		if err != nil {
			return nil, fmt.Errorf("settings: scan: %w", err)
		}
		out = append(out, st)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *PGStore) Get(ctx context.Context, key string) (*Setting, error) {
	row := s.db.QueryRow(ctx, `SELECT `+storedCols+` FROM core.settings WHERE key = $1`, key)
	st, err := scanStored(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("settings: get: %w", err)
	}
	return &st, nil
}

// Set is the write path: it locks the row (or finds none), checks the version
// the caller read, writes the new value, and appends the history entry — all in
// one transaction, so a setting can never have a value without the change that
// produced it.
func (s *PGStore) Set(ctx context.Context, in SetInput) (*Setting, *Change, error) {
	encoded, err := json.Marshal(in.Value)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %s is not encodable", ErrInvalid, in.Key)
	}
	newValue := json.RawMessage(encoded)

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("settings: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var (
		oldRaw []byte
		oldVer int64
	)
	hadRow := true
	err = tx.QueryRow(ctx, `SELECT value, version FROM core.settings WHERE key = $1 FOR UPDATE`, in.Key).
		Scan(&oldRaw, &oldVer)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		hadRow = false
		if in.ExpectedVersion != 0 {
			return nil, nil, fmt.Errorf("%w: %s does not exist", ErrConflict, in.Key)
		}
	case err != nil:
		return nil, nil, fmt.Errorf("settings: read: %w", err)
	default:
		if oldVer != in.ExpectedVersion {
			return nil, nil, fmt.Errorf("%w: %s is at version %d, not %d", ErrConflict, in.Key, oldVer, in.ExpectedVersion)
		}
	}

	var (
		raw2 []byte
		st   Setting
	)
	st.Key = in.Key
	if !hadRow {
		err = tx.QueryRow(ctx, `
			INSERT INTO core.settings (key, value, description, updated_by, updated_at, version)
			VALUES ($1, $2, $3, $4, now(), 1)
			RETURNING value, COALESCE(description,''), version, updated_by, updated_at`,
			in.Key, newValue, in.Description, in.ActorID).
			Scan(&raw2, &st.Description, &st.Version, &st.UpdatedBy, &st.UpdatedAt)
	} else {
		err = tx.QueryRow(ctx, `
			UPDATE core.settings
			   SET value = $2, description = $3, updated_by = $4, updated_at = now(), version = version + 1
			 WHERE key = $1
			RETURNING value, COALESCE(description,''), version, updated_by, updated_at`,
			in.Key, newValue, in.Description, in.ActorID).
			Scan(&raw2, &st.Description, &st.Version, &st.UpdatedBy, &st.UpdatedAt)
	}
	if err != nil {
		return nil, nil, fmt.Errorf("settings: write: %w", err)
	}
	if st.Value, err = decodeJSON(raw2); err != nil {
		return nil, nil, err
	}

	changeID, err := uuid.NewV7()
	if err != nil {
		return nil, nil, fmt.Errorf("settings: new change id: %w", err)
	}
	var oldValue any
	if hadRow {
		oldValue = json.RawMessage(oldRaw)
	}

	var changedAt time.Time
	err = tx.QueryRow(ctx, `
		INSERT INTO core.setting_changes (id, key, version, old_value, new_value, changed_by, reason)
		VALUES ($1,$2,$3,$4,$5,$6,$7)
		RETURNING changed_at`,
		changeID, in.Key, st.Version, oldValue, newValue, in.ActorID, nullIfBlank(in.Reason)).
		Scan(&changedAt)
	if err != nil {
		return nil, nil, fmt.Errorf("settings: history: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, nil, fmt.Errorf("settings: commit: %w", err)
	}

	chg := &Change{
		ID:        changeID,
		Key:       in.Key,
		Version:   st.Version,
		NewValue:  st.Value,
		ChangedBy: &in.ActorID,
		ChangedAt: changedAt,
		Reason:    in.Reason,
	}
	if hadRow {
		old, err := decodeJSON(oldRaw)
		if err != nil {
			return nil, nil, err
		}
		chg.OldValue = old
	}
	return &st, chg, nil
}

func (s *PGStore) History(ctx context.Context, key string, limit int, before *Cursor) ([]Change, error) {
	var (
		beforeTS *time.Time
		beforeID uuid.UUID
	)
	if before != nil {
		ts := before.At.UTC()
		beforeTS = &ts
		beforeID = before.ID
	}

	rows, err := s.db.Query(ctx, `
		SELECT id, key, version, old_value, new_value, changed_by, changed_at, reason
		FROM core.setting_changes
		WHERE key = $1
		  AND ($2::timestamptz IS NULL OR (changed_at, id) < ($2::timestamptz, $3::uuid))
		ORDER BY changed_at DESC, id DESC
		LIMIT $4`, key, beforeTS, beforeID, limit)
	if err != nil {
		return nil, fmt.Errorf("settings: history: %w", err)
	}
	defer rows.Close()

	out := make([]Change, 0, limit)
	for rows.Next() {
		var (
			c      Change
			oldRaw []byte
			newRaw []byte
			reason *string
		)
		if err := rows.Scan(&c.ID, &c.Key, &c.Version, &oldRaw, &newRaw, &c.ChangedBy, &c.ChangedAt, &reason); err != nil {
			return nil, fmt.Errorf("settings: scan history: %w", err)
		}
		if c.OldValue, err = decodeJSON(oldRaw); err != nil {
			return nil, err
		}
		if c.NewValue, err = decodeJSON(newRaw); err != nil {
			return nil, err
		}
		if reason != nil {
			c.Reason = *reason
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
