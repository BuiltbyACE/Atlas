// Package finance owns Phase 4 finance and commercial control: the cash
// position, receivables, project profitability, budgets and spending
// (docs/ATLAS.md §8.4).
//
// This pass is read-only. It answers the questions the finance screens ask —
// what cash is on hand, what clients owe and how overdue it is, what a project
// really earns, what is budgeted against actuals, and which standing costs are
// about to renew. The write lifecycle (expenses.submit/decide, invoices.issue,
// payments.record) is deliberately out of scope; the storage it needs already
// exists (migration 00022).
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
	"strings"
	"time"

	"github.com/google/uuid"
)

// Sentinel errors. The HTTP layer maps these onto problem responses.
var (
	ErrNotFound = errors.New("finance: not found")
	ErrInvalid  = errors.New("finance: invalid")
)

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

// Expense is a claim (§8.4).
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

func validateCurrency(c string) error {
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
