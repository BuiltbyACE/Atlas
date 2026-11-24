package finance

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

// The write lifecycle's rules that never touch the database: the money
// arithmetic, the payment-term parser, the threshold comparison, the state
// predicates, and the claim rule that a big amount needs a receipt. The store
// and a fake Writer are not needed for any of these.

func TestLineArithmeticRounds(t *testing.T) {
	// 0.5 × 101 = 50.5 → 51 (nearest minor unit); 3 × 100000 = 300000.
	if got := lineAmountMinor(0.5, 101); got != 51 {
		t.Errorf("lineAmountMinor(0.5, 101) = %d, want 51", got)
	}
	if got := lineAmountMinor(3, 100000); got != 300000 {
		t.Errorf("lineAmountMinor(3, 100000) = %d, want 300000", got)
	}
	// 16% of 100000 = 16000.
	if got := lineTaxMinor(100000, 1600); got != 16000 {
		t.Errorf("lineTaxMinor(100000, 1600) = %d, want 16000", got)
	}
	// 1500bp of 100000 = 15000 exactly (exempt half-rate).
	if got := lineTaxMinor(100000, 1500); got != 15000 {
		t.Errorf("lineTaxMinor(100000, 1500) = %d, want 15000", got)
	}
}

func TestInvoiceLinesTotalAddsTax(t *testing.T) {
	lines := []InvoiceLineInput{
		{Description: "Build", Quantity: 2, UnitPriceMinor: 50000, TaxRateBP: 1600}, // 100000 + 16000
		{Description: "Hosting", Quantity: 1, UnitPriceMinor: 25000, TaxRateBP: 0},  //  25000
	}
	if got, want := invoiceLinesTotal(lines), int64(141000); got != want {
		t.Errorf("invoiceLinesTotal = %d, want %d", got, want)
	}
	// It must agree with what the validator prices a request into.
	inv, _, err := NewService(nil, nil).validateInvoiceInput(InvoiceInput{
		OrganizationID: uuid.New(), Currency: "KES", Lines: lines,
	})
	if err != nil {
		t.Fatalf("validateInvoiceInput: %v", err)
	}
	if inv.TotalMinor == nil || *inv.TotalMinor != 141000 {
		t.Errorf("validated total = %v, want 141000", inv.TotalMinor)
	}
}

func TestValidateInvoiceInputRejectsBadLines(t *testing.T) {
	org := uuid.New()
	cases := map[string]InvoiceInput{
		"no lines":      {OrganizationID: org, Currency: "KES"},
		"blank desc":    {OrganizationID: org, Currency: "KES", Lines: []InvoiceLineInput{{Quantity: 1, UnitPriceMinor: 1}}},
		"zero qty":      {OrganizationID: org, Currency: "KES", Lines: []InvoiceLineInput{{Description: "x", UnitPriceMinor: 1}}},
		"negative cost": {OrganizationID: org, Currency: "KES", Lines: []InvoiceLineInput{{Description: "x", Quantity: 1, UnitPriceMinor: -1}}},
		"tax too high":  {OrganizationID: org, Currency: "KES", Lines: []InvoiceLineInput{{Description: "x", Quantity: 1, UnitPriceMinor: 1, TaxRateBP: 20000}}},
	}
	svc := NewService(nil, nil)
	for name, in := range cases {
		if _, _, err := svc.validateInvoiceInput(in); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: err = %v, want ErrInvalid", name, err)
		}
	}
	// A due date before the issue date is refused.
	issue, due := time.Now(), time.Now().AddDate(0, 0, -1)
	if _, _, err := svc.validateInvoiceInput(InvoiceInput{
		OrganizationID: org, Currency: "KES", IssueDate: &issue, DueDate: &due,
		Lines: []InvoiceLineInput{{Description: "x", Quantity: 1, UnitPriceMinor: 1}},
	}); !errors.Is(err, ErrInvalid) {
		t.Errorf("due before issue: err = %v, want ErrInvalid", err)
	}
}

func TestPaymentTermDays(t *testing.T) {
	cases := []struct {
		terms string
		days  int
		ok    bool
	}{
		{"Net 30", 30, true},
		{"net 45", 45, true},
		{"30 days", 30, true},
		{"Payment due within 14 calendar days", 14, true},
		{"7 business days", 7, true},
		{"on receipt", 0, false},
		{"", 0, false},
	}
	for _, c := range cases {
		days, ok := paymentTermDays(c.terms)
		if ok != c.ok || (ok && days != c.days) {
			t.Errorf("paymentTermDays(%q) = (%d, %v), want (%d, %v)", c.terms, days, ok, c.days, c.ok)
		}
	}
}

func TestCrossed(t *testing.T) {
	// A budget of 100000 crosses 7500bp (75000) only when it moves from below
	// to at-or-above; an already-over budget produces no second crossing.
	if !crossed(70000, 80000, 100000, 7500) {
		t.Error("70000 → 80000 should cross 75% of 100000")
	}
	if crossed(80000, 90000, 100000, 7500) {
		t.Error("80000 → 90000 must not re-cross 75%")
	}
	if !crossed(99000, 100000, 100000, 10000) {
		t.Error("99000 → 100000 should cross 100%")
	}
	// Landing exactly on the threshold counts as crossing it.
	if !crossed(74999, 75000, 100000, 7500) {
		t.Error("landing exactly on the threshold should count")
	}
}

func TestLifecyclePredicates(t *testing.T) {
	if !expenseEditable(ExpenseDraft) || expenseEditable(ExpenseSubmitted) {
		t.Error("only a draft expense is editable")
	}
	if !expenseSubmittable(ExpenseDraft) || !expenseSubmittable(ExpenseRejected) || expenseSubmittable(ExpenseApproved) {
		t.Error("draft and rejected claims are submittable; approved is not")
	}
	if !expenseDecidable(ExpenseSubmitted) || expenseDecidable(ExpenseDraft) {
		t.Error("only a submitted claim is decidable")
	}
	if !expenseSettleable(ExpenseApproved) || expenseSettleable(ExpenseSubmitted) {
		t.Error("only an approved claim is settleable")
	}
	if !expenseReconcilable(ExpensePaid) || !expenseReconcilable(ExpenseReimbursed) || expenseReconcilable(ExpenseApproved) {
		t.Error("only a settled claim is reconcilable")
	}
	if !invoiceEditable(InvoiceDraft) || !invoiceEditable(InvoiceScheduled) || invoiceEditable(InvoiceIssued) {
		t.Error("draft and scheduled invoices are editable")
	}
	if !invoiceIssuable(InvoiceApproved) || invoiceIssuable(InvoiceDraft) {
		t.Error("only an approved invoice is issuable")
	}
	if !invoiceAllocable(InvoiceIssued) || invoiceAllocable(InvoiceWrittenOff) || invoiceAllocable(InvoicePaid) {
		t.Error("an issued/part-paid/overdue/disputed invoice is allocable; a written-off or paid one is not")
	}
	if !budgetApprovable(BudgetDraft) || budgetApprovable(BudgetApproved) {
		t.Error("only a draft budget is approvable")
	}
	if !budgetClosable(BudgetApproved) || budgetClosable(BudgetDraft) {
		t.Error("only an approved budget is closable")
	}
}

// TestReceiptRequiredAboveThreshold pins the §8.4 claim rule and the stable
// code a client branches on. It runs without a store: the rule only reads the
// configured threshold.
func TestReceiptRequiredAboveThreshold(t *testing.T) {
	svc := NewService(nil, nil)
	ctx := context.Background()
	base := Expense{
		ClaimantPersonID: uuid.New(), CategoryKey: "travel", Currency: "KES",
		SpentOn: time.Now().UTC(),
	}

	over := int64(defaultReceiptMinMinor + 1)
	base.AmountMinor = &over
	err := svc.validateExpenseInput(ctx, base)
	if !errors.Is(err, ErrInvalid) || problemCode(err) != CodeReceiptRequired {
		t.Fatalf("over threshold without receipt: err = %v (code %q), want ErrInvalid/%s",
			err, problemCode(err), CodeReceiptRequired)
	}

	// Attaching a receipt file satisfies the rule.
	file := uuid.New()
	base.ReceiptFileID = &file
	if err := svc.validateExpenseInput(ctx, base); err != nil {
		t.Fatalf("over threshold with receipt: %v", err)
	}

	// At or below the threshold no receipt is needed.
	at := int64(defaultReceiptMinMinor)
	base.ReceiptFileID = nil
	base.AmountMinor = &at
	if err := svc.validateExpenseInput(ctx, base); err != nil {
		t.Fatalf("at threshold: %v", err)
	}

	// A non-positive amount is always refused.
	zero := int64(0)
	base.AmountMinor = &zero
	if err := svc.validateExpenseInput(ctx, base); !errors.Is(err, ErrInvalid) {
		t.Fatalf("zero amount: err = %v, want ErrInvalid", err)
	}
}
