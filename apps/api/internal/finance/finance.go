// Package finance owns Phase 4 finance and commercial control: the cash
// position, receivables, project profitability, budgets and spending
// (docs/ATLAS.md §8.4).
//
// This package owns both halves: the read queries that answer the questions
// the finance screens ask — what cash is on hand, what clients owe and how
// overdue it is, what a project really earns, what is budgeted against
// actuals, and which standing costs are about to renew — and the write
// lifecycle that moves an expense through approval to settlement, an invoice
// from draft to issued and paid, a payment onto the invoices it settles, and a
// budget through approval to close. Storage for both came in migration 00022;
// the receipt, tax-reference and settlement columns came in 00023.
//
// Money is integer minor units with a currency (ADR-010): no floats, and never
// an amount without a currency. Go is the sole writer (ADR-002) and owns every
// rule here. Whether an amount is even shown is a permission decision
// (finance.amount.sensitive.read) made at the HTTP edge: a hidden amount is
// omitted from the response entirely, never replaced with a zero.
package finance

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Sentinel errors. The HTTP layer maps these onto problem responses.
var (
	ErrNotFound  = errors.New("finance: not found")
	ErrInvalid   = errors.New("finance: invalid")
	ErrConflict  = errors.New("finance: conflict")
	ErrForbidden = errors.New("finance: forbidden")
	ErrDuplicate = errors.New("finance: duplicate")
)

// Stable problem codes for the write lifecycle. Clients branch on `code`, so
// every refusal the service can raise has one constant here rather than a
// hand-written string at the raise site.
const (
	CodeExpenseNotEditable     = "FINANCE_EXPENSE_NOT_EDITABLE"
	CodeExpenseNotSubmittable  = "FINANCE_EXPENSE_NOT_SUBMITTABLE"
	CodeExpenseNotDecidable    = "FINANCE_EXPENSE_NOT_DECIDABLE"
	CodeExpenseNotSettleable   = "FINANCE_EXPENSE_NOT_SETTLEABLE"
	CodeExpenseNotReconcilable = "FINANCE_EXPENSE_NOT_RECONCILABLE"
	CodeReceiptRequired        = "FINANCE_RECEIPT_REQUIRED"
	CodeInvoiceNotIssuable     = "FINANCE_INVOICE_NOT_ISSUABLE"
	CodeInvoiceNotEditable     = "FINANCE_INVOICE_NOT_EDITABLE"
	CodeInvoiceNotAllocable    = "FINANCE_INVOICE_NOT_ALLOCABLE"
	CodeBudgetNotEditable      = "FINANCE_BUDGET_NOT_EDITABLE"
	CodeBudgetNotApprovable    = "FINANCE_BUDGET_NOT_APPROVABLE"
	CodeBudgetNotClosable      = "FINANCE_BUDGET_NOT_CLOSABLE"
	CodeReasonRequired         = "FINANCE_REASON_REQUIRED"
	CodePaymentOverAllocated   = "FINANCE_PAYMENT_OVER_ALLOCATED"
)

// codedError overlays a stable problem code on top of a sentinel error. The
// HTTP layer still derives the status from the sentinel; the code is what the
// client keys on.
type codedError struct {
	err  error
	code string
}

func (e *codedError) Error() string { return e.err.Error() }
func (e *codedError) Unwrap() error { return e.err }

// withCode attaches a stable problem code to err. Non-sentinel errors pass
// through untouched so wrapping never hides an unexpected failure.
func withCode(err error, code string) error {
	if err == nil {
		return nil
	}
	return &codedError{err: err, code: code}
}

// problemCode reports the stable problem code for err, or "" when it has none.
func problemCode(err error) string {
	var ce *codedError
	if errors.As(err, &ce) {
		return ce.code
	}
	return ""
}

// BaseCurrency is the currency the finance headline consolidates into. The
// management-finance views report a single figure, so multi-currency
// consolidation through finance.exchange_rates is deferred (§8.4 [DECIDE]) and
// amounts are summed as they stand.
const BaseCurrency = "KES"

// ── enums ───────────────────────────────────────────────────────────────────

// AccountType is the kind of account cash sits in (§8.4).
type AccountType string

const (
	AccountBank         AccountType = "bank"
	AccountCash         AccountType = "cash"
	AccountMpesaTill    AccountType = "mpesa_till"
	AccountMpesaPaybill AccountType = "mpesa_paybill"
	AccountCard         AccountType = "card"
)

func validAccountType(t AccountType) bool {
	switch t {
	case AccountBank, AccountCash, AccountMpesaTill, AccountMpesaPaybill, AccountCard:
		return true
	}
	return false
}

// ExpenseStatus is the expense lifecycle state (§8.4).
type ExpenseStatus string

const (
	ExpenseDraft      ExpenseStatus = "draft"
	ExpenseSubmitted  ExpenseStatus = "submitted"
	ExpenseApproved   ExpenseStatus = "approved"
	ExpenseRejected   ExpenseStatus = "rejected"
	ExpensePaid       ExpenseStatus = "paid"
	ExpenseReimbursed ExpenseStatus = "reimbursed"
	ExpenseReconciled ExpenseStatus = "reconciled"
)

func validExpenseStatus(s ExpenseStatus) bool {
	switch s {
	case ExpenseDraft, ExpenseSubmitted, ExpenseApproved, ExpenseRejected,
		ExpensePaid, ExpenseReimbursed, ExpenseReconciled:
		return true
	}
	return false
}

// An expense has actually moved money only once it is paid, reimbursed or
// reconciled: draft, submitted, approved and rejected claims do not count
// against a budget, a project cost or an account balance. The read queries in
// store.go encode that rule directly.

// InvoiceStatus is the invoice lifecycle state (§8.4). `overdue` is computed by
// Go from the due date, never entered by hand.
type InvoiceStatus string

const (
	InvoiceScheduled     InvoiceStatus = "scheduled"
	InvoiceDraft         InvoiceStatus = "draft"
	InvoiceApproved      InvoiceStatus = "approved"
	InvoiceIssued        InvoiceStatus = "issued"
	InvoicePartiallyPaid InvoiceStatus = "partially_paid"
	InvoicePaid          InvoiceStatus = "paid"
	InvoiceOverdue       InvoiceStatus = "overdue"
	InvoiceDisputed      InvoiceStatus = "disputed"
	InvoiceWrittenOff    InvoiceStatus = "written_off"
)

func validInvoiceStatus(s InvoiceStatus) bool {
	switch s {
	case InvoiceScheduled, InvoiceDraft, InvoiceApproved, InvoiceIssued, InvoicePartiallyPaid,
		InvoicePaid, InvoiceOverdue, InvoiceDisputed, InvoiceWrittenOff:
		return true
	}
	return false
}

// PaymentMethod is how a payment was received (§8.4).
type PaymentMethod string

const (
	PaymentBank  PaymentMethod = "bank"
	PaymentMpesa PaymentMethod = "mpesa"
	PaymentCard  PaymentMethod = "card"
	PaymentCash  PaymentMethod = "cash"
)

func validPaymentMethod(m PaymentMethod) bool {
	switch m {
	case PaymentBank, PaymentMpesa, PaymentCard, PaymentCash:
		return true
	}
	return false
}

// RecurringCycle is how often a recurring cost renews (§8.4).
type RecurringCycle string

const (
	CycleMonthly   RecurringCycle = "monthly"
	CycleQuarterly RecurringCycle = "quarterly"
	CycleAnnual    RecurringCycle = "annual"
)

func validRecurringCycle(c RecurringCycle) bool {
	switch c {
	case CycleMonthly, CycleQuarterly, CycleAnnual:
		return true
	}
	return false
}

// BudgetScope is what a budget is drawn against (§8.4).
type BudgetScope string

const (
	ScopeCompanyPeriod BudgetScope = "company_period"
	ScopeDepartment    BudgetScope = "department"
	ScopeProject       BudgetScope = "project"
	ScopeUnit          BudgetScope = "unit"
)

func validBudgetScope(s BudgetScope) bool {
	switch s {
	case ScopeCompanyPeriod, ScopeDepartment, ScopeProject, ScopeUnit:
		return true
	}
	return false
}

// BudgetStatus is the budget lifecycle state (§8.4).
type BudgetStatus string

const (
	BudgetDraft    BudgetStatus = "draft"
	BudgetApproved BudgetStatus = "approved"
	BudgetClosed   BudgetStatus = "closed"
)

func validBudgetStatus(s BudgetStatus) bool {
	switch s {
	case BudgetDraft, BudgetApproved, BudgetClosed:
		return true
	}
	return false
}

// ── records ─────────────────────────────────────────────────────────────────

// Account is a cash/bank/M-Pesa account (§8.4).
type Account struct {
	ID                  uuid.UUID
	Name                string
	AccountType         AccountType
	Currency            string
	OpeningBalanceMinor *int64
	Active              bool
	Version             int
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

// AccountBalance is an account with its derived current balance (§8.4): the
// opening balance plus payments received minus settled expenses paid from it.
type AccountBalance struct {
	Account
	BalanceMinor int64
}

// ExpenseCategory is a configured spend category (§8.4 [CONFIG]).
type ExpenseCategory struct {
	Key       string
	Label     string
	SortOrder int
	Active    bool
}

// Expense is a claim (§8.4). The decision is one audited step: the approver is
// recorded on the row (decided_by/decided_at/decision_reason) and in the audit
// trail, which is the control that makes self-approval safe (§6.6).
type Expense struct {
	ID               uuid.UUID
	Ref              string
	ClaimantPersonID uuid.UUID
	CategoryKey      string
	ProjectID        *uuid.UUID
	AmountMinor      *int64
	Currency         string
	SpentOn          time.Time
	Vendor           string
	PaymentMethod    *PaymentMethod
	AccountID        *uuid.UUID
	Status           ExpenseStatus
	SubmittedAt      *time.Time
	DecidedBy        *uuid.UUID
	DecidedAt        *time.Time
	DecisionReason   string
	Notes            string
	ReceiptFileID    *uuid.UUID
	TaxInvoiceRef    *string
	PaidOn           *time.Time
	Version          int
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// InvoiceLine is one line of an invoice (§8.4).
type InvoiceLine struct {
	ID             uuid.UUID
	InvoiceID      uuid.UUID
	Description    string
	Quantity       float64
	UnitPriceMinor *int64
	TaxRateBP      int
	AmountMinor    *int64
	SortOrder      int
}

// Invoice is a client invoice (§8.4).
type Invoice struct {
	ID               uuid.UUID
	Ref              string
	OrganizationID   uuid.UUID
	OrganizationName string
	ContractID       *uuid.UUID
	ContractRef      string
	MilestoneID      *uuid.UUID
	IssueDate        *time.Time
	DueDate          *time.Time
	TotalMinor       *int64
	Currency         string
	Status           InvoiceStatus
	IssuedAt         *time.Time
	PaidAt           *time.Time
	EInvoiceRef      string
	Notes            string
	Version          int
	CreatedAt        time.Time
	UpdatedAt        time.Time
	// Derived: allocated payments and what is left outstanding. Outstanding is
	// floored at zero, so an overpayment does not read as negative debt.
	AmountPaidMinor  int64
	OutstandingMinor int64
}

// Payment is money received (§8.4).
type Payment struct {
	ID          uuid.UUID
	Ref         string
	AmountMinor *int64
	Currency    string
	Method      PaymentMethod
	Reference   string
	ReceivedOn  time.Time
	AccountID   *uuid.UUID
	Reconciled  bool
	Notes       string
	Version     int
	CreatedAt   time.Time
	UpdatedAt   time.Time
	// Derived: how much of the payment has been allocated to invoices, and the
	// rest that sits in the reconciliation queue.
	AllocatedMinor   int64
	UnallocatedMinor int64
}

// Budget is a ceiling for a scope and period, with its actuals (§8.4).
type Budget struct {
	ID            uuid.UUID
	Scope         BudgetScope
	ScopeRef      string
	PeriodStart   time.Time
	PeriodEnd     time.Time
	CategoryKey   string
	AmountMinor   *int64
	Currency      string
	Status        BudgetStatus
	OwnerPersonID *uuid.UUID
	Version       int
	CreatedAt     time.Time
	UpdatedAt     time.Time
	// Derived: settled expenses in the period and scope, and the amount left.
	ActualMinor   *int64
	VarianceMinor *int64
}

// RecurringCost is a standing vendor cost (§8.4).
type RecurringCost struct {
	ID            uuid.UUID
	Vendor        string
	Description   string
	AmountMinor   *int64
	Currency      string
	Cycle         RecurringCycle
	NextRenewalOn time.Time
	OwnerPersonID *uuid.UUID
	Active        bool
	Version       int
	CreatedAt     time.Time
	UpdatedAt     time.Time
	// Derived: the cost normalised to a month, so a quarterly and an annual
	// charge can be added to one figure.
	MonthlyEquivalentMinor *int64
}

// ── query filters ───────────────────────────────────────────────────────────

// ExpenseQuery filters an expense listing (§8.4).
type ExpenseQuery struct {
	Status      *ExpenseStatus
	CategoryKey string
	ProjectID   *uuid.UUID
	ClaimantID  *uuid.UUID
	From        *time.Time
	To          *time.Time
}

// InvoiceQuery filters an invoice listing (§8.4). Overdue, when set, keeps only
// invoices past their due date that still have money outstanding.
type InvoiceQuery struct {
	Status         *InvoiceStatus
	OrganizationID *uuid.UUID
	ContractID     *uuid.UUID
	Overdue        bool
}

// PaymentQuery filters a payment listing (§8.4).
type PaymentQuery struct {
	From   *time.Time
	To     *time.Time
	Method *PaymentMethod
}

// BudgetQuery filters a budget listing (§8.4). Period matches the YYYY-MM of a
// budget's start.
type BudgetQuery struct {
	Scope  *BudgetScope
	Status *BudgetStatus
	Period string
}

// ── derived figures ─────────────────────────────────────────────────────────

// Bucket is one receivables ageing band (§8.4).
type Bucket struct {
	Key    string
	Label  string
	Count  int
	Amount int64
}

// AgeingBuckets are the five §8.4 bands, in order. Labels match the metrics
// section (0–30/31–60/61–90/90+); `current` is not yet due.
var AgeingBuckets = []Bucket{
	{Key: "current", Label: "Current"},
	{Key: "1_30", Label: "1-30 days"},
	{Key: "31_60", Label: "31-60 days"},
	{Key: "61_90", Label: "61-90 days"},
	{Key: "90_plus", Label: "90+ days"},
}

// AgeingBucketKey returns the band an invoice falls into from the days it is
// past due: 0 or fewer is current, otherwise the 1-30/31-60/61-90/90+ bands.
func AgeingBucketKey(daysPastDue int) string {
	switch {
	case daysPastDue <= 0:
		return "current"
	case daysPastDue <= 30:
		return "1_30"
	case daysPastDue <= 60:
		return "31_60"
	case daysPastDue <= 90:
		return "61_90"
	default:
		return "90_plus"
	}
}

// OverviewTotals is the finance headline (§8.4): the raw figures the overview
// is assembled from. A nil pointer means "no rows", which the handler renders
// the same as a withheld amount only when the viewer may not see it.
type OverviewTotals struct {
	CashMinor                   int64
	ReceivablesOutstandingMinor int64
	ReceivablesOverdueMinor     int64
	Invoiced90DMinor            int64
	Collected90DMinor           int64
	Expenses30DMinor            int64
	PendingExpenses             int
	OverdueInvoiceCount         int
	UpcomingRenewals30D         int
	ActiveBudgetCount           int
	BudgetsOverThreshold        int
}

// ── helpers ─────────────────────────────────────────────────────────────────

func trim(s string) string { return strings.TrimSpace(s) }

func normalizeCurrency(c string) string {
	c = strings.ToUpper(trim(c))
	if c == "" {
		return BaseCurrency
	}
	return c
}

// validateCurrency accepts a three-letter ISO code in any case. Currency is
// normalised to upper case on the way into a record, so a lower-case code is a
// formatting difference, not a bad value; an omitted code falls back to the
// base currency.
func validateCurrency(c string) error {
	c = normalizeCurrency(c)
	if len(c) != 3 {
		return fmt.Errorf("%w: currency must be a three-letter code", ErrInvalid)
	}
	for _, r := range c {
		if r < 'A' || r > 'Z' {
			return fmt.Errorf("%w: currency must be a three-letter code", ErrInvalid)
		}
	}
	return nil
}

// monthlyEquivalent normalises a recurring amount to a month (§8.4), using
// integer division so the figure stays in minor units: a quarterly charge is a
// third of its amount per month, an annual one a twelfth.
func monthlyEquivalent(amount *int64, cycle RecurringCycle) *int64 {
	if amount == nil {
		return nil
	}
	var months int64
	switch cycle {
	case CycleMonthly:
		months = 1
	case CycleQuarterly:
		months = 3
	case CycleAnnual:
		months = 12
	default:
		return nil
	}
	v := *amount / months
	return &v
}

// ── write inputs (§8.4) ─────────────────────────────────────────────────────

// Entity types carried on audit events and quoted by the audit feed.
const (
	EntityExpense       = "finance.expense"
	EntityInvoice       = "finance.invoice"
	EntityPayment       = "finance.payment"
	EntityBudget        = "finance.budget"
	EntityAccount       = "finance.account"
	EntityRecurringCost = "finance.recurring_cost"
)

// Decision is an approver's answer to a submitted claim. Approval is one
// audited step; a rejection always carries a reason (§8.4).
type Decision string

const (
	DecisionApprove Decision = "approve"
	DecisionReject  Decision = "reject"
)

func validDecision(d Decision) bool {
	return d == DecisionApprove || d == DecisionReject
}

// ExpenseInput is a claim's editable fields. ClaimantPersonID is optional on
// create (it defaults to the actor) and required on update.
type ExpenseInput struct {
	ClaimantPersonID *uuid.UUID
	CategoryKey      string
	ProjectID        *uuid.UUID
	AmountMinor      int64
	Currency         string
	SpentOn          time.Time
	Vendor           string
	PaymentMethod    *PaymentMethod
	AccountID        *uuid.UUID
	ReceiptFileID    *uuid.UUID
	TaxInvoiceRef    string
	Notes            string
}

// ExpenseSettleInput carries a settlement: the target status (paid for money
// that left an account, reimbursed for money owed back to the claimant), the
// account it left from and the day it moved.
type ExpenseSettleInput struct {
	Method    ExpenseStatus
	AccountID *uuid.UUID
	PaidOn    *time.Time
}

// InvoiceLineInput is one line as a client sends it. AmountMinor is derived by
// Go from Quantity × UnitPriceMinor; a client cannot set it.
type InvoiceLineInput struct {
	Description    string
	Quantity       float64
	UnitPriceMinor int64
	TaxRateBP      int
	SortOrder      int
}

// InvoiceInput is an invoice's editable fields and its lines.
type InvoiceInput struct {
	OrganizationID uuid.UUID
	ContractID     *uuid.UUID
	MilestoneID    *uuid.UUID
	IssueDate      *time.Time
	DueDate        *time.Time
	Currency       string
	Notes          string
	Lines          []InvoiceLineInput
}

// InvoiceIssueInput optionally re-dates an invoice at issue. A blank date is
// filled by Go from the contract terms (or today plus the default term).
type InvoiceIssueInput struct {
	IssueDate *time.Time
	DueDate   *time.Time
}

// PaymentAllocationInput says how much of a payment settles which invoice.
type PaymentAllocationInput struct {
	InvoiceID   uuid.UUID
	AmountMinor int64
}

// PaymentInput is money received, optionally allocated in the same write.
type PaymentInput struct {
	AmountMinor int64
	Currency    string
	Method      PaymentMethod
	Reference   string
	ReceivedOn  time.Time
	AccountID   *uuid.UUID
	Notes       string
	Allocations []PaymentAllocationInput
}

// BudgetInput is a ceiling for a scope and period.
type BudgetInput struct {
	Scope         BudgetScope
	ScopeRef      string
	PeriodStart   time.Time
	PeriodEnd     time.Time
	CategoryKey   string
	AmountMinor   int64
	Currency      string
	OwnerPersonID *uuid.UUID
}

// AccountInput is a cash account's editable fields. Active is optional so a
// create defaults to true and an update can leave it untouched.
type AccountInput struct {
	Name                string
	AccountType         AccountType
	Currency            string
	OpeningBalanceMinor *int64
	Active              *bool
}

// RecurringCostInput is a standing vendor cost's editable fields.
type RecurringCostInput struct {
	Vendor        string
	Description   string
	AmountMinor   int64
	Currency      string
	Cycle         RecurringCycle
	NextRenewalOn time.Time
	OwnerPersonID *uuid.UUID
	Active        *bool
}

// ── lifecycle rules (§8.4) ──────────────────────────────────────────────────
//
// Each predicate is the single place a transition is allowed from. The store
// writes the status; the service checks these first, so an illegal move is a
// stable problem code rather than a silent no-op.

func expenseEditable(s ExpenseStatus) bool { return s == ExpenseDraft }

// expenseSubmittable includes rejected: a rejected claim can be corrected and
// sent again without being retyped from scratch.
func expenseSubmittable(s ExpenseStatus) bool {
	return s == ExpenseDraft || s == ExpenseRejected
}

func expenseDecidable(s ExpenseStatus) bool  { return s == ExpenseSubmitted }
func expenseSettleable(s ExpenseStatus) bool { return s == ExpenseApproved }

func expenseReconcilable(s ExpenseStatus) bool {
	return s == ExpensePaid || s == ExpenseReimbursed
}

func invoiceEditable(s InvoiceStatus) bool {
	return s == InvoiceDraft || s == InvoiceScheduled
}

func invoiceApprovable(s InvoiceStatus) bool { return invoiceEditable(s) }
func invoiceIssuable(s InvoiceStatus) bool   { return s == InvoiceApproved }

// invoiceDisputable and invoiceWritableOff cover the open states a client can
// still argue about; a draft has not been sent, so it cannot be disputed.
func invoiceDisputable(s InvoiceStatus) bool {
	return s == InvoiceIssued || s == InvoicePartiallyPaid || s == InvoiceOverdue
}

func invoiceWritableOff(s InvoiceStatus) bool {
	return invoiceDisputable(s) || s == InvoiceDisputed
}

// invoiceAllocable is the set of invoices a payment may be matched against.
// A disputed invoice can still be paid (the dispute may be about part of the
// bill); a written-off one cannot.
func invoiceAllocable(s InvoiceStatus) bool {
	return s == InvoiceIssued || s == InvoicePartiallyPaid || s == InvoiceOverdue || s == InvoiceDisputed
}

func budgetEditable(s BudgetStatus) bool   { return s == BudgetDraft }
func budgetApprovable(s BudgetStatus) bool { return s == BudgetDraft }
func budgetClosable(s BudgetStatus) bool   { return s == BudgetApproved }

// ── money arithmetic ────────────────────────────────────────────────────────

// lineAmountMinor is a line's pre-tax amount: quantity × unit price, rounded
// to the nearest minor unit. Quantity is decimal, the money stays integer.
func lineAmountMinor(quantity float64, unitPriceMinor int64) int64 {
	return int64(math.Round(quantity * float64(unitPriceMinor)))
}

// lineTaxMinor is the tax on an amount at a basis-point rate (1600 = 16%).
func lineTaxMinor(amountMinor int64, taxRateBP int) int64 {
	return int64(math.Round(float64(amountMinor) * float64(taxRateBP) / 10000))
}

// invoiceLinesTotal is what the client owes: the pre-tax lines plus each
// line's tax. It is computed here, never trusted from the request.
func invoiceLinesTotal(lines []InvoiceLineInput) int64 {
	var total int64
	for _, l := range lines {
		amount := lineAmountMinor(l.Quantity, l.UnitPriceMinor)
		total += amount + lineTaxMinor(amount, l.TaxRateBP)
	}
	return total
}

// defaultPaymentTermDays is the fallback due-date term when an invoice is
// issued without one and its contract names no term.
const defaultPaymentTermDays = 30
