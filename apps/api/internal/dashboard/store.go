// Package dashboard serves the executive dashboard (docs/ATLAS.md §8.8): the
// Phase 1 tiles over people, approvals and the audit trail, refreshed live
// through the `exec:dashboard` channel.
//
// It is a read model. Go remains the authority for every record it summarises,
// and nothing here writes: the tiles are aggregates over the tables the other
// engines own.
package dashboard

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PersonStatuses mirrors the people.persons status CHECK (§8.5, migration
// 00002). The summary always reports every status, zero when nobody holds it,
// so the staff tile keeps a stable shape.
var PersonStatuses = []string{"active", "on_leave", "onboarding", "exited"}

// PeopleSummary is the staff-status tile.
type PeopleSummary struct {
	Total    int            `json:"total"`
	ByStatus map[string]int `json:"by_status"`
}

// ApprovalsSummary is the approvals tile. "Pending" matches the engine's
// definition of work waiting on someone — status in (requested, pending) and
// not archived — the same predicate the inbox uses (internal/approvals).
type ApprovalsSummary struct {
	Pending       int            `json:"pending"`
	Escalated     int            `json:"escalated"`
	Overdue       int            `json:"overdue"`
	PendingByRole map[string]int `json:"pending_by_role"`
}

// WorkSummary is the overdue-work tile (§3.2, §5.8): open tasks past their
// deadline, and the escalations the worker sweep has raised for them and not
// yet resolved.
type WorkSummary struct {
	Overdue         int `json:"overdue"`
	OpenEscalations int `json:"open_escalations"`
}

// AuditHighlight is one recent audit event, trimmed to what a dashboard row
// shows. The full before/after snapshots stay in the audit views (§6.3).
type AuditHighlight struct {
	ID         string    `json:"id"`
	OccurredAt time.Time `json:"occurred_at"`
	ActorType  string    `json:"actor_type"`
	ActorID    string    `json:"actor_id"`
	Action     string    `json:"action"`
	EntityType string    `json:"entity_type"`
	EntityID   string    `json:"entity_id,omitempty"`
}

// Summary is the whole dashboard payload.
type Summary struct {
	GeneratedAt time.Time        `json:"generated_at"`
	People      PeopleSummary    `json:"people"`
	Approvals   ApprovalsSummary `json:"approvals"`
	Work        WorkSummary      `json:"work"`
	RecentAudit []AuditHighlight `json:"recent_audit"`
}

// Store reads the aggregates the dashboard shows. It never writes.
type Store interface {
	// PeopleByStatus counts people per status; statuses with nobody are absent.
	PeopleByStatus(ctx context.Context) (map[string]int, error)
	// Approvals counts pending, escalated and overdue approvals and breaks the
	// pending ones down by approver role, as at now.
	Approvals(ctx context.Context, now time.Time) (ApprovalsSummary, error)
	// Work counts overdue tasks and open escalations, as at now.
	Work(ctx context.Context, now time.Time) (WorkSummary, error)
}

// PGStore implements Store over Postgres.
type PGStore struct{ db *pgxpool.Pool }

// NewPGStore builds a store over the pool.
func NewPGStore(db *pgxpool.Pool) *PGStore { return &PGStore{db: db} }

// PeopleByStatus counts people per status.
func (s *PGStore) PeopleByStatus(ctx context.Context) (map[string]int, error) {
	rows, err := s.db.Query(ctx, `SELECT status, count(*) FROM people.persons GROUP BY status`)
	if err != nil {
		return nil, fmt.Errorf("dashboard: people by status: %w", err)
	}
	defer rows.Close()

	counts := make(map[string]int, len(PersonStatuses))
	for rows.Next() {
		var (
			status string
			n      int
		)
		if err := rows.Scan(&status, &n); err != nil {
			return nil, fmt.Errorf("dashboard: scan people: %w", err)
		}
		counts[status] = n
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("dashboard: people rows: %w", err)
	}
	return counts, nil
}

// Approvals counts the approval tiles as at now.
func (s *PGStore) Approvals(ctx context.Context, now time.Time) (ApprovalsSummary, error) {
	var sum ApprovalsSummary
	err := s.db.QueryRow(ctx, `
		SELECT
			count(*) FILTER (WHERE status IN ('requested','pending'))                                   AS pending,
			count(*) FILTER (WHERE status = 'escalated')                                                AS escalated,
			count(*) FILTER (WHERE status IN ('requested','pending')
			                   AND due_at IS NOT NULL AND due_at < $1)                                  AS overdue
		FROM core.approvals
		WHERE archived_at IS NULL`, now,
	).Scan(&sum.Pending, &sum.Escalated, &sum.Overdue)
	if err != nil {
		return ApprovalsSummary{}, fmt.Errorf("dashboard: approvals: %w", err)
	}

	rows, err := s.db.Query(ctx, `
		SELECT approver_role, count(*)
		FROM core.approvals
		WHERE archived_at IS NULL
		  AND status IN ('requested','pending')
		  AND approver_role IS NOT NULL
		GROUP BY approver_role`)
	if err != nil {
		return ApprovalsSummary{}, fmt.Errorf("dashboard: approvals by role: %w", err)
	}
	defer rows.Close()

	sum.PendingByRole = make(map[string]int)
	for rows.Next() {
		var (
			role string
			n    int
		)
		if err := rows.Scan(&role, &n); err != nil {
			return ApprovalsSummary{}, fmt.Errorf("dashboard: scan role: %w", err)
		}
		sum.PendingByRole[role] = n
	}
	if err := rows.Err(); err != nil {
		return ApprovalsSummary{}, fmt.Errorf("dashboard: role rows: %w", err)
	}
	return sum, nil
}

// Work counts the overdue-work tile as at now: open tasks past their deadline
// and the open escalations raised for overdue work.
func (s *PGStore) Work(ctx context.Context, now time.Time) (WorkSummary, error) {
	var sum WorkSummary
	err := s.db.QueryRow(ctx, `
		SELECT
			(SELECT count(*) FROM work.tasks
			  WHERE status NOT IN ('done','cancelled')
			    AND due_at IS NOT NULL AND due_at < $1) AS overdue,
			(SELECT count(*) FROM work.escalations
			  WHERE status = 'open')                    AS open_escalations`, now,
	).Scan(&sum.Overdue, &sum.OpenEscalations)
	if err != nil {
		return WorkSummary{}, fmt.Errorf("dashboard: work: %w", err)
	}
	return sum, nil
}
