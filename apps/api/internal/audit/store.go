package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Cursor is a keyset position in the audit log: (occurred_at, id), newest first.
type Cursor struct {
	At time.Time
	ID uuid.UUID
}

// Query filters the audit log. Every field is optional; the zero value lists
// the whole trail, newest first.
type Query struct {
	EntityType string
	EntityID   *uuid.UUID
	ActorType  string
	ActorID    string
	Action     string
	// Since is inclusive, Until exclusive, both on occurred_at.
	Since *time.Time
	Until *time.Time
}

// Entry is one audit.events row as returned to callers. Before and After carry
// the stored JSON exactly as written — already redacted by the Writer (§6.3),
// so the read side never has to strip sensitive fields.
type Entry struct {
	ID         uuid.UUID
	OccurredAt time.Time
	ActorType  string
	ActorID    string
	OnBehalfOf *uuid.UUID
	Action     string
	EntityType string
	EntityID   *uuid.UUID
	Before     json.RawMessage
	After      json.RawMessage
	Reason     string
	RequestID  string
	IP         string
	UserAgent  string
}

// Store reads the append-only audit log (§6.3).
type Store interface {
	// List returns a page of events, newest first.
	List(ctx context.Context, q Query, limit int, before *Cursor) ([]Entry, error)
}

// PGStore implements Store over Postgres.
type PGStore struct{ db *pgxpool.Pool }

// NewPGStore builds a store over the pool.
func NewPGStore(db *pgxpool.Pool) *PGStore { return &PGStore{db: db} }

// rowScanner is satisfied by both pgx.Row and pgx.Rows.
type rowScanner interface {
	Scan(dest ...any) error
}

// ip is cast to text so the driver returns a plain string (or NULL) rather than
// an inet value the API would have to format.
const entryCols = `id, occurred_at, actor_type, actor_id, on_behalf_of, action,
	entity_type, entity_id, before, "after", reason, request_id, ip::text, user_agent`

func scanEntry(row rowScanner) (Entry, error) {
	var (
		e          Entry
		before     []byte
		after      []byte
		onBehalfOf *uuid.UUID
		entityID   *uuid.UUID
		reason     *string
		requestID  *string
		ip         *string
		userAgent  *string
	)
	if err := row.Scan(
		&e.ID, &e.OccurredAt, &e.ActorType, &e.ActorID, &onBehalfOf,
		&e.Action, &e.EntityType, &entityID, &before, &after,
		&reason, &requestID, &ip, &userAgent,
	); err != nil {
		return e, err
	}
	e.OnBehalfOf = onBehalfOf
	e.EntityID = entityID
	if before != nil {
		e.Before = json.RawMessage(before)
	}
	if after != nil {
		e.After = json.RawMessage(after)
	}
	if reason != nil {
		e.Reason = *reason
	}
	if requestID != nil {
		e.RequestID = *requestID
	}
	if ip != nil {
		e.IP = *ip
	}
	if userAgent != nil {
		e.UserAgent = *userAgent
	}
	return e, nil
}

// List returns a page of the audit trail, newest first.
func (s *PGStore) List(ctx context.Context, q Query, limit int, before *Cursor) ([]Entry, error) {
	where := make([]string, 0, 8)
	args := make([]any, 0, 9)
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
	if q.ActorType != "" {
		add("actor_type = $%d", q.ActorType)
	}
	if q.ActorID != "" {
		add("actor_id = $%d", q.ActorID)
	}
	if q.Action != "" {
		add("action = $%d", q.Action)
	}
	if q.Since != nil {
		add("occurred_at >= $%d", *q.Since)
	}
	if q.Until != nil {
		add("occurred_at < $%d", *q.Until)
	}
	if before != nil {
		args = append(args, before.At.UTC(), before.ID)
		where = append(where, fmt.Sprintf("(occurred_at, id) < ($%d, $%d)", len(args)-1, len(args)))
	}
	args = append(args, limit)

	sql := `SELECT ` + entryCols + ` FROM audit.events`
	if len(where) > 0 {
		sql += " WHERE " + strings.Join(where, " AND ")
	}
	sql += fmt.Sprintf(" ORDER BY occurred_at DESC, id DESC LIMIT $%d", len(args))

	rows, err := s.db.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("audit: list: %w", err)
	}
	defer rows.Close()

	entries := make([]Entry, 0, limit)
	for rows.Next() {
		e, err := scanEntry(rows)
		if err != nil {
			return nil, fmt.Errorf("audit: scan: %w", err)
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("audit: rows: %w", err)
	}
	return entries, nil
}
