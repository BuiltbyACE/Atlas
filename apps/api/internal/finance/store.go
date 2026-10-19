package finance

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/safaristack/atlas/apps/api/internal/outbox"
)

// Cursor is a keyset position (created_at, id), newest first (§6.6).
type Cursor struct {
	At time.Time
	ID uuid.UUID
}

// Store is the finance read model. It answers the queries the finance screens
// make (§8.4). Writes go through Writer.
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

// Writer is the finance write lifecycle (§8.4). Go is the sole writer: every
// transition is a single transaction that also appends its outbox event(s), so
// a claim cannot move without its event and vice versa.
//
// expectedVersion is the optimistic-concurrency check; a stale value is
// ErrConflict (HTTP 412). Each status write is guarded so a transition that
// loses a race updates nothing.
type Writer interface {
	// Expenses.
	CreateExpense(ctx context.Context, e Expense) (Expense, error)
	UpdateExpense(ctx context.Context, e Expense, expectedVersion int) (Expense, error)
	SubmitExpense(ctx context.Context, id uuid.UUID, expectedVersion int, submittedAt time.Time, ev outbox.Event) (Expense, error)
	DecideExpense(ctx context.Context, id uuid.UUID, expectedVersion int, status ExpenseStatus,
		decidedBy uuid.UUID, decidedAt time.Time, reason string, ev outbox.Event) (Expense, error)
	SettleExpense(ctx context.Context, id uuid.UUID, expectedVersion int, status ExpenseStatus,
		accountID *uuid.UUID, paidOn *time.Time) (Expense, error)
	ReconcileExpense(ctx context.Context, id uuid.UUID, expectedVersion int) (Expense, error)
	ExpenseCategoryActive(ctx context.Context, key string) (bool, error)

	// Invoices.
	CreateInvoice(ctx context.Context, inv Invoice, lines []InvoiceLine) (Invoice, error)
	UpdateInvoice(ctx context.Context, inv Invoice, lines []InvoiceLine, expectedVersion int) (Invoice, error)
	ApproveInvoice(ctx context.Context, id uuid.UUID, expectedVersion int) (Invoice, error)
	IssueInvoice(ctx context.Context, id uuid.UUID, expectedVersion int,
		issueDate, dueDate time.Time, issuedAt time.Time, ev outbox.Event) (Invoice, error)
	DisputeInvoice(ctx context.Context, id uuid.UUID, expectedVersion int) (Invoice, error)
	WriteOffInvoice(ctx context.Context, id uuid.UUID, expectedVersion int) (Invoice, error)
	ContractPaymentTermDays(ctx context.Context, contractID uuid.UUID) (int, bool, error)

	// Payments.
	GetPayment(ctx context.Context, id uuid.UUID) (*Payment, error)
	CreatePayment(ctx context.Context, p Payment, allocations []PaymentAllocationInput,
		build func(ref string) (outbox.Event, error)) (Payment, error)
	AllocatePayment(ctx context.Context, id uuid.UUID, expectedVersion int,
		allocations []PaymentAllocationInput) (Payment, error)
	ReconcilePayment(ctx context.Context, id uuid.UUID, expectedVersion int, reconciled bool) (Payment, error)

	// Budgets.
	CreateBudget(ctx context.Context, b Budget) (Budget, error)
	UpdateBudget(ctx context.Context, b Budget, expectedVersion int) (Budget, error)
	ApproveBudget(ctx context.Context, id uuid.UUID, expectedVersion int, ev outbox.Event) (Budget, error)
	CloseBudget(ctx context.Context, id uuid.UUID, expectedVersion int) (Budget, error)

	// Accounts.
	GetAccount(ctx context.Context, id uuid.UUID) (*AccountBalance, error)
	CreateAccount(ctx context.Context, a Account) (AccountBalance, error)
	UpdateAccount(ctx context.Context, a Account, expectedVersion int) (AccountBalance, error)

	// Recurring costs.
	GetRecurringCost(ctx context.Context, id uuid.UUID) (*RecurringCost, error)
	CreateRecurringCost(ctx context.Context, c RecurringCost) (RecurringCost, error)
	UpdateRecurringCost(ctx context.Context, c RecurringCost, expectedVersion int) (RecurringCost, error)
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
const accountBalanceSelect = `
SELECT a.id, a.name, a.account_type, a.currency, a.opening_balance_minor, a.active,
  a.version, a.created_at, a.updated_at,
  COALESCE(a.opening_balance_minor, 0)
    + COALESCE((SELECT SUM(p.amount_minor) FROM finance.payments p WHERE p.account_id = a.id), 0)
    - COALESCE((SELECT SUM(e.amount_minor) FROM finance.expenses e
                WHERE e.account_id = a.id AND e.status IN ('paid','reimbursed','reconciled')), 0)
  AS balance_minor
FROM finance.accounts a`

const accountBalanceQuery = accountBalanceSelect + `
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
	receipt_file_id, COALESCE(tax_invoice_ref,''), paid_on,
	version, created_at, updated_at`

func scanExpense(row rowScanner) (Expense, error) {
	var e Expense
	// payment_method is nullable, and pgx scans a nullable named string through
	// a *string temporary more reliably than a pointer to the named type.
	var method *string
	err := row.Scan(&e.ID, &e.Ref, &e.ClaimantPersonID, &e.CategoryKey, &e.ProjectID, &e.AmountMinor,
		&e.Currency, &e.SpentOn, &e.Vendor, &method, &e.AccountID, &e.Status,
		&e.SubmittedAt, &e.DecidedBy, &e.DecidedAt, &e.DecisionReason, &e.Notes,
		&e.ReceiptFileID, &e.TaxInvoiceRef, &e.PaidOn,
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

// ── write helpers ───────────────────────────────────────────────────────────

// paymentSelect is the payment row with its allocated total, for a single read.
const paymentSelect = `SELECT ` + paymentCols + ` FROM finance.payments p`

// nullIfEmpty stores an optional text field as NULL rather than an empty string,
// so "not recorded" has one representation.
func nullIfEmpty(s string) *string {
	s = trim(s)
	if s == "" {
		return nil
	}
	return &s
}

// allocateRefTx formats the next human reference for a finance entity in the
// year of at (EXP-2026-0142, INV-…, PAY-…) from core.sequences.
func allocateRefTx(ctx context.Context, tx pgx.Tx, refType string, at time.Time) (string, error) {
	if at.IsZero() {
		at = time.Now().UTC()
	}
	var ref string
	if err := tx.QueryRow(ctx, `SELECT core.next_reference($1, $2)`, refType, at.UTC().Year()).Scan(&ref); err != nil {
		return "", fmt.Errorf("finance: allocate ref %q: %w", refType, err)
	}
	return ref, nil
}

// versionConflict turns an empty UPDATE into the precise error: not found, or a
// stale version (§6.6).
func (s *PGStore) versionConflict(ctx context.Context, table string, id uuid.UUID, expected int) error {
	var cur int
	err := s.db.QueryRow(ctx, `SELECT version FROM `+table+` WHERE id = $1`, id).Scan(&cur)
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	if err != nil {
		return fmt.Errorf("finance: read version: %w", err)
	}
	return fmt.Errorf("%w: version %d is stale (current %d)", ErrConflict, expected, cur)
}

// writeErr classifies a database error a client caused: an unknown reference or
// a duplicate human reference is a validation problem, not a 500.
func writeErr(op string, err error) error {
	var pg *pgconn.PgError
	if errors.As(err, &pg) {
		switch pg.Code {
		case "23503":
			return fmt.Errorf("%w: %s: a referenced record does not exist", ErrInvalid, op)
		case "23505":
			return fmt.Errorf("%w: %s: a record with that reference already exists", ErrDuplicate, op)
		case "23514":
			return fmt.Errorf("%w: %s: a value is outside the allowed range", ErrInvalid, op)
		}
	}
	return fmt.Errorf("finance: %s: %w", op, err)
}

func (s *PGStore) returnExpenseTx(ctx context.Context, tx pgx.Tx, id uuid.UUID) (Expense, error) {
	e, err := scanExpense(tx.QueryRow(ctx, `SELECT `+expenseCols+` FROM finance.expenses WHERE id = $1`, id))
	if err != nil {
		return Expense{}, fmt.Errorf("finance: read expense: %w", err)
	}
	return e, nil
}

func (s *PGStore) returnInvoiceTx(ctx context.Context, tx pgx.Tx, id uuid.UUID) (Invoice, error) {
	inv, err := scanInvoice(tx.QueryRow(ctx, invoiceSelect+` WHERE i.id = $1`, id))
	if err != nil {
		return Invoice{}, fmt.Errorf("finance: read invoice: %w", err)
	}
	return inv, nil
}

func (s *PGStore) returnPaymentTx(ctx context.Context, tx pgx.Tx, id uuid.UUID) (Payment, error) {
	p, err := scanPayment(tx.QueryRow(ctx, paymentSelect+` WHERE p.id = $1`, id))
	if err != nil {
		return Payment{}, fmt.Errorf("finance: read payment: %w", err)
	}
	return p, nil
}

// ── expense writes ──────────────────────────────────────────────────────────

func (s *PGStore) CreateExpense(ctx context.Context, e Expense) (Expense, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Expense{}, fmt.Errorf("finance: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	ref, err := allocateRefTx(ctx, tx, "exp", e.CreatedAt)
	if err != nil {
		return Expense{}, err
	}
	e.Ref = ref
	created, err := scanExpense(tx.QueryRow(ctx, `
		INSERT INTO finance.expenses (
			id, ref, claimant_person_id, category_key, project_id, amount_minor, currency,
			spent_on, vendor, payment_method, account_id, status, receipt_file_id,
			tax_invoice_ref, notes, version, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)
		RETURNING `+expenseCols,
		e.ID, e.Ref, e.ClaimantPersonID, e.CategoryKey, e.ProjectID, e.AmountMinor, e.Currency,
		e.SpentOn, nullIfEmpty(e.Vendor), e.PaymentMethod, e.AccountID, string(e.Status),
		e.ReceiptFileID, e.TaxInvoiceRef, nullIfEmpty(e.Notes),
		e.Version, e.CreatedAt, e.UpdatedAt))
	if err != nil {
		return Expense{}, writeErr("create expense", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Expense{}, fmt.Errorf("finance: commit: %w", err)
	}
	return created, nil
}

func (s *PGStore) UpdateExpense(ctx context.Context, e Expense, expectedVersion int) (Expense, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Expense{}, fmt.Errorf("finance: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	updated, err := scanExpense(tx.QueryRow(ctx, `
		UPDATE finance.expenses SET
			claimant_person_id = $2, category_key = $3, project_id = $4, amount_minor = $5,
			currency = $6, spent_on = $7, vendor = $8, payment_method = $9, account_id = $10,
			receipt_file_id = $11, tax_invoice_ref = $12, notes = $13, version = version + 1
		WHERE id = $1 AND version = $14
		RETURNING `+expenseCols,
		e.ID, e.ClaimantPersonID, e.CategoryKey, e.ProjectID, e.AmountMinor, e.Currency,
		e.SpentOn, nullIfEmpty(e.Vendor), e.PaymentMethod, e.AccountID, e.ReceiptFileID,
		e.TaxInvoiceRef, nullIfEmpty(e.Notes), expectedVersion))
	if errors.Is(err, pgx.ErrNoRows) {
		return Expense{}, s.versionConflict(ctx, "finance.expenses", e.ID, expectedVersion)
	}
	if err != nil {
		return Expense{}, writeErr("update expense", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Expense{}, fmt.Errorf("finance: commit: %w", err)
	}
	return updated, nil
}

// writeExpenseStatus runs the versioned UPDATE shared by the lifecycle steps.
func (s *PGStore) writeExpenseStatus(ctx context.Context, tx pgx.Tx, id uuid.UUID, expected int, set string, args ...any) (Expense, error) {
	args = append([]any{id}, args...)
	args = append(args, expected)
	query := `UPDATE finance.expenses SET ` + set + `, version = version + 1
		WHERE id = $1 AND version = $` + strconv.Itoa(len(args)) + ` RETURNING ` + expenseCols
	e, err := scanExpense(tx.QueryRow(ctx, query, args...))
	if errors.Is(err, pgx.ErrNoRows) {
		return Expense{}, s.versionConflict(ctx, "finance.expenses", id, expected)
	}
	if err != nil {
		return Expense{}, writeErr("expense transition", err)
	}
	return e, nil
}

func (s *PGStore) SubmitExpense(ctx context.Context, id uuid.UUID, expectedVersion int, submittedAt time.Time, ev outbox.Event) (Expense, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Expense{}, fmt.Errorf("finance: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	e, err := s.writeExpenseStatus(ctx, tx, id, expectedVersion,
		`status = 'submitted', submitted_at = $2, decided_by = NULL, decided_at = NULL, decision_reason = NULL`,
		submittedAt)
	if err != nil {
		return Expense{}, err
	}
	if _, err := outbox.Write(ctx, tx, ev); err != nil {
		return Expense{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Expense{}, fmt.Errorf("finance: commit: %w", err)
	}
	return e, nil
}

func (s *PGStore) DecideExpense(ctx context.Context, id uuid.UUID, expectedVersion int, status ExpenseStatus,
	decidedBy uuid.UUID, decidedAt time.Time, reason string, ev outbox.Event) (Expense, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Expense{}, fmt.Errorf("finance: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	e, err := s.writeExpenseStatus(ctx, tx, id, expectedVersion,
		`status = $2, decided_by = $3, decided_at = $4, decision_reason = $5`,
		string(status), decidedBy, decidedAt, nullIfEmpty(reason))
	if err != nil {
		return Expense{}, err
	}
	if _, err := outbox.Write(ctx, tx, ev); err != nil {
		return Expense{}, err
	}
	// Approval is the moment a claim starts counting against a budget, so it is
	// the moment a threshold can be crossed (§8.4).
	if status == ExpenseApproved {
		if err := s.writeBudgetCrossingsTx(ctx, tx, e); err != nil {
			return Expense{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Expense{}, fmt.Errorf("finance: commit: %w", err)
	}
	return e, nil
}

func (s *PGStore) SettleExpense(ctx context.Context, id uuid.UUID, expectedVersion int, status ExpenseStatus,
	accountID *uuid.UUID, paidOn *time.Time) (Expense, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Expense{}, fmt.Errorf("finance: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	e, err := s.writeExpenseStatus(ctx, tx, id, expectedVersion,
		`status = $2, account_id = COALESCE($3, account_id), paid_on = $4`,
		string(status), accountID, paidOn)
	if err != nil {
		return Expense{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Expense{}, fmt.Errorf("finance: commit: %w", err)
	}
	return e, nil
}

func (s *PGStore) ReconcileExpense(ctx context.Context, id uuid.UUID, expectedVersion int) (Expense, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Expense{}, fmt.Errorf("finance: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	e, err := s.writeExpenseStatus(ctx, tx, id, expectedVersion, `status = 'reconciled'`)
	if err != nil {
		return Expense{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Expense{}, fmt.Errorf("finance: commit: %w", err)
	}
	return e, nil
}

func (s *PGStore) ExpenseCategoryActive(ctx context.Context, key string) (bool, error) {
	var active bool
	err := s.db.QueryRow(ctx, `SELECT active FROM finance.expense_categories WHERE key = $1`, key).Scan(&active)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("finance: expense category: %w", err)
	}
	return active, nil
}

// ── budget thresholds ───────────────────────────────────────────────────────

// BudgetThresholdsBP are the §8.4 review thresholds: 75%, 90% and 100% of a
// budget's amount, in basis points.
var BudgetThresholdsBP = []int{7500, 9000, 10000}

// writeBudgetCrossingsTx emits finance.budget.threshold_crossed.v1 for every
// approved budget whose actual spend crosses a threshold because this expense
// became approved. The "before" actual excludes the expense being approved, so
// only a genuine crossing — not an already-over budget — produces an event.
func (s *PGStore) writeBudgetCrossingsTx(ctx context.Context, tx pgx.Tx, e Expense) error {
	if e.AmountMinor == nil {
		return nil
	}
	var projectRef any
	if e.ProjectID != nil {
		projectRef = e.ProjectID.String()
	}
	rows, err := tx.Query(ctx, `
		SELECT b.id, b.scope, COALESCE(b.scope_ref,''), b.amount_minor, b.currency,
		  COALESCE((SELECT SUM(x.amount_minor) FROM finance.expenses x
		    WHERE x.status IN ('approved','paid','reimbursed','reconciled')
		      AND x.spent_on BETWEEN b.period_start AND b.period_end
		      AND (b.category_key IS NULL OR x.category_key = b.category_key)
		      AND CASE b.scope
		            WHEN 'company_period' THEN true
		            WHEN 'project' THEN x.project_id::text = b.scope_ref
		            ELSE false
		          END
		      AND x.id <> $1), 0) AS before_minor,
		  COALESCE((SELECT SUM(x.amount_minor) FROM finance.expenses x
		    WHERE x.status IN ('approved','paid','reimbursed','reconciled')
		      AND x.spent_on BETWEEN b.period_start AND b.period_end
		      AND (b.category_key IS NULL OR x.category_key = b.category_key)
		      AND CASE b.scope
		            WHEN 'company_period' THEN true
		            WHEN 'project' THEN x.project_id::text = b.scope_ref
		            ELSE false
		          END), 0) AS after_minor
		FROM finance.budgets b
		WHERE b.status = 'approved' AND b.amount_minor IS NOT NULL AND b.amount_minor > 0
		  AND $2::date BETWEEN b.period_start AND b.period_end
		  AND (b.category_key IS NULL OR b.category_key = $3)
		  AND (b.scope = 'company_period' OR (b.scope = 'project' AND b.scope_ref = $4))`,
		e.ID, e.SpentOn, e.CategoryKey, projectRef)
	if err != nil {
		return fmt.Errorf("finance: budget crossings: %w", err)
	}
	defer rows.Close()

	type crossing struct {
		id       uuid.UUID
		scope    string
		ref      string
		amount   int64
		currency string
		before   int64
		after    int64
	}
	var crossings []crossing
	for rows.Next() {
		var c crossing
		if err := rows.Scan(&c.id, &c.scope, &c.ref, &c.amount, &c.currency, &c.before, &c.after); err != nil {
			return fmt.Errorf("finance: scan budget crossing: %w", err)
		}
		crossings = append(crossings, c)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for _, c := range crossings {
		for _, bp := range BudgetThresholdsBP {
			if crossed(c.before, c.after, c.amount, bp) {
				amount := c.amount
				ev, err := outbox.NewBudgetThresholdCrossed(outbox.BudgetRef{
					ID: c.id, Scope: c.scope, ScopeRef: c.ref, AmountMinor: &amount, Currency: c.currency,
					Status: string(BudgetApproved),
				}, c.after)
				if err != nil {
					return err
				}
				if _, err := outbox.Write(ctx, tx, ev); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// crossed reports whether spend moved across threshold bp of amount. The
// comparison is in basis points so no float touches money.
func crossed(before, after, amount int64, bp int) bool {
	return before*10000 < int64(bp)*amount && after*10000 >= int64(bp)*amount
}

// ── invoice writes ──────────────────────────────────────────────────────────

func (s *PGStore) insertInvoiceLines(ctx context.Context, tx pgx.Tx, invoiceID uuid.UUID, lines []InvoiceLine) error {
	for _, l := range lines {
		if _, err := tx.Exec(ctx, `
			INSERT INTO finance.invoice_lines (
				id, invoice_id, description, quantity, unit_price_minor, tax_rate_bp, amount_minor, sort_order)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
			l.ID, invoiceID, l.Description, l.Quantity, l.UnitPriceMinor, l.TaxRateBP, l.AmountMinor, l.SortOrder); err != nil {
			return writeErr("insert invoice line", err)
		}
	}
	return nil
}

func (s *PGStore) CreateInvoice(ctx context.Context, inv Invoice, lines []InvoiceLine) (Invoice, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Invoice{}, fmt.Errorf("finance: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	ref, err := allocateRefTx(ctx, tx, "inv", inv.CreatedAt)
	if err != nil {
		return Invoice{}, err
	}
	inv.Ref = ref
	if _, err := tx.Exec(ctx, `
		INSERT INTO finance.invoices (
			id, ref, organization_id, contract_id, milestone_id, issue_date, due_date,
			total_minor, currency, status, notes, version, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`,
		inv.ID, inv.Ref, inv.OrganizationID, inv.ContractID, inv.MilestoneID, inv.IssueDate,
		inv.DueDate, inv.TotalMinor, inv.Currency, string(inv.Status), nullIfEmpty(inv.Notes),
		inv.Version, inv.CreatedAt, inv.UpdatedAt); err != nil {
		return Invoice{}, writeErr("create invoice", err)
	}
	if err := s.insertInvoiceLines(ctx, tx, inv.ID, lines); err != nil {
		return Invoice{}, err
	}
	created, err := s.returnInvoiceTx(ctx, tx, inv.ID)
	if err != nil {
		return Invoice{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Invoice{}, fmt.Errorf("finance: commit: %w", err)
	}
	return created, nil
}

func (s *PGStore) UpdateInvoice(ctx context.Context, inv Invoice, lines []InvoiceLine, expectedVersion int) (Invoice, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Invoice{}, fmt.Errorf("finance: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	tag, err := tx.Exec(ctx, `
		UPDATE finance.invoices SET
			organization_id = $2, contract_id = $3, milestone_id = $4, issue_date = $5,
			due_date = $6, total_minor = $7, currency = $8, notes = $9, version = version + 1
		WHERE id = $1 AND version = $10`,
		inv.ID, inv.OrganizationID, inv.ContractID, inv.MilestoneID, inv.IssueDate, inv.DueDate,
		inv.TotalMinor, inv.Currency, nullIfEmpty(inv.Notes), expectedVersion)
	if err != nil {
		return Invoice{}, writeErr("update invoice", err)
	}
	if tag.RowsAffected() == 0 {
		return Invoice{}, s.versionConflict(ctx, "finance.invoices", inv.ID, expectedVersion)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM finance.invoice_lines WHERE invoice_id = $1`, inv.ID); err != nil {
		return Invoice{}, writeErr("replace invoice lines", err)
	}
	if err := s.insertInvoiceLines(ctx, tx, inv.ID, lines); err != nil {
		return Invoice{}, err
	}
	updated, err := s.returnInvoiceTx(ctx, tx, inv.ID)
	if err != nil {
		return Invoice{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Invoice{}, fmt.Errorf("finance: commit: %w", err)
	}
	return updated, nil
}

// writeInvoiceStatus runs a versioned status UPDATE and returns the invoice.
func (s *PGStore) writeInvoiceStatus(ctx context.Context, id uuid.UUID, expected int, set string, args ...any) (Invoice, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Invoice{}, fmt.Errorf("finance: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	args = append([]any{id}, args...)
	args = append(args, expected)
	query := `UPDATE finance.invoices SET ` + set + `, version = version + 1
		WHERE id = $1 AND version = $` + strconv.Itoa(len(args))
	tag, err := tx.Exec(ctx, query, args...)
	if err != nil {
		return Invoice{}, writeErr("invoice transition", err)
	}
	if tag.RowsAffected() == 0 {
		return Invoice{}, s.versionConflict(ctx, "finance.invoices", id, expected)
	}
	inv, err := s.returnInvoiceTx(ctx, tx, id)
	if err != nil {
		return Invoice{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Invoice{}, fmt.Errorf("finance: commit: %w", err)
	}
	return inv, nil
}

func (s *PGStore) ApproveInvoice(ctx context.Context, id uuid.UUID, expected int) (Invoice, error) {
	return s.writeInvoiceStatus(ctx, id, expected, `status = 'approved'`)
}

func (s *PGStore) DisputeInvoice(ctx context.Context, id uuid.UUID, expected int) (Invoice, error) {
	return s.writeInvoiceStatus(ctx, id, expected, `status = 'disputed'`)
}

func (s *PGStore) WriteOffInvoice(ctx context.Context, id uuid.UUID, expected int) (Invoice, error) {
	return s.writeInvoiceStatus(ctx, id, expected, `status = 'written_off'`)
}

func (s *PGStore) IssueInvoice(ctx context.Context, id uuid.UUID, expected int,
	issueDate, dueDate time.Time, issuedAt time.Time, ev outbox.Event) (Invoice, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Invoice{}, fmt.Errorf("finance: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	e, err := s.writeInvoiceStatusTx(ctx, tx, id, expected,
		`status = 'issued', issue_date = $2, due_date = $3, issued_at = $4`,
		issueDate, dueDate, issuedAt)
	if err != nil {
		return Invoice{}, err
	}
	if _, err := outbox.Write(ctx, tx, ev); err != nil {
		return Invoice{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Invoice{}, fmt.Errorf("finance: commit: %w", err)
	}
	return e, nil
}

// writeInvoiceStatusTx is writeInvoiceStatus inside a caller's transaction.
func (s *PGStore) writeInvoiceStatusTx(ctx context.Context, tx pgx.Tx, id uuid.UUID, expected int, set string, args ...any) (Invoice, error) {
	args = append([]any{id}, args...)
	args = append(args, expected)
	query := `UPDATE finance.invoices SET ` + set + `, version = version + 1
		WHERE id = $1 AND version = $` + strconv.Itoa(len(args))
	tag, err := tx.Exec(ctx, query, args...)
	if err != nil {
		return Invoice{}, writeErr("invoice transition", err)
	}
	if tag.RowsAffected() == 0 {
		return Invoice{}, s.versionConflict(ctx, "finance.invoices", id, expected)
	}
	return s.returnInvoiceTx(ctx, tx, id)
}

// ContractPaymentTermDays reads a contract's stated payment term, when it names
// one in days (e.g. "Net 30", "30 days"). The caller falls back to the default.
func (s *PGStore) ContractPaymentTermDays(ctx context.Context, contractID uuid.UUID) (int, bool, error) {
	var terms string
	err := s.db.QueryRow(ctx,
		`SELECT COALESCE(payment_terms,'') FROM contracts.contracts WHERE id = $1`, contractID).Scan(&terms)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("finance: contract payment terms: %w", err)
	}
	n, ok := paymentTermDays(terms)
	return n, ok, nil
}

var (
	paymentDaysPattern = regexp.MustCompile(`(?i)(\d+)\s*(?:calendar\s+|business\s+|working\s+)?days?`)
	paymentNetPattern  = regexp.MustCompile(`(?i)\bnet\s*(\d+)`)
)

// paymentTermDays extracts a day count from a free-text payment term.
func paymentTermDays(terms string) (int, bool) {
	for _, re := range []*regexp.Regexp{paymentDaysPattern, paymentNetPattern} {
		if m := re.FindStringSubmatch(terms); m != nil {
			if n, err := strconv.Atoi(m[1]); err == nil && n >= 0 && n <= 365 {
				return n, true
			}
		}
	}
	return 0, false
}

// ── payment writes ──────────────────────────────────────────────────────────

// applyAllocationsTx matches a payment to invoices. Each invoice is locked,
// checked against its outstanding balance and the payment's remaining
// unallocated amount, then its status is recomputed from the new total paid.
func (s *PGStore) applyAllocationsTx(ctx context.Context, tx pgx.Tx, paymentID uuid.UUID, amount int64, allocations []PaymentAllocationInput, at time.Time) error {
	var already int64
	if err := tx.QueryRow(ctx,
		`SELECT COALESCE(SUM(amount_minor),0) FROM finance.payment_allocations WHERE payment_id = $1`,
		paymentID).Scan(&already); err != nil {
		return fmt.Errorf("finance: payment allocated: %w", err)
	}
	remaining := amount - already
	affected := make([]uuid.UUID, 0, len(allocations))

	for _, a := range allocations {
		if a.AmountMinor <= 0 {
			return fmt.Errorf("%w: an allocation must be a positive amount", ErrInvalid)
		}
		if a.AmountMinor > remaining {
			return withCode(fmt.Errorf("%w: the allocation exceeds the payment's unallocated balance", ErrConflict), CodePaymentOverAllocated)
		}
		var total *int64
		var status InvoiceStatus
		err := tx.QueryRow(ctx,
			`SELECT total_minor, status FROM finance.invoices WHERE id = $1 FOR UPDATE`, a.InvoiceID).Scan(&total, &status)
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("%w: invoice %s", ErrNotFound, a.InvoiceID)
		}
		if err != nil {
			return fmt.Errorf("finance: lock invoice: %w", err)
		}
		if !invoiceAllocable(status) {
			return withCode(fmt.Errorf("%w: invoice %s is %s and cannot take a payment", ErrInvalid, a.InvoiceID, status), CodeInvoiceNotAllocable)
		}
		var paid int64
		if err := tx.QueryRow(ctx,
			`SELECT COALESCE(SUM(amount_minor),0) FROM finance.payment_allocations WHERE invoice_id = $1`,
			a.InvoiceID).Scan(&paid); err != nil {
			return fmt.Errorf("finance: invoice paid: %w", err)
		}
		if total == nil || outstanding(total, paid) < a.AmountMinor {
			return withCode(fmt.Errorf("%w: invoice %s has less than the allocation outstanding", ErrConflict, a.InvoiceID), CodePaymentOverAllocated)
		}
		id, err := uuid.NewV7()
		if err != nil {
			return fmt.Errorf("finance: new allocation id: %w", err)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO finance.payment_allocations (id, payment_id, invoice_id, amount_minor)
			VALUES ($1,$2,$3,$4)`, id, paymentID, a.InvoiceID, a.AmountMinor); err != nil {
			return writeErr("allocate payment", err)
		}
		remaining -= a.AmountMinor
		affected = append(affected, a.InvoiceID)
	}

	for _, invoiceID := range affected {
		if err := refreshInvoiceStatusTx(ctx, tx, invoiceID, at); err != nil {
			return err
		}
	}
	return nil
}

// refreshInvoiceStatusTx recomputes an invoice's status from what has been
// allocated to it: fully settled is paid, partly settled is partially_paid
// (unless it is disputed, which stays disputed until it is settled).
func refreshInvoiceStatusTx(ctx context.Context, tx pgx.Tx, invoiceID uuid.UUID, at time.Time) error {
	var total *int64
	var status InvoiceStatus
	err := tx.QueryRow(ctx, `SELECT total_minor, status FROM finance.invoices WHERE id = $1`, invoiceID).
		Scan(&total, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("%w: %s", ErrNotFound, invoiceID)
	}
	if err != nil {
		return fmt.Errorf("finance: read invoice status: %w", err)
	}
	if total == nil {
		return nil
	}
	var paid int64
	if err := tx.QueryRow(ctx,
		`SELECT COALESCE(SUM(amount_minor),0) FROM finance.payment_allocations WHERE invoice_id = $1`,
		invoiceID).Scan(&paid); err != nil {
		return fmt.Errorf("finance: invoice paid: %w", err)
	}
	switch {
	case paid >= *total:
		if _, err := tx.Exec(ctx, `UPDATE finance.invoices SET status = 'paid', paid_at = $2, version = version + 1
			WHERE id = $1 AND status <> 'paid'`, invoiceID, at); err != nil {
			return writeErr("mark invoice paid", err)
		}
	case paid > 0 && (status == InvoiceIssued || status == InvoiceOverdue):
		if _, err := tx.Exec(ctx, `UPDATE finance.invoices SET status = 'partially_paid', version = version + 1
			WHERE id = $1`, invoiceID); err != nil {
			return writeErr("mark invoice partially paid", err)
		}
	}
	return nil
}

func (s *PGStore) CreatePayment(ctx context.Context, p Payment, allocations []PaymentAllocationInput,
	build func(ref string) (outbox.Event, error)) (Payment, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Payment{}, fmt.Errorf("finance: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	ref, err := allocateRefTx(ctx, tx, "pay", p.CreatedAt)
	if err != nil {
		return Payment{}, err
	}
	p.Ref = ref
	if _, err := tx.Exec(ctx, `
		INSERT INTO finance.payments (
			id, ref, amount_minor, currency, method, reference, received_on, account_id,
			reconciled, notes, version, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`,
		p.ID, p.Ref, p.AmountMinor, p.Currency, string(p.Method), nullIfEmpty(p.Reference),
		p.ReceivedOn, p.AccountID, p.Reconciled, nullIfEmpty(p.Notes),
		p.Version, p.CreatedAt, p.UpdatedAt); err != nil {
		return Payment{}, writeErr("create payment", err)
	}
	if len(allocations) > 0 {
		if err := s.applyAllocationsTx(ctx, tx, p.ID, *p.AmountMinor, allocations, p.CreatedAt); err != nil {
			return Payment{}, err
		}
	}
	if build != nil {
		ev, err := build(ref)
		if err != nil {
			return Payment{}, err
		}
		if _, err := outbox.Write(ctx, tx, ev); err != nil {
			return Payment{}, err
		}
	}
	created, err := s.returnPaymentTx(ctx, tx, p.ID)
	if err != nil {
		return Payment{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Payment{}, fmt.Errorf("finance: commit: %w", err)
	}
	return created, nil
}

func (s *PGStore) GetPayment(ctx context.Context, id uuid.UUID) (*Payment, error) {
	p, err := scanPayment(s.db.QueryRow(ctx, paymentSelect+` WHERE p.id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("finance: get payment: %w", err)
	}
	return &p, nil
}

func (s *PGStore) AllocatePayment(ctx context.Context, id uuid.UUID, expectedVersion int,
	allocations []PaymentAllocationInput) (Payment, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Payment{}, fmt.Errorf("finance: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var amount *int64
	var version int
	err = tx.QueryRow(ctx, `SELECT amount_minor, version FROM finance.payments WHERE id = $1 FOR UPDATE`, id).
		Scan(&amount, &version)
	if errors.Is(err, pgx.ErrNoRows) {
		return Payment{}, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	if err != nil {
		return Payment{}, fmt.Errorf("finance: lock payment: %w", err)
	}
	if version != expectedVersion {
		return Payment{}, fmt.Errorf("%w: version %d is stale (current %d)", ErrConflict, expectedVersion, version)
	}
	if amount == nil {
		return Payment{}, fmt.Errorf("%w: the payment has no amount", ErrInvalid)
	}
	if err := s.applyAllocationsTx(ctx, tx, id, *amount, allocations, time.Now().UTC()); err != nil {
		return Payment{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE finance.payments SET version = version + 1 WHERE id = $1`, id); err != nil {
		return Payment{}, writeErr("allocate payment", err)
	}
	updated, err := s.returnPaymentTx(ctx, tx, id)
	if err != nil {
		return Payment{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Payment{}, fmt.Errorf("finance: commit: %w", err)
	}
	return updated, nil
}

func (s *PGStore) ReconcilePayment(ctx context.Context, id uuid.UUID, expectedVersion int, reconciled bool) (Payment, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Payment{}, fmt.Errorf("finance: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	tag, err := tx.Exec(ctx, `UPDATE finance.payments SET reconciled = $2, version = version + 1
		WHERE id = $1 AND version = $3`, id, reconciled, expectedVersion)
	if err != nil {
		return Payment{}, writeErr("reconcile payment", err)
	}
	if tag.RowsAffected() == 0 {
		return Payment{}, s.versionConflict(ctx, "finance.payments", id, expectedVersion)
	}
	updated, err := s.returnPaymentTx(ctx, tx, id)
	if err != nil {
		return Payment{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Payment{}, fmt.Errorf("finance: commit: %w", err)
	}
	return updated, nil
}

// ── budget writes ───────────────────────────────────────────────────────────

func (s *PGStore) CreateBudget(ctx context.Context, b Budget) (Budget, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Budget{}, fmt.Errorf("finance: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `
		INSERT INTO finance.budgets (
			id, scope, scope_ref, period_start, period_end, category_key, amount_minor,
			currency, status, owner_person_id, version, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`,
		b.ID, string(b.Scope), nullIfEmpty(b.ScopeRef), b.PeriodStart, b.PeriodEnd,
		nullIfEmpty(b.CategoryKey), b.AmountMinor, b.Currency, string(b.Status),
		b.OwnerPersonID, b.Version, b.CreatedAt, b.UpdatedAt); err != nil {
		return Budget{}, writeErr("create budget", err)
	}
	created, err := scanBudget(tx.QueryRow(ctx, budgetSelect+` WHERE b.id = $1`, b.ID))
	if err != nil {
		return Budget{}, fmt.Errorf("finance: read budget: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Budget{}, fmt.Errorf("finance: commit: %w", err)
	}
	return created, nil
}

func (s *PGStore) UpdateBudget(ctx context.Context, b Budget, expectedVersion int) (Budget, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Budget{}, fmt.Errorf("finance: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	tag, err := tx.Exec(ctx, `
		UPDATE finance.budgets SET
			scope = $2, scope_ref = $3, period_start = $4, period_end = $5, category_key = $6,
			amount_minor = $7, currency = $8, owner_person_id = $9, version = version + 1
		WHERE id = $1 AND version = $10`,
		b.ID, string(b.Scope), nullIfEmpty(b.ScopeRef), b.PeriodStart, b.PeriodEnd,
		nullIfEmpty(b.CategoryKey), b.AmountMinor, b.Currency, b.OwnerPersonID, expectedVersion)
	if err != nil {
		return Budget{}, writeErr("update budget", err)
	}
	if tag.RowsAffected() == 0 {
		return Budget{}, s.versionConflict(ctx, "finance.budgets", b.ID, expectedVersion)
	}
	updated, err := scanBudget(tx.QueryRow(ctx, budgetSelect+` WHERE b.id = $1`, b.ID))
	if err != nil {
		return Budget{}, fmt.Errorf("finance: read budget: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Budget{}, fmt.Errorf("finance: commit: %w", err)
	}
	return updated, nil
}

func (s *PGStore) writeBudgetStatus(ctx context.Context, id uuid.UUID, expected int, status BudgetStatus,
	ev *outbox.Event) (Budget, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Budget{}, fmt.Errorf("finance: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	tag, err := tx.Exec(ctx, `UPDATE finance.budgets SET status = $2, version = version + 1
		WHERE id = $1 AND version = $3`, id, string(status), expected)
	if err != nil {
		return Budget{}, writeErr("budget transition", err)
	}
	if tag.RowsAffected() == 0 {
		return Budget{}, s.versionConflict(ctx, "finance.budgets", id, expected)
	}
	if ev != nil {
		if _, err := outbox.Write(ctx, tx, *ev); err != nil {
			return Budget{}, err
		}
	}
	if status == BudgetApproved {
		if err := s.writeBudgetApprovalCrossingsTx(ctx, tx, id); err != nil {
			return Budget{}, err
		}
	}
	updated, err := scanBudget(tx.QueryRow(ctx, budgetSelect+` WHERE b.id = $1`, id))
	if err != nil {
		return Budget{}, fmt.Errorf("finance: read budget: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Budget{}, fmt.Errorf("finance: commit: %w", err)
	}
	return updated, nil
}

func (s *PGStore) ApproveBudget(ctx context.Context, id uuid.UUID, expected int, ev outbox.Event) (Budget, error) {
	return s.writeBudgetStatus(ctx, id, expected, BudgetApproved, &ev)
}

func (s *PGStore) CloseBudget(ctx context.Context, id uuid.UUID, expected int) (Budget, error) {
	return s.writeBudgetStatus(ctx, id, expected, BudgetClosed, nil)
}

// writeBudgetApprovalCrossingsTx emits a threshold event for every review band
// a newly-approved budget is already past. Approving a ceiling below what has
// already been spent is exactly the case a budget owner needs to hear about.
func (s *PGStore) writeBudgetApprovalCrossingsTx(ctx context.Context, tx pgx.Tx, id uuid.UUID) error {
	var amount *int64
	var actual int64
	err := tx.QueryRow(ctx, `
		SELECT b.amount_minor,
		  COALESCE((SELECT SUM(e.amount_minor) FROM finance.expenses e
		    WHERE e.status IN ('approved','paid','reimbursed','reconciled')
		      AND e.spent_on BETWEEN b.period_start AND b.period_end
		      AND (b.category_key IS NULL OR e.category_key = b.category_key)
		      AND CASE b.scope
		            WHEN 'company_period' THEN true
		            WHEN 'project' THEN e.project_id::text = b.scope_ref
		            ELSE false
		          END), 0)
		FROM finance.budgets b WHERE b.id = $1`, id).Scan(&amount, &actual)
	if err != nil {
		return fmt.Errorf("finance: budget actual: %w", err)
	}
	if amount == nil || *amount <= 0 || actual <= 0 {
		return nil
	}
	var scope, scopeRef, currency string
	if err := tx.QueryRow(ctx, `SELECT scope, COALESCE(scope_ref,''), currency FROM finance.budgets WHERE id = $1`, id).
		Scan(&scope, &scopeRef, &currency); err != nil {
		return fmt.Errorf("finance: budget ref: %w", err)
	}
	for _, bp := range BudgetThresholdsBP {
		if actual*10000 >= int64(bp)*(*amount) {
			ev, err := outbox.NewBudgetThresholdCrossed(outbox.BudgetRef{
				ID: id, Scope: scope, ScopeRef: scopeRef, AmountMinor: amount, Currency: currency,
				Status: string(BudgetApproved),
			}, actual)
			if err != nil {
				return err
			}
			if _, err := outbox.Write(ctx, tx, ev); err != nil {
				return err
			}
		}
	}
	return nil
}

// ── account writes ──────────────────────────────────────────────────────────

func (s *PGStore) GetAccount(ctx context.Context, id uuid.UUID) (*AccountBalance, error) {
	a, err := scanAccountBalance(s.db.QueryRow(ctx, accountBalanceSelect+` WHERE a.id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("finance: get account: %w", err)
	}
	return &a, nil
}

func (s *PGStore) CreateAccount(ctx context.Context, a Account) (AccountBalance, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return AccountBalance{}, fmt.Errorf("finance: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `
		INSERT INTO finance.accounts (
			id, name, account_type, currency, opening_balance_minor, active, version, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		a.ID, a.Name, string(a.AccountType), a.Currency, a.OpeningBalanceMinor, a.Active,
		a.Version, a.CreatedAt, a.UpdatedAt); err != nil {
		return AccountBalance{}, writeErr("create account", err)
	}
	created, err := scanAccountBalance(tx.QueryRow(ctx, accountBalanceSelect+` WHERE a.id = $1`, a.ID))
	if err != nil {
		return AccountBalance{}, fmt.Errorf("finance: read account: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return AccountBalance{}, fmt.Errorf("finance: commit: %w", err)
	}
	return created, nil
}

func (s *PGStore) UpdateAccount(ctx context.Context, a Account, expectedVersion int) (AccountBalance, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return AccountBalance{}, fmt.Errorf("finance: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	tag, err := tx.Exec(ctx, `
		UPDATE finance.accounts SET
			name = $2, account_type = $3, currency = $4, opening_balance_minor = $5,
			active = $6, version = version + 1
		WHERE id = $1 AND version = $7`,
		a.ID, a.Name, string(a.AccountType), a.Currency, a.OpeningBalanceMinor, a.Active, expectedVersion)
	if err != nil {
		return AccountBalance{}, writeErr("update account", err)
	}
	if tag.RowsAffected() == 0 {
		return AccountBalance{}, s.versionConflict(ctx, "finance.accounts", a.ID, expectedVersion)
	}
	updated, err := scanAccountBalance(tx.QueryRow(ctx, accountBalanceSelect+` WHERE a.id = $1`, a.ID))
	if err != nil {
		return AccountBalance{}, fmt.Errorf("finance: read account: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return AccountBalance{}, fmt.Errorf("finance: commit: %w", err)
	}
	return updated, nil
}

// ── recurring-cost writes ───────────────────────────────────────────────────

func (s *PGStore) GetRecurringCost(ctx context.Context, id uuid.UUID) (*RecurringCost, error) {
	c, err := scanRecurringCost(s.db.QueryRow(ctx,
		`SELECT `+recurringCostCols+` FROM finance.recurring_costs WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("finance: get recurring cost: %w", err)
	}
	return &c, nil
}

func (s *PGStore) CreateRecurringCost(ctx context.Context, c RecurringCost) (RecurringCost, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return RecurringCost{}, fmt.Errorf("finance: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `
		INSERT INTO finance.recurring_costs (
			id, vendor, description, amount_minor, currency, cycle, next_renewal_on,
			owner_person_id, active, version, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,
		c.ID, c.Vendor, nullIfEmpty(c.Description), c.AmountMinor, c.Currency, string(c.Cycle),
		c.NextRenewalOn, c.OwnerPersonID, c.Active, c.Version, c.CreatedAt, c.UpdatedAt); err != nil {
		return RecurringCost{}, writeErr("create recurring cost", err)
	}
	created, err := scanRecurringCost(tx.QueryRow(ctx,
		`SELECT `+recurringCostCols+` FROM finance.recurring_costs WHERE id = $1`, c.ID))
	if err != nil {
		return RecurringCost{}, fmt.Errorf("finance: read recurring cost: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return RecurringCost{}, fmt.Errorf("finance: commit: %w", err)
	}
	return created, nil
}

func (s *PGStore) UpdateRecurringCost(ctx context.Context, c RecurringCost, expectedVersion int) (RecurringCost, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return RecurringCost{}, fmt.Errorf("finance: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	tag, err := tx.Exec(ctx, `
		UPDATE finance.recurring_costs SET
			vendor = $2, description = $3, amount_minor = $4, currency = $5, cycle = $6,
			next_renewal_on = $7, owner_person_id = $8, active = $9, version = version + 1
		WHERE id = $1 AND version = $10`,
		c.ID, c.Vendor, nullIfEmpty(c.Description), c.AmountMinor, c.Currency, string(c.Cycle),
		c.NextRenewalOn, c.OwnerPersonID, c.Active, expectedVersion)
	if err != nil {
		return RecurringCost{}, writeErr("update recurring cost", err)
	}
	if tag.RowsAffected() == 0 {
		return RecurringCost{}, s.versionConflict(ctx, "finance.recurring_costs", c.ID, expectedVersion)
	}
	updated, err := scanRecurringCost(tx.QueryRow(ctx,
		`SELECT `+recurringCostCols+` FROM finance.recurring_costs WHERE id = $1`, c.ID))
	if err != nil {
		return RecurringCost{}, fmt.Errorf("finance: read recurring cost: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return RecurringCost{}, fmt.Errorf("finance: commit: %w", err)
	}
	return updated, nil
}
