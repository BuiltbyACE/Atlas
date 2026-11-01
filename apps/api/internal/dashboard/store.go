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

// UnitsSummary is the business-units tile (§8.7): how many units, integrations
// and indicator definitions are on record.
type UnitsSummary struct {
	Units       int `json:"units"`
	Integrations int `json:"integrations"`
	Indicators   int `json:"indicators"`
}

// IntelSummary is the executive-intelligence tile (§8.8): how many decisions,
// risks, briefs and metric snapshots are on record.
type IntelSummary struct {
	Decisions   int `json:"decisions"`
	Risks       int `json:"risks"`
	Briefs      int `json:"briefs"`
	Snapshots   int `json:"snapshots"`
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
	Units       UnitsSummary     `json:"units"`
	Intel       IntelSummary     `json:"intel"`
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
	// Units counts business units, integrations and indicators on record.
	Units(ctx context.Context) (UnitsSummary, error)
	// Intel counts decisions, risks, briefs and metric snapshots on record.
	Intel(ctx context.Context) (IntelSummary, error)
	// DailyBrief returns the daily executive brief for the given date (§8.8).
	DailyBrief(ctx context.Context, date time.Time) (DailyBrief, error)
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

// Units counts business units, integrations and indicators on record.
func (s *PGStore) Units(ctx context.Context) (UnitsSummary, error) {
	var sum UnitsSummary
	err := s.db.QueryRow(ctx, `
		SELECT
			(SELECT count(*) FROM units.business_units)               AS units,
			(SELECT count(*) FROM units.unit_integrations)            AS integrations,
			(SELECT count(*) FROM units.unit_indicator_definitions)   AS indicators`,
	).Scan(&sum.Units, &sum.Integrations, &sum.Indicators)
	if err != nil {
		return UnitsSummary{}, fmt.Errorf("dashboard: units: %w", err)
	}
	return sum, nil
}

// Intel counts decisions, risks, briefs and metric snapshots on record.
func (s *PGStore) Intel(ctx context.Context) (IntelSummary, error) {
	var sum IntelSummary
	err := s.db.QueryRow(ctx, `
		SELECT
			(SELECT count(*) FROM intel.decisions)                 AS decisions,
			(SELECT count(*) FROM intel.risks)                     AS risks,
			(SELECT count(*) FROM intel.briefs)                    AS briefs,
			(SELECT count(*) FROM intel.metric_snapshots)          AS snapshots`,
	).Scan(&sum.Decisions, &sum.Risks, &sum.Briefs, &sum.Snapshots)
	if err != nil {
		return IntelSummary{}, fmt.Errorf("dashboard: intel: %w", err)
	}
	return sum, nil
}

// ── daily brief (§8.8) ──────────────────────────────────────────────────────

// DailyBrief is the executive daily brief (§8.8): yesterday's results, what
// needs attention, today's upcoming items, and decisions awaiting the CEOs.
type DailyBrief struct {
	GeneratedAt time.Time        `json:"generated_at"`
	ForDate     time.Time        `json:"for_date"`
	Yesterday   YesterdaySummary `json:"yesterday"`
	NeedsAttention NeedsAttentionSummary `json:"needs_attention"`
	Today       TodaySummary     `json:"today"`
	Decisions   []DecisionBrief  `json:"decisions"`
}

// YesterdaySummary is yesterday's activity across all modules (§8.8).
type YesterdaySummary struct {
	DealsWon         int   `json:"deals_won"`
	DealsLost        int   `json:"deals_lost"`
	PaymentsReceived int64 `json:"payments_received_minor"`
	TasksCompleted   int   `json:"tasks_completed"`
	IncidentsOpened  int   `json:"incidents_opened"`
	IncidentsResolved int  `json:"incidents_resolved"`
}

// NeedsAttentionSummary is items requiring executive attention (§8.8).
type NeedsAttentionSummary struct {
	OverdueTasks         int   `json:"overdue_tasks"`
	OverdueInvoices      int   `json:"overdue_invoices"`
	OverdueInvoicesValue int64 `json:"overdue_invoices_minor"`
	MissedDailyReports   int   `json:"missed_daily_reports"`
	ExpiringCertificates int   `json:"expiring_certificates"`
	ExpiringDomains      int   `json:"expiring_domains"`
	OpenRisks            int   `json:"open_risks"`
}

// TodaySummary is today's upcoming items (§8.8).
type TodaySummary struct {
	FollowUpsDue   int `json:"follow_ups_due"`
	MilestonesDue  int `json:"milestones_due"`
	PeopleAbsent   int `json:"people_absent"`
}

// DecisionBrief is an open decision for the daily brief (§8.8).
type DecisionBrief struct {
	ID        string     `json:"id"`
	Ref       string     `json:"ref"`
	Question  string     `json:"question"`
	Outcome   string     `json:"outcome"`
	DueOn     *time.Time `json:"due_on,omitempty"`
}

// DailyBriefListQuery is the query for the daily brief.
type DailyBriefListQuery struct {
	Date string `json:"date,omitempty"`
}

// DailyBrief returns the daily executive brief for the given date (§8.8).
func (s *PGStore) DailyBrief(ctx context.Context, date time.Time) (DailyBrief, error) {
	yesterday := date.AddDate(0, 0, -1)
	yesterdayStart := time.Date(yesterday.Year(), yesterday.Month(), yesterday.Day(), 0, 0, 0, 0, time.UTC)
	yesterdayEnd := yesterdayStart.Add(24 * time.Hour)
	today := date
	todayStart := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, time.UTC)
	todayEnd := todayStart.Add(24 * time.Hour)
	now := time.Now().UTC()

	var d DailyBrief
	d.ForDate = yesterday

	// ── yesterday's activity ──────────────────────────────────────────

	// Deals won/lost yesterday (crm.opportunities).
	err := s.db.QueryRow(ctx, `
		SELECT
			(SELECT count(*) FROM crm.opportunities
			 WHERE status = 'won' AND updated_at >= $1 AND updated_at < $2) AS won,
			(SELECT count(*) FROM crm.opportunities
			 WHERE status = 'lost' AND updated_at >= $1 AND updated_at < $2) AS lost`,
		yesterdayStart, yesterdayEnd,
	).Scan(&d.Yesterday.DealsWon, &d.Yesterday.DealsLost)
	if err != nil {
		return DailyBrief{}, fmt.Errorf("dashboard: daily brief deals: %w", err)
	}

	// Payments received yesterday (finance.payments).
	err = s.db.QueryRow(ctx, `
		SELECT COALESCE(SUM(amount_minor), 0)
		FROM finance.payments
		WHERE received_at >= $1 AND received_at < $2`,
		yesterdayStart, yesterdayEnd,
	).Scan(&d.Yesterday.PaymentsReceived)
	if err != nil {
		return DailyBrief{}, fmt.Errorf("dashboard: daily brief payments: %w", err)
	}

	// Tasks completed yesterday (work.tasks status = done).
	err = s.db.QueryRow(ctx, `
		SELECT count(*)
		FROM work.tasks
		WHERE status = 'done' AND updated_at >= $1 AND updated_at < $2`,
		yesterdayStart, yesterdayEnd,
	).Scan(&d.Yesterday.TasksCompleted)
	if err != nil {
		return DailyBrief{}, fmt.Errorf("dashboard: daily brief tasks completed: %w", err)
	}

	// Incidents opened/resolved yesterday (systems.incidents).
	err = s.db.QueryRow(ctx, `
		SELECT
			(SELECT count(*) FROM systems.incidents
			 WHERE detected_at >= $1 AND detected_at < $2) AS opened,
			(SELECT count(*) FROM systems.incidents
			 WHERE resolved_at >= $1 AND resolved_at < $2) AS resolved`,
		yesterdayStart, yesterdayEnd,
	).Scan(&d.Yesterday.IncidentsOpened, &d.Yesterday.IncidentsResolved)
	if err != nil {
		return DailyBrief{}, fmt.Errorf("dashboard: daily brief incidents: %w", err)
	}

	// ── needs attention ─────────────────────────────────────────────

	// Overdue tasks (work.tasks not done and past due).
	err = s.db.QueryRow(ctx, `
		SELECT count(*)
		FROM work.tasks
		WHERE status NOT IN ('done','cancelled')
		  AND due_at IS NOT NULL AND due_at < $1`,
		now,
	).Scan(&d.NeedsAttention.OverdueTasks)
	if err != nil {
		return DailyBrief{}, fmt.Errorf("dashboard: daily brief overdue tasks: %w", err)
	}

	// Overdue invoices (finance.invoices status = overdue).
	err = s.db.QueryRow(ctx, `
		SELECT
			count(*),
			COALESCE(SUM(amount_minor), 0)
		FROM finance.invoices
		WHERE status = 'overdue'`,
	).Scan(&d.NeedsAttention.OverdueInvoices, &d.NeedsAttention.OverdueInvoicesValue)
	if err != nil {
		return DailyBrief{}, fmt.Errorf("dashboard: daily brief overdue invoices: %w", err)
	}

	// Missed daily reports yesterday (people.daily_reports status = missed).
	err = s.db.QueryRow(ctx, `
		SELECT count(*)
		FROM people.daily_reports
		WHERE status = 'missed' AND report_date = $1`,
		yesterday,
	).Scan(&d.NeedsAttention.MissedDailyReports)
	if err != nil {
		return DailyBrief{}, fmt.Errorf("dashboard: daily brief missed reports: %w", err)
	}

	// Expiring certificates (systems.certificates within 30 days).
	err = s.db.QueryRow(ctx, `
		SELECT count(*)
		FROM systems.certificates
		WHERE expires_on IS NOT NULL
		  AND expires_at >= $1 AND expires_at <= $2`,
		now, now.AddDate(0, 30, 0),
	).Scan(&d.NeedsAttention.ExpiringCertificates)
	if err != nil {
		return DailyBrief{}, fmt.Errorf("dashboard: daily brief expiring certificates: %w", err)
	}

	// Expiring domains (systems.domains within 30 days).
	err = s.db.QueryRow(ctx, `
		SELECT count(*)
		FROM systems.domains
		WHERE expires_on IS NOT NULL
		  AND expires_on >= $1 AND expires_on <= $2`,
		now, now.AddDate(0, 30, 0),
	).Scan(&d.NeedsAttention.ExpiringDomains)
	if err != nil {
		return DailyBrief{}, fmt.Errorf("dashboard: daily brief expiring domains: %w", err)
	}

	// Open risks (intel.risks status = open).
	err = s.db.QueryRow(ctx, `
		SELECT count(*)
		FROM intel.risks
		WHERE status = 'open'`,
	).Scan(&d.NeedsAttention.OpenRisks)
	if err != nil {
		return DailyBrief{}, fmt.Errorf("dashboard: daily brief open risks: %w", err)
	}

	// ── today's items ─────────────────────────────────────────────

	// Follow-ups due today (crm.followups due today).
	err = s.db.QueryRow(ctx, `
		SELECT count(*)
		FROM crm.followups
		WHERE status = 'scheduled' AND due_at >= $1 AND due_at < $2`,
		todayStart, todayEnd,
	).Scan(&d.Today.FollowUpsDue)
	if err != nil {
		return DailyBrief{}, fmt.Errorf("dashboard: daily brief follow-ups due: %w", err)
	}

	// Milestones due today (work.milestones due today).
	err = s.db.QueryRow(ctx, `
		SELECT count(*)
		FROM work.milestones
		WHERE due_at >= $1 AND due_at < $2`,
		todayStart, todayEnd,
	).Scan(&d.Today.MilestonesDue)
	if err != nil {
		return DailyBrief{}, fmt.Errorf("dashboard: daily brief milestones due: %w", err)
	}

	// People absent today (people.persons status = on_leave).
	err = s.db.QueryRow(ctx, `
		SELECT count(*)
		FROM people.persons
		WHERE status = 'on_leave'`,
	).Scan(&d.Today.PeopleAbsent)
	if err != nil {
		return DailyBrief{}, fmt.Errorf("dashboard: daily brief people absent: %w", err)
	}

	// ── decisions awaiting ──────────────────────────────────────────

	rows, err := s.db.Query(ctx, `
		SELECT id, ref, question, outcome, due_on
		FROM intel.decisions
		WHERE outcome = 'open'
		ORDER BY due_on ASC NULLS LAST, created_at DESC
		LIMIT 10`,
	)
	if err != nil {
		return DailyBrief{}, fmt.Errorf("dashboard: daily brief decisions: %w", err)
	}
	defer rows.Close()

	var decisions []DecisionBrief
	for rows.Next() {
		var d DecisionBrief
		var dueOn *time.Time
		if err := rows.Scan(&d.ID, &d.Ref, &d.Question, &d.Outcome, &dueOn); err != nil {
			return DailyBrief{}, fmt.Errorf("dashboard: scan decision: %w", err)
		}
		d.DueOn = dueOn
		decisions = append(decisions, d)
	}
	if err := rows.Err(); err != nil {
		return DailyBrief{}, fmt.Errorf("dashboard: decisions rows: %w", err)
	}

	d.Decisions = decisions

	return d, nil
}