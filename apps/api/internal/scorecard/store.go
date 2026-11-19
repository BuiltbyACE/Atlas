// Package scorecard serves the company scorecard (docs/ATLAS.md §8.9): a
// cross-module roll-up of company health — financial, operational, commercial,
// systems and people — for executive review.
//
// It is a read model. Go remains the authority for every record it summarises,
// and nothing here writes: the scorecard is aggregates over the tables the
// other engines own.
package scorecard

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Scorecard is the company health roll-up (§8.9): financial, operational,
// commercial, systems and people metrics in one payload.
type Scorecard struct {
	GeneratedAt time.Time   `json:"generated_at"`
	Financial   Financial   `json:"financial"`
	Operational Operational `json:"operational"`
	Commercial  Commercial  `json:"commercial"`
	Systems     Systems     `json:"systems"`
	People      People      `json:"people"`
}

// Financial is the company's financial health (§8.9).
type Financial struct {
	CashMinor              int64 `json:"cash_minor"`
	ReceivablesOutstanding int64 `json:"receivables_outstanding_minor"`
	ReceivablesOverdue     int64 `json:"receivables_overdue_minor"`
	Invoiced90D            int64 `json:"invoiced_90d_minor"`
	Collected90D           int64 `json:"collected_90d_minor"`
	Expenses30D            int64 `json:"expenses_30d_minor"`
	PendingExpenses        int   `json:"pending_expenses"`
	OverdueInvoiceCount    int   `json:"overdue_invoice_count"`
	UpcomingRenewals30D    int   `json:"upcoming_renewals_30d"`
	ActiveBudgetCount      int   `json:"active_budget_count"`
	BudgetsOverThreshold   int   `json:"budgets_over_threshold"`
}

// Operational is the company's operational health (§8.9).
type Operational struct {
	ActiveProjects    int `json:"active_projects"`
	PlanningProjects  int `json:"planning_projects"`
	OverdueTasks      int `json:"overdue_tasks"`
	OpenEscalations   int `json:"open_escalations"`
	CompletedTasks30D int `json:"completed_tasks_30d"`
}

// Commercial is the company's commercial health (§8.9).
type Commercial struct {
	PipelineValueMinor   int64 `json:"pipeline_value_minor"`
	OpenOpportunities    int   `json:"open_opportunities"`
	WonOpportunities30D  int   `json:"won_opportunities_30d"`
	LostOpportunities30D int   `json:"lost_opportunities_30d"`
	OpenLeads            int   `json:"open_leads"`
	QualifiedLeads30D    int   `json:"qualified_leads_30d"`
}

// Systems is the company's systems health (§8.9).
type Systems struct {
	SystemsTotal    int `json:"systems_total"`
	SystemsTier1    int `json:"systems_tier_1"`
	SystemsTier2    int `json:"systems_tier_2"`
	SystemsTier3    int `json:"systems_tier_3"`
	ChecksTotal     int `json:"checks_total"`
	ChecksFailing   int `json:"checks_failing"`
	ChecksWarning   int `json:"checks_warning"`
	ChecksUnprobed  int `json:"checks_unprobed"`
	OpenIncidents   int `json:"open_incidents"`
	OpenSev1        int `json:"open_sev_1"`
	OpenSev2        int `json:"open_sev_2"`
	Incidents30D    int `json:"incidents_30d"`
	AvailabilityBP  int `json:"availability_bp"`
}

// People is the company's people health (§8.9).
type People struct {
	Headcount         int `json:"headcount"`
	Active            int `json:"active"`
	OnLeave           int `json:"on_leave"`
	Onboarding        int `json:"onboarding"`
	Exited            int `json:"exited"`
	MissedReports30D  int `json:"missed_reports_30d"`
	ApprovedLeaveToday int `json:"approved_leave_today"`
}

// Store reads the aggregates the scorecard shows. It never writes.
type Store interface {
	// Scorecard returns the full company health roll-up as at now.
	Scorecard(ctx context.Context, now time.Time) (Scorecard, error)
}

// PGStore implements Store over Postgres.
type PGStore struct{ db *pgxpool.Pool }

// NewPGStore builds a store over the pool.
func NewPGStore(db *pgxpool.Pool) *PGStore { return &PGStore{db: db} }

// Scorecard returns the company health roll-up as at now (§8.9).
func (s *PGStore) Scorecard(ctx context.Context, now time.Time) (Scorecard, error) {
	var sc Scorecard
	sc.GeneratedAt = now

	// ── financial ──────────────────────────────────────────────
	// Reuses the same rules as finance.OverviewTotals (§8.4): cash from
	// accounts + payments - settled expenses; receivables from open invoices
	// net of allocations; expenses and budgets over trailing windows.
	err := s.db.QueryRow(ctx, `
		WITH cash AS (
			SELECT
				COALESCE((SELECT SUM(COALESCE(opening_balance_minor, 0)) FROM finance.accounts), 0)
				+ COALESCE((SELECT SUM(amount_minor) FROM finance.payments WHERE account_id IS NOT NULL), 0)
				- COALESCE((SELECT SUM(amount_minor) FROM finance.expenses
				            WHERE account_id IS NOT NULL AND status IN ('paid','reimbursed','reconciled')), 0)
				AS total
		),
		open_inv AS (
			SELECT i.id, i.due_date,
				COALESCE(i.total_minor, 0) - COALESCE(SUM(pa.amount_minor), 0) AS outstanding
			FROM finance.invoices i
			LEFT JOIN finance.payment_allocations pa ON pa.invoice_id = i.id
			WHERE i.status IN ('issued','partially_paid','overdue')
			GROUP BY i.id, i.total_minor, i.due_date
		)
		SELECT
			(SELECT total FROM cash) AS cash_minor,
			COALESCE((SELECT SUM(outstanding) FROM open_inv WHERE outstanding > 0), 0) AS receivables_outstanding_minor,
			COALESCE((SELECT SUM(outstanding) FROM open_inv WHERE outstanding > 0 AND due_date < $1::date), 0) AS receivables_overdue_minor,
			COALESCE((SELECT SUM(total_minor) FROM finance.invoices WHERE issue_date >= $1::date - 90), 0) AS invoiced_90d_minor,
			COALESCE((SELECT SUM(amount_minor) FROM finance.payments WHERE received_on >= $1::date - 90), 0) AS collected_90d_minor,
			COALESCE((SELECT SUM(amount_minor) FROM finance.expenses WHERE spent_on >= $1::date - 30), 0) AS expenses_30d_minor,
			(SELECT count(*) FROM finance.expenses WHERE status = 'submitted') AS pending_expenses,
			(SELECT count(*) FROM open_inv WHERE outstanding > 0 AND due_date < $1::date) AS overdue_invoice_count,
			(SELECT count(*) FROM finance.recurring_costs
				WHERE active AND next_renewal_on BETWEEN $1::date AND $1::date + 30) AS upcoming_renewals_30d,
			(SELECT count(*) FROM finance.budgets WHERE status = 'approved') AS active_budget_count,
			(SELECT count(*) FROM finance.budgets b
				WHERE b.status = 'approved' AND COALESCE(b.amount_minor, 0) > 0
				AND (SELECT COALESCE(SUM(e.amount_minor), 0) FROM finance.expenses e
					WHERE e.status IN ('approved','paid','reimbursed','reconciled')
						AND e.spent_on BETWEEN b.period_start AND b.period_end
						AND (b.category_key IS NULL OR e.category_key = b.category_key)
						AND CASE b.scope
								WHEN 'company_period' THEN true
								WHEN 'project' THEN e.project_id::text = b.scope_ref
								ELSE false
							END
				) > COALESCE(b.amount_minor, 0) * 0.9
			) AS budgets_over_threshold`,
		now,
	).Scan(
		&sc.Financial.CashMinor,
		&sc.Financial.ReceivablesOutstanding,
		&sc.Financial.ReceivablesOverdue,
		&sc.Financial.Invoiced90D,
		&sc.Financial.Collected90D,
		&sc.Financial.Expenses30D,
		&sc.Financial.PendingExpenses,
		&sc.Financial.OverdueInvoiceCount,
		&sc.Financial.UpcomingRenewals30D,
		&sc.Financial.ActiveBudgetCount,
		&sc.Financial.BudgetsOverThreshold,
	)
	if err != nil {
		return Scorecard{}, fmt.Errorf("scorecard: financial: %w", err)
	}

	// ── operational ────────────────────────────────────────────
	// Project counts by status (work.projects).
	err = s.db.QueryRow(ctx, `
		SELECT
			(SELECT count(*) FROM work.projects WHERE status = 'active') AS active,
			(SELECT count(*) FROM work.projects WHERE status = 'planning') AS planning`).
		Scan(&sc.Operational.ActiveProjects, &sc.Operational.PlanningProjects)
	if err != nil {
		return Scorecard{}, fmt.Errorf("scorecard: operational projects: %w", err)
	}

	// Overdue tasks (work.tasks not done and past due).
	err = s.db.QueryRow(ctx, `
		SELECT count(*)
		FROM work.tasks
		WHERE status NOT IN ('done','cancelled')
		  AND due_at IS NOT NULL AND due_at < $1`,
		now,
	).Scan(&sc.Operational.OverdueTasks)
	if err != nil {
		return Scorecard{}, fmt.Errorf("scorecard: operational overdue tasks: %w", err)
	}

	// Open escalations (work.escalations status = open).
	err = s.db.QueryRow(ctx, `
		SELECT count(*) FROM work.escalations WHERE status = 'open'`).
		Scan(&sc.Operational.OpenEscalations)
	if err != nil {
		return Scorecard{}, fmt.Errorf("scorecard: operational escalations: %w", err)
	}

	// Tasks completed in the last 30 days.
	err = s.db.QueryRow(ctx, `
		SELECT count(*)
		FROM work.tasks
		WHERE status = 'done' AND updated_at >= $1`,
		now.AddDate(0, -30, 0),
	).Scan(&sc.Operational.CompletedTasks30D)
	if err != nil {
		return Scorecard{}, fmt.Errorf("scorecard: operational completed tasks: %w", err)
	}

	// ── commercial ─────────────────────────────────────────────
	// Pipeline value and open opportunities (crm.opportunities).
	err = s.db.QueryRow(ctx, `
		SELECT
			COALESCE((SELECT sum(estimated_value_minor) FROM crm.opportunities
			          WHERE stage_key NOT IN ('won','lost','rejected')), 0) AS pipe,
			(SELECT count(*) FROM crm.opportunities
			 WHERE stage_key NOT IN ('won','lost','rejected')) AS open_opp`).
		Scan(&sc.Commercial.PipelineValueMinor, &sc.Commercial.OpenOpportunities)
	if err != nil {
		return Scorecard{}, fmt.Errorf("scorecard: commercial pipeline: %w", err)
	}

	// Won/lost opportunities in the last 30 days.
	err = s.db.QueryRow(ctx, `
		SELECT
			(SELECT count(*) FROM crm.opportunities
			 WHERE stage_key = 'won' AND updated_at >= $1) AS won,
			(SELECT count(*) FROM crm.opportunities
			 WHERE stage_key = 'lost' AND updated_at >= $1) AS lost`,
		now.AddDate(0, -30, 0),
	).Scan(&sc.Commercial.WonOpportunities30D, &sc.Commercial.LostOpportunities30D)
	if err != nil {
		return Scorecard{}, fmt.Errorf("scorecard: commercial won/lost: %w", err)
	}

	// Open leads and qualified leads in the last 30 days (crm.leads).
	err = s.db.QueryRow(ctx, `
		SELECT
			(SELECT count(*) FROM crm.leads
			 WHERE status NOT IN ('converted','disqualified')) AS open,
			(SELECT count(*) FROM crm.leads
			 WHERE status = 'qualified' AND updated_at >= $1) AS qualified`,
		now.AddDate(0, -30, 0),
	).Scan(&sc.Commercial.OpenLeads, &sc.Commercial.QualifiedLeads30D)
	if err != nil {
		return Scorecard{}, fmt.Errorf("scorecard: commercial leads: %w", err)
	}

	// ── systems ────────────────────────────────────────────────
	// Register totals (systems systems + checks).
	err = s.db.QueryRow(ctx, `
		SELECT
			(SELECT count(*) FROM systems.systems) AS total,
			(SELECT count(*) FROM systems.systems WHERE tier = 1) AS t1,
			(SELECT count(*) FROM systems.systems WHERE tier = 2) AS t2,
			(SELECT count(*) FROM systems.systems WHERE tier = 3) AS t3,
			(SELECT count(*) FROM systems.checks) AS checks,
			(SELECT count(*) FROM systems.checks WHERE status = 'failing') AS failing,
			(SELECT count(*) FROM systems.checks WHERE status = 'warning') AS warning,
			(SELECT count(*) FROM systems.checks WHERE status = 'unprobed') AS unprobed`,
	).Scan(
		&sc.Systems.SystemsTotal,
		&sc.Systems.SystemsTier1,
		&sc.Systems.SystemsTier2,
		&sc.Systems.SystemsTier3,
		&sc.Systems.ChecksTotal,
		&sc.Systems.ChecksFailing,
		&sc.Systems.ChecksWarning,
		&sc.Systems.ChecksUnprobed,
	)
	if err != nil {
		return Scorecard{}, fmt.Errorf("scorecard: systems register: %w", err)
	}

	// Incident metrics.
	err = s.db.QueryRow(ctx, `
		SELECT
			(SELECT count(*) FROM systems.incidents WHERE resolved_at IS NULL AND severity = 1) AS sev1,
			(SELECT count(*) FROM systems.incidents WHERE resolved_at IS NULL AND severity = 2) AS sev2,
			(SELECT count(*) FROM systems.incidents WHERE resolved_at IS NULL) AS open,
			(SELECT count(*) FROM systems.incidents WHERE detected_at >= $1) AS last30d`,
		now.AddDate(0, -30, 0),
	).Scan(&sc.Systems.OpenSev1, &sc.Systems.OpenSev2, &sc.Systems.OpenIncidents, &sc.Systems.Incidents30D)
	if err != nil {
		return Scorecard{}, fmt.Errorf("scorecard: systems incidents: %w", err)
	}

	// Availability basis points over the last 90 days (rolling window).
	err = s.db.QueryRow(ctx, `
		SELECT COALESCE(avg(availability_bp), 10000)
		FROM systems.system_availability
		WHERE measured_at >= $1`,
		now.AddDate(0, -3, 0),
	).Scan(&sc.Systems.AvailabilityBP)
	if err != nil {
		return Scorecard{}, fmt.Errorf("scorecard: systems availability: %w", err)
	}

	// ── people ─────────────────────────────────────────────────
	// Headcount by status (people.persons).
	err = s.db.QueryRow(ctx, `
		SELECT
			(SELECT count(*) FROM people.persons) AS total,
			(SELECT count(*) FROM people.persons WHERE status = 'active') AS active,
			(SELECT count(*) FROM people.persons WHERE status = 'on_leave') AS on_leave,
			(SELECT count(*) FROM people.persons WHERE status = 'onboarding') AS onboarding,
			(SELECT count(*) FROM people.persons WHERE status = 'exited') AS exited`,
	).Scan(
		&sc.People.Headcount,
		&sc.People.Active,
		&sc.People.OnLeave,
		&sc.People.Onboarding,
		&sc.People.Exited,
	)
	if err != nil {
		return Scorecard{}, fmt.Errorf("scorecard: people headcount: %w", err)
	}

	// Missed daily reports in the last 30 days.
	err = s.db.QueryRow(ctx, `
		SELECT count(*)
		FROM people.daily_reports
		WHERE status = 'missed' AND report_date >= $1`,
		now.AddDate(0, -30, 0),
	).Scan(&sc.People.MissedReports30D)
	if err != nil {
		return Scorecard{}, fmt.Errorf("scorecard: people missed reports: %w", err)
	}

	// Approved leave today.
	err = s.db.QueryRow(ctx, `
		SELECT count(*)
		FROM people.leave_requests
		WHERE status = 'approved' AND from_date <= $1 AND to_date >= $1`,
		now,
	).Scan(&sc.People.ApprovedLeaveToday)
	if err != nil {
		return Scorecard{}, fmt.Errorf("scorecard: people approved leave: %w", err)
	}

	return sc, nil
}
