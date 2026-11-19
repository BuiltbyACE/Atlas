package finance

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/safaristack/atlas/apps/api/internal/authz"
	"github.com/safaristack/atlas/apps/api/internal/platform/httpx"
)

// sensitiveAmount is the permission that decides whether a monetary field is
// shown at all (§8.4). It is read once per request and threaded through the
// converters: a hidden amount is omitted from the JSON, never sent as zero.
const sensitiveAmount = "finance.amount.sensitive.read"

// Handler serves the finance HTTP surface (§8.4).
type Handler struct {
	service *Service
	logger  *slog.Logger
}

// NewHandler builds a finance handler.
func NewHandler(service *Service, logger *slog.Logger) *Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Handler{service: service, logger: logger}
}

// ── views ───────────────────────────────────────────────────────────────────

// Every `*_minor` field is a pointer with omitempty: the zero value is a
// legitimate amount, so only the pointer's absence can mean "withheld".

type overviewView struct {
	AsOf                        string `json:"as_of"`
	Currency                    string `json:"currency"`
	CashMinor                   *int64 `json:"cash_minor,omitempty"`
	ReceivablesOutstandingMinor *int64 `json:"receivables_outstanding_minor,omitempty"`
	ReceivablesOverdueMinor     *int64 `json:"receivables_overdue_minor,omitempty"`
	Invoiced90DMinor            *int64 `json:"invoiced_90d_minor,omitempty"`
	Collected90DMinor           *int64 `json:"collected_90d_minor,omitempty"`
	Expenses30DMinor            *int64 `json:"expenses_30d_minor,omitempty"`
	PendingExpenses             int    `json:"pending_expenses"`
	OverdueInvoiceCount         int    `json:"overdue_invoice_count"`
	UpcomingRenewals30D         int    `json:"upcoming_renewals_30d"`
	ActiveBudgetCount           int    `json:"active_budget_count"`
	BudgetsOverThreshold        int    `json:"budgets_over_threshold"`
}

type accountView struct {
	ID                  string `json:"id"`
	Name                string `json:"name"`
	AccountType         string `json:"account_type"`
	Currency            string `json:"currency"`
	OpeningBalanceMinor *int64 `json:"opening_balance_minor,omitempty"`
	BalanceMinor        *int64 `json:"balance_minor,omitempty"`
	Active              bool   `json:"active"`
	Version             int    `json:"version"`
	CreatedAt           string `json:"created_at"`
	UpdatedAt           string `json:"updated_at"`
}

type cashAccountView struct {
	AccountID    string `json:"account_id"`
	Name         string `json:"name"`
	AccountType  string `json:"account_type"`
	BalanceMinor *int64 `json:"balance_minor,omitempty"`
}

type cashPositionView struct {
	AsOf            string            `json:"as_of"`
	Currency        string            `json:"currency"`
	TotalMinor      *int64            `json:"total_minor,omitempty"`
	ByAccount       []cashAccountView `json:"by_account"`
	Inflow30DMinor  *int64            `json:"inflow_30d_minor,omitempty"`
	Outflow30DMinor *int64            `json:"outflow_30d_minor,omitempty"`
	Net30DMinor     *int64            `json:"net_30d_minor,omitempty"`
}

type bucketView struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Count       int    `json:"count"`
	AmountMinor *int64 `json:"amount_minor,omitempty"`
}

type receivablesView struct {
	AsOf                  string        `json:"as_of"`
	Currency              string        `json:"currency"`
	TotalOutstandingMinor *int64        `json:"total_outstanding_minor,omitempty"`
	Buckets               []bucketView  `json:"buckets"`
	Invoices              []invoiceView `json:"invoices"`
}

type expenseView struct {
	ID               string `json:"id"`
	Ref              string `json:"ref"`
	ClaimantPersonID string `json:"claimant_person_id"`
	CategoryKey      string `json:"category_key"`
	ProjectID        string `json:"project_id,omitempty"`
	AmountMinor      *int64 `json:"amount_minor,omitempty"`
	Currency         string `json:"currency"`
	SpentOn          string `json:"spent_on"`
	Vendor           string `json:"vendor,omitempty"`
	PaymentMethod    string `json:"payment_method,omitempty"`
	AccountID        string `json:"account_id,omitempty"`
	Status           string `json:"status"`
	SubmittedAt      string `json:"submitted_at,omitempty"`
	DecidedBy        string `json:"decided_by,omitempty"`
	DecidedAt        string `json:"decided_at,omitempty"`
	DecisionReason   string `json:"decision_reason,omitempty"`
	Notes            string `json:"notes,omitempty"`
	ReceiptFileID    string `json:"receipt_file_id,omitempty"`
	TaxInvoiceRef    string `json:"tax_invoice_ref,omitempty"`
	PaidOn           string `json:"paid_on,omitempty"`
	Version          int    `json:"version"`
	CreatedAt        string `json:"created_at"`
	UpdatedAt        string `json:"updated_at"`
}

type expenseCategoryView struct {
	Key       string `json:"key"`
	Label     string `json:"label"`
	SortOrder int    `json:"sort_order"`
	Active    bool   `json:"active"`
}

type invoiceLineView struct {
	ID             string  `json:"id"`
	Description    string  `json:"description"`
	Quantity       float64 `json:"quantity"`
	UnitPriceMinor *int64  `json:"unit_price_minor,omitempty"`
	TaxRateBP      int     `json:"tax_rate_bp"`
	AmountMinor    *int64  `json:"amount_minor,omitempty"`
	SortOrder      int     `json:"sort_order"`
}

type invoiceView struct {
	ID               string            `json:"id"`
	Ref              string            `json:"ref"`
	OrganizationID   string            `json:"organization_id"`
	OrganizationName string            `json:"organization_name,omitempty"`
	ContractID       string            `json:"contract_id,omitempty"`
	ContractRef      string            `json:"contract_ref,omitempty"`
	MilestoneID      string            `json:"milestone_id,omitempty"`
	IssueDate        string            `json:"issue_date,omitempty"`
	DueDate          string            `json:"due_date,omitempty"`
	TotalMinor       *int64            `json:"total_minor,omitempty"`
	Currency         string            `json:"currency"`
	Status           string            `json:"status"`
	AmountPaidMinor  *int64            `json:"amount_paid_minor,omitempty"`
	OutstandingMinor *int64            `json:"outstanding_minor,omitempty"`
	IssuedAt         string            `json:"issued_at,omitempty"`
	PaidAt           string            `json:"paid_at,omitempty"`
	EInvoiceRef      string            `json:"e_invoice_ref,omitempty"`
	Notes            string            `json:"notes,omitempty"`
	Lines            []invoiceLineView `json:"lines,omitempty"`
	Version          int               `json:"version"`
	CreatedAt        string            `json:"created_at"`
	UpdatedAt        string            `json:"updated_at"`
}

type paymentView struct {
	ID               string `json:"id"`
	Ref              string `json:"ref"`
	AmountMinor      *int64 `json:"amount_minor,omitempty"`
	Currency         string `json:"currency"`
	Method           string `json:"method"`
	Reference        string `json:"reference,omitempty"`
	ReceivedOn       string `json:"received_on"`
	AccountID        string `json:"account_id,omitempty"`
	Reconciled       bool   `json:"reconciled"`
	AllocatedMinor   *int64 `json:"allocated_minor,omitempty"`
	UnallocatedMinor *int64 `json:"unallocated_minor,omitempty"`
	Notes            string `json:"notes,omitempty"`
	Version          int    `json:"version"`
	CreatedAt        string `json:"created_at"`
	UpdatedAt        string `json:"updated_at"`
}

type budgetView struct {
	ID            string `json:"id"`
	Scope         string `json:"scope"`
	ScopeRef      string `json:"scope_ref,omitempty"`
	PeriodStart   string `json:"period_start"`
	PeriodEnd     string `json:"period_end"`
	CategoryKey   string `json:"category_key,omitempty"`
	AmountMinor   *int64 `json:"amount_minor,omitempty"`
	Currency      string `json:"currency"`
	Status        string `json:"status"`
	OwnerPersonID string `json:"owner_person_id,omitempty"`
	ActualMinor   *int64 `json:"actual_minor,omitempty"`
	VarianceMinor *int64 `json:"variance_minor,omitempty"`
	Version       int    `json:"version"`
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`
}

type recurringCostView struct {
	ID                     string `json:"id"`
	Vendor                 string `json:"vendor"`
	Description            string `json:"description,omitempty"`
	AmountMinor            *int64 `json:"amount_minor,omitempty"`
	Currency               string `json:"currency"`
	Cycle                  string `json:"cycle"`
	NextRenewalOn          string `json:"next_renewal_on"`
	OwnerPersonID          string `json:"owner_person_id,omitempty"`
	Active                 bool   `json:"active"`
	MonthlyEquivalentMinor *int64 `json:"monthly_equivalent_minor,omitempty"`
	Version                int    `json:"version"`
	CreatedAt              string `json:"created_at"`
	UpdatedAt              string `json:"updated_at"`
}

type projectProfitabilityView struct {
	ProjectID             string `json:"project_id"`
	ProjectName           string `json:"project_name,omitempty"`
	Currency              string `json:"currency"`
	RevenueInvoicedMinor  *int64 `json:"revenue_invoiced_minor,omitempty"`
	RevenueCollectedMinor *int64 `json:"revenue_collected_minor,omitempty"`
	CostExpensesMinor     *int64 `json:"cost_expenses_minor,omitempty"`
	CostTimeMinor         *int64 `json:"cost_time_minor,omitempty"`
	MarginMinor           *int64 `json:"margin_minor,omitempty"`
	MarginBP              *int   `json:"margin_bp,omitempty"`
}

// ── converters ──────────────────────────────────────────────────────────────

func rfc(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func rfcPtr(t *time.Time) string {
	if t == nil {
		return ""
	}
	return rfc(*t)
}

func day(t time.Time) string { return t.Format("2006-01-02") }

func dayPtr(t *time.Time) string {
	if t == nil {
		return ""
	}
	return day(*t)
}

func idStr(id *uuid.UUID) string {
	if id == nil {
		return ""
	}
	return id.String()
}

func strOrEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// money returns the amount when the viewer may see it, and nil when they may
// not. A withheld amount is absent from the JSON; it is never a zero, which
// would read as a real balance.
func money(visible bool, v int64) *int64 {
	if !visible {
		return nil
	}
	return &v
}

// moneyPtr is money for an amount that may itself be null in the database: the
// nil is preserved (no amount recorded), and visibility only governs the rest.
func moneyPtr(visible bool, v *int64) *int64 {
	if !visible {
		return nil
	}
	return v
}

func overviewOf(t OverviewTotals, asOf time.Time, visible bool) overviewView {
	return overviewView{
		AsOf: day(asOf), Currency: BaseCurrency,
		CashMinor:                   money(visible, t.CashMinor),
		ReceivablesOutstandingMinor: money(visible, t.ReceivablesOutstandingMinor),
		ReceivablesOverdueMinor:     money(visible, t.ReceivablesOverdueMinor),
		Invoiced90DMinor:            money(visible, t.Invoiced90DMinor),
		Collected90DMinor:           money(visible, t.Collected90DMinor),
		Expenses30DMinor:            money(visible, t.Expenses30DMinor),
		PendingExpenses:             t.PendingExpenses,
		OverdueInvoiceCount:         t.OverdueInvoiceCount,
		UpcomingRenewals30D:         t.UpcomingRenewals30D,
		ActiveBudgetCount:           t.ActiveBudgetCount,
		BudgetsOverThreshold:        t.BudgetsOverThreshold,
	}
}

func accountOf(a AccountBalance, visible bool) accountView {
	return accountView{
		ID: a.ID.String(), Name: a.Name, AccountType: string(a.AccountType), Currency: a.Currency,
		OpeningBalanceMinor: moneyPtr(visible, a.OpeningBalanceMinor),
		BalanceMinor:        money(visible, a.BalanceMinor),
		Active:              a.Active, Version: a.Version,
		CreatedAt: rfc(a.CreatedAt), UpdatedAt: rfc(a.UpdatedAt),
	}
}

func cashPositionOf(c CashPosition, visible bool) cashPositionView {
	byAccount := make([]cashAccountView, 0, len(c.Accounts))
	for _, a := range c.Accounts {
		byAccount = append(byAccount, cashAccountView{
			AccountID: a.ID.String(), Name: a.Name, AccountType: string(a.AccountType),
			BalanceMinor: money(visible, a.BalanceMinor),
		})
	}
	return cashPositionView{
		AsOf: day(c.AsOf), Currency: BaseCurrency, TotalMinor: money(visible, c.TotalMinor),
		ByAccount:       byAccount,
		Inflow30DMinor:  money(visible, c.InflowMinor),
		Outflow30DMinor: money(visible, c.OutflowMinor),
		Net30DMinor:     money(visible, c.InflowMinor-c.OutflowMinor),
	}
}

func receivablesOf(rec Receivables, visible bool) receivablesView {
	buckets := make([]bucketView, 0, len(rec.Buckets))
	for _, b := range rec.Buckets {
		buckets = append(buckets, bucketView{
			Key: b.Key, Label: b.Label, Count: b.Count, AmountMinor: money(visible, b.Amount),
		})
	}
	invoices := make([]invoiceView, 0, len(rec.Invoices))
	for _, inv := range rec.Invoices {
		invoices = append(invoices, invoiceOf(inv, visible, nil))
	}
	return receivablesView{
		AsOf: day(rec.AsOf), Currency: BaseCurrency,
		TotalOutstandingMinor: money(visible, rec.TotalOutstanding), Buckets: buckets, Invoices: invoices,
	}
}

func expenseOf(e Expense, visible bool) expenseView {
	v := expenseView{
		ID: e.ID.String(), Ref: e.Ref, ClaimantPersonID: e.ClaimantPersonID.String(),
		CategoryKey: e.CategoryKey, ProjectID: idStr(e.ProjectID),
		AmountMinor: moneyPtr(visible, e.AmountMinor), Currency: e.Currency,
		SpentOn: day(e.SpentOn), Vendor: e.Vendor, AccountID: idStr(e.AccountID),
		Status: string(e.Status), SubmittedAt: rfcPtr(e.SubmittedAt), DecidedBy: idStr(e.DecidedBy),
		DecidedAt: rfcPtr(e.DecidedAt), DecisionReason: e.DecisionReason, Notes: e.Notes,
		ReceiptFileID: idStr(e.ReceiptFileID), TaxInvoiceRef: strOrEmpty(e.TaxInvoiceRef), PaidOn: dayPtr(e.PaidOn),
		Version: e.Version, CreatedAt: rfc(e.CreatedAt), UpdatedAt: rfc(e.UpdatedAt),
	}
	if e.PaymentMethod != nil {
		v.PaymentMethod = string(*e.PaymentMethod)
	}
	return v
}

func expenseCategoryOf(c ExpenseCategory) expenseCategoryView {
	// The view mirrors the record field for field, so it converts rather than
	// restates each one.
	return expenseCategoryView(c)
}

func invoiceLineOf(l InvoiceLine, visible bool) invoiceLineView {
	return invoiceLineView{
		ID: l.ID.String(), Description: l.Description, Quantity: l.Quantity,
		UnitPriceMinor: moneyPtr(visible, l.UnitPriceMinor), TaxRateBP: l.TaxRateBP,
		AmountMinor: moneyPtr(visible, l.AmountMinor), SortOrder: l.SortOrder,
	}
}

// invoiceOf renders an invoice. lines is nil for the list (which does not carry
// lines) and set for the single-record read.
func invoiceOf(inv Invoice, visible bool, lines []InvoiceLine) invoiceView {
	v := invoiceView{
		ID: inv.ID.String(), Ref: inv.Ref, OrganizationID: inv.OrganizationID.String(),
		OrganizationName: inv.OrganizationName, ContractID: idStr(inv.ContractID),
		ContractRef: inv.ContractRef, MilestoneID: idStr(inv.MilestoneID),
		IssueDate: dayPtr(inv.IssueDate), DueDate: dayPtr(inv.DueDate),
		TotalMinor: moneyPtr(visible, inv.TotalMinor), Currency: inv.Currency, Status: string(inv.Status),
		AmountPaidMinor:  money(visible, inv.AmountPaidMinor),
		OutstandingMinor: money(visible, inv.OutstandingMinor),
		IssuedAt:         rfcPtr(inv.IssuedAt), PaidAt: rfcPtr(inv.PaidAt), EInvoiceRef: inv.EInvoiceRef,
		Notes: inv.Notes, Version: inv.Version, CreatedAt: rfc(inv.CreatedAt), UpdatedAt: rfc(inv.UpdatedAt),
	}
	if lines != nil {
		views := make([]invoiceLineView, 0, len(lines))
		for _, l := range lines {
			views = append(views, invoiceLineOf(l, visible))
		}
		v.Lines = views
	}
	return v
}

func paymentOf(p Payment, visible bool) paymentView {
	return paymentView{
		ID: p.ID.String(), Ref: p.Ref, AmountMinor: moneyPtr(visible, p.AmountMinor),
		Currency: p.Currency, Method: string(p.Method), Reference: p.Reference,
		ReceivedOn: day(p.ReceivedOn), AccountID: idStr(p.AccountID), Reconciled: p.Reconciled,
		AllocatedMinor:   money(visible, p.AllocatedMinor),
		UnallocatedMinor: money(visible, p.UnallocatedMinor), Notes: p.Notes,
		Version: p.Version, CreatedAt: rfc(p.CreatedAt), UpdatedAt: rfc(p.UpdatedAt),
	}
}

func budgetOf(b Budget, visible bool) budgetView {
	return budgetView{
		ID: b.ID.String(), Scope: string(b.Scope), ScopeRef: b.ScopeRef,
		PeriodStart: day(b.PeriodStart), PeriodEnd: day(b.PeriodEnd), CategoryKey: b.CategoryKey,
		AmountMinor: moneyPtr(visible, b.AmountMinor), Currency: b.Currency, Status: string(b.Status),
		OwnerPersonID: idStr(b.OwnerPersonID),
		ActualMinor:   moneyPtr(visible, b.ActualMinor),
		VarianceMinor: moneyPtr(visible, b.VarianceMinor),
		Version:       b.Version, CreatedAt: rfc(b.CreatedAt), UpdatedAt: rfc(b.UpdatedAt),
	}
}

func recurringCostOf(c RecurringCost, visible bool) recurringCostView {
	return recurringCostView{
		ID: c.ID.String(), Vendor: c.Vendor, Description: c.Description,
		AmountMinor: moneyPtr(visible, c.AmountMinor), Currency: c.Currency, Cycle: string(c.Cycle),
		NextRenewalOn: day(c.NextRenewalOn), OwnerPersonID: idStr(c.OwnerPersonID), Active: c.Active,
		MonthlyEquivalentMinor: moneyPtr(visible, c.MonthlyEquivalentMinor),
		Version:                c.Version, CreatedAt: rfc(c.CreatedAt), UpdatedAt: rfc(c.UpdatedAt),
	}
}

// projectProfitabilityOf renders a project's P&L. The margin is revenue
// invoiced less expenses; the basis points are that margin over the invoiced
// total, and are omitted when nothing has been invoiced (a margin over zero has
// no meaning). cost_time is always absent: no hourly-cost rate exists yet.
func projectProfitabilityOf(p ProjectProfitability, visible bool) projectProfitabilityView {
	v := projectProfitabilityView{
		ProjectID: p.ProjectID.String(), ProjectName: p.ProjectName, Currency: p.Currency,
		RevenueInvoicedMinor:  money(visible, p.RevenueInvoicedMinor),
		RevenueCollectedMinor: money(visible, p.RevenueCollectedMinor),
		CostExpensesMinor:     money(visible, p.CostExpensesMinor),
	}
	margin := p.RevenueInvoicedMinor - p.CostExpensesMinor
	v.MarginMinor = money(visible, margin)
	if visible && p.RevenueInvoicedMinor > 0 {
		bp := int(margin * 10000 / p.RevenueInvoicedMinor)
		v.MarginBP = &bp
	}
	return v
}

// ── headline ────────────────────────────────────────────────────────────────

// Overview handles GET /api/v1/finance/overview.
func (h *Handler) Overview(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	totals, asOf, err := h.service.Overview(r.Context())
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, overviewOf(totals, asOf, principal.Can(sensitiveAmount)))
}

// ── cash and accounts ───────────────────────────────────────────────────────

// ListAccounts handles GET /api/v1/finance/accounts. It is not paged: the
// account list is a small, fixed set the cash screen shows in full.
func (h *Handler) ListAccounts(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	accounts, err := h.service.Accounts(r.Context())
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	visible := principal.Can(sensitiveAmount)
	views := make([]accountView, 0, len(accounts))
	for _, a := range accounts {
		views = append(views, accountOf(a, visible))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": views})
}

// CashPosition handles GET /api/v1/finance/cash-position.
func (h *Handler) CashPosition(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	position, err := h.service.CashPosition(r.Context())
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, cashPositionOf(position, principal.Can(sensitiveAmount)))
}

// Receivables handles GET /api/v1/finance/receivables.
func (h *Handler) Receivables(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	ageing, err := h.service.Receivables(r.Context())
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, receivablesOf(ageing, principal.Can(sensitiveAmount)))
}

// RecurringCosts handles GET /api/v1/finance/recurring-costs. Like accounts,
// the standing-cost list is unpaged.
func (h *Handler) RecurringCosts(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	costs, err := h.service.RecurringCosts(r.Context())
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	visible := principal.Can(sensitiveAmount)
	views := make([]recurringCostView, 0, len(costs))
	for _, c := range costs {
		views = append(views, recurringCostOf(c, visible))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": views})
}

// ── project profitability ───────────────────────────────────────────────────

// ProjectProfitability handles GET /api/v1/finance/projects/{id}/profitability.
func (h *Handler) ProjectProfitability(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	id, ok := h.idParam(w, r, "id", "project")
	if !ok {
		return
	}
	profit, err := h.service.ProjectProfitability(r.Context(), id)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, projectProfitabilityOf(profit, principal.Can(sensitiveAmount)))
}

// ── expenses ────────────────────────────────────────────────────────────────

// ListExpenseCategories handles GET /api/v1/finance/expense-categories. It
// carries no amounts, so it needs no masking.
func (h *Handler) ListExpenseCategories(w http.ResponseWriter, r *http.Request) {
	categories, err := h.service.ExpenseCategories(r.Context())
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	views := make([]expenseCategoryView, 0, len(categories))
	for _, c := range categories {
		views = append(views, expenseCategoryOf(c))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": views})
}

// ListExpenses handles GET /api/v1/finance/expenses.
func (h *Handler) ListExpenses(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	limit, cursor, err := httpx.ParsePage(r)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	before, err := cursorOrNil(cursor)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	var q ExpenseQuery
	if raw := trim(r.URL.Query().Get("status")); raw != "" {
		s := ExpenseStatus(raw)
		q.Status = &s
	}
	q.CategoryKey = trim(r.URL.Query().Get("category_key"))
	if q.ProjectID, ok = h.uuidParam(w, r, "project_id"); !ok {
		return
	}
	if q.ClaimantID, ok = h.uuidParam(w, r, "claimant_person_id"); !ok {
		return
	}
	if q.From, ok = h.timeParam(w, r, "from"); !ok {
		return
	}
	if q.To, ok = h.timeParam(w, r, "to"); !ok {
		return
	}
	items, err := h.service.Expenses(r.Context(), q, limit+1, before)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	next := ""
	if len(items) > limit {
		items = items[:limit]
		last := items[len(items)-1]
		next = encodeCursor(last.CreatedAt, last.ID)
	}
	visible := principal.Can(sensitiveAmount)
	views := make([]expenseView, 0, len(items))
	for _, e := range items {
		views = append(views, expenseOf(e, visible))
	}
	httpx.WriteJSON(w, http.StatusOK, httpx.NewPage(views, next, limit))
}

// GetExpense handles GET /api/v1/finance/expenses/{id}.
func (h *Handler) GetExpense(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	id, ok := h.idParam(w, r, "id", "expense")
	if !ok {
		return
	}
	e, err := h.service.Expense(r.Context(), id)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.SetETag(w, int64(e.Version))
	httpx.WriteJSON(w, http.StatusOK, expenseOf(e, principal.Can(sensitiveAmount)))
}

// ── invoices ────────────────────────────────────────────────────────────────

// ListInvoices handles GET /api/v1/finance/invoices.
func (h *Handler) ListInvoices(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	limit, cursor, err := httpx.ParsePage(r)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	before, err := cursorOrNil(cursor)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	var q InvoiceQuery
	if raw := trim(r.URL.Query().Get("status")); raw != "" {
		s := InvoiceStatus(raw)
		q.Status = &s
	}
	if q.OrganizationID, ok = h.uuidParam(w, r, "organization_id"); !ok {
		return
	}
	if q.ContractID, ok = h.uuidParam(w, r, "contract_id"); !ok {
		return
	}
	if q.Overdue, ok = h.boolParam(w, r, "overdue"); !ok {
		return
	}
	items, err := h.service.Invoices(r.Context(), q, limit+1, before)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	next := ""
	if len(items) > limit {
		items = items[:limit]
		last := items[len(items)-1]
		next = encodeCursor(last.CreatedAt, last.ID)
	}
	visible := principal.Can(sensitiveAmount)
	views := make([]invoiceView, 0, len(items))
	for _, inv := range items {
		views = append(views, invoiceOf(inv, visible, nil))
	}
	httpx.WriteJSON(w, http.StatusOK, httpx.NewPage(views, next, limit))
}

// GetInvoice handles GET /api/v1/finance/invoices/{id}. Unlike the list, it
// carries the invoice's lines.
func (h *Handler) GetInvoice(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	id, ok := h.idParam(w, r, "id", "invoice")
	if !ok {
		return
	}
	inv, lines, err := h.service.Invoice(r.Context(), id)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.SetETag(w, int64(inv.Version))
	httpx.WriteJSON(w, http.StatusOK, invoiceOf(inv, principal.Can(sensitiveAmount), lines))
}

// ── payments ────────────────────────────────────────────────────────────────

// ListPayments handles GET /api/v1/finance/payments.
func (h *Handler) ListPayments(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	limit, cursor, err := httpx.ParsePage(r)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	before, err := cursorOrNil(cursor)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	var q PaymentQuery
	if q.From, ok = h.timeParam(w, r, "from"); !ok {
		return
	}
	if q.To, ok = h.timeParam(w, r, "to"); !ok {
		return
	}
	if raw := trim(r.URL.Query().Get("method")); raw != "" {
		m := PaymentMethod(raw)
		q.Method = &m
	}
	items, err := h.service.Payments(r.Context(), q, limit+1, before)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	next := ""
	if len(items) > limit {
		items = items[:limit]
		last := items[len(items)-1]
		next = encodeCursor(last.CreatedAt, last.ID)
	}
	visible := principal.Can(sensitiveAmount)
	views := make([]paymentView, 0, len(items))
	for _, p := range items {
		views = append(views, paymentOf(p, visible))
	}
	httpx.WriteJSON(w, http.StatusOK, httpx.NewPage(views, next, limit))
}

// ── budgets ─────────────────────────────────────────────────────────────────

// ListBudgets handles GET /api/v1/finance/budgets.
func (h *Handler) ListBudgets(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	limit, cursor, err := httpx.ParsePage(r)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	before, err := cursorOrNil(cursor)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	var q BudgetQuery
	if raw := trim(r.URL.Query().Get("scope")); raw != "" {
		s := BudgetScope(raw)
		q.Scope = &s
	}
	if raw := trim(r.URL.Query().Get("status")); raw != "" {
		s := BudgetStatus(raw)
		q.Status = &s
	}
	if raw := trim(r.URL.Query().Get("period")); raw != "" {
		if _, err := time.Parse("2006-01", raw); err != nil {
			httpx.WriteProblem(w, r, httpx.BadRequest("period must be a year and month, e.g. 2026-09."))
			return
		}
		q.Period = raw
	}
	items, err := h.service.Budgets(r.Context(), q, limit+1, before)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	next := ""
	if len(items) > limit {
		items = items[:limit]
		last := items[len(items)-1]
		next = encodeCursor(last.CreatedAt, last.ID)
	}
	visible := principal.Can(sensitiveAmount)
	views := make([]budgetView, 0, len(items))
	for _, b := range items {
		views = append(views, budgetOf(b, visible))
	}
	httpx.WriteJSON(w, http.StatusOK, httpx.NewPage(views, next, limit))
}

// GetBudget handles GET /api/v1/finance/budgets/{id}.
func (h *Handler) GetBudget(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	id, ok := h.idParam(w, r, "id", "budget")
	if !ok {
		return
	}
	b, err := h.service.Budget(r.Context(), id)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.SetETag(w, int64(b.Version))
	httpx.WriteJSON(w, http.StatusOK, budgetOf(b, principal.Can(sensitiveAmount)))
}

// ── writes (§8.4) ───────────────────────────────────────────────────────────
//
// Every write route carries its own permission (declared in the router), takes
// If-Match where the resource is versioned, and returns the record after the
// change together with its new ETag. Go owns the money: amounts and totals are
// computed here, never accepted from the request.

// expenseReq is a claim as a client sends it. AmountMinor is a pointer so a
// missing amount is rejected rather than read as zero.
type expenseReq struct {
	ClaimantPersonID *string `json:"claimant_person_id"`
	CategoryKey      string  `json:"category_key"`
	ProjectID        *string `json:"project_id"`
	AmountMinor      *int64  `json:"amount_minor"`
	Currency         string  `json:"currency"`
	SpentOn          string  `json:"spent_on"`
	Vendor           string  `json:"vendor"`
	PaymentMethod    string  `json:"payment_method"`
	AccountID        *string `json:"account_id"`
	ReceiptFileID    *string `json:"receipt_file_id"`
	TaxInvoiceRef    string  `json:"tax_invoice_ref"`
	Notes            string  `json:"notes"`
}

func (req expenseReq) input() (ExpenseInput, error) {
	var in ExpenseInput
	var err error
	if in.ClaimantPersonID, err = optUUID(req.ClaimantPersonID, "claimant_person_id"); err != nil {
		return in, err
	}
	if in.ProjectID, err = optUUID(req.ProjectID, "project_id"); err != nil {
		return in, err
	}
	if in.AccountID, err = optUUID(req.AccountID, "account_id"); err != nil {
		return in, err
	}
	if in.ReceiptFileID, err = optUUID(req.ReceiptFileID, "receipt_file_id"); err != nil {
		return in, err
	}
	in.CategoryKey = trim(req.CategoryKey)
	if req.AmountMinor != nil {
		in.AmountMinor = *req.AmountMinor
	}
	in.Currency = req.Currency
	if in.SpentOn, err = parseDateField(req.SpentOn, "spent_on"); err != nil {
		return in, err
	}
	in.Vendor = req.Vendor
	if raw := trim(req.PaymentMethod); raw != "" {
		m := PaymentMethod(raw)
		in.PaymentMethod = &m
	}
	in.TaxInvoiceRef = req.TaxInvoiceRef
	in.Notes = req.Notes
	return in, nil
}

// CreateExpense handles POST /api/v1/finance/expenses.
func (h *Handler) CreateExpense(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	var req expenseReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		h.writeError(w, r, err)
		return
	}
	in, err := req.input()
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	e, err := h.service.CreateExpense(r.Context(), principal.PersonID, in)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.SetETag(w, int64(e.Version))
	httpx.WriteJSON(w, http.StatusCreated, expenseOf(e, principal.Can(sensitiveAmount)))
}

// UpdateExpense handles PUT /api/v1/finance/expenses/{id}.
func (h *Handler) UpdateExpense(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	id, ok := h.idParam(w, r, "id", "expense")
	if !ok {
		return
	}
	current, err := h.service.Expense(r.Context(), id)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	if err := httpx.RequireIfMatch(r, int64(current.Version)); err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	var req expenseReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		h.writeError(w, r, err)
		return
	}
	in, err := req.input()
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	e, err := h.service.UpdateExpense(r.Context(), principal.PersonID, id, current.Version, in)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.SetETag(w, int64(e.Version))
	httpx.WriteJSON(w, http.StatusOK, expenseOf(e, principal.Can(sensitiveAmount)))
}

// SubmitExpense handles POST /api/v1/finance/expenses/{id}/submit.
func (h *Handler) SubmitExpense(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	id, ok := h.idParam(w, r, "id", "expense")
	if !ok {
		return
	}
	current, err := h.service.Expense(r.Context(), id)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	if err := httpx.RequireIfMatch(r, int64(current.Version)); err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	e, err := h.service.SubmitExpense(r.Context(), principal.PersonID, id, current.Version)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.SetETag(w, int64(e.Version))
	httpx.WriteJSON(w, http.StatusOK, expenseOf(e, principal.Can(sensitiveAmount)))
}

type decideReq struct {
	Decision string `json:"decision"`
	Reason   string `json:"reason"`
}

// DecideExpense handles POST /api/v1/finance/expenses/{id}/decide.
func (h *Handler) DecideExpense(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	id, ok := h.idParam(w, r, "id", "expense")
	if !ok {
		return
	}
	current, err := h.service.Expense(r.Context(), id)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	if err := httpx.RequireIfMatch(r, int64(current.Version)); err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	var req decideReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		h.writeError(w, r, err)
		return
	}
	e, err := h.service.DecideExpense(r.Context(), principal.PersonID, id, current.Version,
		Decision(trim(req.Decision)), req.Reason)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.SetETag(w, int64(e.Version))
	httpx.WriteJSON(w, http.StatusOK, expenseOf(e, principal.Can(sensitiveAmount)))
}

type settleReq struct {
	Method    string  `json:"method"`
	AccountID *string `json:"account_id"`
	PaidOn    *string `json:"paid_on"`
}

// SettleExpense handles POST /api/v1/finance/expenses/{id}/settle.
func (h *Handler) SettleExpense(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	id, ok := h.idParam(w, r, "id", "expense")
	if !ok {
		return
	}
	current, err := h.service.Expense(r.Context(), id)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	if err := httpx.RequireIfMatch(r, int64(current.Version)); err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	var req settleReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		h.writeError(w, r, err)
		return
	}
	in := ExpenseSettleInput{Method: ExpenseStatus(trim(req.Method))}
	if in.AccountID, err = optUUID(req.AccountID, "account_id"); err != nil {
		h.writeError(w, r, err)
		return
	}
	if req.PaidOn != nil && trim(*req.PaidOn) != "" {
		d, err := parseDateField(*req.PaidOn, "paid_on")
		if err != nil {
			h.writeError(w, r, err)
			return
		}
		in.PaidOn = &d
	}
	e, err := h.service.SettleExpense(r.Context(), principal.PersonID, id, current.Version, in)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.SetETag(w, int64(e.Version))
	httpx.WriteJSON(w, http.StatusOK, expenseOf(e, principal.Can(sensitiveAmount)))
}

// ReconcileExpense handles POST /api/v1/finance/expenses/{id}/reconcile.
func (h *Handler) ReconcileExpense(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	id, ok := h.idParam(w, r, "id", "expense")
	if !ok {
		return
	}
	current, err := h.service.Expense(r.Context(), id)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	if err := httpx.RequireIfMatch(r, int64(current.Version)); err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	e, err := h.service.ReconcileExpense(r.Context(), principal.PersonID, id, current.Version)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.SetETag(w, int64(e.Version))
	httpx.WriteJSON(w, http.StatusOK, expenseOf(e, principal.Can(sensitiveAmount)))
}

// ── invoice writes ──────────────────────────────────────────────────────────

type invoiceLineReq struct {
	Description    string  `json:"description"`
	Quantity       float64 `json:"quantity"`
	UnitPriceMinor int64   `json:"unit_price_minor"`
	TaxRateBP      int     `json:"tax_rate_bp"`
	SortOrder      int     `json:"sort_order"`
}

type invoiceReq struct {
	OrganizationID string           `json:"organization_id"`
	ContractID     *string          `json:"contract_id"`
	MilestoneID    *string          `json:"milestone_id"`
	IssueDate      *string          `json:"issue_date"`
	DueDate        *string          `json:"due_date"`
	Currency       string           `json:"currency"`
	Notes          string           `json:"notes"`
	Lines          []invoiceLineReq `json:"lines"`
}

func (req invoiceReq) input() (InvoiceInput, error) {
	var in InvoiceInput
	var err error
	if in.OrganizationID, err = reqUUID(req.OrganizationID, "organization_id"); err != nil {
		return in, err
	}
	if in.ContractID, err = optUUID(req.ContractID, "contract_id"); err != nil {
		return in, err
	}
	if in.MilestoneID, err = optUUID(req.MilestoneID, "milestone_id"); err != nil {
		return in, err
	}
	if in.IssueDate, err = optDateField(req.IssueDate, "issue_date"); err != nil {
		return in, err
	}
	if in.DueDate, err = optDateField(req.DueDate, "due_date"); err != nil {
		return in, err
	}
	in.Currency = req.Currency
	in.Notes = req.Notes
	in.Lines = make([]InvoiceLineInput, 0, len(req.Lines))
	for i, l := range req.Lines {
		sortOrder := l.SortOrder
		if sortOrder == 0 {
			sortOrder = i
		}
		in.Lines = append(in.Lines, InvoiceLineInput{
			Description: l.Description, Quantity: l.Quantity, UnitPriceMinor: l.UnitPriceMinor,
			TaxRateBP: l.TaxRateBP, SortOrder: sortOrder,
		})
	}
	return in, nil
}

// CreateInvoice handles POST /api/v1/finance/invoices.
func (h *Handler) CreateInvoice(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	var req invoiceReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		h.writeError(w, r, err)
		return
	}
	in, err := req.input()
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	inv, err := h.service.CreateInvoice(r.Context(), principal.PersonID, in)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.SetETag(w, int64(inv.Version))
	httpx.WriteJSON(w, http.StatusCreated, invoiceOf(inv, principal.Can(sensitiveAmount), nil))
}

// UpdateInvoice handles PUT /api/v1/finance/invoices/{id}.
func (h *Handler) UpdateInvoice(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	id, ok := h.idParam(w, r, "id", "invoice")
	if !ok {
		return
	}
	current, _, err := h.service.Invoice(r.Context(), id)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	if err := httpx.RequireIfMatch(r, int64(current.Version)); err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	var req invoiceReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		h.writeError(w, r, err)
		return
	}
	in, err := req.input()
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	inv, err := h.service.UpdateInvoice(r.Context(), principal.PersonID, id, current.Version, in)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.SetETag(w, int64(inv.Version))
	httpx.WriteJSON(w, http.StatusOK, invoiceOf(inv, principal.Can(sensitiveAmount), nil))
}

// ApproveInvoice handles POST /api/v1/finance/invoices/{id}/approve.
func (h *Handler) ApproveInvoice(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	id, ok := h.idParam(w, r, "id", "invoice")
	if !ok {
		return
	}
	current, _, err := h.service.Invoice(r.Context(), id)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	if err := httpx.RequireIfMatch(r, int64(current.Version)); err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	inv, err := h.service.ApproveInvoice(r.Context(), principal.PersonID, id, current.Version)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.SetETag(w, int64(inv.Version))
	httpx.WriteJSON(w, http.StatusOK, invoiceOf(inv, principal.Can(sensitiveAmount), nil))
}

type issueReq struct {
	IssueDate *string `json:"issue_date"`
	DueDate   *string `json:"due_date"`
}

// IssueInvoice handles POST /api/v1/finance/invoices/{id}/issue.
func (h *Handler) IssueInvoice(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	id, ok := h.idParam(w, r, "id", "invoice")
	if !ok {
		return
	}
	current, _, err := h.service.Invoice(r.Context(), id)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	if err := httpx.RequireIfMatch(r, int64(current.Version)); err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	var req issueReq
	if err := decodeOptionalJSON(w, r, &req); err != nil {
		h.writeError(w, r, err)
		return
	}
	in := InvoiceIssueInput{}
	if in.IssueDate, err = optDateField(req.IssueDate, "issue_date"); err != nil {
		h.writeError(w, r, err)
		return
	}
	if in.DueDate, err = optDateField(req.DueDate, "due_date"); err != nil {
		h.writeError(w, r, err)
		return
	}
	inv, err := h.service.IssueInvoice(r.Context(), principal.PersonID, id, current.Version, in)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.SetETag(w, int64(inv.Version))
	httpx.WriteJSON(w, http.StatusOK, invoiceOf(inv, principal.Can(sensitiveAmount), nil))
}

type reasonReq struct {
	Reason string `json:"reason"`
}

// DisputeInvoice handles POST /api/v1/finance/invoices/{id}/dispute.
func (h *Handler) DisputeInvoice(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	id, ok := h.idParam(w, r, "id", "invoice")
	if !ok {
		return
	}
	current, _, err := h.service.Invoice(r.Context(), id)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	if err := httpx.RequireIfMatch(r, int64(current.Version)); err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	var req reasonReq
	if err := decodeOptionalJSON(w, r, &req); err != nil {
		h.writeError(w, r, err)
		return
	}
	inv, err := h.service.DisputeInvoice(r.Context(), principal.PersonID, id, current.Version, req.Reason)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.SetETag(w, int64(inv.Version))
	httpx.WriteJSON(w, http.StatusOK, invoiceOf(inv, principal.Can(sensitiveAmount), nil))
}

// WriteOffInvoice handles POST /api/v1/finance/invoices/{id}/write-off.
func (h *Handler) WriteOffInvoice(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	id, ok := h.idParam(w, r, "id", "invoice")
	if !ok {
		return
	}
	current, _, err := h.service.Invoice(r.Context(), id)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	if err := httpx.RequireIfMatch(r, int64(current.Version)); err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	var req reasonReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		h.writeError(w, r, err)
		return
	}
	inv, err := h.service.WriteOffInvoice(r.Context(), principal.PersonID, id, current.Version, req.Reason)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.SetETag(w, int64(inv.Version))
	httpx.WriteJSON(w, http.StatusOK, invoiceOf(inv, principal.Can(sensitiveAmount), nil))
}

// ── payment writes ──────────────────────────────────────────────────────────

type allocationReq struct {
	InvoiceID   string `json:"invoice_id"`
	AmountMinor int64  `json:"amount_minor"`
}

func allocationsOf(reqs []allocationReq) ([]PaymentAllocationInput, error) {
	out := make([]PaymentAllocationInput, 0, len(reqs))
	for _, a := range reqs {
		invoiceID, err := reqUUID(a.InvoiceID, "allocations.invoice_id")
		if err != nil {
			return nil, err
		}
		out = append(out, PaymentAllocationInput{InvoiceID: invoiceID, AmountMinor: a.AmountMinor})
	}
	return out, nil
}

type paymentReq struct {
	AmountMinor *int64          `json:"amount_minor"`
	Currency    string          `json:"currency"`
	Method      string          `json:"method"`
	Reference   string          `json:"reference"`
	ReceivedOn  string          `json:"received_on"`
	AccountID   *string         `json:"account_id"`
	Notes       string          `json:"notes"`
	Allocations []allocationReq `json:"allocations"`
}

func (req paymentReq) input() (PaymentInput, error) {
	var in PaymentInput
	var err error
	if req.AmountMinor != nil {
		in.AmountMinor = *req.AmountMinor
	}
	in.Currency = req.Currency
	in.Method = PaymentMethod(trim(req.Method))
	in.Reference = req.Reference
	if in.ReceivedOn, err = parseDateField(req.ReceivedOn, "received_on"); err != nil {
		return in, err
	}
	if in.AccountID, err = optUUID(req.AccountID, "account_id"); err != nil {
		return in, err
	}
	in.Notes = req.Notes
	if in.Allocations, err = allocationsOf(req.Allocations); err != nil {
		return in, err
	}
	return in, nil
}

// CreatePayment handles POST /api/v1/finance/payments.
func (h *Handler) CreatePayment(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	var req paymentReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		h.writeError(w, r, err)
		return
	}
	in, err := req.input()
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	p, err := h.service.CreatePayment(r.Context(), principal.PersonID, in)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.SetETag(w, int64(p.Version))
	httpx.WriteJSON(w, http.StatusCreated, paymentOf(p, principal.Can(sensitiveAmount)))
}

type allocateReq struct {
	Allocations []allocationReq `json:"allocations"`
}

// AllocatePayment handles POST /api/v1/finance/payments/{id}/allocate.
func (h *Handler) AllocatePayment(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	id, ok := h.idParam(w, r, "id", "payment")
	if !ok {
		return
	}
	current, err := h.service.Payment(r.Context(), id)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	if err := httpx.RequireIfMatch(r, int64(current.Version)); err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	var req allocateReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		h.writeError(w, r, err)
		return
	}
	allocations, err := allocationsOf(req.Allocations)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	p, err := h.service.AllocatePayment(r.Context(), principal.PersonID, id, current.Version, allocations)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.SetETag(w, int64(p.Version))
	httpx.WriteJSON(w, http.StatusOK, paymentOf(p, principal.Can(sensitiveAmount)))
}

type reconcilePaymentReq struct {
	Reconciled *bool `json:"reconciled"`
}

// ReconcilePayment handles POST /api/v1/finance/payments/{id}/reconcile.
func (h *Handler) ReconcilePayment(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	id, ok := h.idParam(w, r, "id", "payment")
	if !ok {
		return
	}
	current, err := h.service.Payment(r.Context(), id)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	if err := httpx.RequireIfMatch(r, int64(current.Version)); err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	var req reconcilePaymentReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		h.writeError(w, r, err)
		return
	}
	if req.Reconciled == nil {
		h.writeError(w, r, errInvalidField("reconciled"))
		return
	}
	p, err := h.service.ReconcilePayment(r.Context(), principal.PersonID, id, current.Version, *req.Reconciled)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.SetETag(w, int64(p.Version))
	httpx.WriteJSON(w, http.StatusOK, paymentOf(p, principal.Can(sensitiveAmount)))
}

// ── budget writes ───────────────────────────────────────────────────────────

type budgetReq struct {
	Scope         string  `json:"scope"`
	ScopeRef      string  `json:"scope_ref"`
	PeriodStart   string  `json:"period_start"`
	PeriodEnd     string  `json:"period_end"`
	CategoryKey   string  `json:"category_key"`
	AmountMinor   *int64  `json:"amount_minor"`
	Currency      string  `json:"currency"`
	OwnerPersonID *string `json:"owner_person_id"`
}

func (req budgetReq) input() (BudgetInput, error) {
	var in BudgetInput
	var err error
	in.Scope = BudgetScope(trim(req.Scope))
	in.ScopeRef = req.ScopeRef
	if in.PeriodStart, err = parseDateField(req.PeriodStart, "period_start"); err != nil {
		return in, err
	}
	if in.PeriodEnd, err = parseDateField(req.PeriodEnd, "period_end"); err != nil {
		return in, err
	}
	in.CategoryKey = req.CategoryKey
	if req.AmountMinor != nil {
		in.AmountMinor = *req.AmountMinor
	}
	in.Currency = req.Currency
	if in.OwnerPersonID, err = optUUID(req.OwnerPersonID, "owner_person_id"); err != nil {
		return in, err
	}
	return in, nil
}

// CreateBudget handles POST /api/v1/finance/budgets.
func (h *Handler) CreateBudget(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	var req budgetReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		h.writeError(w, r, err)
		return
	}
	in, err := req.input()
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	b, err := h.service.CreateBudget(r.Context(), principal.PersonID, in)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.SetETag(w, int64(b.Version))
	httpx.WriteJSON(w, http.StatusCreated, budgetOf(b, principal.Can(sensitiveAmount)))
}

// UpdateBudget handles PUT /api/v1/finance/budgets/{id}.
func (h *Handler) UpdateBudget(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	id, ok := h.idParam(w, r, "id", "budget")
	if !ok {
		return
	}
	current, err := h.service.Budget(r.Context(), id)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	if err := httpx.RequireIfMatch(r, int64(current.Version)); err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	var req budgetReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		h.writeError(w, r, err)
		return
	}
	in, err := req.input()
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	b, err := h.service.UpdateBudget(r.Context(), principal.PersonID, id, current.Version, in)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.SetETag(w, int64(b.Version))
	httpx.WriteJSON(w, http.StatusOK, budgetOf(b, principal.Can(sensitiveAmount)))
}

// ApproveBudget handles POST /api/v1/finance/budgets/{id}/approve.
func (h *Handler) ApproveBudget(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	id, ok := h.idParam(w, r, "id", "budget")
	if !ok {
		return
	}
	current, err := h.service.Budget(r.Context(), id)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	if err := httpx.RequireIfMatch(r, int64(current.Version)); err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	b, err := h.service.ApproveBudget(r.Context(), principal.PersonID, id, current.Version)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.SetETag(w, int64(b.Version))
	httpx.WriteJSON(w, http.StatusOK, budgetOf(b, principal.Can(sensitiveAmount)))
}

// CloseBudget handles POST /api/v1/finance/budgets/{id}/close.
func (h *Handler) CloseBudget(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	id, ok := h.idParam(w, r, "id", "budget")
	if !ok {
		return
	}
	current, err := h.service.Budget(r.Context(), id)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	if err := httpx.RequireIfMatch(r, int64(current.Version)); err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	b, err := h.service.CloseBudget(r.Context(), principal.PersonID, id, current.Version)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.SetETag(w, int64(b.Version))
	httpx.WriteJSON(w, http.StatusOK, budgetOf(b, principal.Can(sensitiveAmount)))
}

// ── account writes ──────────────────────────────────────────────────────────

type accountReq struct {
	Name                string `json:"name"`
	AccountType         string `json:"account_type"`
	Currency            string `json:"currency"`
	OpeningBalanceMinor *int64 `json:"opening_balance_minor"`
	Active              *bool  `json:"active"`
}

func (req accountReq) input() AccountInput {
	return AccountInput{
		Name: req.Name, AccountType: AccountType(trim(req.AccountType)), Currency: req.Currency,
		OpeningBalanceMinor: req.OpeningBalanceMinor, Active: req.Active,
	}
}

// CreateAccount handles POST /api/v1/finance/accounts.
func (h *Handler) CreateAccount(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	var req accountReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		h.writeError(w, r, err)
		return
	}
	a, err := h.service.CreateAccount(r.Context(), principal.PersonID, req.input())
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.SetETag(w, int64(a.Version))
	httpx.WriteJSON(w, http.StatusCreated, accountOf(a, principal.Can(sensitiveAmount)))
}

// UpdateAccount handles PUT /api/v1/finance/accounts/{id}.
func (h *Handler) UpdateAccount(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	id, ok := h.idParam(w, r, "id", "account")
	if !ok {
		return
	}
	current, err := h.service.Account(r.Context(), id)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	if err := httpx.RequireIfMatch(r, int64(current.Version)); err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	var req accountReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		h.writeError(w, r, err)
		return
	}
	a, err := h.service.UpdateAccount(r.Context(), principal.PersonID, id, current.Version, req.input())
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.SetETag(w, int64(a.Version))
	httpx.WriteJSON(w, http.StatusOK, accountOf(a, principal.Can(sensitiveAmount)))
}

// ── recurring-cost writes ───────────────────────────────────────────────────

type recurringCostReq struct {
	Vendor        string  `json:"vendor"`
	Description   string  `json:"description"`
	AmountMinor   *int64  `json:"amount_minor"`
	Currency      string  `json:"currency"`
	Cycle         string  `json:"cycle"`
	NextRenewalOn string  `json:"next_renewal_on"`
	OwnerPersonID *string `json:"owner_person_id"`
	Active        *bool   `json:"active"`
}

func (req recurringCostReq) input() (RecurringCostInput, error) {
	var in RecurringCostInput
	var err error
	in.Vendor = req.Vendor
	in.Description = req.Description
	if req.AmountMinor != nil {
		in.AmountMinor = *req.AmountMinor
	}
	in.Currency = req.Currency
	in.Cycle = RecurringCycle(trim(req.Cycle))
	if in.NextRenewalOn, err = parseDateField(req.NextRenewalOn, "next_renewal_on"); err != nil {
		return in, err
	}
	if in.OwnerPersonID, err = optUUID(req.OwnerPersonID, "owner_person_id"); err != nil {
		return in, err
	}
	in.Active = req.Active
	return in, nil
}

// CreateRecurringCost handles POST /api/v1/finance/recurring-costs.
func (h *Handler) CreateRecurringCost(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	var req recurringCostReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		h.writeError(w, r, err)
		return
	}
	in, err := req.input()
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	c, err := h.service.CreateRecurringCost(r.Context(), principal.PersonID, in)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.SetETag(w, int64(c.Version))
	httpx.WriteJSON(w, http.StatusCreated, recurringCostOf(c, principal.Can(sensitiveAmount)))
}

// UpdateRecurringCost handles PUT /api/v1/finance/recurring-costs/{id}.
func (h *Handler) UpdateRecurringCost(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	id, ok := h.idParam(w, r, "id", "recurring cost")
	if !ok {
		return
	}
	current, err := h.service.RecurringCost(r.Context(), id)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	if err := httpx.RequireIfMatch(r, int64(current.Version)); err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	var req recurringCostReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		h.writeError(w, r, err)
		return
	}
	in, err := req.input()
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	c, err := h.service.UpdateRecurringCost(r.Context(), principal.PersonID, id, current.Version, in)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.SetETag(w, int64(c.Version))
	httpx.WriteJSON(w, http.StatusOK, recurringCostOf(c, principal.Can(sensitiveAmount)))
}

// ── helpers ─────────────────────────────────────────────────────────────────

func (h *Handler) principal(w http.ResponseWriter, r *http.Request) (*authz.Principal, bool) {
	principal, ok := authz.PrincipalFrom(r.Context())
	if !ok || !principal.IsUser() {
		httpx.WriteProblem(w, r, httpx.Unauthenticated("Authentication is required."))
		return nil, false
	}
	return principal, true
}

func (h *Handler) idParam(w http.ResponseWriter, r *http.Request, param, what string) (uuid.UUID, bool) {
	id, err := uuid.Parse(trim(chi.URLParam(r, param)))
	if err != nil {
		httpx.WriteProblem(w, r, httpx.BadRequest("id is not a valid "+what+" id."))
		return uuid.Nil, false
	}
	return id, true
}

// uuidParam reads an optional uuid query parameter. An absent or empty value is
// nil (no filter); a present but malformed one is a validation error.
func (h *Handler) uuidParam(w http.ResponseWriter, r *http.Request, key string) (*uuid.UUID, bool) {
	raw := trim(r.URL.Query().Get(key))
	if raw == "" {
		return nil, true
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		httpx.WriteProblem(w, r, httpx.BadRequest(key+" is not a valid id."))
		return nil, false
	}
	return &id, true
}

// timeParam reads an optional date query parameter. Both a calendar date and an
// RFC 3339 instant are accepted, matching how clients send them.
func (h *Handler) timeParam(w http.ResponseWriter, r *http.Request, key string) (*time.Time, bool) {
	raw := trim(r.URL.Query().Get(key))
	if raw == "" {
		return nil, true
	}
	if ts, err := time.Parse(time.RFC3339, raw); err == nil {
		ts = ts.UTC()
		return &ts, true
	}
	if d, err := time.Parse("2006-01-02", raw); err == nil {
		return &d, true
	}
	httpx.WriteProblem(w, r, httpx.BadRequest(key+" is not a valid date."))
	return nil, false
}

// boolParam reads an optional boolean query parameter.
func (h *Handler) boolParam(w http.ResponseWriter, r *http.Request, key string) (bool, bool) {
	raw := trim(r.URL.Query().Get(key))
	if raw == "" {
		return false, true
	}
	v, err := strconv.ParseBool(raw)
	if err != nil {
		httpx.WriteProblem(w, r, httpx.BadRequest(key+" must be true or false."))
		return false, false
	}
	return v, true
}

func (h *Handler) writeError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		httpx.WriteProblem(w, r, httpx.NotFound("No such finance record."))
	case errors.Is(err, ErrInvalid):
		// A lifecycle refusal carries a stable code so the client can branch on
		// it; a plain validation failure keeps the generic code.
		code := problemCode(err)
		if code == "" {
			code = httpx.CodeValidationFailed
		}
		httpx.WriteProblem(w, r, httpx.New(http.StatusBadRequest, code, err.Error()))
	case errors.Is(err, ErrForbidden):
		httpx.WriteProblem(w, r, httpx.Forbidden(err.Error()))
	case errors.Is(err, ErrDuplicate):
		httpx.WriteProblem(w, r, httpx.Conflict(err.Error()))
	case errors.Is(err, ErrConflict):
		httpx.WriteProblem(w, r, httpx.PreconditionFailed(err.Error()))
	default:
		httpx.WriteError(w, r, h.logger, err)
	}
}

// optUUID parses an optional uuid field. Absent or blank is nil; malformed is a
// validation error naming the field.
func optUUID(raw *string, field string) (*uuid.UUID, error) {
	if raw == nil || trim(*raw) == "" {
		return nil, nil
	}
	id, err := uuid.Parse(trim(*raw))
	if err != nil {
		return nil, errInvalidField(field)
	}
	return &id, nil
}

// reqUUID parses a required uuid field.
func reqUUID(raw, field string) (uuid.UUID, error) {
	id, err := uuid.Parse(trim(raw))
	if err != nil {
		return uuid.Nil, errInvalidField(field)
	}
	return id, nil
}

// parseDateField parses a calendar date (2026-10-19) or an RFC 3339 instant,
// matching how the web sends date inputs and how the API accepts instants.
func parseDateField(raw, field string) (time.Time, error) {
	raw = trim(raw)
	if raw == "" {
		return time.Time{}, errInvalidField(field)
	}
	if d, err := time.Parse("2006-01-02", raw); err == nil {
		return d, nil
	}
	ts, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}, errInvalidField(field)
	}
	return time.Date(ts.Year(), ts.Month(), ts.Day(), 0, 0, 0, 0, time.UTC), nil
}

// optDateField parses an optional date field.
func optDateField(raw *string, field string) (*time.Time, error) {
	if raw == nil || trim(*raw) == "" {
		return nil, nil
	}
	d, err := parseDateField(*raw, field)
	if err != nil {
		return nil, err
	}
	return &d, nil
}

// decodeOptionalJSON decodes a body only when one was sent, so an action with
// optional fields (issue, dispute) can also be called with no body at all.
func decodeOptionalJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	if r.ContentLength == 0 && !httpx.IsJSONContentType(r.Header.Get("Content-Type")) {
		return nil
	}
	return httpx.DecodeJSON(w, r, dst)
}

func errInvalidField(field string) error { return &fieldError{field: field} }

type fieldError struct{ field string }

func (e *fieldError) Error() string { return "invalid " + e.field }
func (e *fieldError) Is(target error) bool {
	return target == ErrInvalid
}

func cursorOrNil(cursor string) (*Cursor, error) {
	if cursor == "" {
		return nil, nil
	}
	c, err := decodeCursor(cursor)
	if err != nil {
		return nil, errInvalidField("cursor")
	}
	return c, nil
}

func encodeCursor(at time.Time, id uuid.UUID) string {
	return strconv.FormatInt(at.UTC().UnixNano(), 10) + ":" + id.String()
}

func decodeCursor(encoded string) (*Cursor, error) {
	parts := strings.SplitN(encoded, ":", 2)
	if len(parts) != 2 {
		return nil, errors.New("finance: malformed cursor")
	}
	nanos, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return nil, err
	}
	id, err := uuid.Parse(parts[1])
	if err != nil {
		return nil, err
	}
	return &Cursor{At: time.Unix(0, nanos).UTC(), ID: id}, nil
}
