package finance

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"time"

	"github.com/google/uuid"

	"github.com/safaristack/atlas/apps/api/internal/audit"
	"github.com/safaristack/atlas/apps/api/internal/outbox"
	"github.com/safaristack/atlas/apps/api/internal/settings"
)

// Service owns the finance rules (§8.4): both the read questions the finance
// screens ask — what cash is on hand, what clients owe and how overdue it is,
// what a project really earns, what is budgeted against actuals, and which
// standing costs are about to renew — and the write lifecycle that moves an
// expense through approval to settlement, an invoice from draft to issued and
// paid, a payment onto the invoices it settles, and a budget through approval
// to close.
//
// Every write is attributed: the service stamps the actor onto the audit row
// (§6.6), which is what makes it safe for both CEOs to carry the module end to
// end — the footprint, not the sequence, is the control.
type Service struct {
	store  Store
	writer Writer
	audit  *audit.Writer
	// settings carries the §8.4 [CONFIG] thresholds. It may be nil (tests, or a
	// deployment that has not wired it), in which case the defaults apply.
	settings        *settings.Service
	now             func() time.Time
	logger          *slog.Logger
	receiptMinMinor int64
}

// defaultReceiptMinMinor is the amount above which a claim needs a receipt
// (§8.4 [CONFIG]) when the setting has not been stored: KES 1,000.
const defaultReceiptMinMinor = 100000

// NewService builds the finance service over a store. A store that also
// implements Writer enables the write lifecycle; SetAudit and SetSettings
// attach attribution and configuration afterwards.
func NewService(store Store, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	s := &Service{
		store:           store,
		now:             func() time.Time { return time.Now().UTC() },
		logger:          logger,
		receiptMinMinor: defaultReceiptMinMinor,
	}
	if w, ok := store.(Writer); ok {
		s.writer = w
	}
	return s
}

// SetAudit attaches the audit writer that records who moved what (§6.6).
func (s *Service) SetAudit(aw *audit.Writer) { s.audit = aw }

// SetSettings attaches the settings service, so the §8.4 [CONFIG] thresholds
// follow an operator's configuration rather than a compile-time constant.
func (s *Service) SetSettings(svc *settings.Service) { s.settings = svc }

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

// ── write lifecycle ─────────────────────────────────────────────────────────
//
// Each method follows the same shape: read the record, check the one rule that
// governs the transition, write it with the caller's version, then leave a
// footprint — an audit row naming the actor, and an outbox event where another
// module reacts (ADR-009).

// writes returns the write store, or an internal error when it was never wired.
func (s *Service) writes() (Writer, error) {
	if s.writer == nil {
		return nil, fmt.Errorf("finance: the write store is not configured")
	}
	return s.writer, nil
}

func newID() (uuid.UUID, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return uuid.Nil, fmt.Errorf("finance: new id: %w", err)
	}
	return id, nil
}

// ── expenses ────────────────────────────────────────────────────────────────

// receiptThreshold is the amount above which a claim must carry a receipt
// (§8.4 [CONFIG]).
func (s *Service) receiptThreshold(ctx context.Context) int64 {
	if s.settings == nil {
		return s.receiptMinMinor
	}
	st, err := s.settings.Get(ctx, "finance.expense.receipt_min_minor")
	if err != nil {
		return s.receiptMinMinor
	}
	switch v := st.Value.(type) {
	case float64:
		return int64(v)
	case int64:
		return v
	}
	return s.receiptMinMinor
}

// validateExpenseInput is the §8.4 claim rule: a category, an amount, a date, a
// claimant — and a receipt above the configured threshold.
func (s *Service) validateExpenseInput(ctx context.Context, e Expense) error {
	if e.ClaimantPersonID == uuid.Nil {
		return errInvalidField("claimant_person_id")
	}
	if trim(e.CategoryKey) == "" {
		return errInvalidField("category_key")
	}
	if e.AmountMinor == nil || *e.AmountMinor <= 0 {
		return errInvalidField("amount_minor")
	}
	if err := validateCurrency(e.Currency); err != nil {
		return err
	}
	if e.SpentOn.IsZero() {
		return errInvalidField("spent_on")
	}
	if e.PaymentMethod != nil && !validPaymentMethod(*e.PaymentMethod) {
		return errInvalidField("payment_method")
	}
	if e.ReceiptFileID == nil && *e.AmountMinor > s.receiptThreshold(ctx) {
		return withCode(fmt.Errorf("%w: a receipt is required above %d %s", ErrInvalid, s.receiptThreshold(ctx), e.Currency), CodeReceiptRequired)
	}
	return nil
}

// CreateExpense records a claim as a draft (§8.4). It starts as draft: the
// claimant submits it deliberately, so a saved correction never reaches an
// approver by accident.
func (s *Service) CreateExpense(ctx context.Context, actor uuid.UUID, in ExpenseInput) (Expense, error) {
	w, err := s.writes()
	if err != nil {
		return Expense{}, err
	}
	id, err := newID()
	if err != nil {
		return Expense{}, err
	}
	claimant := in.ClaimantPersonID
	if claimant == nil {
		claimant = &actor
	}
	method := in.PaymentMethod
	e := Expense{
		ID: id, ClaimantPersonID: *claimant, CategoryKey: in.CategoryKey, ProjectID: in.ProjectID,
		AmountMinor: &in.AmountMinor, Currency: normalizeCurrency(in.Currency), SpentOn: in.SpentOn,
		Vendor: trim(in.Vendor), PaymentMethod: method, AccountID: in.AccountID,
		Status: ExpenseDraft, ReceiptFileID: in.ReceiptFileID, TaxInvoiceRef: nullIfEmpty(in.TaxInvoiceRef),
		Notes: trim(in.Notes), Version: 1, CreatedAt: s.now(), UpdatedAt: s.now(),
	}
	if err := s.validateExpenseInput(ctx, e); err != nil {
		return Expense{}, err
	}
	if !e.SpentOn.IsZero() && e.SpentOn.After(s.dayEnd(e.SpentOn)) {
		return Expense{}, fmt.Errorf("%w: spent_on cannot be in the future", ErrInvalid)
	}
	if err := s.requireCategory(ctx, e.CategoryKey); err != nil {
		return Expense{}, err
	}
	created, err := w.CreateExpense(ctx, e)
	if err != nil {
		return Expense{}, err
	}
	s.writeAudit(ctx, actor, "finance.expense.created", EntityExpense, created.ID, nil, expenseSnapshot(created))
	return created, nil
}

// UpdateExpense rewrites a draft. Only a draft moves: a submitted claim is in
// someone else's hands and a settled one is history (§8.4).
func (s *Service) UpdateExpense(ctx context.Context, actor uuid.UUID, id uuid.UUID, expectedVersion int, in ExpenseInput) (Expense, error) {
	w, err := s.writes()
	if err != nil {
		return Expense{}, err
	}
	current, err := s.Expense(ctx, id)
	if err != nil {
		return Expense{}, err
	}
	if !expenseEditable(current.Status) {
		return Expense{}, withCode(fmt.Errorf("%w: a %s claim cannot be edited", ErrInvalid, current.Status), CodeExpenseNotEditable)
	}
	claimant := in.ClaimantPersonID
	if claimant == nil {
		claimant = &current.ClaimantPersonID
	}
	e := Expense{
		ID: id, Ref: current.Ref, ClaimantPersonID: *claimant, CategoryKey: in.CategoryKey,
		ProjectID: in.ProjectID, AmountMinor: &in.AmountMinor, Currency: normalizeCurrency(in.Currency),
		SpentOn: in.SpentOn, Vendor: trim(in.Vendor), PaymentMethod: in.PaymentMethod, AccountID: in.AccountID,
		Status: current.Status, ReceiptFileID: in.ReceiptFileID, TaxInvoiceRef: nullIfEmpty(in.TaxInvoiceRef),
		Notes: trim(in.Notes), Version: current.Version, CreatedAt: current.CreatedAt, UpdatedAt: s.now(),
	}
	if err := s.validateExpenseInput(ctx, e); err != nil {
		return Expense{}, err
	}
	if err := s.requireCategory(ctx, e.CategoryKey); err != nil {
		return Expense{}, err
	}
	updated, err := w.UpdateExpense(ctx, e, expectedVersion)
	if err != nil {
		return Expense{}, err
	}
	s.writeAudit(ctx, actor, "finance.expense.updated", EntityExpense, updated.ID, expenseSnapshot(current), expenseSnapshot(updated))
	return updated, nil
}

// SubmitExpense sends a draft — or a corrected rejection — to an approver.
func (s *Service) SubmitExpense(ctx context.Context, actor uuid.UUID, id uuid.UUID, expectedVersion int) (Expense, error) {
	w, err := s.writes()
	if err != nil {
		return Expense{}, err
	}
	current, err := s.Expense(ctx, id)
	if err != nil {
		return Expense{}, err
	}
	if !expenseSubmittable(current.Status) {
		return Expense{}, withCode(fmt.Errorf("%w: a %s claim cannot be submitted", ErrInvalid, current.Status), CodeExpenseNotSubmittable)
	}
	if current.AmountMinor == nil {
		return Expense{}, errInvalidField("amount_minor")
	}
	ev, err := outbox.NewExpenseSubmitted(&actor, expenseRef(current, ExpenseSubmitted))
	if err != nil {
		return Expense{}, err
	}
	submitted, err := w.SubmitExpense(ctx, id, expectedVersion, s.now(), ev)
	if err != nil {
		return Expense{}, err
	}
	s.writeAudit(ctx, actor, "finance.expense.submitted", EntityExpense, submitted.ID, expenseSnapshot(current), expenseSnapshot(submitted))
	return submitted, nil
}

// DecideExpense is the single audited approval step (§8.4): whoever holds the
// permission decides — including on their own claim — and the audit row names
// them. A rejection always carries a reason, because the claimant has to be
// able to correct it.
func (s *Service) DecideExpense(ctx context.Context, actor uuid.UUID, id uuid.UUID, expectedVersion int, decision Decision, reason string) (Expense, error) {
	w, err := s.writes()
	if err != nil {
		return Expense{}, err
	}
	if !validDecision(decision) {
		return Expense{}, fmt.Errorf("%w: decision must be approve or reject", ErrInvalid)
	}
	reason = trim(reason)
	if decision == DecisionReject && reason == "" {
		return Expense{}, withCode(fmt.Errorf("%w: a rejected claim must say why", ErrInvalid), CodeReasonRequired)
	}
	current, err := s.Expense(ctx, id)
	if err != nil {
		return Expense{}, err
	}
	if !expenseDecidable(current.Status) {
		return Expense{}, withCode(fmt.Errorf("%w: a %s claim cannot be decided", ErrInvalid, current.Status), CodeExpenseNotDecidable)
	}
	status := ExpenseApproved
	if decision == DecisionReject {
		status = ExpenseRejected
	}
	var ev outbox.Event
	if status == ExpenseApproved {
		if current.AmountMinor == nil {
			return Expense{}, errInvalidField("amount_minor")
		}
		ev, err = outbox.NewExpenseApproved(&actor, expenseRef(current, status))
	} else {
		ev, err = outbox.NewExpenseRejected(&actor, expenseRef(current, status), reason)
	}
	if err != nil {
		return Expense{}, err
	}
	decided, err := w.DecideExpense(ctx, id, expectedVersion, status, actor, s.now(), reason, ev)
	if err != nil {
		return Expense{}, err
	}
	action := "finance.expense.approved"
	if status == ExpenseRejected {
		action = "finance.expense.rejected"
	}
	s.writeAuditReason(ctx, actor, action, EntityExpense, decided.ID, expenseSnapshot(current), expenseSnapshot(decided), reason)
	return decided, nil
}

// SettleExpense moves an approved claim to paid (money left an account) or
// reimbursed (money is owed back to the claimant), stamping the day it moved.
func (s *Service) SettleExpense(ctx context.Context, actor uuid.UUID, id uuid.UUID, expectedVersion int, in ExpenseSettleInput) (Expense, error) {
	w, err := s.writes()
	if err != nil {
		return Expense{}, err
	}
	if in.Method != ExpensePaid && in.Method != ExpenseReimbursed {
		return Expense{}, fmt.Errorf("%w: settle method must be paid or reimbursed", ErrInvalid)
	}
	current, err := s.Expense(ctx, id)
	if err != nil {
		return Expense{}, err
	}
	if !expenseSettleable(current.Status) {
		return Expense{}, withCode(fmt.Errorf("%w: a %s claim has not been approved for settlement", ErrInvalid, current.Status), CodeExpenseNotSettleable)
	}
	paidOn := in.PaidOn
	if paidOn == nil {
		today := s.asOf()
		paidOn = &today
	}
	settled, err := w.SettleExpense(ctx, id, expectedVersion, in.Method, in.AccountID, paidOn)
	if err != nil {
		return Expense{}, err
	}
	s.writeAudit(ctx, actor, "finance.expense."+string(in.Method), EntityExpense, settled.ID,
		expenseSnapshot(current), expenseSnapshot(settled))
	return settled, nil
}

// ReconcileExpense confirms a settled claim against the bank/M-Pesa line.
func (s *Service) ReconcileExpense(ctx context.Context, actor uuid.UUID, id uuid.UUID, expectedVersion int) (Expense, error) {
	w, err := s.writes()
	if err != nil {
		return Expense{}, err
	}
	current, err := s.Expense(ctx, id)
	if err != nil {
		return Expense{}, err
	}
	if !expenseReconcilable(current.Status) {
		return Expense{}, withCode(fmt.Errorf("%w: a %s claim has nothing to reconcile", ErrInvalid, current.Status), CodeExpenseNotReconcilable)
	}
	reconciled, err := w.ReconcileExpense(ctx, id, expectedVersion)
	if err != nil {
		return Expense{}, err
	}
	s.writeAudit(ctx, actor, "finance.expense.reconciled", EntityExpense, reconciled.ID,
		expenseSnapshot(current), expenseSnapshot(reconciled))
	return reconciled, nil
}

func (s *Service) requireCategory(ctx context.Context, key string) error {
	w, err := s.writes()
	if err != nil {
		return err
	}
	ok, err := w.ExpenseCategoryActive(ctx, key)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("%w: unknown or inactive expense category %q", ErrInvalid, key)
	}
	return nil
}

// ── invoices ────────────────────────────────────────────────────────────────

// validateInvoiceInput checks the invoice's shape and prices its lines. Total
// is computed here and never read from the request.
func (s *Service) validateInvoiceInput(in InvoiceInput) (Invoice, []InvoiceLine, error) {
	var inv Invoice
	if in.OrganizationID == uuid.Nil {
		return inv, nil, errInvalidField("organization_id")
	}
	if err := validateCurrency(in.Currency); err != nil {
		return inv, nil, err
	}
	if in.IssueDate != nil && in.DueDate != nil && in.DueDate.Before(*in.IssueDate) {
		return inv, nil, errInvalidField("due_date")
	}
	if len(in.Lines) == 0 {
		return inv, nil, errInvalidField("lines")
	}
	lines := make([]InvoiceLine, 0, len(in.Lines))
	var total int64
	for i, l := range in.Lines {
		if trim(l.Description) == "" {
			return inv, nil, errInvalidField(fmt.Sprintf("lines[%d].description", i))
		}
		if l.Quantity <= 0 || math.IsNaN(l.Quantity) || math.IsInf(l.Quantity, 0) || l.Quantity > 1000000 {
			return inv, nil, errInvalidField(fmt.Sprintf("lines[%d].quantity", i))
		}
		if l.UnitPriceMinor < 0 {
			return inv, nil, errInvalidField(fmt.Sprintf("lines[%d].unit_price_minor", i))
		}
		if l.TaxRateBP < 0 || l.TaxRateBP > 10000 {
			return inv, nil, errInvalidField(fmt.Sprintf("lines[%d].tax_rate_bp", i))
		}
		id, err := newID()
		if err != nil {
			return inv, nil, err
		}
		amount := lineAmountMinor(l.Quantity, l.UnitPriceMinor)
		lines = append(lines, InvoiceLine{
			ID: id, Description: trim(l.Description),
			Quantity: l.Quantity, UnitPriceMinor: &l.UnitPriceMinor, TaxRateBP: l.TaxRateBP,
			AmountMinor: &amount, SortOrder: l.SortOrder,
		})
		total += amount + lineTaxMinor(amount, l.TaxRateBP)
	}
	inv = Invoice{
		OrganizationID: in.OrganizationID, ContractID: in.ContractID, MilestoneID: in.MilestoneID,
		IssueDate: in.IssueDate, DueDate: in.DueDate, TotalMinor: &total,
		Currency: normalizeCurrency(in.Currency), Status: InvoiceDraft, Notes: trim(in.Notes),
	}
	return inv, lines, nil
}

// CreateInvoice drafts an invoice against an organisation (§8.4).
func (s *Service) CreateInvoice(ctx context.Context, actor uuid.UUID, in InvoiceInput) (Invoice, error) {
	w, err := s.writes()
	if err != nil {
		return Invoice{}, err
	}
	inv, lines, err := s.validateInvoiceInput(in)
	if err != nil {
		return Invoice{}, err
	}
	id, err := newID()
	if err != nil {
		return Invoice{}, err
	}
	inv.ID, inv.Version, inv.CreatedAt, inv.UpdatedAt = id, 1, s.now(), s.now()
	created, err := w.CreateInvoice(ctx, inv, lines)
	if err != nil {
		return Invoice{}, err
	}
	s.writeAudit(ctx, actor, "finance.invoice.created", EntityInvoice, created.ID, nil, invoiceSnapshot(created))
	return created, nil
}

// UpdateInvoice rewrites a draft invoice and its lines.
func (s *Service) UpdateInvoice(ctx context.Context, actor uuid.UUID, id uuid.UUID, expectedVersion int, in InvoiceInput) (Invoice, error) {
	w, err := s.writes()
	if err != nil {
		return Invoice{}, err
	}
	current, _, err := s.Invoice(ctx, id)
	if err != nil {
		return Invoice{}, err
	}
	if !invoiceEditable(current.Status) {
		return Invoice{}, withCode(fmt.Errorf("%w: a %s invoice cannot be edited", ErrInvalid, current.Status), CodeInvoiceNotEditable)
	}
	next, lines, err := s.validateInvoiceInput(in)
	if err != nil {
		return Invoice{}, err
	}
	next.ID, next.Ref, next.Status = id, current.Ref, current.Status
	next.Version, next.CreatedAt, next.UpdatedAt = current.Version, current.CreatedAt, s.now()
	updated, err := w.UpdateInvoice(ctx, next, lines, expectedVersion)
	if err != nil {
		return Invoice{}, err
	}
	s.writeAudit(ctx, actor, "finance.invoice.updated", EntityInvoice, updated.ID, invoiceSnapshot(current), invoiceSnapshot(updated))
	return updated, nil
}

// ApproveInvoice signs an invoice off internally, before it is issued.
func (s *Service) ApproveInvoice(ctx context.Context, actor uuid.UUID, id uuid.UUID, expectedVersion int) (Invoice, error) {
	w, err := s.writes()
	if err != nil {
		return Invoice{}, err
	}
	current, _, err := s.Invoice(ctx, id)
	if err != nil {
		return Invoice{}, err
	}
	if !invoiceApprovable(current.Status) {
		return Invoice{}, withCode(fmt.Errorf("%w: a %s invoice cannot be approved", ErrInvalid, current.Status), CodeInvoiceNotIssuable)
	}
	approved, err := w.ApproveInvoice(ctx, id, expectedVersion)
	if err != nil {
		return Invoice{}, err
	}
	s.writeAudit(ctx, actor, "finance.invoice.approved", EntityInvoice, approved.ID, invoiceSnapshot(current), invoiceSnapshot(approved))
	return approved, nil
}

// IssueInvoice sends an approved invoice: it gets its dates (from the request,
// from the contract's stated term, or the default term) and its outbox event,
// which is what the client-facing notices hang off.
func (s *Service) IssueInvoice(ctx context.Context, actor uuid.UUID, id uuid.UUID, expectedVersion int, in InvoiceIssueInput) (Invoice, error) {
	w, err := s.writes()
	if err != nil {
		return Invoice{}, err
	}
	current, _, err := s.Invoice(ctx, id)
	if err != nil {
		return Invoice{}, err
	}
	if !invoiceIssuable(current.Status) {
		return Invoice{}, withCode(fmt.Errorf("%w: a %s invoice must be approved before it is issued", ErrInvalid, current.Status), CodeInvoiceNotIssuable)
	}
	issuedAt := s.now()
	issueDate := in.IssueDate
	if issueDate == nil {
		if current.IssueDate != nil {
			issueDate = current.IssueDate
		} else {
			today := s.asOf()
			issueDate = &today
		}
	}
	dueDate := in.DueDate
	if dueDate == nil {
		if current.DueDate != nil {
			dueDate = current.DueDate
		} else {
			term := defaultPaymentTermDays
			if current.ContractID != nil {
				if days, ok, err := w.ContractPaymentTermDays(ctx, *current.ContractID); err != nil {
					return Invoice{}, err
				} else if ok {
					term = days
				}
			}
			d := issueDate.AddDate(0, 0, term)
			dueDate = &d
		}
	}
	if dueDate.Before(*issueDate) {
		return Invoice{}, errInvalidField("due_date")
	}
	ev, err := outbox.NewInvoiceIssued(&actor, outbox.InvoiceRef{
		ID: current.ID, Ref: current.Ref, OrganizationID: current.OrganizationID, ContractID: current.ContractID,
		TotalMinor: current.TotalMinor, Currency: current.Currency, Status: string(InvoiceIssued),
		DueDate: day(*dueDate),
	})
	if err != nil {
		return Invoice{}, err
	}
	issued, err := w.IssueInvoice(ctx, id, expectedVersion, *issueDate, *dueDate, issuedAt, ev)
	if err != nil {
		return Invoice{}, err
	}
	s.writeAudit(ctx, actor, "finance.invoice.issued", EntityInvoice, issued.ID, invoiceSnapshot(current), invoiceSnapshot(issued))
	return issued, nil
}

// DisputeInvoice flags a sent invoice as contested.
func (s *Service) DisputeInvoice(ctx context.Context, actor uuid.UUID, id uuid.UUID, expectedVersion int, reason string) (Invoice, error) {
	w, err := s.writes()
	if err != nil {
		return Invoice{}, err
	}
	current, _, err := s.Invoice(ctx, id)
	if err != nil {
		return Invoice{}, err
	}
	if !invoiceDisputable(current.Status) {
		return Invoice{}, withCode(fmt.Errorf("%w: a %s invoice cannot be disputed", ErrInvalid, current.Status), CodeInvoiceNotEditable)
	}
	disputed, err := w.DisputeInvoice(ctx, id, expectedVersion)
	if err != nil {
		return Invoice{}, err
	}
	s.writeAuditReason(ctx, actor, "finance.invoice.disputed", EntityInvoice, disputed.ID,
		invoiceSnapshot(current), invoiceSnapshot(disputed), trim(reason))
	return disputed, nil
}

// WriteOffInvoice closes an issued invoice as uncollectable (§8.4). It rides on
// finance.invoice.issue: writing off ends the same lifecycle, so there is no
// second permission for it.
func (s *Service) WriteOffInvoice(ctx context.Context, actor uuid.UUID, id uuid.UUID, expectedVersion int, reason string) (Invoice, error) {
	w, err := s.writes()
	if err != nil {
		return Invoice{}, err
	}
	reason = trim(reason)
	if reason == "" {
		return Invoice{}, withCode(fmt.Errorf("%w: a write-off must say why", ErrInvalid), CodeReasonRequired)
	}
	current, _, err := s.Invoice(ctx, id)
	if err != nil {
		return Invoice{}, err
	}
	if !invoiceWritableOff(current.Status) {
		return Invoice{}, withCode(fmt.Errorf("%w: a %s invoice cannot be written off", ErrInvalid, current.Status), CodeInvoiceNotEditable)
	}
	writtenOff, err := w.WriteOffInvoice(ctx, id, expectedVersion)
	if err != nil {
		return Invoice{}, err
	}
	s.writeAuditReason(ctx, actor, "finance.invoice.written_off", EntityInvoice, writtenOff.ID,
		invoiceSnapshot(current), invoiceSnapshot(writtenOff), reason)
	return writtenOff, nil
}

// ── payments ────────────────────────────────────────────────────────────────

// Payment returns one payment, or ErrNotFound. It reads through the write store
// because it exists to carry the version an update must match.
func (s *Service) Payment(ctx context.Context, id uuid.UUID) (Payment, error) {
	w, err := s.writes()
	if err != nil {
		return Payment{}, err
	}
	p, err := w.GetPayment(ctx, id)
	if err != nil {
		return Payment{}, err
	}
	if p == nil {
		return Payment{}, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	return *p, nil
}

// CreatePayment records money received (§8.4), optionally allocating it to
// invoices in the same write so a receipt that settles an invoice is one
// transaction, never two half-happened ones.
func (s *Service) CreatePayment(ctx context.Context, actor uuid.UUID, in PaymentInput) (Payment, error) {
	w, err := s.writes()
	if err != nil {
		return Payment{}, err
	}
	if in.AmountMinor <= 0 {
		return Payment{}, errInvalidField("amount_minor")
	}
	if !validPaymentMethod(in.Method) {
		return Payment{}, errInvalidField("method")
	}
	if err := validateCurrency(in.Currency); err != nil {
		return Payment{}, err
	}
	if in.ReceivedOn.IsZero() {
		return Payment{}, errInvalidField("received_on")
	}
	if in.ReceivedOn.After(s.dayEnd(in.ReceivedOn)) {
		return Payment{}, errInvalidField("received_on")
	}
	if err := validateAllocations(in.Allocations); err != nil {
		return Payment{}, err
	}
	id, err := newID()
	if err != nil {
		return Payment{}, err
	}
	amount := in.AmountMinor
	p := Payment{
		ID: id, AmountMinor: &amount, Currency: normalizeCurrency(in.Currency), Method: in.Method,
		Reference: trim(in.Reference), ReceivedOn: in.ReceivedOn, AccountID: in.AccountID,
		Reconciled: false, Notes: trim(in.Notes), Version: 1, CreatedAt: s.now(), UpdatedAt: s.now(),
	}
	created, err := w.CreatePayment(ctx, p, in.Allocations, func(ref string) (outbox.Event, error) {
		return outbox.NewPaymentReceived(&actor, outbox.PaymentRef{
			ID: p.ID, Ref: ref, AmountMinor: p.AmountMinor, Currency: p.Currency,
			Method: string(p.Method), AccountID: p.AccountID,
		})
	})
	if err != nil {
		return Payment{}, err
	}
	s.writeAudit(ctx, actor, "finance.payment.recorded", EntityPayment, created.ID,
		nil, paymentSnapshotWithAllocations(created, in.Allocations))
	return created, nil
}

// AllocatePayment matches more of a payment's unallocated balance to invoices.
func (s *Service) AllocatePayment(ctx context.Context, actor uuid.UUID, id uuid.UUID, expectedVersion int, allocations []PaymentAllocationInput) (Payment, error) {
	w, err := s.writes()
	if err != nil {
		return Payment{}, err
	}
	if len(allocations) == 0 {
		return Payment{}, errInvalidField("allocations")
	}
	if err := validateAllocations(allocations); err != nil {
		return Payment{}, err
	}
	current, err := s.Payment(ctx, id)
	if err != nil {
		return Payment{}, err
	}
	updated, err := w.AllocatePayment(ctx, id, expectedVersion, allocations)
	if err != nil {
		return Payment{}, err
	}
	s.writeAudit(ctx, actor, "finance.payment.allocated", EntityPayment, updated.ID,
		paymentSnapshot(current), paymentSnapshotWithAllocations(updated, allocations))
	return updated, nil
}

// ReconcilePayment flags a payment as matched against the bank statement.
func (s *Service) ReconcilePayment(ctx context.Context, actor uuid.UUID, id uuid.UUID, expectedVersion int, reconciled bool) (Payment, error) {
	w, err := s.writes()
	if err != nil {
		return Payment{}, err
	}
	current, err := s.Payment(ctx, id)
	if err != nil {
		return Payment{}, err
	}
	updated, err := w.ReconcilePayment(ctx, id, expectedVersion, reconciled)
	if err != nil {
		return Payment{}, err
	}
	action := "finance.payment.reconciled"
	if !reconciled {
		action = "finance.payment.unreconciled"
	}
	s.writeAudit(ctx, actor, action, EntityPayment, updated.ID, paymentSnapshot(current), paymentSnapshot(updated))
	return updated, nil
}

func validateAllocations(allocations []PaymentAllocationInput) error {
	for _, a := range allocations {
		if a.InvoiceID == uuid.Nil {
			return errInvalidField("allocations.invoice_id")
		}
		if a.AmountMinor <= 0 {
			return errInvalidField("allocations.amount_minor")
		}
	}
	return nil
}

// ── budgets ─────────────────────────────────────────────────────────────────

// CreateBudget opens a ceiling as a draft (§8.4).
func (s *Service) CreateBudget(ctx context.Context, actor uuid.UUID, in BudgetInput) (Budget, error) {
	w, err := s.writes()
	if err != nil {
		return Budget{}, err
	}
	b, err := budgetInput(in)
	if err != nil {
		return Budget{}, err
	}
	id, err := newID()
	if err != nil {
		return Budget{}, err
	}
	b.ID, b.Version, b.CreatedAt, b.UpdatedAt = id, 1, s.now(), s.now()
	created, err := w.CreateBudget(ctx, b)
	if err != nil {
		return Budget{}, err
	}
	s.writeAudit(ctx, actor, "finance.budget.created", EntityBudget, created.ID, nil, budgetSnapshot(created))
	return created, nil
}

// budgetInput validates and shapes a budget from its input.
func budgetInput(in BudgetInput) (Budget, error) {
	var b Budget
	if !validBudgetScope(in.Scope) {
		return b, errInvalidField("scope")
	}
	if (in.Scope == ScopeProject || in.Scope == ScopeDepartment || in.Scope == ScopeUnit) && trim(in.ScopeRef) == "" {
		return b, errInvalidField("scope_ref")
	}
	if in.PeriodStart.IsZero() || in.PeriodEnd.IsZero() {
		return b, errInvalidField("period_start")
	}
	if in.PeriodEnd.Before(in.PeriodStart) {
		return b, errInvalidField("period_end")
	}
	if in.AmountMinor <= 0 {
		return b, errInvalidField("amount_minor")
	}
	if err := validateCurrency(in.Currency); err != nil {
		return b, err
	}
	return Budget{
		Scope: in.Scope, ScopeRef: trim(in.ScopeRef), PeriodStart: in.PeriodStart, PeriodEnd: in.PeriodEnd,
		CategoryKey: trim(in.CategoryKey), AmountMinor: &in.AmountMinor,
		Currency: normalizeCurrency(in.Currency), Status: BudgetDraft, OwnerPersonID: in.OwnerPersonID,
	}, nil
}

// UpdateBudget rewrites a draft. An approved budget is already a commitment.
func (s *Service) UpdateBudget(ctx context.Context, actor uuid.UUID, id uuid.UUID, expectedVersion int, in BudgetInput) (Budget, error) {
	w, err := s.writes()
	if err != nil {
		return Budget{}, err
	}
	current, err := s.Budget(ctx, id)
	if err != nil {
		return Budget{}, err
	}
	if !budgetEditable(current.Status) {
		return Budget{}, withCode(fmt.Errorf("%w: an %s budget cannot be edited", ErrInvalid, current.Status), CodeBudgetNotEditable)
	}
	next, err := budgetInput(in)
	if err != nil {
		return Budget{}, err
	}
	next.ID, next.Status = id, current.Status
	next.Version, next.CreatedAt, next.UpdatedAt = current.Version, current.CreatedAt, s.now()
	updated, err := w.UpdateBudget(ctx, next, expectedVersion)
	if err != nil {
		return Budget{}, err
	}
	s.writeAudit(ctx, actor, "finance.budget.updated", EntityBudget, updated.ID, budgetSnapshot(current), budgetSnapshot(updated))
	return updated, nil
}

// ApproveBudget activates a ceiling and publishes it as allocated.
func (s *Service) ApproveBudget(ctx context.Context, actor uuid.UUID, id uuid.UUID, expectedVersion int) (Budget, error) {
	w, err := s.writes()
	if err != nil {
		return Budget{}, err
	}
	current, err := s.Budget(ctx, id)
	if err != nil {
		return Budget{}, err
	}
	if !budgetApprovable(current.Status) {
		return Budget{}, withCode(fmt.Errorf("%w: an %s budget cannot be approved", ErrInvalid, current.Status), CodeBudgetNotApprovable)
	}
	ev, err := outbox.NewBudgetAllocated(&actor, budgetRef(current, BudgetApproved))
	if err != nil {
		return Budget{}, err
	}
	approved, err := w.ApproveBudget(ctx, id, expectedVersion, ev)
	if err != nil {
		return Budget{}, err
	}
	s.writeAudit(ctx, actor, "finance.budget.approved", EntityBudget, approved.ID, budgetSnapshot(current), budgetSnapshot(approved))
	return approved, nil
}

// CloseBudget retires an approved budget at the end of its period.
func (s *Service) CloseBudget(ctx context.Context, actor uuid.UUID, id uuid.UUID, expectedVersion int) (Budget, error) {
	w, err := s.writes()
	if err != nil {
		return Budget{}, err
	}
	current, err := s.Budget(ctx, id)
	if err != nil {
		return Budget{}, err
	}
	if !budgetClosable(current.Status) {
		return Budget{}, withCode(fmt.Errorf("%w: an %s budget cannot be closed", ErrInvalid, current.Status), CodeBudgetNotClosable)
	}
	closed, err := w.CloseBudget(ctx, id, expectedVersion)
	if err != nil {
		return Budget{}, err
	}
	s.writeAudit(ctx, actor, "finance.budget.closed", EntityBudget, closed.ID, budgetSnapshot(current), budgetSnapshot(closed))
	return closed, nil
}

// ── accounts ────────────────────────────────────────────────────────────────

// Account returns one account with its derived balance, or ErrNotFound.
func (s *Service) Account(ctx context.Context, id uuid.UUID) (AccountBalance, error) {
	w, err := s.writes()
	if err != nil {
		return AccountBalance{}, err
	}
	a, err := w.GetAccount(ctx, id)
	if err != nil {
		return AccountBalance{}, err
	}
	if a == nil {
		return AccountBalance{}, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	return *a, nil
}

// CreateAccount opens a cash account (§8.4).
func (s *Service) CreateAccount(ctx context.Context, actor uuid.UUID, in AccountInput) (AccountBalance, error) {
	w, err := s.writes()
	if err != nil {
		return AccountBalance{}, err
	}
	a, err := accountInput(in)
	if err != nil {
		return AccountBalance{}, err
	}
	id, err := newID()
	if err != nil {
		return AccountBalance{}, err
	}
	a.ID, a.Version, a.CreatedAt, a.UpdatedAt = id, 1, s.now(), s.now()
	created, err := w.CreateAccount(ctx, a)
	if err != nil {
		return AccountBalance{}, err
	}
	s.writeAudit(ctx, actor, "finance.account.created", EntityAccount, created.ID, nil, accountSnapshot(created.Account))
	return created, nil
}

// UpdateAccount rewrites a cash account's details. Balances stay derived: they
// are recomputed from payments and settled expenses, never stored on the row.
func (s *Service) UpdateAccount(ctx context.Context, actor uuid.UUID, id uuid.UUID, expectedVersion int, in AccountInput) (AccountBalance, error) {
	w, err := s.writes()
	if err != nil {
		return AccountBalance{}, err
	}
	current, err := s.Account(ctx, id)
	if err != nil {
		return AccountBalance{}, err
	}
	a, err := accountInput(in)
	if err != nil {
		return AccountBalance{}, err
	}
	a.ID, a.Version, a.CreatedAt, a.UpdatedAt = id, current.Version, current.CreatedAt, s.now()
	updated, err := w.UpdateAccount(ctx, a, expectedVersion)
	if err != nil {
		return AccountBalance{}, err
	}
	s.writeAudit(ctx, actor, "finance.account.updated", EntityAccount, updated.ID,
		accountSnapshot(current.Account), accountSnapshot(updated.Account))
	return updated, nil
}

func accountInput(in AccountInput) (Account, error) {
	var a Account
	if trim(in.Name) == "" {
		return a, errInvalidField("name")
	}
	if !validAccountType(in.AccountType) {
		return a, errInvalidField("account_type")
	}
	if err := validateCurrency(in.Currency); err != nil {
		return a, err
	}
	active := true
	if in.Active != nil {
		active = *in.Active
	}
	return Account{
		Name: trim(in.Name), AccountType: in.AccountType, Currency: normalizeCurrency(in.Currency),
		OpeningBalanceMinor: in.OpeningBalanceMinor, Active: active,
	}, nil
}

// ── recurring costs ─────────────────────────────────────────────────────────

// RecurringCost returns one standing cost, or ErrNotFound.
func (s *Service) RecurringCost(ctx context.Context, id uuid.UUID) (RecurringCost, error) {
	w, err := s.writes()
	if err != nil {
		return RecurringCost{}, err
	}
	c, err := w.GetRecurringCost(ctx, id)
	if err != nil {
		return RecurringCost{}, err
	}
	if c == nil {
		return RecurringCost{}, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	return *c, nil
}

// CreateRecurringCost records a standing vendor charge (§8.4).
func (s *Service) CreateRecurringCost(ctx context.Context, actor uuid.UUID, in RecurringCostInput) (RecurringCost, error) {
	w, err := s.writes()
	if err != nil {
		return RecurringCost{}, err
	}
	c, err := recurringCostInput(in)
	if err != nil {
		return RecurringCost{}, err
	}
	id, err := newID()
	if err != nil {
		return RecurringCost{}, err
	}
	c.ID, c.Version, c.CreatedAt, c.UpdatedAt = id, 1, s.now(), s.now()
	created, err := w.CreateRecurringCost(ctx, c)
	if err != nil {
		return RecurringCost{}, err
	}
	s.writeAudit(ctx, actor, "finance.recurring_cost.created", EntityRecurringCost, created.ID, nil, recurringCostSnapshot(created))
	return created, nil
}

// UpdateRecurringCost rewrites a standing cost.
func (s *Service) UpdateRecurringCost(ctx context.Context, actor uuid.UUID, id uuid.UUID, expectedVersion int, in RecurringCostInput) (RecurringCost, error) {
	w, err := s.writes()
	if err != nil {
		return RecurringCost{}, err
	}
	current, err := s.RecurringCost(ctx, id)
	if err != nil {
		return RecurringCost{}, err
	}
	next, err := recurringCostInput(in)
	if err != nil {
		return RecurringCost{}, err
	}
	next.ID, next.Version, next.CreatedAt, next.UpdatedAt = id, current.Version, current.CreatedAt, s.now()
	updated, err := w.UpdateRecurringCost(ctx, next, expectedVersion)
	if err != nil {
		return RecurringCost{}, err
	}
	s.writeAudit(ctx, actor, "finance.recurring_cost.updated", EntityRecurringCost, updated.ID,
		recurringCostSnapshot(current), recurringCostSnapshot(updated))
	return updated, nil
}

func recurringCostInput(in RecurringCostInput) (RecurringCost, error) {
	var c RecurringCost
	if trim(in.Vendor) == "" {
		return c, errInvalidField("vendor")
	}
	if in.AmountMinor <= 0 {
		return c, errInvalidField("amount_minor")
	}
	if !validRecurringCycle(in.Cycle) {
		return c, errInvalidField("cycle")
	}
	if in.NextRenewalOn.IsZero() {
		return c, errInvalidField("next_renewal_on")
	}
	if err := validateCurrency(in.Currency); err != nil {
		return c, err
	}
	active := true
	if in.Active != nil {
		active = *in.Active
	}
	return RecurringCost{
		Vendor: trim(in.Vendor), Description: trim(in.Description), AmountMinor: &in.AmountMinor,
		Currency: normalizeCurrency(in.Currency), Cycle: in.Cycle, NextRenewalOn: in.NextRenewalOn,
		OwnerPersonID: in.OwnerPersonID, Active: active,
	}, nil
}

// ── write helpers ───────────────────────────────────────────────────────────

// dayEnd is the end of a day, so a date-only claim cannot be "in the future"
// merely because it is read at 23:00.
func (s *Service) dayEnd(d time.Time) time.Time {
	return time.Date(d.Year(), d.Month(), d.Day(), 23, 59, 59, 0, time.UTC)
}

func expenseRef(e Expense, status ExpenseStatus) outbox.ExpenseRef {
	return outbox.ExpenseRef{
		ID: e.ID, Ref: e.Ref, ClaimantPersonID: e.ClaimantPersonID, CategoryKey: e.CategoryKey,
		ProjectID: e.ProjectID, AmountMinor: e.AmountMinor, Currency: e.Currency, Status: string(status),
	}
}

func budgetRef(b Budget, status BudgetStatus) outbox.BudgetRef {
	return outbox.BudgetRef{
		ID: b.ID, Scope: string(b.Scope), ScopeRef: b.ScopeRef, PeriodStart: day(b.PeriodStart),
		PeriodEnd: day(b.PeriodEnd), CategoryKey: b.CategoryKey, AmountMinor: b.AmountMinor,
		Currency: b.Currency, Status: string(status),
	}
}

func (s *Service) writeAudit(ctx context.Context, actor uuid.UUID, action, entityType string, entityID uuid.UUID, before, after any) {
	s.writeAuditReason(ctx, actor, action, entityType, entityID, before, after, "")
}

func (s *Service) writeAuditReason(ctx context.Context, actor uuid.UUID, action, entityType string,
	entityID uuid.UUID, before, after any, reason string) {
	if s.audit == nil {
		return
	}
	id := entityID
	if err := s.audit.Write(ctx, nil, audit.Event{
		ActorType: audit.ActorUser, ActorID: actor.String(), Action: action,
		EntityType: entityType, EntityID: &id, Before: before, After: after, Reason: reason,
	}); err != nil {
		s.logger.ErrorContext(ctx, "finance: audit write failed",
			slog.String("action", action), slog.String("error", err.Error()))
	}
}

// ── snapshots (audit payloads) ──────────────────────────────────────────────

// A snapshot carries the fields a reviewer needs to see what moved: the state
// and the money, never the whole row (§6.3).
func expenseSnapshot(e Expense) map[string]any {
	return map[string]any{
		"ref": e.Ref, "status": string(e.Status), "category_key": e.CategoryKey,
		"claimant_person_id": e.ClaimantPersonID.String(), "amount_minor": e.AmountMinor,
		"currency": e.Currency, "project_id": idOrEmpty(e.ProjectID), "paid_on": dayPtr(e.PaidOn),
		"decision_reason": e.DecisionReason, "decided_by": idOrEmpty(e.DecidedBy),
	}
}

func invoiceSnapshot(i Invoice) map[string]any {
	return map[string]any{
		"ref": i.Ref, "status": string(i.Status), "organization_id": i.OrganizationID.String(),
		"contract_id": idOrEmpty(i.ContractID), "total_minor": i.TotalMinor, "currency": i.Currency,
		"issue_date": dayPtr(i.IssueDate), "due_date": dayPtr(i.DueDate),
	}
}

func paymentSnapshot(p Payment) map[string]any {
	return map[string]any{
		"ref": p.Ref, "amount_minor": p.AmountMinor, "currency": p.Currency,
		"method": string(p.Method), "reconciled": p.Reconciled, "allocated_minor": p.AllocatedMinor,
	}
}

func paymentSnapshotWithAllocations(p Payment, allocations []PaymentAllocationInput) map[string]any {
	ids := make([]string, 0, len(allocations))
	var total int64
	for _, a := range allocations {
		ids = append(ids, a.InvoiceID.String())
		total += a.AmountMinor
	}
	return map[string]any{
		"ref": p.Ref, "amount_minor": p.AmountMinor, "currency": p.Currency,
		"method": string(p.Method), "reconciled": p.Reconciled, "allocated_minor": p.AllocatedMinor,
		"allocated_now_minor": total, "invoice_ids": ids,
	}
}

func budgetSnapshot(b Budget) map[string]any {
	return map[string]any{
		"scope": string(b.Scope), "scope_ref": b.ScopeRef, "status": string(b.Status),
		"period_start": day(b.PeriodStart), "period_end": day(b.PeriodEnd),
		"category_key": b.CategoryKey, "amount_minor": b.AmountMinor, "currency": b.Currency,
	}
}

func accountSnapshot(a Account) map[string]any {
	return map[string]any{
		"name": a.Name, "account_type": string(a.AccountType), "currency": a.Currency,
		"opening_balance_minor": a.OpeningBalanceMinor, "active": a.Active,
	}
}

func recurringCostSnapshot(c RecurringCost) map[string]any {
	return map[string]any{
		"vendor": c.Vendor, "amount_minor": c.AmountMinor, "currency": c.Currency,
		"cycle": string(c.Cycle), "next_renewal_on": day(c.NextRenewalOn), "active": c.Active,
	}
}

func idOrEmpty(id *uuid.UUID) string {
	if id == nil {
		return ""
	}
	return id.String()
}
