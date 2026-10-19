package finance

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/safaristack/atlas/apps/api/internal/authz"
)

// fakeStore is an in-memory Store for handler tests. Every method returns the
// field it was given, so tests state only the rows they care about.
type fakeStore struct {
	totals      OverviewTotals
	accounts    []AccountBalance
	inflow      int64
	outflow     int64
	receivables []Invoice
	recurring   []RecurringCost

	projectName string
	projectOK   bool
	invoiced    int64
	collected   int64
	costs       int64

	categories []ExpenseCategory
	expenses   []Expense
	expense    *Expense

	invoices []Invoice
	invoice  *Invoice
	lines    []InvoiceLine

	payments []Payment
	budgets  []Budget
	budget   *Budget
}

func (f *fakeStore) OverviewTotals(context.Context, time.Time) (OverviewTotals, error) {
	return f.totals, nil
}
func (f *fakeStore) ListAccounts(context.Context) ([]AccountBalance, error) { return f.accounts, nil }
func (f *fakeStore) CashFlow(context.Context, time.Time) (int64, int64, error) {
	return f.inflow, f.outflow, nil
}
func (f *fakeStore) OpenReceivables(context.Context, time.Time) ([]Invoice, error) {
	return f.receivables, nil
}
func (f *fakeStore) ListRecurringCosts(context.Context) ([]RecurringCost, error) {
	return f.recurring, nil
}
func (f *fakeStore) ProjectName(context.Context, uuid.UUID) (string, bool, error) {
	return f.projectName, f.projectOK, nil
}
func (f *fakeStore) ProjectRevenue(context.Context, uuid.UUID) (int64, int64, error) {
	return f.invoiced, f.collected, nil
}
func (f *fakeStore) ProjectCosts(context.Context, uuid.UUID) (int64, error) { return f.costs, nil }
func (f *fakeStore) ListExpenseCategories(context.Context) ([]ExpenseCategory, error) {
	return f.categories, nil
}
func (f *fakeStore) ListExpenses(context.Context, ExpenseQuery, int, *Cursor) ([]Expense, error) {
	return f.expenses, nil
}
func (f *fakeStore) GetExpense(context.Context, uuid.UUID) (*Expense, error) { return f.expense, nil }
func (f *fakeStore) ListInvoices(context.Context, InvoiceQuery, time.Time, int, *Cursor) ([]Invoice, error) {
	return f.invoices, nil
}
func (f *fakeStore) GetInvoice(context.Context, uuid.UUID) (*Invoice, error) { return f.invoice, nil }
func (f *fakeStore) ListInvoiceLines(context.Context, uuid.UUID) ([]InvoiceLine, error) {
	return f.lines, nil
}
func (f *fakeStore) ListPayments(context.Context, PaymentQuery, int, *Cursor) ([]Payment, error) {
	return f.payments, nil
}
func (f *fakeStore) ListBudgets(context.Context, BudgetQuery, int, *Cursor) ([]Budget, error) {
	return f.budgets, nil
}
func (f *fakeStore) GetBudget(context.Context, uuid.UUID) (*Budget, error) { return f.budget, nil }

// ── request plumbing ────────────────────────────────────────────────────────

func financeRouter(h *Handler) http.Handler {
	r := chi.NewRouter()
	r.Get("/api/v1/finance/overview", h.Overview)
	r.Get("/api/v1/finance/accounts", h.ListAccounts)
	r.Get("/api/v1/finance/cash-position", h.CashPosition)
	r.Get("/api/v1/finance/receivables", h.Receivables)
	r.Get("/api/v1/finance/recurring-costs", h.RecurringCosts)
	r.Get("/api/v1/finance/projects/{id}/profitability", h.ProjectProfitability)
	r.Get("/api/v1/finance/expense-categories", h.ListExpenseCategories)
	r.Get("/api/v1/finance/expenses", h.ListExpenses)
	r.Get("/api/v1/finance/expenses/{id}", h.GetExpense)
	r.Get("/api/v1/finance/invoices", h.ListInvoices)
	r.Get("/api/v1/finance/invoices/{id}", h.GetInvoice)
	r.Get("/api/v1/finance/payments", h.ListPayments)
	r.Get("/api/v1/finance/budgets", h.ListBudgets)
	r.Get("/api/v1/finance/budgets/{id}", h.GetBudget)
	return r
}

func financeUser(perms ...string) *authz.Principal {
	p := &authz.Principal{Kind: authz.KindUser, Subject: "user:test", UserID: uuid.New(), PersonID: uuid.New()}
	if len(perms) > 0 {
		p.Permissions = make(map[string]struct{}, len(perms))
		for _, k := range perms {
			p.Permissions[k] = struct{}{}
		}
	}
	return p
}

func authRequest(method, target string, as *authz.Principal) *http.Request {
	req := httptest.NewRequest(method, target, nil)
	if as != nil {
		req = req.WithContext(authz.WithPrincipal(req.Context(), as))
	}
	return req
}

func decode(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode %q: %v", rec.Body.String(), err)
	}
	return body
}

// ── masking at the edge ─────────────────────────────────────────────────────

func TestOverviewHidesAmountsWithoutPermission(t *testing.T) {
	h := NewHandler(NewService(&fakeStore{totals: OverviewTotals{CashMinor: 100, PendingExpenses: 2}}, nil), nil)

	rec := httptest.NewRecorder()
	financeRouter(h).ServeHTTP(rec, authRequest(http.MethodGet, "/api/v1/finance/overview", financeUser()))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
	body := decode(t, rec)
	if _, present := body["cash_minor"]; present {
		t.Errorf("cash_minor should be omitted without the sensitive permission: %v", body)
	}
	if body["pending_expenses"] != float64(2) {
		t.Errorf("pending_expenses = %v, want 2", body["pending_expenses"])
	}
}

func TestOverviewShowsAmountsWithPermission(t *testing.T) {
	h := NewHandler(NewService(&fakeStore{totals: OverviewTotals{CashMinor: 0}}, nil), nil)

	rec := httptest.NewRecorder()
	financeRouter(h).ServeHTTP(rec, authRequest(http.MethodGet, "/api/v1/finance/overview",
		financeUser(sensitiveAmount)))

	body := decode(t, rec)
	cash, present := body["cash_minor"]
	if !present {
		t.Fatalf("cash_minor should be present with the sensitive permission: %v", body)
	}
	if cash != float64(0) {
		t.Errorf("cash_minor = %v, want a visible 0", cash)
	}
}

func TestListAccountsMasksBalance(t *testing.T) {
	opening := int64(100)
	store := &fakeStore{accounts: []AccountBalance{{
		Account:      Account{OpeningBalanceMinor: &opening},
		BalanceMinor: 250,
	}}}
	h := NewHandler(NewService(store, nil), nil)

	rec := httptest.NewRecorder()
	financeRouter(h).ServeHTTP(rec, authRequest(http.MethodGet, "/api/v1/finance/accounts", financeUser()))
	body := decode(t, rec)
	items := body["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("items = %d, want 1", len(items))
	}
	item := items[0].(map[string]any)
	if _, present := item["balance_minor"]; present {
		t.Errorf("balance_minor should be omitted: %v", item)
	}
}

func TestCashPositionNetAndMasking(t *testing.T) {
	store := &fakeStore{inflow: 500, outflow: 200}
	h := NewHandler(NewService(store, nil), nil)

	// Without the permission, no amount survives — including the derived net.
	rec := httptest.NewRecorder()
	financeRouter(h).ServeHTTP(rec, authRequest(http.MethodGet, "/api/v1/finance/cash-position", financeUser()))
	if _, present := decode(t, rec)["net_30d_minor"]; present {
		t.Errorf("net_30d_minor should be omitted without the sensitive permission")
	}

	// With it, the net is the inflow less the outflow.
	rec = httptest.NewRecorder()
	financeRouter(h).ServeHTTP(rec, authRequest(http.MethodGet, "/api/v1/finance/cash-position",
		financeUser(sensitiveAmount)))
	body := decode(t, rec)
	if body["net_30d_minor"] != float64(300) {
		t.Errorf("net_30d_minor = %v, want 300", body["net_30d_minor"])
	}
}

// ── error and edge cases ────────────────────────────────────────────────────

func TestListAccountsUnauthenticatedIs401(t *testing.T) {
	h := NewHandler(NewService(&fakeStore{}, nil), nil)
	rec := httptest.NewRecorder()
	financeRouter(h).ServeHTTP(rec, authRequest(http.MethodGet, "/api/v1/finance/accounts", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestGetExpenseBadIDIs400(t *testing.T) {
	h := NewHandler(NewService(&fakeStore{}, nil), nil)
	rec := httptest.NewRecorder()
	financeRouter(h).ServeHTTP(rec, authRequest(http.MethodGet, "/api/v1/finance/expenses/not-a-uuid", financeUser()))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (%s)", rec.Code, rec.Body.String())
	}
}

func TestGetExpenseUnknownIs404(t *testing.T) {
	h := NewHandler(NewService(&fakeStore{}, nil), nil)
	rec := httptest.NewRecorder()
	financeRouter(h).ServeHTTP(rec, authRequest(http.MethodGet, "/api/v1/finance/expenses/"+uuid.New().String(), financeUser()))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (%s)", rec.Code, rec.Body.String())
	}
}

func TestListExpensesUnknownStatusIs400(t *testing.T) {
	h := NewHandler(NewService(&fakeStore{}, nil), nil)
	rec := httptest.NewRecorder()
	financeRouter(h).ServeHTTP(rec, authRequest(http.MethodGet, "/api/v1/finance/expenses?status=cancelled", financeUser()))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (%s)", rec.Code, rec.Body.String())
	}
}

func TestListBudgetsInvalidPeriodIs400(t *testing.T) {
	h := NewHandler(NewService(&fakeStore{}, nil), nil)
	rec := httptest.NewRecorder()
	financeRouter(h).ServeHTTP(rec, authRequest(http.MethodGet, "/api/v1/finance/budgets?period=2026", financeUser()))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (%s)", rec.Code, rec.Body.String())
	}
}

func TestListBudgetsPagination(t *testing.T) {
	store := &fakeStore{budgets: []Budget{{ID: uuid.Must(uuid.NewV7())}, {ID: uuid.Must(uuid.NewV7())}}}
	h := NewHandler(NewService(store, nil), nil)

	rec := httptest.NewRecorder()
	financeRouter(h).ServeHTTP(rec, authRequest(http.MethodGet, "/api/v1/finance/budgets?limit=1", financeUser()))
	body := decode(t, rec)
	items := body["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("items = %d, want 1", len(items))
	}
	if body["has_more"] != true {
		t.Errorf("has_more = %v, want true", body["has_more"])
	}
	if cur, _ := body["next_cursor"].(string); cur == "" {
		t.Errorf("next_cursor should be set: %v", body)
	}
}

func TestProjectProfitabilityUnknownIs404(t *testing.T) {
	h := NewHandler(NewService(&fakeStore{projectOK: false}, nil), nil)
	rec := httptest.NewRecorder()
	financeRouter(h).ServeHTTP(rec,
		authRequest(http.MethodGet, "/api/v1/finance/projects/"+uuid.New().String()+"/profitability", financeUser()))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (%s)", rec.Code, rec.Body.String())
	}
}

// TestGetInvoiceCarriesLines proves the single-record read includes lines while
// the list does not.
func TestGetInvoiceCarriesLines(t *testing.T) {
	id := uuid.Must(uuid.NewV7())
	store := &fakeStore{
		invoice: &Invoice{ID: id},
		lines:   []InvoiceLine{{ID: uuid.Must(uuid.NewV7()), Description: "Design", Quantity: 2}},
	}
	h := NewHandler(NewService(store, nil), nil)

	rec := httptest.NewRecorder()
	financeRouter(h).ServeHTTP(rec, authRequest(http.MethodGet, "/api/v1/finance/invoices/"+id.String(), financeUser()))
	body := decode(t, rec)
	lines, ok := body["lines"].([]any)
	if !ok || len(lines) != 1 {
		t.Fatalf("lines = %v, want one line", body["lines"])
	}
}
