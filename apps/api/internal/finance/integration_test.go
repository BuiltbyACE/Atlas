package finance

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// The finance read model is almost entirely SQL: the account balance rule, the
// ageing banding, the budget actuals and the milestone-to-project revenue join
// all live in queries that a fake store cannot exercise. These tests run
// against a real Postgres only when ATLAS_TEST_DATABASE_URL is set, and skip
// otherwise so the unit suite stays hermetic.

func financeTestURL(t *testing.T) string {
	t.Helper()
	url := os.Getenv("ATLAS_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("ATLAS_TEST_DATABASE_URL is not set; skipping finance integration tests")
	}
	return url
}

func financeTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), financeTestURL(t))
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	if err := pool.Ping(context.Background()); err != nil {
		t.Fatalf("ping: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func newUUID(t *testing.T) uuid.UUID {
	t.Helper()
	id, err := uuid.NewV7()
	if err != nil {
		t.Fatalf("new uuid: %v", err)
	}
	return id
}

func mustExec(t *testing.T, pool *pgxpool.Pool, sql string, args ...any) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), sql, args...); err != nil {
		t.Fatalf("exec %q: %v", sql, err)
	}
}

// fixture owns the rows one test inserts and removes them in dependency order
// when the test ends, so tests share a database without a reset between them.
// Every id is explicit and every cleanup deletes by id, so no other row is
// touched.
type fixture struct {
	pool        *pgxpool.Pool
	orgs        []uuid.UUID
	persons     []uuid.UUID
	projects    []uuid.UUID
	milestones  []uuid.UUID
	contracts   []uuid.UUID
	cMilestones []uuid.UUID
	invoices    []uuid.UUID
	payments    []uuid.UUID
	expenses    []uuid.UUID
	budgets     []uuid.UUID
	standings   []uuid.UUID
	accounts    []uuid.UUID
	categories  []string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	f := &fixture{pool: financeTestPool(t)}
	t.Cleanup(func() { f.cleanup(t) })
	return f
}

// cleanup removes children before parents. A test that failed part-way through
// leaves some slices empty, and an id-less delete matches nothing, so running
// the whole sequence is always safe.
func (f *fixture) cleanup(t *testing.T) {
	ctx := context.Background()
	deleteByID := func(table string, ids []uuid.UUID) {
		if len(ids) == 0 {
			return
		}
		if _, err := f.pool.Exec(ctx, `DELETE FROM `+table+` WHERE id = ANY($1)`, ids); err != nil {
			t.Logf("cleanup %s: %v", table, err)
		}
	}
	// Allocations go with the payments (ON DELETE CASCADE); the lines go with
	// the invoices.
	deleteByID("finance.invoice_lines", f.invoices)
	deleteByID("finance.payments", f.payments)
	deleteByID("finance.invoices", f.invoices)
	deleteByID("finance.expenses", f.expenses)
	deleteByID("finance.budgets", f.budgets)
	deleteByID("finance.recurring_costs", f.standings)
	deleteByID("finance.accounts", f.accounts)
	if len(f.categories) > 0 {
		if _, err := f.pool.Exec(ctx, `DELETE FROM finance.expense_categories WHERE key = ANY($1)`, f.categories); err != nil {
			t.Logf("cleanup expense_categories: %v", err)
		}
	}
	deleteByID("contracts.milestones", f.cMilestones)
	deleteByID("contracts.contracts", f.contracts)
	deleteByID("work.milestones", f.milestones)
	deleteByID("work.projects", f.projects)
	deleteByID("people.persons", f.persons)
	deleteByID("crm.organizations", f.orgs)
}

func (f *fixture) org(t *testing.T) uuid.UUID {
	t.Helper()
	id := newUUID(t)
	name := "IT Co " + id.String()[:8]
	mustExec(t, f.pool, `INSERT INTO crm.organizations (id, name, name_normalized, status)
		VALUES ($1, $2, $3, 'prospect')`, id, name, "it co "+id.String()[:8])
	f.orgs = append(f.orgs, id)
	return id
}

func (f *fixture) person(t *testing.T) uuid.UUID {
	t.Helper()
	id := newUUID(t)
	mustExec(t, f.pool, `INSERT INTO people.persons (id, full_name, employment_type, status)
		VALUES ($1, 'Finance Test Person', 'full_time', 'active')`, id)
	f.persons = append(f.persons, id)
	return id
}

func (f *fixture) category(t *testing.T) string {
	t.Helper()
	key := "it_finance_" + uuid.NewString()[:8]
	mustExec(t, f.pool, `INSERT INTO finance.expense_categories (key, label, sort_order, active)
		VALUES ($1, 'Integration', 900, true)`, key)
	f.categories = append(f.categories, key)
	return key
}

func (f *fixture) account(t *testing.T, opening int64) uuid.UUID {
	t.Helper()
	id := newUUID(t)
	mustExec(t, f.pool, `INSERT INTO finance.accounts (id, name, account_type, currency, opening_balance_minor)
		VALUES ($1, $2, 'bank', 'KES', $3)`, id, "IT Bank "+id.String()[:8], opening)
	f.accounts = append(f.accounts, id)
	return id
}

// invoice inserts an invoice. contract and milestone are optional: only an
// invoice billed against a milestone is attributed to a project.
func (f *fixture) invoice(t *testing.T, org uuid.UUID, contract, milestone *uuid.UUID, status InvoiceStatus, total int64, issue, due time.Time) uuid.UUID {
	t.Helper()
	id := newUUID(t)
	mustExec(t, f.pool, `INSERT INTO finance.invoices
		(id, ref, organization_id, contract_id, milestone_id, issue_date, due_date, total_minor, currency, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'KES', $9)`,
		id, "INV-IT-"+id.String(), org, contract, milestone, issue, due, total, string(status))
	f.invoices = append(f.invoices, id)
	return id
}

func (f *fixture) invoiceLine(t *testing.T, invoice uuid.UUID, description string, quantity float64, unitPrice, amount int64) uuid.UUID {
	t.Helper()
	id := newUUID(t)
	mustExec(t, f.pool, `INSERT INTO finance.invoice_lines
		(id, invoice_id, description, quantity, unit_price_minor, tax_rate_bp, amount_minor, sort_order)
		VALUES ($1, $2, $3, $4, $5, 1600, $6, 0)`,
		id, invoice, description, quantity, unitPrice, amount)
	return id
}

func (f *fixture) payment(t *testing.T, amount int64, method PaymentMethod, received time.Time, account *uuid.UUID) uuid.UUID {
	t.Helper()
	id := newUUID(t)
	mustExec(t, f.pool, `INSERT INTO finance.payments
		(id, ref, amount_minor, currency, method, received_on, account_id)
		VALUES ($1, $2, $3, 'KES', $4, $5, $6)`,
		id, "PAY-IT-"+id.String(), amount, string(method), received, account)
	f.payments = append(f.payments, id)
	return id
}

func (f *fixture) allocate(t *testing.T, payment, invoice uuid.UUID, amount int64) {
	t.Helper()
	mustExec(t, f.pool, `INSERT INTO finance.payment_allocations (id, payment_id, invoice_id, amount_minor)
		VALUES ($1, $2, $3, $4)`, newUUID(t), payment, invoice, amount)
}

func (f *fixture) expense(t *testing.T, claimant uuid.UUID, category string, amount int64, status ExpenseStatus, spentOn time.Time, account, project *uuid.UUID) uuid.UUID {
	t.Helper()
	id := newUUID(t)
	mustExec(t, f.pool, `INSERT INTO finance.expenses
		(id, ref, claimant_person_id, category_key, project_id, amount_minor, currency, spent_on, account_id, status)
		VALUES ($1, $2, $3, $4, $5, $6, 'KES', $7, $8, $9)`,
		id, "EXP-IT-"+id.String(), claimant, category, project, amount, spentOn, account, string(status))
	f.expenses = append(f.expenses, id)
	return id
}

func (f *fixture) budget(t *testing.T, scope BudgetScope, amount int64, status BudgetStatus, start, end time.Time) uuid.UUID {
	t.Helper()
	id := newUUID(t)
	mustExec(t, f.pool, `INSERT INTO finance.budgets
		(id, scope, period_start, period_end, amount_minor, currency, status)
		VALUES ($1, $2, $3, $4, $5, 'KES', $6)`, id, string(scope), start, end, amount, string(status))
	f.budgets = append(f.budgets, id)
	return id
}

func (f *fixture) recurring(t *testing.T, amount int64, cycle RecurringCycle, renewal time.Time) uuid.UUID {
	t.Helper()
	id := newUUID(t)
	mustExec(t, f.pool, `INSERT INTO finance.recurring_costs
		(id, vendor, amount_minor, currency, cycle, next_renewal_on, active)
		VALUES ($1, $2, $3, 'KES', $4, $5, true)`, id, "IT Vendor "+id.String()[:8], amount, string(cycle), renewal)
	f.standings = append(f.standings, id)
	return id
}

func (f *fixture) project(t *testing.T) uuid.UUID {
	t.Helper()
	id := newUUID(t)
	mustExec(t, f.pool, `INSERT INTO work.projects (id, ref, name, status) VALUES ($1, $2, $3, 'active')`,
		id, "PRJ-IT-"+id.String(), "IT Project "+id.String()[:8])
	f.projects = append(f.projects, id)
	return id
}

func (f *fixture) milestone(t *testing.T, project uuid.UUID) uuid.UUID {
	t.Helper()
	id := newUUID(t)
	mustExec(t, f.pool, `INSERT INTO work.milestones (id, project_id, name) VALUES ($1, $2, 'Delivery')`, id, project)
	f.milestones = append(f.milestones, id)
	return id
}

func (f *fixture) contract(t *testing.T, org uuid.UUID) uuid.UUID {
	t.Helper()
	id := newUUID(t)
	mustExec(t, f.pool, `INSERT INTO contracts.contracts (id, ref, organization_id, contract_type)
		VALUES ($1, $2, $3, 'fixed_price_project')`, id, "CTR-IT-"+id.String(), org)
	f.contracts = append(f.contracts, id)
	return id
}

func (f *fixture) contractMilestone(t *testing.T, contract, workMilestone uuid.UUID, amount int64) uuid.UUID {
	t.Helper()
	id := newUUID(t)
	mustExec(t, f.pool, `INSERT INTO contracts.milestones
		(id, contract_id, title, amount_minor, currency, project_milestone_id)
		VALUES ($1, $2, 'Billing milestone', $3, 'KES', $4)`, id, contract, amount, workMilestone)
	f.cMilestones = append(f.cMilestones, id)
	return id
}

func findAccount(t *testing.T, accounts []AccountBalance, id uuid.UUID) AccountBalance {
	t.Helper()
	for _, a := range accounts {
		if a.ID == id {
			return a
		}
	}
	t.Fatalf("account %s not in %d rows", id, len(accounts))
	return AccountBalance{}
}

func findInvoice(t *testing.T, invoices []Invoice, id uuid.UUID) Invoice {
	t.Helper()
	for _, inv := range invoices {
		if inv.ID == id {
			return inv
		}
	}
	t.Fatalf("invoice %s not in %d rows", id, len(invoices))
	return Invoice{}
}

func bucketMap(buckets []Bucket) map[string]Bucket {
	m := make(map[string]Bucket, len(buckets))
	for _, b := range buckets {
		m[b.Key] = b
	}
	return m
}

// TestFinanceReadModel proves the store's SQL against a real database: the
// account balance rule, the ageing banding, the budget actuals and the
// headline totals, all measured as the difference the inserted rows make so
// the assertions hold whatever else is in the database.
func TestFinanceReadModel(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	store := NewPGStore(f.pool)

	asOf := time.Date(2026, time.October, 19, 0, 0, 0, 0, time.UTC)
	svc := NewService(store, nil)
	svc.now = func() time.Time { return asOf }
	day := func(offset int) time.Time { return asOf.AddDate(0, 0, offset) }

	// Snapshot the shared aggregates first; only the difference is asserted.
	before, beforeAsOf, err := svc.Overview(ctx)
	if err != nil {
		t.Fatalf("baseline overview: %v", err)
	}
	if !beforeAsOf.Equal(asOf) {
		t.Fatalf("overview as-of = %s, want %s", beforeAsOf, asOf)
	}
	beforeRecv, err := svc.Receivables(ctx)
	if err != nil {
		t.Fatalf("baseline receivables: %v", err)
	}

	org := f.org(t)
	person := f.person(t)
	category := f.category(t)
	account := f.account(t, 100_000)

	// Four invoices: one not yet due, one overdue, one long overdue and only
	// partly paid, and one settled in full (which must drop out of the open
	// set even though its status is still partially_paid).
	invCurrent := f.invoice(t, org, nil, nil, InvoiceIssued, 50_000, day(-10), day(5))
	invOverdue := f.invoice(t, org, nil, nil, InvoiceIssued, 30_000, day(-40), day(-10))
	invOld := f.invoice(t, org, nil, nil, InvoicePartiallyPaid, 20_000, day(-200), day(-100))
	invSettled := f.invoice(t, org, nil, nil, InvoicePartiallyPaid, 6_000, day(-8), day(2))
	f.invoiceLine(t, invCurrent, "Design", 2.5, 20_000, 50_000)

	payOld := f.payment(t, 10_000, PaymentBank, day(-5), &account)
	f.allocate(t, payOld, invOld, 5_000)
	paySettled := f.payment(t, 6_000, PaymentBank, day(-7), &account)
	f.allocate(t, paySettled, invSettled, 6_000)

	// Only paid/reimbursed/reconciled expenses leave the account; a submitted
	// claim is pending, a draft is neither.
	f.expense(t, person, category, 7_000, ExpensePaid, day(-3), &account, nil)
	f.expense(t, person, category, 4_000, ExpenseSubmitted, day(-2), &account, nil)
	f.expense(t, person, category, 1_000, ExpenseDraft, day(-1), nil, nil)

	// A budget under its ceiling and one over 90% of it.
	bigBudget := f.budget(t, ScopeCompanyPeriod, 100_000, BudgetApproved, day(-15), day(15))
	smallBudget := f.budget(t, ScopeCompanyPeriod, 5_000, BudgetApproved, day(-15), day(15))

	recurring := f.recurring(t, 12_000, CycleAnnual, day(10))

	// ── headline ────────────────────────────────────────────────────────────
	after, _, err := svc.Overview(ctx)
	if err != nil {
		t.Fatalf("overview: %v", err)
	}
	wantOverview := OverviewTotals{
		CashMinor:                   109_000, // opening 100k + payments 16k - paid expense 7k
		ReceivablesOutstandingMinor: 95_000,  // 50k + 30k + (20k - 5k)
		ReceivablesOverdueMinor:     45_000,  // 30k + 15k
		Invoiced90DMinor:            86_000,  // 50k + 30k + 6k issued in the last 90 days
		Collected90DMinor:           16_000,  // 10k + 6k received
		Expenses30DMinor:            12_000,  // 7k + 4k + 1k spent in the last 30 days
		PendingExpenses:             1,
		OverdueInvoiceCount:         2,
		UpcomingRenewals30D:         1,
		ActiveBudgetCount:           2,
		BudgetsOverThreshold:        1,
	}
	gotOverview := OverviewTotals{
		CashMinor:                   after.CashMinor - before.CashMinor,
		ReceivablesOutstandingMinor: after.ReceivablesOutstandingMinor - before.ReceivablesOutstandingMinor,
		ReceivablesOverdueMinor:     after.ReceivablesOverdueMinor - before.ReceivablesOverdueMinor,
		Invoiced90DMinor:            after.Invoiced90DMinor - before.Invoiced90DMinor,
		Collected90DMinor:           after.Collected90DMinor - before.Collected90DMinor,
		Expenses30DMinor:            after.Expenses30DMinor - before.Expenses30DMinor,
		PendingExpenses:             after.PendingExpenses - before.PendingExpenses,
		OverdueInvoiceCount:         after.OverdueInvoiceCount - before.OverdueInvoiceCount,
		UpcomingRenewals30D:         after.UpcomingRenewals30D - before.UpcomingRenewals30D,
		ActiveBudgetCount:           after.ActiveBudgetCount - before.ActiveBudgetCount,
		BudgetsOverThreshold:        after.BudgetsOverThreshold - before.BudgetsOverThreshold,
	}
	if gotOverview != wantOverview {
		t.Errorf("overview delta = %+v, want %+v", gotOverview, wantOverview)
	}

	// ── account balances ────────────────────────────────────────────────────
	accounts, err := store.ListAccounts(ctx)
	if err != nil {
		t.Fatalf("ListAccounts: %v", err)
	}
	got := findAccount(t, accounts, account)
	if got.BalanceMinor != 109_000 {
		t.Errorf("account balance = %d, want 109000", got.BalanceMinor)
	}
	if got.OpeningBalanceMinor == nil || *got.OpeningBalanceMinor != 100_000 {
		t.Errorf("opening balance = %v, want 100000", got.OpeningBalanceMinor)
	}

	// ── open receivables ────────────────────────────────────────────────────
	open, err := store.OpenReceivables(ctx, asOf)
	if err != nil {
		t.Fatalf("OpenReceivables: %v", err)
	}
	for _, id := range []uuid.UUID{invCurrent, invOverdue, invOld} {
		if inv := findInvoice(t, open, id); inv.OutstandingMinor <= 0 {
			t.Errorf("invoice %s outstanding = %d, want positive", id, inv.OutstandingMinor)
		}
	}
	for _, inv := range open {
		if inv.ID == invSettled {
			t.Errorf("settled invoice %s should not be open", invSettled)
		}
	}

	gotOld := findInvoice(t, open, invOld)
	if gotOld.OutstandingMinor != 15_000 {
		t.Errorf("partly paid outstanding = %d, want 15000", gotOld.OutstandingMinor)
	}
	if gotOld.AmountPaidMinor != 5_000 {
		t.Errorf("partly paid allocated = %d, want 5000", gotOld.AmountPaidMinor)
	}

	// ── ageing ──────────────────────────────────────────────────────────────
	afterRecv, err := svc.Receivables(ctx)
	if err != nil {
		t.Fatalf("Receivables: %v", err)
	}
	if d := afterRecv.TotalOutstanding - beforeRecv.TotalOutstanding; d != 95_000 {
		t.Errorf("receivables total delta = %d, want 95000", d)
	}
	oldBuckets, newBuckets := bucketMap(beforeRecv.Buckets), bucketMap(afterRecv.Buckets)
	wantBuckets := map[string]Bucket{
		"current": {Key: "current", Count: 1, Amount: 50_000},
		"1_30":    {Key: "1_30", Count: 1, Amount: 30_000},
		"31_60":   {Key: "31_60", Count: 0, Amount: 0},
		"61_90":   {Key: "61_90", Count: 0, Amount: 0},
		"90_plus": {Key: "90_plus", Count: 1, Amount: 15_000},
	}
	for key, want := range wantBuckets {
		gotCount := newBuckets[key].Count - oldBuckets[key].Count
		gotAmount := newBuckets[key].Amount - oldBuckets[key].Amount
		if gotCount != want.Count || gotAmount != want.Amount {
			t.Errorf("bucket %s delta = (%d, %d), want (%d, %d)", key, gotCount, gotAmount, want.Count, want.Amount)
		}
	}

	// ── invoice listing and lines ───────────────────────────────────────────
	invoices, err := svc.Invoices(ctx, InvoiceQuery{OrganizationID: &org}, 50, nil)
	if err != nil {
		t.Fatalf("Invoices: %v", err)
	}
	if len(invoices) != 4 {
		t.Fatalf("invoices for org = %d, want 4", len(invoices))
	}
	gotCurrent := findInvoice(t, invoices, invCurrent)
	if gotCurrent.OutstandingMinor != 50_000 {
		t.Errorf("current outstanding = %d, want 50000", gotCurrent.OutstandingMinor)
	}
	if gotCurrent.OrganizationName == "" {
		t.Error("invoice organization_name should be joined from crm.organizations")
	}

	overdue, err := svc.Invoices(ctx, InvoiceQuery{OrganizationID: &org, Overdue: true}, 50, nil)
	if err != nil {
		t.Fatalf("overdue invoices: %v", err)
	}
	if len(overdue) != 2 {
		t.Fatalf("overdue invoices = %d, want 2", len(overdue))
	}

	inv, lines, err := svc.Invoice(ctx, invCurrent)
	if err != nil {
		t.Fatalf("Invoice: %v", err)
	}
	if inv.ID != invCurrent {
		t.Errorf("invoice id = %s, want %s", inv.ID, invCurrent)
	}
	if len(lines) != 1 {
		t.Fatalf("lines = %d, want 1", len(lines))
	}
	if lines[0].Quantity != 2.5 {
		t.Errorf("line quantity = %v, want 2.5", lines[0].Quantity)
	}
	if lines[0].UnitPriceMinor == nil || *lines[0].UnitPriceMinor != 20_000 {
		t.Errorf("line unit price = %v, want 20000", lines[0].UnitPriceMinor)
	}

	// ── expenses ────────────────────────────────────────────────────────────
	claims, err := svc.Expenses(ctx, ExpenseQuery{ClaimantID: &person}, 50, nil)
	if err != nil {
		t.Fatalf("Expenses: %v", err)
	}
	if len(claims) != 3 {
		t.Fatalf("claims = %d, want 3", len(claims))
	}
	submitted := ExpenseSubmitted
	pending, err := svc.Expenses(ctx, ExpenseQuery{ClaimantID: &person, Status: &submitted}, 50, nil)
	if err != nil {
		t.Fatalf("Expenses(submitted): %v", err)
	}
	if len(pending) != 1 {
		t.Fatalf("submitted claims = %d, want 1", len(pending))
	}
	if pending[0].AmountMinor == nil || *pending[0].AmountMinor != 4_000 {
		t.Errorf("submitted amount = %v, want 4000", pending[0].AmountMinor)
	}

	// ── budgets ─────────────────────────────────────────────────────────────
	companyPeriod := ScopeCompanyPeriod
	budgets, err := svc.Budgets(ctx, BudgetQuery{Scope: &companyPeriod}, 50, nil)
	if err != nil {
		t.Fatalf("Budgets: %v", err)
	}
	foundBig, foundSmall := false, false
	for _, b := range budgets {
		switch b.ID {
		case bigBudget:
			foundBig = true
			assertBudget(t, b, 100_000, 7_000, 93_000)
		case smallBudget:
			foundSmall = true
			assertBudget(t, b, 5_000, 7_000, -2_000)
		}
	}
	if !foundBig || !foundSmall {
		t.Fatalf("budgets found big=%v small=%v, want both", foundBig, foundSmall)
	}

	one, err := svc.Budget(ctx, bigBudget)
	if err != nil {
		t.Fatalf("Budget: %v", err)
	}
	assertBudget(t, one, 100_000, 7_000, 93_000)

	// ── recurring costs ─────────────────────────────────────────────────────
	costs, err := svc.RecurringCosts(ctx)
	if err != nil {
		t.Fatalf("RecurringCosts: %v", err)
	}
	var found bool
	for _, c := range costs {
		if c.ID != recurring {
			continue
		}
		found = true
		if c.MonthlyEquivalentMinor == nil || *c.MonthlyEquivalentMinor != 1_000 {
			t.Errorf("monthly equivalent = %v, want 1000", c.MonthlyEquivalentMinor)
		}
	}
	if !found {
		t.Fatalf("recurring cost %s not listed", recurring)
	}
}

func assertBudget(t *testing.T, b Budget, amount, actual, variance int64) {
	t.Helper()
	if b.AmountMinor == nil || *b.AmountMinor != amount {
		t.Errorf("budget %s amount = %v, want %d", b.ID, b.AmountMinor, amount)
	}
	if b.ActualMinor == nil || *b.ActualMinor != actual {
		t.Errorf("budget %s actual = %v, want %d", b.ID, b.ActualMinor, actual)
	}
	if b.VarianceMinor == nil || *b.VarianceMinor != variance {
		t.Errorf("budget %s variance = %v, want %d", b.ID, b.VarianceMinor, variance)
	}
}

// TestProjectProfitabilityAttributesMilestoneRevenue exercises the join that
// attributes an invoice to a project: the invoice names a billing milestone,
// the milestone names the work milestone, the work milestone names the project.
// Expenses are attributed directly by their project link.
func TestProjectProfitabilityAttributesMilestoneRevenue(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	store := NewPGStore(f.pool)
	svc := NewService(store, nil)
	asOf := time.Date(2026, time.October, 19, 0, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return asOf }

	org := f.org(t)
	person := f.person(t)
	category := f.category(t)
	project := f.project(t)
	workMilestone := f.milestone(t, project)
	contract := f.contract(t, org)
	billingMilestone := f.contractMilestone(t, contract, workMilestone, 60_000)

	issued := f.invoice(t, org, &contract, &billingMilestone, InvoiceIssued, 60_000, asOf.AddDate(0, 0, -20), asOf.AddDate(0, 0, 10))
	f.invoice(t, org, &contract, &billingMilestone, InvoiceDraft, 40_000, asOf.AddDate(0, 0, -5), asOf.AddDate(0, 0, 25))
	pay := f.payment(t, 25_000, PaymentBank, asOf.AddDate(0, 0, -3), nil)
	f.allocate(t, pay, issued, 25_000)
	f.expense(t, person, category, 30_000, ExpenseApproved, asOf.AddDate(0, 0, -10), nil, &project)

	profit, err := svc.ProjectProfitability(ctx, project)
	if err != nil {
		t.Fatalf("ProjectProfitability: %v", err)
	}
	if profit.ProjectName == "" {
		t.Error("project name should be read from work.projects")
	}
	if profit.RevenueInvoicedMinor != 100_000 {
		t.Errorf("revenue invoiced = %d, want 100000", profit.RevenueInvoicedMinor)
	}
	if profit.RevenueCollectedMinor != 25_000 {
		t.Errorf("revenue collected = %d, want 25000", profit.RevenueCollectedMinor)
	}
	if profit.CostExpensesMinor != 30_000 {
		t.Errorf("cost expenses = %d, want 30000", profit.CostExpensesMinor)
	}

	view := projectProfitabilityOf(profit, true)
	if view.MarginMinor == nil || *view.MarginMinor != 70_000 {
		t.Errorf("margin = %v, want 70000", view.MarginMinor)
	}
	if view.MarginBP == nil || *view.MarginBP != 7_000 {
		t.Errorf("margin_bp = %v, want 7000", view.MarginBP)
	}
}

// TestProjectProfitabilityUnknownProjectAgainstDatabase covers the not-found
// path against the real store: an unknown id is a miss, not a zero report.
func TestProjectProfitabilityUnknownProjectAgainstDatabase(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	svc := NewService(NewPGStore(f.pool), nil)

	p, err := svc.ProjectProfitability(ctx, newUUID(t))
	if err == nil {
		t.Fatalf("ProjectProfitability = %+v, want an error", p)
	}
}
