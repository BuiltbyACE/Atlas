package finance

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func mustTime(t *testing.T) time.Time {
	t.Helper()
	return time.Date(2026, time.October, 19, 0, 0, 0, 0, time.UTC)
}

func int64Ptr(v int64) *int64 { return &v }

func TestAgeingBucketKey(t *testing.T) {
	cases := []struct {
		days int
		want string
	}{
		{-5, "current"},
		{0, "current"},
		{1, "1_30"},
		{30, "1_30"},
		{31, "31_60"},
		{60, "31_60"},
		{61, "61_90"},
		{90, "61_90"},
		{91, "90_plus"},
		{365, "90_plus"},
	}
	for _, c := range cases {
		if got := AgeingBucketKey(c.days); got != c.want {
			t.Errorf("AgeingBucketKey(%d) = %q, want %q", c.days, got, c.want)
		}
	}
}

func TestMonthlyEquivalent(t *testing.T) {
	amount := int64(12000)
	cases := []struct {
		cycle RecurringCycle
		want  int64
	}{
		{CycleMonthly, 12000},
		{CycleQuarterly, 4000},
		{CycleAnnual, 1000},
	}
	for _, c := range cases {
		got := monthlyEquivalent(&amount, c.cycle)
		if got == nil || *got != c.want {
			t.Errorf("monthlyEquivalent(12000, %s) = %v, want %d", c.cycle, got, c.want)
		}
	}
	if monthlyEquivalent(nil, CycleMonthly) != nil {
		t.Error("monthlyEquivalent(nil) should be nil, not zero")
	}
	if monthlyEquivalent(&amount, RecurringCycle("weekly")) != nil {
		t.Error("an unknown cycle should not normalise")
	}
}

func TestEnumValidators(t *testing.T) {
	for _, v := range []AccountType{AccountBank, AccountCash, AccountMpesaTill, AccountMpesaPaybill, AccountCard} {
		if !validAccountType(v) {
			t.Errorf("validAccountType(%q) = false", v)
		}
	}
	if validAccountType("wallet") {
		t.Error("validAccountType(wallet) = true")
	}

	for _, v := range []ExpenseStatus{ExpenseDraft, ExpenseSubmitted, ExpenseApproved, ExpenseRejected, ExpensePaid, ExpenseReimbursed, ExpenseReconciled} {
		if !validExpenseStatus(v) {
			t.Errorf("validExpenseStatus(%q) = false", v)
		}
	}
	if validExpenseStatus("cancelled") {
		t.Error("validExpenseStatus(cancelled) = true")
	}

	for _, v := range []InvoiceStatus{InvoiceScheduled, InvoiceDraft, InvoiceApproved, InvoiceIssued, InvoicePartiallyPaid, InvoicePaid, InvoiceOverdue, InvoiceDisputed, InvoiceWrittenOff} {
		if !validInvoiceStatus(v) {
			t.Errorf("validInvoiceStatus(%q) = false", v)
		}
	}
	if validInvoiceStatus("void") {
		t.Error("validInvoiceStatus(void) = true")
	}

	for _, v := range []PaymentMethod{PaymentBank, PaymentMpesa, PaymentCard, PaymentCash} {
		if !validPaymentMethod(v) {
			t.Errorf("validPaymentMethod(%q) = false", v)
		}
	}
	if validPaymentMethod("cheque") {
		t.Error("validPaymentMethod(cheque) = true")
	}

	for _, v := range []RecurringCycle{CycleMonthly, CycleQuarterly, CycleAnnual} {
		if !validRecurringCycle(v) {
			t.Errorf("validRecurringCycle(%q) = false", v)
		}
	}

	for _, v := range []BudgetScope{ScopeCompanyPeriod, ScopeDepartment, ScopeProject, ScopeUnit} {
		if !validBudgetScope(v) {
			t.Errorf("validBudgetScope(%q) = false", v)
		}
	}

	for _, v := range []BudgetStatus{BudgetDraft, BudgetApproved, BudgetClosed} {
		if !validBudgetStatus(v) {
			t.Errorf("validBudgetStatus(%q) = false", v)
		}
	}
}

func TestNormalizeAndValidateCurrency(t *testing.T) {
	if got := normalizeCurrency(" kes "); got != "KES" {
		t.Errorf("normalizeCurrency = %q, want KES", got)
	}
	if got := normalizeCurrency(""); got != BaseCurrency {
		t.Errorf("empty currency = %q, want %q", got, BaseCurrency)
	}
	if err := validateCurrency("KES"); err != nil {
		t.Errorf("validateCurrency(KES) = %v", err)
	}
	if err := validateCurrency("ke"); !errors.Is(err, ErrInvalid) {
		t.Errorf("short currency err = %v, want ErrInvalid", err)
	}
	if err := validateCurrency("K3S"); !errors.Is(err, ErrInvalid) {
		t.Errorf("non-letter currency err = %v, want ErrInvalid", err)
	}
}

func TestCursorRoundTrip(t *testing.T) {
	at := mustTime(t)
	id := uuid.Must(uuid.NewV7())

	encoded := encodeCursor(at, id)
	got, err := decodeCursor(encoded)
	if err != nil {
		t.Fatalf("decodeCursor: %v", err)
	}
	if got.ID != id || !got.At.Equal(at) {
		t.Fatalf("cursor = %s/%s, want %s/%s", got.At, got.ID, at, id)
	}
	if _, err := decodeCursor("not-a-cursor"); err == nil {
		t.Fatal("malformed cursor should not decode")
	}
	if !errors.Is(errInvalidField("cursor"), ErrInvalid) {
		t.Fatal("a field error should satisfy errors.Is(ErrInvalid)")
	}
}

func TestDaysPastDue(t *testing.T) {
	asOf := mustTime(t)
	due := asOf.AddDate(0, 0, -10)
	if got := daysPastDue(&due, asOf); got != 10 {
		t.Errorf("daysPastDue = %d, want 10", got)
	}
	future := asOf.AddDate(0, 0, 3)
	if got := daysPastDue(&future, asOf); got != 0 {
		t.Errorf("future due date = %d, want 0", got)
	}
	if got := daysPastDue(nil, asOf); got != 0 {
		t.Errorf("no due date = %d, want 0", got)
	}
}

// TestServiceRejectsUnknownQueryEnums proves the service validates a filter
// before the store sees it: an unheard-of enum is a validation error, not an
// empty page. A nil store is safe here because validation short-circuits.
func TestServiceRejectsUnknownQueryEnums(t *testing.T) {
	svc := NewService(nil, nil)
	ctx := context.Background()

	badExpense := ExpenseStatus("cancelled")
	if _, err := svc.Expenses(ctx, ExpenseQuery{Status: &badExpense}, 10, nil); !errors.Is(err, ErrInvalid) {
		t.Errorf("expense status: err = %v, want ErrInvalid", err)
	}
	badInvoice := InvoiceStatus("void")
	if _, err := svc.Invoices(ctx, InvoiceQuery{Status: &badInvoice}, 10, nil); !errors.Is(err, ErrInvalid) {
		t.Errorf("invoice status: err = %v, want ErrInvalid", err)
	}
	badMethod := PaymentMethod("cheque")
	if _, err := svc.Payments(ctx, PaymentQuery{Method: &badMethod}, 10, nil); !errors.Is(err, ErrInvalid) {
		t.Errorf("payment method: err = %v, want ErrInvalid", err)
	}
	badScope := BudgetScope("team")
	if _, err := svc.Budgets(ctx, BudgetQuery{Scope: &badScope}, 10, nil); !errors.Is(err, ErrInvalid) {
		t.Errorf("budget scope: err = %v, want ErrInvalid", err)
	}
	badStatus := BudgetStatus("frozen")
	if _, err := svc.Budgets(ctx, BudgetQuery{Status: &badStatus}, 10, nil); !errors.Is(err, ErrInvalid) {
		t.Errorf("budget status: err = %v, want ErrInvalid", err)
	}
}

// TestReceivablesBandsOpenInvoices drives the service's ageing from a fake
// store, so the banding rule is tested without a database.
func TestReceivablesBandsOpenInvoices(t *testing.T) {
	asOf := mustTime(t)
	due := func(daysAgo int) *time.Time {
		d := asOf.AddDate(0, 0, -daysAgo)
		return &d
	}
	mk := func(total int64, daysAgo int) Invoice {
		return Invoice{TotalMinor: int64Ptr(total), OutstandingMinor: total, DueDate: due(daysAgo)}
	}
	store := &fakeStore{receivables: []Invoice{
		mk(100, -5),  // not yet due
		mk(200, 10),  // 1-30
		mk(300, 45),  // 31-60
		mk(400, 100), // 90+
	}}
	svc := NewService(store, nil)
	svc.now = func() time.Time { return asOf }

	rec, err := svc.Receivables(context.Background())
	if err != nil {
		t.Fatalf("Receivables: %v", err)
	}
	if rec.TotalOutstanding != 1000 {
		t.Errorf("total outstanding = %d, want 1000", rec.TotalOutstanding)
	}
	want := map[string]struct {
		count  int
		amount int64
	}{
		"current": {1, 100},
		"1_30":    {1, 200},
		"31_60":   {1, 300},
		"61_90":   {0, 0},
		"90_plus": {1, 400},
	}
	if len(rec.Buckets) != len(AgeingBuckets) {
		t.Fatalf("bucket count = %d, want %d", len(rec.Buckets), len(AgeingBuckets))
	}
	for _, b := range rec.Buckets {
		w := want[b.Key]
		if b.Count != w.count || b.Amount != w.amount {
			t.Errorf("bucket %s = (%d, %d), want (%d, %d)", b.Key, b.Count, b.Amount, w.count, w.amount)
		}
	}
}

// TestProjectProfitabilityMargin checks the margin and basis points are struck
// from revenue and expenses, and that no time cost is invented.
func TestProjectProfitabilityMargin(t *testing.T) {
	store := &fakeStore{projectName: "Atlas rollout", projectOK: true, invoiced: 1_000_000, collected: 600_000, costs: 685_000}
	svc := NewService(store, nil)

	p, err := svc.ProjectProfitability(context.Background(), uuid.Must(uuid.NewV7()))
	if err != nil {
		t.Fatalf("ProjectProfitability: %v", err)
	}
	view := projectProfitabilityOf(p, true)
	if view.MarginMinor == nil || *view.MarginMinor != 315_000 {
		t.Fatalf("margin = %v, want 315000", view.MarginMinor)
	}
	if view.MarginBP == nil || *view.MarginBP != 3150 {
		t.Fatalf("margin_bp = %v, want 3150", view.MarginBP)
	}
	if view.CostTimeMinor != nil {
		t.Fatalf("cost_time_minor = %v, want omitted", view.CostTimeMinor)
	}
}

func TestProjectProfitabilityUnknownProject(t *testing.T) {
	svc := NewService(&fakeStore{projectOK: false}, nil)
	if _, err := svc.ProjectProfitability(context.Background(), uuid.Must(uuid.NewV7())); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

// TestMaskingOmitsEveryAmount is the contract for finance.amount.sensitive.read:
// without it no `*_minor` (or margin_bp) key appears anywhere in the response,
// and no hidden amount is sent as a zero.
func TestMaskingOmitsEveryAmount(t *testing.T) {
	amount := int64(500)
	opening := int64(100)
	total := int64(900)
	unit := int64(300)
	asOf := mustTime(t)

	views := []any{
		overviewOf(OverviewTotals{CashMinor: 100, PendingExpenses: 2}, asOf, false),
		accountOf(AccountBalance{Account: Account{OpeningBalanceMinor: &opening}, BalanceMinor: 5}, false),
		cashPositionOf(CashPosition{AsOf: asOf, TotalMinor: 7, InflowMinor: 3, OutflowMinor: 1}, false),
		receivablesOf(Receivables{AsOf: asOf, TotalOutstanding: 42, Buckets: []Bucket{{Key: "current", Count: 1, Amount: 42}}, Invoices: []Invoice{{TotalMinor: &total, OutstandingMinor: 42}}}, false),
		expenseOf(Expense{AmountMinor: &amount, Status: ExpenseSubmitted}, false),
		invoiceOf(Invoice{TotalMinor: &total, AmountPaidMinor: 100, OutstandingMinor: 800}, false, []InvoiceLine{{UnitPriceMinor: &unit, AmountMinor: &unit}}),
		paymentOf(Payment{AmountMinor: &amount, AllocatedMinor: 100, UnallocatedMinor: 400}, false),
		budgetOf(Budget{AmountMinor: &amount, ActualMinor: &opening, VarianceMinor: &opening}, false),
		recurringCostOf(RecurringCost{AmountMinor: &amount, MonthlyEquivalentMinor: &amount}, false),
		projectProfitabilityOf(ProjectProfitability{RevenueInvoicedMinor: 1000, CostExpensesMinor: 700}, false),
	}
	for _, v := range views {
		if keys := sensitiveKeys(t, v); len(keys) != 0 {
			t.Errorf("masked view %T still exposed %v", v, keys)
		}
	}
}

// TestMaskingShowsZeroWhenPermitted: a zero balance is a real figure, so it must
// appear (as 0) when the viewer may see amounts — absence is reserved for
// withholding.
func TestMaskingShowsZeroWhenPermitted(t *testing.T) {
	view := accountOf(AccountBalance{BalanceMinor: 0}, true)
	keys := sensitiveKeys(t, view)
	if len(keys) != 1 || keys[0] != "balance_minor" {
		t.Fatalf("visible view keys = %v, want [balance_minor]", keys)
	}
	if view.BalanceMinor == nil || *view.BalanceMinor != 0 {
		t.Fatalf("balance = %v, want a visible zero", view.BalanceMinor)
	}

	overview := overviewOf(OverviewTotals{CashMinor: 0}, mustTime(t), true)
	if overview.CashMinor == nil || *overview.CashMinor != 0 {
		t.Fatalf("cash = %v, want a visible zero", overview.CashMinor)
	}
}

// sensitiveKeys marshals a view and returns every *_minor / margin_bp key.
func sensitiveKeys(t *testing.T, v any) []string {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	var found []string
	var walk func(any)
	walk = func(x any) {
		switch node := x.(type) {
		case map[string]any:
			for k, val := range node {
				if strings.HasSuffix(k, "_minor") || k == "margin_bp" {
					found = append(found, k)
				}
				walk(val)
			}
		case []any:
			for _, e := range node {
				walk(e)
			}
		}
	}
	walk(decoded)
	return found
}
