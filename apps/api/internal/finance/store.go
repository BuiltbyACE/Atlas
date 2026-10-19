package finance

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Cursor is a keyset position (created_at, id), newest first (§6.6).
type Cursor struct {
	At time.Time
	ID uuid.UUID
}

// Store is the finance read model. This pass is read-only: every method
// answers a query the finance screens make (§8.4). The write lifecycle lands in
// a later pass, against the same tables.
type Store interface {
	// OverviewTotals gathers the finance headline in one round trip.
	OverviewTotals(ctx context.Context, asOf time.Time) (OverviewTotals, error)

	// Accounts and cash position.
	ListAccounts(ctx context.Context) ([]AccountBalance, error)
	CashFlow(ctx context.Context, since time.Time) (inflow, outflow int64, err error)

	// Receivables: open invoices with their outstanding amount and paid total.
	OpenReceivables(ctx context.Context, asOf time.Time) ([]Invoice, error)

	// Standing costs (calendar).
	ListRecurringCosts(ctx context.Context) ([]RecurringCost, error)

	// Project profitability.
	ProjectName(ctx context.Context, id uuid.UUID) (string, bool, error)
	ProjectRevenue(ctx context.Context, projectID uuid.UUID) (invoiced, collected int64, err error)
	ProjectCosts(ctx context.Context, projectID uuid.UUID) (int64, error)

	// Expenses.
	ListExpenseCategories(ctx context.Context) ([]ExpenseCategory, error)
	ListExpenses(ctx context.Context, q ExpenseQuery, limit int, before *Cursor) ([]Expense, error)
	GetExpense(ctx context.Context, id uuid.UUID) (*Expense, error)

	// Invoices.
	ListInvoices(ctx context.Context, q InvoiceQuery, asOf time.Time, limit int, before *Cursor) ([]Invoice, error)
	GetInvoice(ctx context.Context, id uuid.UUID) (*Invoice, error)
	ListInvoiceLines(ctx context.Context, invoiceID uuid.UUID) ([]InvoiceLine, error)

	// Payments.
	ListPayments(ctx context.Context, q PaymentQuery, limit int, before *Cursor) ([]Payment, error)

	// Budgets.
	ListBudgets(ctx context.Context, q BudgetQuery, limit int, before *Cursor) ([]Budget, error)
	GetBudget(ctx context.Context, id uuid.UUID) (*Budget, error)
}

// PGStore is the Postgres-backed store.
type PGStore struct{ db *pgxpool.Pool }

// NewPGStore builds a store over the pool.
func NewPGStore(db *pgxpool.Pool) *PGStore { return &PGStore{db: db} }

// rowScanner is satisfied by both pgx.Row and pgx.Rows.
type rowScanner interface {
	Scan(dest ...any) error
}

// ── overview ────────────────────────────────────────────────────────────────

// overviewQuery assembles the §8.4 headline from the tables directly. The
// budget subqueries reuse the same actual-spend rule as ListBudgets: settled
// expenses in the period and scope, with department/unit budgets matching no
// expense attribution yet.
const overviewQuery = `
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
  ) AS budgets_over_threshold`

func (s *PGStore) OverviewTotals(ctx context.Context, asOf time.Time) (OverviewTotals, error) {
	var t OverviewTotals
	err := s.db.QueryRow(ctx, overviewQuery, asOf).Scan(
		&t.CashMinor, &t.ReceivablesOutstandingMinor, &t.ReceivablesOverdueMinor,
		&t.Invoiced90DMinor, &t.Collected90DMinor, &t.Expenses30DMinor,
		&t.PendingExpenses, &t.OverdueInvoiceCount, &t.UpcomingRenewals30D,
		&t.ActiveBudgetCount, &t.BudgetsOverThreshold)
	if err != nil {
		return OverviewTotals{}, fmt.Errorf("finance: overview totals: %w", err)
	}
	return t, nil
}

// ── accounts and cash ───────────────────────────────────────────────────────

// accountBalanceQuery derives each account's current balance (§8.4): the
// opening balance plus payments received into it, minus settled expenses paid
// from it. Only paid/reimbursed/reconciled expenses have left the account.
const accountBalanceQuery = `
SELECT a.id, a.name, a.account_type, a.currency, a.opening_balance_minor, a.active,
  a.version, a.created_at, a.updated_at,
  COALESCE(a.opening_balance_minor, 0)
    + COALESCE((SELECT SUM(p.amount_minor) FROM finance.payments p WHERE p.account_id = a.id), 0)
    - COALESCE((SELECT SUM(e.amount_minor) FROM finance.expenses e
                WHERE e.account_id = a.id AND e.status IN ('paid','reimbursed','reconciled')), 0)
  AS balance_minor
FROM finance.accounts a
ORDER BY a.name, a.id`

func scanAccountBalance(row rowScanner) (AccountBalance, error) {
	var a AccountBalance
	err := row.Scan(&a.ID, &a.Name, &a.AccountType, &a.Currency, &a.OpeningBalanceMinor,
		&a.Active, &a.Version, &a.CreatedAt, &a.UpdatedAt, &a.BalanceMinor)
	return a, err
}

func (s *PGStore) ListAccounts(ctx context.Context) ([]AccountBalance, error) {
	rows, err := s.db.Query(ctx, accountBalanceQuery)
	if err != nil {
		return nil, fmt.Errorf("finance: list accounts: %w", err)
	}
	defer rows.Close()
	out := []AccountBalance{}
	for rows.Next() {
		a, err := scanAccountBalance(rows)
		if err != nil {
			return nil, fmt.Errorf("finance: scan account: %w", err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *PGStore) CashFlow(ctx context.Context, since time.Time) (int64, int64, error) {
	var inflow, outflow int64
	err := s.db.QueryRow(ctx, `
		SELECT
		  COALESCE((SELECT SUM(amount_minor) FROM finance.payments WHERE received_on >= $1::date), 0),
		  COALESCE((SELECT SUM(amount_minor) FROM finance.expenses
		            WHERE spent_on >= $1::date AND status IN ('paid','reimbursed','reconciled')), 0)`,
		since).Scan(&inflow, &outflow)
	if err != nil {
		return 0, 0, fmt.Errorf("finance: cash flow: %w", err)
	}
	return inflow, outflow, nil
}

// ── receivables ─────────────────────────────────────────────────────────────

// OpenReceivables returns every issued/partially-paid/overdue invoice that
// still has money outstanding, with its allocated total so the handler can age
// it. An invoice whose lines were never priced (total_minor NULL) is treated as
// zero outstanding and drops out.
func (s *PGStore) OpenReceivables(ctx context.Context, asOf time.Time) ([]Invoice, error) {
	rows, err := s.db.Query(ctx, invoiceSelect+`
		WHERE i.status IN ('issued','partially_paid','overdue')
		ORDER BY i.due_date NULLS LAST, i.created_at, i.id`)
	if err != nil {
		return nil, fmt.Errorf("finance: open receivables: %w", err)
	}
	defer rows.Close()
	out := []Invoice{}
	for rows.Next() {
		inv, err := scanInvoice(rows)
		if err != nil {
			return nil, fmt.Errorf("finance: scan receivable: %w", err)
		}
		if inv.OutstandingMinor > 0 {
			out = append(out, inv)
		}
	}
	_ = asOf // the outstanding rule does not depend on the as-of date
	return out, rows.Err()
}

// ── recurring costs ─────────────────────────────────────────────────────────

const recurringCostCols = `id, vendor, COALESCE(description,''), amount_minor, currency, cycle,
	next_renewal_on, owner_person_id, active, version, created_at, updated_at`

func scanRecurringCost(row rowScanner) (RecurringCost, error) {
	var c RecurringCost
	err := row.Scan(&c.ID, &c.Vendor, &c.Description, &c.AmountMinor, &c.Currency, &c.Cycle,
		&c.NextRenewalOn, &c.OwnerPersonID, &c.Active, &c.Version, &c.CreatedAt, &c.UpdatedAt)
	c.MonthlyEquivalentMinor = monthlyEquivalent(c.AmountMinor, c.Cycle)
	return c, err
}

func (s *PGStore) ListRecurringCosts(ctx context.Context) ([]RecurringCost, error) {
	rows, err := s.db.Query(ctx, `SELECT `+recurringCostCols+` FROM finance.recurring_costs
		ORDER BY next_renewal_on, vendor, id`)
	if err != nil {
		return nil, fmt.Errorf("finance: list recurring costs: %w", err)
	}
	defer rows.Close()
	out := []RecurringCost{}
	for rows.Next() {
		c, err := scanRecurringCost(rows)
		if err != nil {
			return nil, fmt.Errorf("finance: scan recurring cost: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// ── project profitability ───────────────────────────────────────────────────

func (s *PGStore) ProjectName(ctx context.Context, id uuid.UUID) (string, bool, error) {
	var name string
	err := s.db.QueryRow(ctx, `SELECT name FROM work.projects WHERE id = $1`, id).Scan(&name)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("finance: project name: %w", err)
	}
	return name, true, nil
}

// projectInvoicesCTE links an invoice to a project through its billing
// milestone: an invoice names a contracts.milestones row, and that milestone
// names the work milestone, which names the project. Invoices with no milestone
// (recurring schedules) are therefore not attributed to a project yet.
const projectInvoicesCTE = `
WITH project_invoices AS (
  SELECT i.id, i.total_minor
  FROM finance.invoices i
  WHERE i.milestone_id IN (
    SELECT cm.id FROM contracts.milestones cm
    JOIN work.milestones wm ON wm.id = cm.project_milestone_id
    WHERE wm.project_id = $1
  )
)`

func (s *PGStore) ProjectRevenue(ctx context.Context, projectID uuid.UUID) (int64, int64, error) {
	var invoiced, collected int64
	err := s.db.QueryRow(ctx, projectInvoicesCTE+`
		SELECT
		  COALESCE((SELECT SUM(total_minor) FROM project_invoices), 0),
		  COALESCE((SELECT SUM(pa.amount_minor) FROM finance.payment_allocations pa
		            WHERE pa.invoice_id IN (SELECT id FROM project_invoices)), 0)`,
		projectID).Scan(&invoiced, &collected)
	if err != nil {
		return 0, 0, fmt.Errorf("finance: project revenue: %w", err)
	}
	return invoiced, collected, nil
}

func (s *PGStore) ProjectCosts(ctx context.Context, projectID uuid.UUID) (int64, error) {
	var costs int64
	err := s.db.QueryRow(ctx, `
		SELECT COALESCE(SUM(amount_minor), 0) FROM finance.expenses
		WHERE project_id = $1 AND status IN ('approved','paid','reimbursed','reconciled')`,
		projectID).Scan(&costs)
	if err != nil {
		return 0, fmt.Errorf("finance: project costs: %w", err)
	}
	return costs, nil
}

// ── expense categories ──────────────────────────────────────────────────────

func (s *PGStore) ListExpenseCategories(ctx context.Context) ([]ExpenseCategory, error) {
	rows, err := s.db.Query(ctx, `SELECT key, label, sort_order, active
		FROM finance.expense_categories ORDER BY sort_order, key`)
	if err != nil {
		return nil, fmt.Errorf("finance: list expense categories: %w", err)
	}
	defer rows.Close()
	out := []ExpenseCategory{}
	for rows.Next() {
		var c ExpenseCategory
		if err := rows.Scan(&c.Key, &c.Label, &c.SortOrder, &c.Active); err != nil {
			return nil, fmt.Errorf("finance: scan expense category: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// ── expenses ────────────────────────────────────────────────────────────────

const expenseCols = `id, ref, claimant_person_id, category_key, project_id, amount_minor, currency,
	spent_on, COALESCE(vendor,''), payment_method, account_id, status,
	submitted_at, decided_by, decided_at, COALESCE(decision_reason,''), COALESCE(notes,''),
	version, created_at, updated_at`

func scanExpense(row rowScanner) (Expense, error) {
	var e Expense
	// payment_method is nullable, and pgx scans a nullable named string through
	// a *string temporary more reliably than a pointer to the named type.
	var method *string
	err := row.Scan(&e.ID, &e.Ref, &e.ClaimantPersonID, &e.CategoryKey, &e.ProjectID, &e.AmountMinor,
		&e.Currency, &e.SpentOn, &e.Vendor, &method, &e.AccountID, &e.Status,
		&e.SubmittedAt, &e.DecidedBy, &e.DecidedAt, &e.DecisionReason, &e.Notes,
		&e.Version, &e.CreatedAt, &e.UpdatedAt)
	if method != nil {
		m := PaymentMethod(*method)
		e.PaymentMethod = &m
	}
	return e, err
}

func (s *PGStore) ListExpenses(ctx context.Context, q ExpenseQuery, limit int, before *Cursor) ([]Expense, error) {
	args := []any{}
	where := "TRUE"
	add := func(cond string, v any) {
		args = append(args, v)
		where += fmt.Sprintf(" AND "+cond, len(args))
	}
	if q.Status != nil {
		add("status = $%d", string(*q.Status))
	}
	if q.CategoryKey != "" {
		add("category_key = $%d", q.CategoryKey)
	}
	if q.ProjectID != nil {
		add("project_id = $%d", *q.ProjectID)
	}
	if q.ClaimantID != nil {
		add("claimant_person_id = $%d", *q.ClaimantID)
	}
	if q.From != nil {
		add("spent_on >= $%d", *q.From)
	}
	if q.To != nil {
		add("spent_on <= $%d", *q.To)
	}
	if before != nil {
		args = append(args, before.At, before.ID)
		where += fmt.Sprintf(" AND (created_at, id) < ($%d, $%d)", len(args)-1, len(args))
	}
	args = append(args, limit)
	rows, err := s.db.Query(ctx, `SELECT `+expenseCols+` FROM finance.expenses
		WHERE `+where+` ORDER BY created_at DESC, id DESC LIMIT $`+fmt.Sprint(len(args)), args...)
	if err != nil {
		return nil, fmt.Errorf("finance: list expenses: %w", err)
	}
	defer rows.Close()
	out := []Expense{}
	for rows.Next() {
		e, err := scanExpense(rows)
		if err != nil {
			return nil, fmt.Errorf("finance: scan expense: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *PGStore) GetExpense(ctx context.Context, id uuid.UUID) (*Expense, error) {
	e, err := scanExpense(s.db.QueryRow(ctx, `SELECT `+expenseCols+` FROM finance.expenses WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("finance: get expense: %w", err)
	}
	return &e, nil
}

// ── invoices ────────────────────────────────────────────────────────────────

// invoiceSelect joins the organization and contract names the view shows, and
// derives the allocated total so outstanding can be computed.
const invoiceSelect = `SELECT i.id, i.ref, i.organization_id, COALESCE(o.name,''), i.contract_id,
	COALESCE(c.ref,''), i.milestone_id, i.issue_date, i.due_date, i.total_minor, i.currency,
	i.status, i.issued_at, i.paid_at, COALESCE(i.e_invoice_ref,''), COALESCE(i.notes,''),
	i.version, i.created_at, i.updated_at,
	COALESCE((SELECT SUM(pa.amount_minor) FROM finance.payment_allocations pa WHERE pa.invoice_id = i.id), 0)
	FROM finance.invoices i
	LEFT JOIN crm.organizations o ON o.id = i.organization_id
	LEFT JOIN contracts.contracts c ON c.id = i.contract_id`

func scanInvoice(row rowScanner) (Invoice, error) {
	var inv Invoice
	err := row.Scan(&inv.ID, &inv.Ref, &inv.OrganizationID, &inv.OrganizationName, &inv.ContractID,
		&inv.ContractRef, &inv.MilestoneID, &inv.IssueDate, &inv.DueDate, &inv.TotalMinor, &inv.Currency,
		&inv.Status, &inv.IssuedAt, &inv.PaidAt, &inv.EInvoiceRef, &inv.Notes,
		&inv.Version, &inv.CreatedAt, &inv.UpdatedAt, &inv.AmountPaidMinor)
	inv.OutstandingMinor = outstanding(inv.TotalMinor, inv.AmountPaidMinor)
	return inv, err
}

// outstanding is the total minus what has been allocated, floored at zero:
// an overpayment must not read as negative debt.
func outstanding(total *int64, paid int64) int64 {
	if total == nil {
		return 0
	}
	v := *total - paid
	if v < 0 {
		return 0
	}
	return v
}

func (s *PGStore) ListInvoices(ctx context.Context, q InvoiceQuery, asOf time.Time, limit int, before *Cursor) ([]Invoice, error) {
	args := []any{}
	where := "TRUE"
	add := func(cond string, v any) {
		args = append(args, v)
		where += fmt.Sprintf(" AND "+cond, len(args))
	}
	if q.Status != nil {
		add("i.status = $%d", string(*q.Status))
	}
	if q.OrganizationID != nil {
		add("i.organization_id = $%d", *q.OrganizationID)
	}
	if q.ContractID != nil {
		add("i.contract_id = $%d", *q.ContractID)
	}
	if q.Overdue {
		add("i.due_date < $%d::date", asOf)
		where += " AND i.status IN ('issued','partially_paid','overdue')"
	}
	if before != nil {
		args = append(args, before.At, before.ID)
		where += fmt.Sprintf(" AND (i.created_at, i.id) < ($%d, $%d)", len(args)-1, len(args))
	}
	args = append(args, limit)
	rows, err := s.db.Query(ctx, invoiceSelect+` WHERE `+where+
		` ORDER BY i.created_at DESC, i.id DESC LIMIT $`+fmt.Sprint(len(args)), args...)
	if err != nil {
		return nil, fmt.Errorf("finance: list invoices: %w", err)
	}
	defer rows.Close()
	out := []Invoice{}
	for rows.Next() {
		inv, err := scanInvoice(rows)
		if err != nil {
			return nil, fmt.Errorf("finance: scan invoice: %w", err)
		}
		// `overdue` keeps only invoices that still owe money.
		if q.Overdue && inv.OutstandingMinor <= 0 {
			continue
		}
		out = append(out, inv)
	}
	return out, rows.Err()
}

func (s *PGStore) GetInvoice(ctx context.Context, id uuid.UUID) (*Invoice, error) {
	inv, err := scanInvoice(s.db.QueryRow(ctx, invoiceSelect+` WHERE i.id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("finance: get invoice: %w", err)
	}
	return &inv, nil
}

func (s *PGStore) ListInvoiceLines(ctx context.Context, invoiceID uuid.UUID) ([]InvoiceLine, error) {
	rows, err := s.db.Query(ctx, `SELECT id, invoice_id, description, quantity, unit_price_minor,
		tax_rate_bp, amount_minor, sort_order
		FROM finance.invoice_lines WHERE invoice_id = $1 ORDER BY sort_order, id`, invoiceID)
	if err != nil {
		return nil, fmt.Errorf("finance: list invoice lines: %w", err)
	}
	defer rows.Close()
	out := []InvoiceLine{}
	for rows.Next() {
		var l InvoiceLine
		if err := rows.Scan(&l.ID, &l.InvoiceID, &l.Description, &l.Quantity, &l.UnitPriceMinor,
			&l.TaxRateBP, &l.AmountMinor, &l.SortOrder); err != nil {
			return nil, fmt.Errorf("finance: scan invoice line: %w", err)
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// ── payments ────────────────────────────────────────────────────────────────

const paymentCols = `p.id, p.ref, p.amount_minor, p.currency, p.method, COALESCE(p.reference,''),
	p.received_on, p.account_id, p.reconciled, COALESCE(p.notes,''), p.version, p.created_at, p.updated_at,
	COALESCE((SELECT SUM(pa.amount_minor) FROM finance.payment_allocations pa WHERE pa.payment_id = p.id), 0)`

func scanPayment(row rowScanner) (Payment, error) {
	var p Payment
	err := row.Scan(&p.ID, &p.Ref, &p.AmountMinor, &p.Currency, &p.Method, &p.Reference,
		&p.ReceivedOn, &p.AccountID, &p.Reconciled, &p.Notes, &p.Version, &p.CreatedAt, &p.UpdatedAt,
		&p.AllocatedMinor)
	p.UnallocatedMinor = unallocated(p.AmountMinor, p.AllocatedMinor)
	return p, err
}

// unallocated is the part of a payment not yet matched to an invoice.
func unallocated(amount *int64, allocated int64) int64 {
	if amount == nil {
		return 0
	}
	v := *amount - allocated
	if v < 0 {
		return 0
	}
	return v
}

func (s *PGStore) ListPayments(ctx context.Context, q PaymentQuery, limit int, before *Cursor) ([]Payment, error) {
	args := []any{}
	where := "TRUE"
	add := func(cond string, v any) {
		args = append(args, v)
		where += fmt.Sprintf(" AND "+cond, len(args))
	}
	if q.From != nil {
		add("p.received_on >= $%d", *q.From)
	}
	if q.To != nil {
		add("p.received_on <= $%d", *q.To)
	}
	if q.Method != nil {
		add("p.method = $%d", string(*q.Method))
	}
	if before != nil {
		args = append(args, before.At, before.ID)
		where += fmt.Sprintf(" AND (p.created_at, p.id) < ($%d, $%d)", len(args)-1, len(args))
	}
	args = append(args, limit)
	rows, err := s.db.Query(ctx, `SELECT `+paymentCols+` FROM finance.payments p
		WHERE `+where+` ORDER BY p.created_at DESC, p.id DESC LIMIT $`+fmt.Sprint(len(args)), args...)
	if err != nil {
		return nil, fmt.Errorf("finance: list payments: %w", err)
	}
	defer rows.Close()
	out := []Payment{}
	for rows.Next() {
		p, err := scanPayment(rows)
		if err != nil {
			return nil, fmt.Errorf("finance: scan payment: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// ── budgets ─────────────────────────────────────────────────────────────────

// budgetSelect derives the actual spend for each budget (§8.4). The scope
// decides which expenses count: a company-period budget matches every expense
// in the period; a project budget matches that project's expenses; department
// and unit budgets have no expense attribution in the schema yet, so their
// actual is zero rather than a fabricated figure.
const budgetSelect = `SELECT b.id, b.scope, COALESCE(b.scope_ref,''), b.period_start, b.period_end,
	COALESCE(b.category_key,''), b.amount_minor, b.currency, b.status, b.owner_person_id,
	b.version, b.created_at, b.updated_at,
	(SELECT COALESCE(SUM(e.amount_minor), 0) FROM finance.expenses e
	   WHERE e.status IN ('approved','paid','reimbursed','reconciled')
	     AND e.spent_on BETWEEN b.period_start AND b.period_end
	     AND (b.category_key IS NULL OR e.category_key = b.category_key)
	     AND CASE b.scope
	           WHEN 'company_period' THEN true
	           WHEN 'project' THEN e.project_id::text = b.scope_ref
	           ELSE false
	         END) AS actual_minor
	FROM finance.budgets b`

func scanBudget(row rowScanner) (Budget, error) {
	var b Budget
	var actual int64
	err := row.Scan(&b.ID, &b.Scope, &b.ScopeRef, &b.PeriodStart, &b.PeriodEnd,
		&b.CategoryKey, &b.AmountMinor, &b.Currency, &b.Status, &b.OwnerPersonID,
		&b.Version, &b.CreatedAt, &b.UpdatedAt, &actual)
	b.ActualMinor = &actual
	if b.AmountMinor != nil {
		v := *b.AmountMinor - actual
		b.VarianceMinor = &v
	}
	return b, err
}

func (s *PGStore) ListBudgets(ctx context.Context, q BudgetQuery, limit int, before *Cursor) ([]Budget, error) {
	args := []any{}
	where := "TRUE"
	add := func(cond string, v any) {
		args = append(args, v)
		where += fmt.Sprintf(" AND "+cond, len(args))
	}
	if q.Scope != nil {
		add("b.scope = $%d", string(*q.Scope))
	}
	if q.Status != nil {
		add("b.status = $%d", string(*q.Status))
	}
	if q.Period != "" {
		add("to_char(b.period_start, 'YYYY-MM') = $%d", q.Period)
	}
	if before != nil {
		args = append(args, before.At, before.ID)
		where += fmt.Sprintf(" AND (b.created_at, b.id) < ($%d, $%d)", len(args)-1, len(args))
	}
	args = append(args, limit)
	rows, err := s.db.Query(ctx, budgetSelect+` WHERE `+where+
		` ORDER BY b.created_at DESC, b.id DESC LIMIT $`+fmt.Sprint(len(args)), args...)
	if err != nil {
		return nil, fmt.Errorf("finance: list budgets: %w", err)
	}
	defer rows.Close()
	out := []Budget{}
	for rows.Next() {
		b, err := scanBudget(rows)
		if err != nil {
			return nil, fmt.Errorf("finance: scan budget: %w", err)
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func (s *PGStore) GetBudget(ctx context.Context, id uuid.UUID) (*Budget, error) {
	b, err := scanBudget(s.db.QueryRow(ctx, budgetSelect+` WHERE b.id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("finance: get budget: %w", err)
	}
	return &b, nil
}
