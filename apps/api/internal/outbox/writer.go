package outbox

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Execer is the subset of pgx that Write needs. Both *pgxpool.Pool and pgx.Tx
// satisfy it, so a caller can pass a pool for a one-off write or — the point of
// the outbox — pass the transaction that is already carrying the business
// change, guaranteeing the event and the change commit or roll back together.
type Execer interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

const insertEventSQL = `
INSERT INTO outbox.events (
  id, type, aggregate_type, aggregate_id, occurred_at,
  actor_id, correlation_id, causation_id, payload
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING seq`

// Write appends e to outbox.events and returns the assigned seq. The NOTIFY
// trigger fires on insert, so consumers wake the moment the transaction commits.
//
// Call this inside the caller's transaction (§7.1). Writing an event and its
// business change through the same Execer is the entire reliability story: if
// the transaction rolls back, the event never existed.
func Write(ctx context.Context, db Execer, e Event) (int64, error) {
	if e.ID == [16]byte{} {
		return 0, fmt.Errorf("outbox: event id is required")
	}
	if e.Type == "" || e.AggregateType == "" {
		return 0, fmt.Errorf("outbox: event type and aggregate type are required")
	}
	if len(e.Payload) == 0 {
		return 0, fmt.Errorf("outbox: payload is required")
	}

	var seq int64
	err := db.QueryRow(ctx, insertEventSQL,
		e.ID,
		e.Type,
		e.AggregateType,
		e.AggregateID,
		e.OccurredAt,
		e.ActorID,
		e.CorrelationID,
		e.CausationID,
		e.Payload,
	).Scan(&seq)
	if err != nil {
		return 0, fmt.Errorf("outbox: insert %s: %w", e.Type, err)
	}
	return seq, nil
}
