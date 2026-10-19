package finance

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
)

// Service is the finance read model. This pass is read-only: every method
// answers a question the finance screens ask (§8.4) and Go owns the rules —
// what "outstanding" means, how receivables are banded, which expenses a budget
// covers, how a project's margin is struck. The write lifecycle
// (expenses.submit/decide, invoices.issue, payments.record) lands in a later
// pass, against the same tables.
type Service struct {
	store  Store
	now    func() time.Time
	logger *slog.Logger
}

// NewService builds the finance service.
func NewService(store Store, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{store: store, now: func() time.Time { return time.Now().UTC() }, logger: logger}
}

// asOf is the day a report is measured on, truncated to UTC midnight so a report
// read at 23:59 and one read at 00:01 agree on "today" and the store's
// date-based arithmetic matches the day the response names.
func (s *Service) asOf() time.Time {
	n := s.now()
	return time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, time.UTC)
}

// ── headline ────────────────────────────────────────────────────────────────

// Overview returns the finance headline and the day it was measured on.
func (s *Service) Overview(ctx context.Context) (OverviewTotals, time.Time, error) {
	asOf := s.asOf()
	totals, err := s.store.OverviewTotals(ctx, asOf)
	if err != nil {
		return OverviewTotals{}, asOf, err
	}
	return totals, asOf, nil
}

// ── cash and accounts ───────────────────────────────────────────────────────

// Accounts lists every account with its derived balance (§8.4).
func (s *Service) Accounts(ctx context.Context) ([]AccountBalance, error) {
	return s.store.ListAccounts(ctx)
}

// CashPosition is the assembled cash view (§8.4): the accounts, their summed
// balance, and the money in and out over the trailing 30 days.
type CashPosition struct {
	AsOf         time.Time
	TotalMinor   int64
	Accounts     []AccountBalance
	InflowMinor  int64
	OutflowMinor int64
}

// CashPosition gathers the cash report. The total is the sum of the account
// balances, so it reconciles with the list beside it rather than being a
// separately-computed figure.
func (s *Service) CashPosition(ctx context.Context) (CashPosition, error) {
	asOf := s.asOf()
	accounts, err := s.store.ListAccounts(ctx)
	if err != nil {
		return CashPosition{}, err
	}
	var total int64
	for _, a := range accounts {
		total += a.BalanceMinor
	}
	// The trailing window includes today, so `asOf - 30` is the first day
	// counted.
	inflow, outflow, err := s.store.CashFlow(ctx, asOf.AddDate(0, 0, -30))
	if err != nil {
		return CashPosition{}, err
	}
	return CashPosition{
		AsOf: asOf, TotalMinor: total, Accounts: accounts,
		InflowMinor: inflow, OutflowMinor: outflow,
	}, nil
}

// ── receivables ─────────────────────────────────────────────────────────────

// Receivables is the ageing report (§8.4): open invoices banded by how far past
// due they are, with a per-band and overall total.
type Receivables struct {
	AsOf             time.Time
	TotalOutstanding int64
	Buckets          []Bucket
	Invoices         []Invoice
}

// Receivables ages the open invoices. An invoice is banded by the days between
// its due date and the report day: not yet due is current, then 1-30, 31-60,
// 61-90 and 90+. The bands always appear, even empty, so the screen is stable.
func (s *Service) Receivables(ctx context.Context) (Receivables, error) {
	asOf := s.asOf()
	invoices, err := s.store.OpenReceivables(ctx, asOf)
	if err != nil {
		return Receivables{}, err
	}
	buckets := make([]Bucket, len(AgeingBuckets))
	copy(buckets, AgeingBuckets)
	index := make(map[string]int, len(buckets))
	for i, b := range buckets {
		index[b.Key] = i
	}
	var total int64
	for _, inv := range invoices {
		total += inv.OutstandingMinor
		if i, ok := index[AgeingBucketKey(daysPastDue(inv.DueDate, asOf))]; ok {
			buckets[i].Count++
			buckets[i].Amount += inv.OutstandingMinor
		}
	}
	return Receivables{AsOf: asOf, TotalOutstanding: total, Buckets: buckets, Invoices: invoices}, nil
}

// daysPastDue is how many days an invoice is past its due date on asOf. A due
// date in the future (or none at all) is not yet overdue, so it is zero.
func daysPastDue(due *time.Time, asOf time.Time) int {
	if due == nil {
		return 0
	}
	days := int(asOf.Sub(*due).Hours() / 24)
	if days < 0 {
		return 0
	}
	return days
}

// ── recurring costs ─────────────────────────────────────────────────────────

// RecurringCosts lists standing vendor costs by next renewal (§8.4).
func (s *Service) RecurringCosts(ctx context.Context) ([]RecurringCost, error) {
	return s.store.ListRecurringCosts(ctx)
}

// ── project profitability ───────────────────────────────────────────────────

// ProjectProfitability is a project's profit and loss (§8.4). CostTimeMinor is
// always nil: no loaded hourly-cost rate exists to value logged time, so it is
// omitted rather than guessed, and the margin is struck from expenses alone.
type ProjectProfitability struct {
	ProjectID             uuid.UUID
	ProjectName           string
	Currency              string
	RevenueInvoicedMinor  int64
	RevenueCollectedMinor int64
	CostExpensesMinor     int64
}

// ProjectProfitability reports a project's P&L, or ErrNotFound when the project
// does not exist. Revenue is attributed through the contract milestones billed
// against the project; expenses by their project link.
func (s *Service) ProjectProfitability(ctx context.Context, projectID uuid.UUID) (ProjectProfitability, error) {
	name, ok, err := s.store.ProjectName(ctx, projectID)
	if err != nil {
		return ProjectProfitability{}, err
	}
	if !ok {
		return ProjectProfitability{}, fmt.Errorf("%w: project %s", ErrNotFound, projectID)
	}
	invoiced, collected, err := s.store.ProjectRevenue(ctx, projectID)
	if err != nil {
		return ProjectProfitability{}, err
	}
	costs, err := s.store.ProjectCosts(ctx, projectID)
	if err != nil {
		return ProjectProfitability{}, err
	}
	return ProjectProfitability{
		ProjectID: projectID, ProjectName: name, Currency: BaseCurrency,
		RevenueInvoicedMinor: invoiced, RevenueCollectedMinor: collected,
		CostExpensesMinor: costs,
	}, nil
}

// ── expenses ────────────────────────────────────────────────────────────────

// ExpenseCategories lists the configured spend categories (§8.4 [CONFIG]).
func (s *Service) ExpenseCategories(ctx context.Context) ([]ExpenseCategory, error) {
	return s.store.ListExpenseCategories(ctx)
}

// Expenses lists claims, keyset-paged. An unknown status filter is a validation
// error, not an empty page, so a client learns the value is unheard of.
func (s *Service) Expenses(ctx context.Context, q ExpenseQuery, limit int, before *Cursor) ([]Expense, error) {
	if q.Status != nil && !validExpenseStatus(*q.Status) {
		return nil, fmt.Errorf("%w: unknown expense status", ErrInvalid)
	}
	return s.store.ListExpenses(ctx, q, limit, before)
}

// Expense returns one claim, or ErrNotFound.
func (s *Service) Expense(ctx context.Context, id uuid.UUID) (Expense, error) {
	e, err := s.store.GetExpense(ctx, id)
	if err != nil {
		return Expense{}, err
	}
	if e == nil {
		return Expense{}, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	return *e, nil
}

// ── invoices ────────────────────────────────────────────────────────────────

// Invoices lists invoices, keyset-paged.
func (s *Service) Invoices(ctx context.Context, q InvoiceQuery, limit int, before *Cursor) ([]Invoice, error) {
	if q.Status != nil && !validInvoiceStatus(*q.Status) {
		return nil, fmt.Errorf("%w: unknown invoice status", ErrInvalid)
	}
	return s.store.ListInvoices(ctx, q, s.asOf(), limit, before)
}

// Invoice returns one invoice together with its lines. The single-record read
// carries the lines; the list does not (§8.4).
func (s *Service) Invoice(ctx context.Context, id uuid.UUID) (Invoice, []InvoiceLine, error) {
	inv, err := s.store.GetInvoice(ctx, id)
	if err != nil {
		return Invoice{}, nil, err
	}
	if inv == nil {
		return Invoice{}, nil, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	lines, err := s.store.ListInvoiceLines(ctx, id)
	if err != nil {
		return Invoice{}, nil, err
	}
	return *inv, lines, nil
}

// ── payments ────────────────────────────────────────────────────────────────

// Payments lists money received, keyset-paged.
func (s *Service) Payments(ctx context.Context, q PaymentQuery, limit int, before *Cursor) ([]Payment, error) {
	if q.Method != nil && !validPaymentMethod(*q.Method) {
		return nil, fmt.Errorf("%w: unknown payment method", ErrInvalid)
	}
	return s.store.ListPayments(ctx, q, limit, before)
}

// ── budgets ─────────────────────────────────────────────────────────────────

// Budgets lists budgets with their actuals, keyset-paged.
func (s *Service) Budgets(ctx context.Context, q BudgetQuery, limit int, before *Cursor) ([]Budget, error) {
	if q.Scope != nil && !validBudgetScope(*q.Scope) {
		return nil, fmt.Errorf("%w: unknown budget scope", ErrInvalid)
	}
	if q.Status != nil && !validBudgetStatus(*q.Status) {
		return nil, fmt.Errorf("%w: unknown budget status", ErrInvalid)
	}
	return s.store.ListBudgets(ctx, q, limit, before)
}

// Budget returns one budget with its actuals, or ErrNotFound.
func (s *Service) Budget(ctx context.Context, id uuid.UUID) (Budget, error) {
	b, err := s.store.GetBudget(ctx, id)
	if err != nil {
		return Budget{}, err
	}
	if b == nil {
		return Budget{}, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	return *b, nil
}
