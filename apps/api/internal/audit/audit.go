// Package audit records who did what, when (docs/ATLAS.md §6.3).
//
// Events are normally written inside the same transaction as the change they
// describe, so a rolled-back change leaves no audit row and an audit row always
// has a real change behind it: build a Writer with NewWriter(tx) and pass the
// transaction to Write. A Writer built with NewWriter(pool) can instead write on
// its own connection when no transaction is supplied, which is what best-effort
// audit after a committed change needs. `audit.events` is append-only: the
// database rejects UPDATE and DELETE, and only `atlas_api`/`atlas_worker` may
// INSERT.
//
// Sensitive fields (salaries, contract values, personal phone numbers) are
// replaced by a marker before the row is written, so the audit log can be read
// by an auditor without exposing fields gated behind `*.sensitive.read`.
package audit

import (
	"context"
	"fmt"
	"net/netip"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Actor types.
const (
	ActorUser    = "user"
	ActorService = "service"
	ActorSystem  = "system"
)

// RedactedMarker replaces a suppressed value.
const RedactedMarker = "[redacted]"

// Execer is the subset of pgx shared by *pgxpool.Pool and pgx.Tx, so a Writer
// can either join a caller's transaction or run on its own connection.
type Execer interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
}

// Event is one audited action.
type Event struct {
	OccurredAt time.Time
	ActorType  string
	ActorID    string
	OnBehalfOf *uuid.UUID
	Action     string
	EntityType string
	EntityID   *uuid.UUID
	Before     any
	After      any
	Reason     string
	RequestID  string
	IP         netip.Addr
	UserAgent  string
}

// Writer persists audit events. It is stateless apart from the redaction rules
// and the optional executor used when Write is called without a transaction.
type Writer struct {
	redactor *Redactor
	db       Execer
}

// NewWriter returns a Writer with the default redaction rules. Pass a
// *pgxpool.Pool to let Write run on its own when no transaction is supplied; a
// bare NewWriter() requires every Write to carry a transaction.
func NewWriter(db ...Execer) *Writer {
	w := &Writer{redactor: NewRedactor()}
	if len(db) > 0 {
		w.db = db[0]
	}
	return w
}

// Redactor exposes the field-level redaction rules for configuration.
func (w *Writer) Redactor() *Redactor { return w.redactor }

// Write inserts an audit row. Prefer calling it with the transaction that
// carries the change being audited (§6.3); when tx is nil the Writer's own
// executor is used (best-effort audit after commit).
func (w *Writer) Write(ctx context.Context, tx pgx.Tx, e Event) error {
	if e.ActorType == "" || e.ActorID == "" {
		return fmt.Errorf("audit: actor type and id are required")
	}
	if e.Action == "" || e.EntityType == "" {
		return fmt.Errorf("audit: action and entity type are required")
	}

	exec := Execer(tx)
	if tx == nil {
		if w.db == nil {
			return fmt.Errorf("audit: no transaction supplied and the writer has no database")
		}
		exec = w.db
	}

	id, err := uuid.NewV7()
	if err != nil {
		return fmt.Errorf("audit: new id: %w", err)
	}
	occurred := e.OccurredAt
	if occurred.IsZero() {
		occurred = time.Now().UTC()
	}
	before := w.redactor.Redact(e.EntityType, e.Before)
	after := w.redactor.Redact(e.EntityType, e.After)

	const q = `
		INSERT INTO audit.events
			(id, occurred_at, actor_type, actor_id, on_behalf_of, action,
			 entity_type, entity_id, before, "after", reason, request_id, ip, user_agent)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`

	var ip any
	if e.IP.IsValid() {
		ip = e.IP
	}

	_, err = exec.Exec(ctx, q,
		id, occurred, e.ActorType, e.ActorID, e.OnBehalfOf, e.Action,
		e.EntityType, e.EntityID, jsonOrNil(before), jsonOrNil(after),
		nullIfEmpty(e.Reason), nullIfEmpty(e.RequestID), ip, nullIfEmpty(e.UserAgent),
	)
	if err != nil {
		return fmt.Errorf("audit: insert: %w", err)
	}
	return nil
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}
