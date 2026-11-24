package finance

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/safaristack/atlas/apps/api/internal/audit"
)

// These tests exercise the write lifecycle against a real Postgres: the
// transitions, the money arithmetic that lands in columns, the invoice-status
// refresh driven by allocations, the budget threshold events, and the audit
// footprint. They skip unless ATLAS_TEST_DATABASE_URL is set, so the unit suite
// stays hermetic.

// writeService builds the service over the real store with an audit writer
// attached, so every test can assert on the footprint as well as the record.
func writeService(t *testing.T, f *fixture) *Service {
	t.Helper()
	svc := NewService(NewPGStore(f.pool), nil)
	svc.SetAudit(audit.NewWriter(f.pool))
	return svc
}

func countRows(t *testing.T, pool *pgxpool.Pool, sql string, args ...any) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), sql, args...).Scan(&n); err != nil {
		t.Fatalf("count %q: %v", sql, err)
	}
	return n
}

// today is the day the service would stamp on a fresh write, so a test's
// "spent yesterday" stays in the past whatever time it runs.
func today() time.Time {
	n := time.Now().UTC()
	return time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, time.UTC)
}

func TestExpenseWriteLifecycle(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	svc := writeService(t, f)

	category := f.category(t)
	claimant := f.person(t)
	actor := f.person(t)
	account := f.account(t, 0)

	amount := int64(50000) // below the receipt threshold
	spent := today().AddDate(0, 0, -2)
	created, err := svc.CreateExpense(ctx, actor, ExpenseInput{
		ClaimantPersonID: &claimant, CategoryKey: category, AmountMinor: amount,
		Currency: "kes", SpentOn: spent, Vendor: "  Acme Supplies  ",
	})
	if err != nil {
		t.Fatalf("create expense: %v", err)
	}
	f.expenses = append(f.expenses, created.ID)

	if created.Status != ExpenseDraft {
		t.Errorf("new claim status = %s, want draft", created.Status)
	}
	if created.Ref == "" {
		t.Error("new claim has no reference")
	}
	if created.Vendor != "Acme Supplies" {
		t.Errorf("vendor = %q, want trimmed", created.Vendor)
	}
	if created.Currency != "KES" {
		t.Errorf("currency = %q, want KES", created.Currency)
	}

	// An edit is allowed while the claim is a draft, and it bumps the version.
	edited, err := svc.UpdateExpense(ctx, actor, created.ID, created.Version, ExpenseInput{
		ClaimantPersonID: &claimant, CategoryKey: category, AmountMinor: amount,
		Currency: "KES", SpentOn: spent, Vendor: "Acme Supplies Ltd",
	})
	if err != nil {
		t.Fatalf("edit draft: %v", err)
	}
	if edited.Version != created.Version+1 {
		t.Errorf("version after edit = %d, want %d", edited.Version, created.Version+1)
	}

	// A stale version on a still-editable record is a conflict, not a silent
	// overwrite.
	if _, err := svc.UpdateExpense(ctx, actor, created.ID, created.Version, ExpenseInput{
		ClaimantPersonID: &claimant, CategoryKey: category, AmountMinor: amount,
		Currency: "KES", SpentOn: spent, Vendor: "Stale write",
	}); !errors.Is(err, ErrConflict) {
		t.Errorf("stale edit: err = %v, want ErrConflict", err)
	}

	submitted, err := svc.SubmitExpense(ctx, actor, created.ID, edited.Version)
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	if submitted.Status != ExpenseSubmitted || submitted.SubmittedAt == nil {
		t.Errorf("submit: status = %s, submitted_at = %v", submitted.Status, submitted.SubmittedAt)
	}

	// A rejection must say why; the failed attempt must not move the version.
	if _, err := svc.DecideExpense(ctx, actor, created.ID, submitted.Version, DecisionReject, "   "); problemCode(err) != CodeReasonRequired {
		t.Fatalf("reject without reason: err = %v (code %q), want %s", err, problemCode(err), CodeReasonRequired)
	}
	fresh, err := svc.Expense(ctx, created.ID)
	if err != nil {
		t.Fatalf("re-read after refused reject: %v", err)
	}
	if fresh.Version != submitted.Version {
		t.Errorf("version moved after a refused reject: %d → %d", submitted.Version, fresh.Version)
	}

	approved, err := svc.DecideExpense(ctx, actor, created.ID, fresh.Version, DecisionApprove, "")
	if err != nil {
		t.Fatalf("approve: %v", err)
	}
	if approved.Status != ExpenseApproved {
		t.Errorf("approved status = %s, want approved", approved.Status)
	}
	if approved.DecidedBy == nil || *approved.DecidedBy != actor {
		t.Errorf("decided_by = %v, want the actor %s", approved.DecidedBy, actor)
	}

	// An approved claim is out of the claimant's hands.
	if _, err := svc.UpdateExpense(ctx, actor, created.ID, approved.Version, ExpenseInput{
		ClaimantPersonID: &claimant, CategoryKey: category, AmountMinor: amount,
		Currency: "KES", SpentOn: spent,
	}); problemCode(err) != CodeExpenseNotEditable {
		t.Errorf("edit approved: code = %q, want %s", problemCode(err), CodeExpenseNotEditable)
	}

	paid, err := svc.SettleExpense(ctx, actor, created.ID, approved.Version, ExpenseSettleInput{
		Method: ExpensePaid, AccountID: &account,
	})
	if err != nil {
		t.Fatalf("settle: %v", err)
	}
	if paid.Status != ExpensePaid || paid.PaidOn == nil {
		t.Errorf("settle: status = %s, paid_on = %v", paid.Status, paid.PaidOn)
	}

	reconciled, err := svc.ReconcileExpense(ctx, actor, created.ID, paid.Version)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if reconciled.Status != ExpenseReconciled {
		t.Errorf("reconciled status = %s, want reconciled", reconciled.Status)
	}

	// A settled claim cannot go back for approval.
	if _, err := svc.SubmitExpense(ctx, actor, created.ID, reconciled.Version); problemCode(err) != CodeExpenseNotSubmittable {
		t.Errorf("submit settled: code = %q, want %s", problemCode(err), CodeExpenseNotSubmittable)
	}

	// Every step left a footprint naming the actor.
	actions := countRows(t, f.pool,
		`SELECT count(DISTINCT action) FROM audit.events WHERE entity_id = $1 AND actor_id = $2`,
		created.ID, actor.String())
	if actions < 6 {
		t.Errorf("audit rows for the claim = %d distinct actions, want at least 6", actions)
	}
}

func TestExpenseReceiptRequiredOverThreshold(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	svc := writeService(t, f)
	category := f.category(t)
	actor := f.person(t)

	_, err := svc.CreateExpense(ctx, actor, ExpenseInput{
		CategoryKey: category, AmountMinor: defaultReceiptMinMinor + 1,
		Currency: "KES", SpentOn: today().AddDate(0, 0, -1),
	})
	if !errors.Is(err, ErrInvalid) || problemCode(err) != CodeReceiptRequired {
		t.Fatalf("large claim without receipt: err = %v (code %q), want %s", err, problemCode(err), CodeReceiptRequired)
	}
}

func TestInvoicePaymentWriteLifecycle(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	svc := writeService(t, f)

	org := f.org(t)
	actor := f.person(t)
	account := f.account(t, 0)

	inv, err := svc.CreateInvoice(ctx, actor, InvoiceInput{
		OrganizationID: org, Currency: "KES",
		Lines: []InvoiceLineInput{
			{Description: "Discovery", Quantity: 1, UnitPriceMinor: 100000, TaxRateBP: 1600},
			{Description: "Build", Quantity: 2, UnitPriceMinor: 50000, TaxRateBP: 1600},
		},
	})
	if err != nil {
		t.Fatalf("create invoice: %v", err)
	}
	f.invoices = append(f.invoices, inv.ID)

	// 100000 + 16000 + 100000 + 16000 = 232000, all computed by Go.
	if inv.TotalMinor == nil || *inv.TotalMinor != 232000 {
		t.Fatalf("invoice total = %v, want 232000", inv.TotalMinor)
	}
	if inv.Status != InvoiceDraft {
		t.Errorf("new invoice status = %s, want draft", inv.Status)
	}
	lines := countRows(t, f.pool, `SELECT count(*) FROM finance.invoice_lines WHERE invoice_id = $1`, inv.ID)
	if lines != 2 {
		t.Errorf("invoice lines = %d, want 2", lines)
	}

	approved, err := svc.ApproveInvoice(ctx, actor, inv.ID, inv.Version)
	if err != nil {
		t.Fatalf("approve invoice: %v", err)
	}

	issued, err := svc.IssueInvoice(ctx, actor, inv.ID, approved.Version, InvoiceIssueInput{})
	if err != nil {
		t.Fatalf("issue invoice: %v", err)
	}
	if issued.Status != InvoiceIssued {
		t.Fatalf("issued status = %s, want issued", issued.Status)
	}
	if issued.IssueDate == nil || issued.DueDate == nil {
		t.Fatalf("issue dates missing: issue=%v due=%v", issued.IssueDate, issued.DueDate)
	}
	// No contract and no stated term, so the default 30-day term applies.
	if got := issued.DueDate.Sub(*issued.IssueDate).Hours() / 24; got != defaultPaymentTermDays {
		t.Errorf("due term = %.0f days, want %d", got, defaultPaymentTermDays)
	}

	// Recording the receipt settles the invoice in the same write.
	pay, err := svc.CreatePayment(ctx, actor, PaymentInput{
		AmountMinor: 232000, Currency: "KES", Method: PaymentBank,
		ReceivedOn: today(), AccountID: &account,
		Allocations: []PaymentAllocationInput{{InvoiceID: inv.ID, AmountMinor: 232000}},
	})
	if err != nil {
		t.Fatalf("record payment: %v", err)
	}
	f.payments = append(f.payments, pay.ID)
	if pay.AllocatedMinor != 232000 || pay.UnallocatedMinor != 0 {
		t.Errorf("payment allocations: allocated = %d, unallocated = %d", pay.AllocatedMinor, pay.UnallocatedMinor)
	}

	after, _, err := svc.Invoice(ctx, inv.ID)
	if err != nil {
		t.Fatalf("re-read invoice: %v", err)
	}
	if after.Status != InvoicePaid {
		t.Errorf("invoice status after full payment = %s, want paid", after.Status)
	}
	if after.OutstandingMinor != 0 {
		t.Errorf("outstanding after full payment = %d, want 0", after.OutstandingMinor)
	}

	reconciled, err := svc.ReconcilePayment(ctx, actor, pay.ID, pay.Version, true)
	if err != nil {
		t.Fatalf("reconcile payment: %v", err)
	}
	if !reconciled.Reconciled {
		t.Error("payment not marked reconciled")
	}

	// Over-allocating a second payment onto a settled invoice is refused.
	if _, err := svc.CreatePayment(ctx, actor, PaymentInput{
		AmountMinor: 1000, Currency: "KES", Method: PaymentMpesa,
		ReceivedOn: today(), Allocations: []PaymentAllocationInput{{InvoiceID: inv.ID, AmountMinor: 1000}},
	}); problemCode(err) != CodeInvoiceNotAllocable {
		t.Errorf("allocating to a paid invoice: code = %q, want %s", problemCode(err), CodeInvoiceNotAllocable)
	}

	// The payment's outbox event is what downstream notices hang off.
	events := countRows(t, f.pool,
		`SELECT count(*) FROM outbox.events WHERE aggregate_id = $1 AND type = 'finance.payment.received.v1'`, pay.ID)
	if events != 1 {
		t.Errorf("payment.received events = %d, want 1", events)
	}
}

func TestBudgetThresholdCrossedOnExpenseApproval(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	svc := writeService(t, f)

	category := f.category(t)
	claimant := f.person(t)
	actor := f.person(t)
	start := today().AddDate(0, 0, -1)
	end := today().AddDate(0, 0, 30)
	amount := int64(100000)

	budget, err := svc.CreateBudget(ctx, actor, BudgetInput{
		Scope: ScopeCompanyPeriod, PeriodStart: start, PeriodEnd: end,
		CategoryKey: category, AmountMinor: amount, Currency: "KES",
	})
	if err != nil {
		t.Fatalf("create budget: %v", err)
	}
	f.budgets = append(f.budgets, budget.ID)

	approvedBudget, err := svc.ApproveBudget(ctx, actor, budget.ID, budget.Version)
	if err != nil {
		t.Fatalf("approve budget: %v", err)
	}
	if approvedBudget.Status != BudgetApproved {
		t.Fatalf("budget status = %s, want approved", approvedBudget.Status)
	}
	// Approving an empty budget crosses nothing.
	early := countRows(t, f.pool,
		`SELECT count(*) FROM outbox.events WHERE aggregate_id = $1 AND type = 'finance.budget.threshold_crossed.v1'`, budget.ID)
	if early != 0 {
		t.Errorf("budget crossings before any spend = %d, want 0", early)
	}

	// An approved claim of 80000 is 80% of the ceiling: it crosses 75%.
	exp, err := svc.CreateExpense(ctx, actor, ExpenseInput{
		ClaimantPersonID: &claimant, CategoryKey: category, AmountMinor: 80000,
		Currency: "KES", SpentOn: today(),
	})
	if err != nil {
		t.Fatalf("create expense: %v", err)
	}
	f.expenses = append(f.expenses, exp.ID)

	submitted, err := svc.SubmitExpense(ctx, actor, exp.ID, exp.Version)
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	if _, err := svc.DecideExpense(ctx, actor, exp.ID, submitted.Version, DecisionApprove, ""); err != nil {
		t.Fatalf("approve expense: %v", err)
	}

	crossed := countRows(t, f.pool,
		`SELECT count(*) FROM outbox.events WHERE aggregate_id = $1 AND type = 'finance.budget.threshold_crossed.v1'`, budget.ID)
	if crossed != 1 {
		t.Errorf("threshold_crossed events = %d, want 1 (75%% only)", crossed)
	}
}

func TestAccountAndRecurringCostWrites(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	svc := writeService(t, f)
	actor := f.person(t)

	opening := int64(250000)
	account, err := svc.CreateAccount(ctx, actor, AccountInput{
		Name: "  M-Pesa Float  ", AccountType: AccountMpesaTill, Currency: "kes", OpeningBalanceMinor: &opening,
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	f.accounts = append(f.accounts, account.ID)
	if account.Name != "M-Pesa Float" {
		t.Errorf("account name = %q, want trimmed", account.Name)
	}
	if account.Currency != "KES" {
		t.Errorf("account currency = %q, want KES", account.Currency)
	}
	if account.BalanceMinor != opening {
		t.Errorf("account balance = %d, want opening %d", account.BalanceMinor, opening)
	}

	inactive := false
	edited, err := svc.UpdateAccount(ctx, actor, account.ID, account.Version, AccountInput{
		Name: account.Name, AccountType: AccountMpesaTill, Currency: "KES", Active: &inactive,
	})
	if err != nil {
		t.Fatalf("update account: %v", err)
	}
	if edited.Active {
		t.Error("account still active after being deactivated")
	}

	// A bad account type is refused before any write.
	if _, err := svc.CreateAccount(ctx, actor, AccountInput{
		Name: "Bad", AccountType: AccountType("crypto"), Currency: "KES",
	}); !errors.Is(err, ErrInvalid) {
		t.Errorf("bad account type: err = %v, want ErrInvalid", err)
	}

	renewal := today().AddDate(0, 1, 0)
	cost, err := svc.CreateRecurringCost(ctx, actor, RecurringCostInput{
		Vendor: "  Cloud Hosting  ", AmountMinor: 30000, Currency: "KES",
		Cycle: CycleMonthly, NextRenewalOn: renewal,
	})
	if err != nil {
		t.Fatalf("create recurring cost: %v", err)
	}
	f.standings = append(f.standings, cost.ID)
	if cost.Vendor != "Cloud Hosting" {
		t.Errorf("recurring vendor = %q, want trimmed", cost.Vendor)
	}
	// A monthly cost's monthly equivalent is the amount itself.
	if cost.MonthlyEquivalentMinor == nil || *cost.MonthlyEquivalentMinor != 30000 {
		t.Errorf("monthly equivalent = %v, want 30000", cost.MonthlyEquivalentMinor)
	}

	if _, err := svc.UpdateRecurringCost(ctx, actor, cost.ID, cost.Version, RecurringCostInput{
		Vendor: cost.Vendor, AmountMinor: 35000, Currency: "KES", Cycle: CycleMonthly, NextRenewalOn: renewal,
	}); err != nil {
		t.Fatalf("update recurring cost: %v", err)
	}
}

// TestWriteMethodsRefuseWithoutStore proves the guard rail: a service built over
// a read-only store refuses every write rather than panicking.
func TestWriteMethodsRefuseWithoutStore(t *testing.T) {
	svc := NewService(readOnlyStore{}, nil)
	if _, err := svc.CreateExpense(context.Background(), uuid.New(), ExpenseInput{}); err == nil {
		t.Error("create expense over a read-only store: want an error")
	}
}

// readOnlyStore implements Store but not Writer.
type readOnlyStore struct{}

func (readOnlyStore) OverviewTotals(context.Context, time.Time) (OverviewTotals, error) {
	return OverviewTotals{}, nil
}
func (readOnlyStore) ListAccounts(context.Context) ([]AccountBalance, error) { return nil, nil }
func (readOnlyStore) CashFlow(context.Context, time.Time) (int64, int64, error) {
	return 0, 0, nil
}
func (readOnlyStore) OpenReceivables(context.Context, time.Time) ([]Invoice, error) {
	return nil, nil
}
func (readOnlyStore) ListRecurringCosts(context.Context) ([]RecurringCost, error) { return nil, nil }
func (readOnlyStore) ProjectName(context.Context, uuid.UUID) (string, bool, error) {
	return "", false, nil
}
func (readOnlyStore) ProjectRevenue(context.Context, uuid.UUID) (int64, int64, error) {
	return 0, 0, nil
}
func (readOnlyStore) ProjectCosts(context.Context, uuid.UUID) (int64, error) { return 0, nil }
func (readOnlyStore) ListExpenseCategories(context.Context) ([]ExpenseCategory, error) {
	return nil, nil
}
func (readOnlyStore) ListExpenses(context.Context, ExpenseQuery, int, *Cursor) ([]Expense, error) {
	return nil, nil
}
func (readOnlyStore) GetExpense(context.Context, uuid.UUID) (*Expense, error) { return nil, nil }
func (readOnlyStore) ListInvoices(context.Context, InvoiceQuery, time.Time, int, *Cursor) ([]Invoice, error) {
	return nil, nil
}
func (readOnlyStore) GetInvoice(context.Context, uuid.UUID) (*Invoice, error) { return nil, nil }
func (readOnlyStore) ListInvoiceLines(context.Context, uuid.UUID) ([]InvoiceLine, error) {
	return nil, nil
}
func (readOnlyStore) ListPayments(context.Context, PaymentQuery, int, *Cursor) ([]Payment, error) {
	return nil, nil
}
func (readOnlyStore) ListBudgets(context.Context, BudgetQuery, int, *Cursor) ([]Budget, error) {
	return nil, nil
}
func (readOnlyStore) GetBudget(context.Context, uuid.UUID) (*Budget, error) { return nil, nil }
