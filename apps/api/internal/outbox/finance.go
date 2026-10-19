package outbox

import (
	"time"

	"github.com/google/uuid"
)

// Finance event types (§7.4, §8.4). The write lifecycle that emits these —
// submitting and deciding an expense, issuing an invoice, recording a payment,
// allocating a budget — lands in a later pass; the types are declared now so the
// topics and their schemas exist alongside the rest of the catalogue, and the
// read slice can document what it will publish.
const (
	TypeExpenseSubmittedV1       = "finance.expense.submitted.v1"
	TypeExpenseApprovedV1        = "finance.expense.approved.v1"
	TypeExpenseRejectedV1        = "finance.expense.rejected.v1"
	TypeInvoiceIssuedV1          = "finance.invoice.issued.v1"
	TypeInvoiceOverdueV1         = "finance.invoice.overdue.v1"
	TypePaymentReceivedV1        = "finance.payment.received.v1"
	TypeBudgetAllocatedV1        = "finance.budget.allocated.v1"
	TypeBudgetThresholdCrossedV1 = "finance.budget.threshold_crossed.v1"
)

// Aggregate types for finance events.
const (
	AggregateExpense = "expense"
	AggregateInvoice = "invoice"
	AggregatePayment = "payment"
	AggregateBudget  = "budget"
)

// ExpensePayload is emitted as an expense moves through its lifecycle (§8.4).
type ExpensePayload struct {
	ExpenseID        uuid.UUID  `json:"expense_id"`
	Ref              string     `json:"ref"`
	ClaimantPersonID uuid.UUID  `json:"claimant_person_id"`
	CategoryKey      string     `json:"category_key"`
	ProjectID        *uuid.UUID `json:"project_id,omitempty"`
	AmountMinor      *int64     `json:"amount_minor,omitempty"`
	Currency         string     `json:"currency"`
	Status           string     `json:"status"`
	// Reason is set on a rejection, so the claimant's notification can say why.
	Reason     string     `json:"reason,omitempty"`
	ActorID    *uuid.UUID `json:"actor_id,omitempty"`
	OccurredAt time.Time  `json:"occurred_at"`
}

// InvoicePayload is emitted when an invoice is issued or turns overdue (§8.4).
type InvoicePayload struct {
	InvoiceID      uuid.UUID  `json:"invoice_id"`
	Ref            string     `json:"ref"`
	OrganizationID uuid.UUID  `json:"organization_id"`
	ContractID     *uuid.UUID `json:"contract_id,omitempty"`
	TotalMinor     *int64     `json:"total_minor,omitempty"`
	Currency       string     `json:"currency"`
	Status         string     `json:"status"`
	DueDate        string     `json:"due_date,omitempty"`
	ActorID        *uuid.UUID `json:"actor_id,omitempty"`
	OccurredAt     time.Time  `json:"occurred_at"`
}

// PaymentPayload is emitted when money is received (§8.4).
type PaymentPayload struct {
	PaymentID   uuid.UUID  `json:"payment_id"`
	Ref         string     `json:"ref"`
	AmountMinor *int64     `json:"amount_minor,omitempty"`
	Currency    string     `json:"currency"`
	Method      string     `json:"method"`
	AccountID   *uuid.UUID `json:"account_id,omitempty"`
	ActorID     *uuid.UUID `json:"actor_id,omitempty"`
	OccurredAt  time.Time  `json:"occurred_at"`
}

// BudgetPayload is emitted when a budget is approved or allocated (§8.4).
type BudgetPayload struct {
	BudgetID    uuid.UUID  `json:"budget_id"`
	Scope       string     `json:"scope"`
	ScopeRef    string     `json:"scope_ref,omitempty"`
	PeriodStart string     `json:"period_start"`
	PeriodEnd   string     `json:"period_end"`
	CategoryKey string     `json:"category_key,omitempty"`
	AmountMinor *int64     `json:"amount_minor,omitempty"`
	Currency    string     `json:"currency"`
	Status      string     `json:"status"`
	ActorID     *uuid.UUID `json:"actor_id,omitempty"`
	OccurredAt  time.Time  `json:"occurred_at"`
}

// BudgetThresholdPayload is emitted when a budget's actual spend crosses the
// review threshold, so a budget owner is warned before the ceiling is breached
// (§8.4).
type BudgetThresholdPayload struct {
	BudgetID    uuid.UUID `json:"budget_id"`
	Scope       string    `json:"scope"`
	ScopeRef    string    `json:"scope_ref,omitempty"`
	AmountMinor *int64    `json:"amount_minor,omitempty"`
	ActualMinor *int64    `json:"actual_minor,omitempty"`
	Currency    string    `json:"currency"`
	OccurredAt  time.Time `json:"occurred_at"`
}

// ExpenseRef is the minimal expense view the builders need.
type ExpenseRef struct {
	ID               uuid.UUID
	Ref              string
	ClaimantPersonID uuid.UUID
	CategoryKey      string
	ProjectID        *uuid.UUID
	AmountMinor      *int64
	Currency         string
	Status           string
}

// InvoiceRef is the minimal invoice view the builders need.
type InvoiceRef struct {
	ID             uuid.UUID
	Ref            string
	OrganizationID uuid.UUID
	ContractID     *uuid.UUID
	TotalMinor     *int64
	Currency       string
	Status         string
	DueDate        string
}

// PaymentRef is the minimal payment view the builders need.
type PaymentRef struct {
	ID          uuid.UUID
	Ref         string
	AmountMinor *int64
	Currency    string
	Method      string
	AccountID   *uuid.UUID
}

// BudgetRef is the minimal budget view the builders need.
type BudgetRef struct {
	ID          uuid.UUID
	Scope       string
	ScopeRef    string
	PeriodStart string
	PeriodEnd   string
	CategoryKey string
	AmountMinor *int64
	Currency    string
	Status      string
}

// NewExpenseSubmitted builds a finance.expense.submitted.v1 event.
func NewExpenseSubmitted(actorID *uuid.UUID, e ExpenseRef) (Event, error) {
	return buildEvent(TypeExpenseSubmittedV1, AggregateExpense, e.ID, actorID, nil, ExpensePayload{
		ExpenseID: e.ID, Ref: e.Ref, ClaimantPersonID: e.ClaimantPersonID, CategoryKey: e.CategoryKey,
		ProjectID: e.ProjectID, AmountMinor: e.AmountMinor, Currency: e.Currency, Status: e.Status,
		ActorID: actorID, OccurredAt: time.Now().UTC(),
	})
}

// NewExpenseApproved builds a finance.expense.approved.v1 event.
func NewExpenseApproved(actorID *uuid.UUID, e ExpenseRef) (Event, error) {
	return buildEvent(TypeExpenseApprovedV1, AggregateExpense, e.ID, actorID, nil, ExpensePayload{
		ExpenseID: e.ID, Ref: e.Ref, ClaimantPersonID: e.ClaimantPersonID, CategoryKey: e.CategoryKey,
		ProjectID: e.ProjectID, AmountMinor: e.AmountMinor, Currency: e.Currency, Status: e.Status,
		ActorID: actorID, OccurredAt: time.Now().UTC(),
	})
}

// NewExpenseRejected builds a finance.expense.rejected.v1 event. The reason is
// carried so the claimant's notification can explain the decision.
func NewExpenseRejected(actorID *uuid.UUID, e ExpenseRef, reason string) (Event, error) {
	return buildEvent(TypeExpenseRejectedV1, AggregateExpense, e.ID, actorID, nil, ExpensePayload{
		ExpenseID: e.ID, Ref: e.Ref, ClaimantPersonID: e.ClaimantPersonID, CategoryKey: e.CategoryKey,
		ProjectID: e.ProjectID, AmountMinor: e.AmountMinor, Currency: e.Currency, Status: e.Status,
		Reason: reason, ActorID: actorID, OccurredAt: time.Now().UTC(),
	})
}

// NewInvoiceIssued builds a finance.invoice.issued.v1 event.
func NewInvoiceIssued(actorID *uuid.UUID, inv InvoiceRef) (Event, error) {
	return buildEvent(TypeInvoiceIssuedV1, AggregateInvoice, inv.ID, actorID, nil, InvoicePayload{
		InvoiceID: inv.ID, Ref: inv.Ref, OrganizationID: inv.OrganizationID, ContractID: inv.ContractID,
		TotalMinor: inv.TotalMinor, Currency: inv.Currency, Status: inv.Status, DueDate: inv.DueDate,
		ActorID: actorID, OccurredAt: time.Now().UTC(),
	})
}

// NewInvoiceOverdue builds a finance.invoice.overdue.v1 event.
func NewInvoiceOverdue(inv InvoiceRef) (Event, error) {
	return buildEvent(TypeInvoiceOverdueV1, AggregateInvoice, inv.ID, nil, nil, InvoicePayload{
		InvoiceID: inv.ID, Ref: inv.Ref, OrganizationID: inv.OrganizationID, ContractID: inv.ContractID,
		TotalMinor: inv.TotalMinor, Currency: inv.Currency, Status: inv.Status, DueDate: inv.DueDate,
		OccurredAt: time.Now().UTC(),
	})
}

// NewPaymentReceived builds a finance.payment.received.v1 event.
func NewPaymentReceived(actorID *uuid.UUID, p PaymentRef) (Event, error) {
	return buildEvent(TypePaymentReceivedV1, AggregatePayment, p.ID, actorID, nil, PaymentPayload{
		PaymentID: p.ID, Ref: p.Ref, AmountMinor: p.AmountMinor, Currency: p.Currency,
		Method: p.Method, AccountID: p.AccountID, ActorID: actorID, OccurredAt: time.Now().UTC(),
	})
}

// NewBudgetAllocated builds a finance.budget.allocated.v1 event.
func NewBudgetAllocated(actorID *uuid.UUID, b BudgetRef) (Event, error) {
	return buildEvent(TypeBudgetAllocatedV1, AggregateBudget, b.ID, actorID, nil, BudgetPayload{
		BudgetID: b.ID, Scope: b.Scope, ScopeRef: b.ScopeRef, PeriodStart: b.PeriodStart,
		PeriodEnd: b.PeriodEnd, CategoryKey: b.CategoryKey, AmountMinor: b.AmountMinor,
		Currency: b.Currency, Status: b.Status, ActorID: actorID, OccurredAt: time.Now().UTC(),
	})
}

// NewBudgetThresholdCrossed builds a finance.budget.threshold_crossed.v1 event.
func NewBudgetThresholdCrossed(b BudgetRef, actualMinor int64) (Event, error) {
	actual := actualMinor
	return buildEvent(TypeBudgetThresholdCrossedV1, AggregateBudget, b.ID, nil, nil, BudgetThresholdPayload{
		BudgetID: b.ID, Scope: b.Scope, ScopeRef: b.ScopeRef, AmountMinor: b.AmountMinor,
		ActualMinor: &actual, Currency: b.Currency, OccurredAt: time.Now().UTC(),
	})
}
