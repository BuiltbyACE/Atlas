package finance

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/safaristack/atlas/apps/api/internal/authz"
	"github.com/safaristack/atlas/apps/api/internal/outbox"
)

// The handler tests for the write lifecycle. fakeWriter embeds the read-only
// fakeStore and adds the Writer methods, mutating one record so a handler that
// reads after a write sees the change. It is what pins the wire contract: the
// status codes, the ETag, the If-Match rule and the stable problem codes.
//
// The record fields (expense, invoice, budget) live on the embedded fakeStore so
// a read after a write is consistent; only the payment needs its own pointer
// because the read store keeps a list, not a single payment.

type fakeWriter struct {
	fakeStore
	categoryActive bool
	conflict       bool
	payment        *Payment
}

func (w *fakeWriter) currentExpense() Expense { return *w.fakeStore.expense }

func (w *fakeWriter) CreateExpense(_ context.Context, e Expense) (Expense, error) {
	if w.conflict {
		return Expense{}, ErrConflict
	}
	e.Ref, e.Version = "EXP-TEST-1", 1
	e.CreatedAt, e.UpdatedAt = time.Now().UTC(), time.Now().UTC()
	w.fakeStore.expense = &e
	return e, nil
}

func (w *fakeWriter) UpdateExpense(_ context.Context, e Expense, _ int) (Expense, error) {
	if w.conflict {
		return Expense{}, ErrConflict
	}
	e.Version++
	w.fakeStore.expense = &e
	return e, nil
}

func (w *fakeWriter) SubmitExpense(_ context.Context, _ uuid.UUID, _ int, submittedAt time.Time, _ outbox.Event) (Expense, error) {
	if w.conflict {
		return Expense{}, ErrConflict
	}
	e := w.currentExpense()
	e.Status, e.SubmittedAt, e.Version = ExpenseSubmitted, &submittedAt, e.Version+1
	w.fakeStore.expense = &e
	return e, nil
}

func (w *fakeWriter) DecideExpense(_ context.Context, _ uuid.UUID, _ int, status ExpenseStatus, by uuid.UUID, at time.Time, reason string, _ outbox.Event) (Expense, error) {
	if w.conflict {
		return Expense{}, ErrConflict
	}
	e := w.currentExpense()
	e.Status, e.DecidedBy, e.DecidedAt, e.DecisionReason = status, &by, &at, reason
	e.Version++
	w.fakeStore.expense = &e
	return e, nil
}

func (w *fakeWriter) SettleExpense(_ context.Context, _ uuid.UUID, _ int, status ExpenseStatus, accountID *uuid.UUID, paidOn *time.Time) (Expense, error) {
	if w.conflict {
		return Expense{}, ErrConflict
	}
	e := w.currentExpense()
	e.Status, e.AccountID, e.PaidOn = status, accountID, paidOn
	e.Version++
	w.fakeStore.expense = &e
	return e, nil
}

func (w *fakeWriter) ReconcileExpense(_ context.Context, _ uuid.UUID, _ int) (Expense, error) {
	if w.conflict {
		return Expense{}, ErrConflict
	}
	e := w.currentExpense()
	e.Status, e.Version = ExpenseReconciled, e.Version+1
	w.fakeStore.expense = &e
	return e, nil
}

func (w *fakeWriter) ExpenseCategoryActive(context.Context, string) (bool, error) {
	return w.categoryActive, nil
}

func (w *fakeWriter) CreateInvoice(_ context.Context, inv Invoice, lines []InvoiceLine) (Invoice, error) {
	if w.conflict {
		return Invoice{}, ErrConflict
	}
	inv.Ref, inv.Version = "INV-TEST-1", 1
	inv.CreatedAt, inv.UpdatedAt = time.Now().UTC(), time.Now().UTC()
	w.fakeStore.invoice, w.fakeStore.lines = &inv, lines
	return inv, nil
}

func (w *fakeWriter) UpdateInvoice(_ context.Context, inv Invoice, lines []InvoiceLine, _ int) (Invoice, error) {
	if w.conflict {
		return Invoice{}, ErrConflict
	}
	inv.Version++
	w.fakeStore.invoice, w.fakeStore.lines = &inv, lines
	return inv, nil
}

func (w *fakeWriter) ApproveInvoice(_ context.Context, _ uuid.UUID, _ int) (Invoice, error) {
	if w.conflict {
		return Invoice{}, ErrConflict
	}
	w.fakeStore.invoice.Status, w.fakeStore.invoice.Version = InvoiceApproved, w.fakeStore.invoice.Version+1
	return *w.fakeStore.invoice, nil
}

func (w *fakeWriter) IssueInvoice(_ context.Context, _ uuid.UUID, _ int, issueDate, dueDate, issuedAt time.Time, _ outbox.Event) (Invoice, error) {
	if w.conflict {
		return Invoice{}, ErrConflict
	}
	inv := w.fakeStore.invoice
	inv.Status, inv.IssuedAt, inv.IssueDate, inv.DueDate = InvoiceIssued, &issuedAt, &issueDate, &dueDate
	inv.Version++
	return *inv, nil
}

func (w *fakeWriter) DisputeInvoice(_ context.Context, _ uuid.UUID, _ int) (Invoice, error) {
	if w.conflict {
		return Invoice{}, ErrConflict
	}
	w.fakeStore.invoice.Status, w.fakeStore.invoice.Version = InvoiceDisputed, w.fakeStore.invoice.Version+1
	return *w.fakeStore.invoice, nil
}

func (w *fakeWriter) WriteOffInvoice(_ context.Context, _ uuid.UUID, _ int) (Invoice, error) {
	if w.conflict {
		return Invoice{}, ErrConflict
	}
	w.fakeStore.invoice.Status, w.fakeStore.invoice.Version = InvoiceWrittenOff, w.fakeStore.invoice.Version+1
	return *w.fakeStore.invoice, nil
}

func (w *fakeWriter) ContractPaymentTermDays(context.Context, uuid.UUID) (int, bool, error) {
	return 0, false, nil
}

func (w *fakeWriter) GetPayment(context.Context, uuid.UUID) (*Payment, error) { return w.payment, nil }

func (w *fakeWriter) CreatePayment(_ context.Context, p Payment, allocations []PaymentAllocationInput, _ func(string) (outbox.Event, error)) (Payment, error) {
	if w.conflict {
		return Payment{}, ErrConflict
	}
	p.Ref, p.Version = "PAY-TEST-1", 1
	for _, a := range allocations {
		p.AllocatedMinor += a.AmountMinor
	}
	p.UnallocatedMinor = *p.AmountMinor - p.AllocatedMinor
	w.payment = &p
	return p, nil
}

func (w *fakeWriter) AllocatePayment(_ context.Context, _ uuid.UUID, _ int, allocations []PaymentAllocationInput) (Payment, error) {
	if w.conflict {
		return Payment{}, ErrConflict
	}
	for _, a := range allocations {
		w.payment.AllocatedMinor += a.AmountMinor
	}
	w.payment.UnallocatedMinor = *w.payment.AmountMinor - w.payment.AllocatedMinor
	w.payment.Version++
	return *w.payment, nil
}

func (w *fakeWriter) ReconcilePayment(_ context.Context, _ uuid.UUID, _ int, reconciled bool) (Payment, error) {
	if w.conflict {
		return Payment{}, ErrConflict
	}
	w.payment.Reconciled, w.payment.Version = reconciled, w.payment.Version+1
	return *w.payment, nil
}

func (w *fakeWriter) CreateBudget(_ context.Context, b Budget) (Budget, error) {
	if w.conflict {
		return Budget{}, ErrConflict
	}
	b.Version = 1
	w.fakeStore.budget = &b
	return b, nil
}

func (w *fakeWriter) UpdateBudget(_ context.Context, b Budget, _ int) (Budget, error) {
	if w.conflict {
		return Budget{}, ErrConflict
	}
	b.Version++
	w.fakeStore.budget = &b
	return b, nil
}

func (w *fakeWriter) ApproveBudget(_ context.Context, _ uuid.UUID, _ int, _ outbox.Event) (Budget, error) {
	if w.conflict {
		return Budget{}, ErrConflict
	}
	w.fakeStore.budget.Status, w.fakeStore.budget.Version = BudgetApproved, w.fakeStore.budget.Version+1
	return *w.fakeStore.budget, nil
}

func (w *fakeWriter) CloseBudget(_ context.Context, _ uuid.UUID, _ int) (Budget, error) {
	if w.conflict {
		return Budget{}, ErrConflict
	}
	w.fakeStore.budget.Status, w.fakeStore.budget.Version = BudgetClosed, w.fakeStore.budget.Version+1
	return *w.fakeStore.budget, nil
}

func (w *fakeWriter) GetAccount(context.Context, uuid.UUID) (*AccountBalance, error) { return nil, nil }

func (w *fakeWriter) CreateAccount(_ context.Context, a Account) (AccountBalance, error) {
	if w.conflict {
		return AccountBalance{}, ErrConflict
	}
	a.Version = 1
	return AccountBalance{Account: a}, nil
}

func (w *fakeWriter) UpdateAccount(_ context.Context, a Account, _ int) (AccountBalance, error) {
	if w.conflict {
		return AccountBalance{}, ErrConflict
	}
	a.Version++
	return AccountBalance{Account: a}, nil
}

func (w *fakeWriter) GetRecurringCost(context.Context, uuid.UUID) (*RecurringCost, error) {
	return nil, nil
}

func (w *fakeWriter) CreateRecurringCost(_ context.Context, c RecurringCost) (RecurringCost, error) {
	if w.conflict {
		return RecurringCost{}, ErrConflict
	}
	c.Version = 1
	return c, nil
}

func (w *fakeWriter) UpdateRecurringCost(_ context.Context, c RecurringCost, _ int) (RecurringCost, error) {
	if w.conflict {
		return RecurringCost{}, ErrConflict
	}
	c.Version++
	return c, nil
}

// financeWriteRouter wires the write routes the same way the real router does.
func financeWriteRouter(h *Handler) http.Handler {
	r := financeRouter(h)
	r.Post("/api/v1/finance/expenses", h.CreateExpense)
	r.Put("/api/v1/finance/expenses/{id}", h.UpdateExpense)
	r.Post("/api/v1/finance/expenses/{id}/submit", h.SubmitExpense)
	r.Post("/api/v1/finance/expenses/{id}/decide", h.DecideExpense)
	r.Post("/api/v1/finance/expenses/{id}/settle", h.SettleExpense)
	r.Post("/api/v1/finance/expenses/{id}/reconcile", h.ReconcileExpense)
	r.Post("/api/v1/finance/invoices", h.CreateInvoice)
	r.Post("/api/v1/finance/invoices/{id}/issue", h.IssueInvoice)
	r.Post("/api/v1/finance/invoices/{id}/write-off", h.WriteOffInvoice)
	r.Post("/api/v1/finance/payments", h.CreatePayment)
	r.Post("/api/v1/finance/payments/{id}/reconcile", h.ReconcilePayment)
	r.Post("/api/v1/finance/budgets", h.CreateBudget)
	r.Post("/api/v1/finance/budgets/{id}/approve", h.ApproveBudget)
	r.Post("/api/v1/finance/accounts", h.CreateAccount)
	return r
}

func jsonRequest(method, target, body string, as *authz.Principal) *http.Request {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if as != nil {
		req = req.WithContext(authz.WithPrincipal(req.Context(), as))
	}
	return req
}

func serve(t *testing.T, h *Handler, req *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	financeWriteRouter(h).ServeHTTP(rec, req)
	return rec
}

func TestCreateExpenseReturns201AndETag(t *testing.T) {
	w := &fakeWriter{categoryActive: true}
	h := NewHandler(NewService(w, nil), nil)

	rec := serve(t, h, jsonRequest(http.MethodPost, "/api/v1/finance/expenses",
		`{"category_key":"travel","amount_minor":50000,"currency":"kes","spent_on":"2026-10-01","vendor":"Acme"}`,
		financeUser("finance.expense.write")))

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 (%s)", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("ETag"); got != `"1"` {
		t.Errorf("ETag = %q, want \"1\"", got)
	}
	body := decode(t, rec)
	if body["status"] != "draft" || body["currency"] != "KES" {
		t.Errorf("body = %v", body)
	}
}

func TestCreateExpenseMissingAmountIs400(t *testing.T) {
	w := &fakeWriter{categoryActive: true}
	h := NewHandler(NewService(w, nil), nil)

	rec := serve(t, h, jsonRequest(http.MethodPost, "/api/v1/finance/expenses",
		`{"category_key":"travel","amount_minor":0,"spent_on":"2026-10-01"}`,
		financeUser("finance.expense.write")))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	if code := decode(t, rec)["code"]; code != "VALIDATION_FAILED" {
		t.Errorf("code = %v, want VALIDATION_FAILED", code)
	}
}

func TestCreateExpenseReceiptRequiredIs400WithCode(t *testing.T) {
	w := &fakeWriter{categoryActive: true}
	h := NewHandler(NewService(w, nil), nil)

	rec := serve(t, h, jsonRequest(http.MethodPost, "/api/v1/finance/expenses",
		`{"category_key":"travel","amount_minor":500000,"currency":"KES","spent_on":"2026-10-01"}`,
		financeUser("finance.expense.write")))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (%s)", rec.Code, rec.Body.String())
	}
	if code := decode(t, rec)["code"]; code != CodeReceiptRequired {
		t.Errorf("code = %v, want %s", code, CodeReceiptRequired)
	}
}

func TestUpdateExpenseRequiresIfMatch(t *testing.T) {
	e := Expense{ID: uuid.New(), Status: ExpenseDraft, Version: 3, ClaimantPersonID: uuid.New()}
	w := &fakeWriter{fakeStore: fakeStore{expense: &e}, categoryActive: true}
	h := NewHandler(NewService(w, nil), nil)

	body := `{"category_key":"travel","amount_minor":50000,"currency":"KES","spent_on":"2026-10-01"}`
	target := "/api/v1/finance/expenses/" + e.ID.String()

	// No If-Match → 428.
	rec := serve(t, h, jsonRequest(http.MethodPut, target, body, financeUser("finance.expense.write")))
	if rec.Code != http.StatusPreconditionRequired {
		t.Fatalf("without If-Match: status = %d, want 428", rec.Code)
	}

	// Stale If-Match → 412.
	req := jsonRequest(http.MethodPut, target, body, financeUser("finance.expense.write"))
	req.Header.Set("If-Match", `"1"`)
	if rec := serve(t, h, req); rec.Code != http.StatusPreconditionFailed {
		t.Fatalf("stale If-Match: status = %d, want 412", rec.Code)
	}

	// A raced write under a matching If-Match → 412 from the store.
	w.conflict = true
	req = jsonRequest(http.MethodPut, target, body, financeUser("finance.expense.write"))
	req.Header.Set("If-Match", `"3"`)
	if rec := serve(t, h, req); rec.Code != http.StatusPreconditionFailed {
		t.Fatalf("raced write: status = %d, want 412", rec.Code)
	}
}

func TestExpenseActionUnknownIs404(t *testing.T) {
	w := &fakeWriter{} // GetExpense returns nil → ErrNotFound
	h := NewHandler(NewService(w, nil), nil)

	rec := serve(t, h, authRequest(http.MethodPost,
		"/api/v1/finance/expenses/"+uuid.NewString()+"/submit", financeUser("finance.expense.write")))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestSubmitExpenseRefusedFromWrongState(t *testing.T) {
	e := Expense{ID: uuid.New(), Status: ExpenseApproved, Version: 2, AmountMinor: ptr(int64(50000))}
	w := &fakeWriter{fakeStore: fakeStore{expense: &e}}
	h := NewHandler(NewService(w, nil), nil)

	req := authRequest(http.MethodPost, "/api/v1/finance/expenses/"+e.ID.String()+"/submit",
		financeUser("finance.expense.write"))
	req.Header.Set("If-Match", `"2"`)
	rec := serve(t, h, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	if code := decode(t, rec)["code"]; code != CodeExpenseNotSubmittable {
		t.Errorf("code = %v, want %s", code, CodeExpenseNotSubmittable)
	}
}

func TestDecideRejectWithoutReasonIs400(t *testing.T) {
	e := Expense{ID: uuid.New(), Status: ExpenseSubmitted, Version: 2, AmountMinor: ptr(int64(50000))}
	w := &fakeWriter{fakeStore: fakeStore{expense: &e}}
	h := NewHandler(NewService(w, nil), nil)

	req := jsonRequest(http.MethodPost, "/api/v1/finance/expenses/"+e.ID.String()+"/decide",
		`{"decision":"reject"}`, financeUser("finance.expense.approve"))
	req.Header.Set("If-Match", `"2"`)
	rec := serve(t, h, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (%s)", rec.Code, rec.Body.String())
	}
	if code := decode(t, rec)["code"]; code != CodeReasonRequired {
		t.Errorf("code = %v, want %s", code, CodeReasonRequired)
	}
}

func TestCreateInvoiceComputesTotal(t *testing.T) {
	w := &fakeWriter{}
	h := NewHandler(NewService(w, nil), nil)

	rec := serve(t, h, jsonRequest(http.MethodPost, "/api/v1/finance/invoices",
		`{"organization_id":"`+uuid.NewString()+`","currency":"KES","lines":[
			{"description":"Build","quantity":2,"unit_price_minor":50000,"tax_rate_bp":1600}]}`,
		financeUser("finance.invoice.write", "finance.amount.sensitive.read")))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 (%s)", rec.Code, rec.Body.String())
	}
	body := decode(t, rec)
	// 100000 + 16000 = 116000, computed by Go from the line.
	if got, ok := body["total_minor"].(float64); !ok || int64(got) != 116000 {
		t.Errorf("total_minor = %v, want 116000", body["total_minor"])
	}
}

func TestCreateInvoiceWithoutLinesIs400(t *testing.T) {
	h := NewHandler(NewService(&fakeWriter{}, nil), nil)

	rec := serve(t, h, jsonRequest(http.MethodPost, "/api/v1/finance/invoices",
		`{"organization_id":"`+uuid.NewString()+`","currency":"KES","lines":[]}`,
		financeUser("finance.invoice.write")))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestWriteOffWithoutReasonIs400(t *testing.T) {
	inv := Invoice{ID: uuid.New(), Status: InvoiceIssued, Version: 2, Currency: "KES"}
	w := &fakeWriter{fakeStore: fakeStore{invoice: &inv}}
	h := NewHandler(NewService(w, nil), nil)

	req := jsonRequest(http.MethodPost, "/api/v1/finance/invoices/"+inv.ID.String()+"/write-off",
		`{}`, financeUser("finance.invoice.issue"))
	req.Header.Set("If-Match", `"2"`)
	rec := serve(t, h, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (%s)", rec.Code, rec.Body.String())
	}
	if code := decode(t, rec)["code"]; code != CodeReasonRequired {
		t.Errorf("code = %v, want %s", code, CodeReasonRequired)
	}
}

func TestCreatePaymentRequiresMethod(t *testing.T) {
	h := NewHandler(NewService(&fakeWriter{}, nil), nil)

	rec := serve(t, h, jsonRequest(http.MethodPost, "/api/v1/finance/payments",
		`{"amount_minor":1000,"currency":"KES","method":"crypto","received_on":"2026-10-01"}`,
		financeUser("finance.payment.record")))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestReconcilePaymentRequiresReconciledFlag(t *testing.T) {
	p := Payment{ID: uuid.New(), Version: 1}
	w := &fakeWriter{payment: &p}
	h := NewHandler(NewService(w, nil), nil)

	req := jsonRequest(http.MethodPost, "/api/v1/finance/payments/"+p.ID.String()+"/reconcile", `{}`,
		financeUser("finance.payment.record"))
	req.Header.Set("If-Match", `"1"`)
	rec := serve(t, h, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (%s)", rec.Code, rec.Body.String())
	}
}

func ptr[T any](v T) *T { return &v }
