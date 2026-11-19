package contracts

import (
	"encoding/json"
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

// Handler serves the contracts HTTP surface (§8.3).
type Handler struct {
	service *Service
	logger  *slog.Logger
}

// NewHandler builds a contracts handler.
func NewHandler(service *Service, logger *slog.Logger) *Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Handler{service: service, logger: logger}
}

// ── views ───────────────────────────────────────────────────────────────────

type contractView struct {
	ID             string          `json:"id"`
	Ref            string          `json:"ref"`
	OrganizationID string          `json:"organization_id"`
	OpportunityID  string          `json:"opportunity_id,omitempty"`
	ContractType   string          `json:"contract_type"`
	Status         string          `json:"status"`
	ValueMinor     *int64          `json:"value_minor,omitempty"`
	Currency       string          `json:"currency"`
	StartAt        string          `json:"start_at,omitempty"`
	EndAt          string          `json:"end_at,omitempty"`
	PaymentTerms   string          `json:"payment_terms,omitempty"`
	RenewalTerms   string          `json:"renewal_terms,omitempty"`
	GoverningDocs  json.RawMessage `json:"governing_docs"`
	Signatories    json.RawMessage `json:"signatories"`
	SignedAt       string          `json:"signed_at,omitempty"`
	Version        int             `json:"version"`
	CreatedAt      string          `json:"created_at"`
	UpdatedAt      string          `json:"updated_at"`
}

type milestoneView struct {
	ID                 string `json:"id"`
	ContractID         string `json:"contract_id"`
	Title              string `json:"title"`
	DueAt              string `json:"due_at,omitempty"`
	AmountMinor        *int64 `json:"amount_minor,omitempty"`
	Currency           string `json:"currency"`
	ProjectMilestoneID string `json:"project_milestone_id,omitempty"`
	Version            int    `json:"version"`
}

type deliverableView struct {
	ID                 string `json:"id"`
	ContractID         string `json:"contract_id"`
	Description        string `json:"description"`
	AcceptanceCriteria string `json:"acceptance_criteria,omitempty"`
	MilestoneID        string `json:"milestone_id,omitempty"`
	Version            int    `json:"version"`
}

type obligationView struct {
	ID            string `json:"id"`
	ContractID    string `json:"contract_id"`
	Party         string `json:"party"`
	Description   string `json:"description"`
	DueAt         string `json:"due_at,omitempty"`
	Recurrence    string `json:"recurrence,omitempty"`
	Status        string `json:"status"`
	OwnerPersonID string `json:"owner_person_id,omitempty"`
	Version       int    `json:"version"`
}

type slaTermView struct {
	ID                string   `json:"id"`
	ContractID        string   `json:"contract_id"`
	Metric            string   `json:"metric"`
	Target            string   `json:"target"`
	MeasurementWindow string   `json:"measurement_window,omitempty"`
	SystemIDs         []string `json:"system_ids"`
	Version           int      `json:"version"`
}

type changeRequestView struct {
	ID                 string `json:"id"`
	Ref                string `json:"ref"`
	ContractID         string `json:"contract_id"`
	RequestedBy        string `json:"requested_by,omitempty"`
	Description        string `json:"description"`
	EffortImpact       string `json:"effort_impact,omitempty"`
	PriceImpactMinor   *int64 `json:"price_impact_minor,omitempty"`
	Currency           string `json:"currency"`
	DeadlineImpactDays *int   `json:"deadline_impact_days,omitempty"`
	Status             string `json:"status"`
	ApprovalID         string `json:"approval_id,omitempty"`
	Version            int    `json:"version"`
	CreatedAt          string `json:"created_at"`
	UpdatedAt          string `json:"updated_at"`
}

// ── converters ──────────────────────────────────────────────────────────────

func rfc(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func date(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format("2006-01-02")
}

func idStr(id *uuid.UUID) string {
	if id == nil {
		return ""
	}
	return id.String()
}

// contractOf renders a contract. When visible is false the sensitive monetary
// value is withheld (contracts.contract_value.sensitive.read).
func contractOf(c Contract, visible bool) contractView {
	v := contractView{
		ID: c.ID.String(), Ref: c.Ref, OrganizationID: c.OrganizationID.String(),
		OpportunityID: idStr(c.OpportunityID), ContractType: string(c.ContractType), Status: string(c.Status),
		Currency: c.Currency, StartAt: date(c.StartAt), EndAt: date(c.EndAt),
		PaymentTerms: c.PaymentTerms, RenewalTerms: c.RenewalTerms,
		GoverningDocs: c.GoverningDocs, Signatories: c.Signatories, SignedAt: rfcPtr(c.SignedAt),
		Version: c.Version, CreatedAt: rfc(c.CreatedAt), UpdatedAt: rfc(c.UpdatedAt),
	}
	if visible {
		v.ValueMinor = c.ValueMinor
	}
	return v
}

func rfcPtr(t *time.Time) string {
	if t == nil {
		return ""
	}
	return rfc(*t)
}

func milestoneOf(m Milestone) milestoneView {
	return milestoneView{
		ID: m.ID.String(), ContractID: m.ContractID.String(), Title: m.Title, DueAt: date(m.DueAt),
		AmountMinor: m.AmountMinor, Currency: m.Currency, ProjectMilestoneID: idStr(m.ProjectMilestoneID),
		Version: m.Version,
	}
}

func deliverableOf(d Deliverable) deliverableView {
	return deliverableView{
		ID: d.ID.String(), ContractID: d.ContractID.String(), Description: d.Description,
		AcceptanceCriteria: d.AcceptanceCriteria, MilestoneID: idStr(d.MilestoneID), Version: d.Version,
	}
}

func obligationOf(o Obligation) obligationView {
	return obligationView{
		ID: o.ID.String(), ContractID: o.ContractID.String(), Party: string(o.Party),
		Description: o.Description, DueAt: date(o.DueAt), Recurrence: o.Recurrence,
		Status: string(o.Status), OwnerPersonID: idStr(o.OwnerPersonID), Version: o.Version,
	}
}

func slaTermOf(t SLATerm) slaTermView {
	ids := make([]string, 0, len(t.SystemIDs))
	for _, id := range t.SystemIDs {
		ids = append(ids, id.String())
	}
	return slaTermView{
		ID: t.ID.String(), ContractID: t.ContractID.String(), Metric: t.Metric, Target: t.Target,
		MeasurementWindow: t.MeasurementWindow, SystemIDs: ids, Version: t.Version,
	}
}

func changeRequestOf(c ChangeRequest, visible bool) changeRequestView {
	v := changeRequestView{
		ID: c.ID.String(), Ref: c.Ref, ContractID: c.ContractID.String(), RequestedBy: c.RequestedBy,
		Description: c.Description, EffortImpact: c.EffortImpact, Currency: c.Currency,
		DeadlineImpactDays: c.DeadlineImpactDays, Status: string(c.Status), ApprovalID: idStr(c.ApprovalID),
		Version: c.Version, CreatedAt: rfc(c.CreatedAt), UpdatedAt: rfc(c.UpdatedAt),
	}
	if visible {
		v.PriceImpactMinor = c.PriceImpactMinor
	}
	return v
}

// ── contracts ───────────────────────────────────────────────────────────────

// List handles GET /api/v1/contracts.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
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
	var status *ContractStatus
	if raw := strings.TrimSpace(r.URL.Query().Get("status")); raw != "" {
		s := ContractStatus(raw)
		status = &s
	}
	var orgID *uuid.UUID
	if raw := strings.TrimSpace(r.URL.Query().Get("organization_id")); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			httpx.WriteProblem(w, r, httpx.BadRequest("organization_id is not a valid id."))
			return
		}
		orgID = &id
	}
	items, err := h.service.ListContracts(r.Context(), status, orgID, limit+1, before)
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
	visible := principal.Can("contracts.contract_value.sensitive.read")
	views := make([]contractView, 0, len(items))
	for _, c := range items {
		views = append(views, contractOf(c, visible))
	}
	httpx.WriteJSON(w, http.StatusOK, httpx.NewPage(views, next, limit))
}

// Get handles GET /api/v1/contracts/{id}.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	id, ok := h.idParam(w, r, "id", "contract")
	if !ok {
		return
	}
	c, err := h.service.GetContract(r.Context(), id)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.SetETag(w, int64(c.Version))
	httpx.WriteJSON(w, http.StatusOK, contractOf(c, principal.Can("contracts.contract_value.sensitive.read")))
}

// Create handles POST /api/v1/contracts.
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	var req contractReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		h.writeError(w, r, err)
		return
	}
	in, err := toContractInput(req)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	c, err := h.service.CreateContract(r.Context(), principal.PersonID, in)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.SetETag(w, int64(c.Version))
	httpx.WriteJSON(w, http.StatusCreated, contractOf(c, principal.Can("contracts.contract_value.sensitive.read")))
}

// Update handles PUT /api/v1/contracts/{id}.
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	id, ok := h.idParam(w, r, "id", "contract")
	if !ok {
		return
	}
	current, err := h.service.GetContract(r.Context(), id)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	if err := httpx.RequireIfMatch(r, int64(current.Version)); err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	var req contractReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		h.writeError(w, r, err)
		return
	}
	in, err := toContractInput(req)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	c, err := h.service.UpdateContract(r.Context(), principal.PersonID, id, current.Version, in)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.SetETag(w, int64(c.Version))
	httpx.WriteJSON(w, http.StatusOK, contractOf(c, principal.Can("contracts.contract_value.sensitive.read")))
}

// Submit handles POST /api/v1/contracts/{id}/submit.
func (h *Handler) Submit(w http.ResponseWriter, r *http.Request) { h.lifecycle(w, r, "submit") }

// Approve handles POST /api/v1/contracts/{id}/approve.
func (h *Handler) Approve(w http.ResponseWriter, r *http.Request) { h.lifecycle(w, r, "approve") }

// Send handles POST /api/v1/contracts/{id}/send.
func (h *Handler) Send(w http.ResponseWriter, r *http.Request) { h.lifecycle(w, r, "send") }

// Activate handles POST /api/v1/contracts/{id}/activate.
func (h *Handler) Activate(w http.ResponseWriter, r *http.Request) { h.lifecycle(w, r, "activate") }

func (h *Handler) lifecycle(w http.ResponseWriter, r *http.Request, action string) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	id, ok := h.idParam(w, r, "id", "contract")
	if !ok {
		return
	}
	current, err := h.service.GetContract(r.Context(), id)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	if err := httpx.RequireIfMatch(r, int64(current.Version)); err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	var c Contract
	switch action {
	case "submit":
		c, err = h.service.Submit(r.Context(), principal.PersonID, id, current.Version)
	case "approve":
		c, err = h.service.Approve(r.Context(), principal.PersonID, id, current.Version)
	case "send":
		c, err = h.service.Send(r.Context(), principal.PersonID, id, current.Version)
	case "activate":
		c, err = h.service.Activate(r.Context(), principal.PersonID, id, current.Version)
	}
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.SetETag(w, int64(c.Version))
	httpx.WriteJSON(w, http.StatusOK, contractOf(c, principal.Can("contracts.contract_value.sensitive.read")))
}

// Terminate handles POST /api/v1/contracts/{id}/terminate.
func (h *Handler) Terminate(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	id, ok := h.idParam(w, r, "id", "contract")
	if !ok {
		return
	}
	current, err := h.service.GetContract(r.Context(), id)
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
	c, err := h.service.Terminate(r.Context(), principal.PersonID, id, current.Version, req.Reason)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.SetETag(w, int64(c.Version))
	httpx.WriteJSON(w, http.StatusOK, contractOf(c, principal.Can("contracts.contract_value.sensitive.read")))
}

// Sign handles POST /api/v1/contracts/{id}/sign.
func (h *Handler) Sign(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	id, ok := h.idParam(w, r, "id", "contract")
	if !ok {
		return
	}
	current, err := h.service.GetContract(r.Context(), id)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	if err := httpx.RequireIfMatch(r, int64(current.Version)); err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	var req signReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		h.writeError(w, r, err)
		return
	}
	c, err := h.service.Sign(r.Context(), principal.PersonID, id, current.Version,
		SignInput{Name: req.Name, Role: req.Role, Method: req.Method})
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.SetETag(w, int64(c.Version))
	httpx.WriteJSON(w, http.StatusOK, contractOf(c, principal.Can("contracts.contract_value.sensitive.read")))
}

// ── milestones ──────────────────────────────────────────────────────────────

// ListMilestones handles GET /api/v1/contracts/{id}/milestones.
func (h *Handler) ListMilestones(w http.ResponseWriter, r *http.Request) {
	id, ok := h.idParam(w, r, "id", "contract")
	if !ok {
		return
	}
	items, err := h.service.ListMilestones(r.Context(), id)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	views := make([]milestoneView, 0, len(items))
	for _, m := range items {
		views = append(views, milestoneOf(m))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": views})
}

// CreateMilestone handles POST /api/v1/contracts/{id}/milestones.
func (h *Handler) CreateMilestone(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	id, ok := h.idParam(w, r, "id", "contract")
	if !ok {
		return
	}
	var req milestoneReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		h.writeError(w, r, err)
		return
	}
	in, err := toMilestoneInput(req)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	m, err := h.service.CreateMilestone(r.Context(), principal.PersonID, id, in)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, milestoneOf(m))
}

// ── deliverables ────────────────────────────────────────────────────────────

// ListDeliverables handles GET /api/v1/contracts/{id}/deliverables.
func (h *Handler) ListDeliverables(w http.ResponseWriter, r *http.Request) {
	id, ok := h.idParam(w, r, "id", "contract")
	if !ok {
		return
	}
	items, err := h.service.ListDeliverables(r.Context(), id)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	views := make([]deliverableView, 0, len(items))
	for _, d := range items {
		views = append(views, deliverableOf(d))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": views})
}

// CreateDeliverable handles POST /api/v1/contracts/{id}/deliverables.
func (h *Handler) CreateDeliverable(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	id, ok := h.idParam(w, r, "id", "contract")
	if !ok {
		return
	}
	var req deliverableReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		h.writeError(w, r, err)
		return
	}
	milestone, err := optUUID(req.MilestoneID, "milestone_id")
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	d, err := h.service.CreateDeliverable(r.Context(), principal.PersonID, id, DeliverableInput{
		Description: req.Description, AcceptanceCriteria: req.AcceptanceCriteria, MilestoneID: milestone,
	})
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, deliverableOf(d))
}

// ── obligations ─────────────────────────────────────────────────────────────

// ListObligations handles GET /api/v1/contracts/{id}/obligations.
func (h *Handler) ListObligations(w http.ResponseWriter, r *http.Request) {
	id, ok := h.idParam(w, r, "id", "contract")
	if !ok {
		return
	}
	items, err := h.service.ListObligations(r.Context(), id)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	views := make([]obligationView, 0, len(items))
	for _, o := range items {
		views = append(views, obligationOf(o))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": views})
}

// CreateObligation handles POST /api/v1/contracts/{id}/obligations.
func (h *Handler) CreateObligation(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	id, ok := h.idParam(w, r, "id", "contract")
	if !ok {
		return
	}
	var req obligationReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		h.writeError(w, r, err)
		return
	}
	owner, err := optUUID(req.OwnerPersonID, "owner_person_id")
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	due, err := optTime(req.DueAt, "due_at")
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	o, err := h.service.CreateObligation(r.Context(), principal.PersonID, id, ObligationInput{
		Party: ObligationParty(strings.TrimSpace(req.Party)), Description: req.Description,
		DueAt: due, Recurrence: req.Recurrence, OwnerPersonID: owner,
	})
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, obligationOf(o))
}

// ── SLA terms ───────────────────────────────────────────────────────────────

// ListSLATerms handles GET /api/v1/contracts/{id}/sla-terms.
func (h *Handler) ListSLATerms(w http.ResponseWriter, r *http.Request) {
	id, ok := h.idParam(w, r, "id", "contract")
	if !ok {
		return
	}
	items, err := h.service.ListSLATerms(r.Context(), id)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	views := make([]slaTermView, 0, len(items))
	for _, t := range items {
		views = append(views, slaTermOf(t))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": views})
}

// CreateSLATerm handles POST /api/v1/contracts/{id}/sla-terms.
func (h *Handler) CreateSLATerm(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	id, ok := h.idParam(w, r, "id", "contract")
	if !ok {
		return
	}
	var req slaTermReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		h.writeError(w, r, err)
		return
	}
	ids := make([]uuid.UUID, 0, len(req.SystemIDs))
	for _, raw := range req.SystemIDs {
		sid, err := uuid.Parse(strings.TrimSpace(raw))
		if err != nil {
			httpx.WriteProblem(w, r, httpx.BadRequest("system_ids contains an invalid id."))
			return
		}
		ids = append(ids, sid)
	}
	t, err := h.service.CreateSLATerm(r.Context(), principal.PersonID, id, SLATermInput{
		Metric: req.Metric, Target: req.Target, MeasurementWindow: req.MeasurementWindow, SystemIDs: ids,
	})
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, slaTermOf(t))
}

// ── change requests ─────────────────────────────────────────────────────────

// ListChangeRequests handles GET /api/v1/contracts/{id}/change-requests.
func (h *Handler) ListChangeRequests(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	id, ok := h.idParam(w, r, "id", "contract")
	if !ok {
		return
	}
	items, err := h.service.ListChangeRequests(r.Context(), id)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	visible := principal.Can("contracts.contract_value.sensitive.read")
	views := make([]changeRequestView, 0, len(items))
	for _, c := range items {
		views = append(views, changeRequestOf(c, visible))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": views})
}

// CreateChangeRequest handles POST /api/v1/contracts/{id}/change-requests.
func (h *Handler) CreateChangeRequest(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	id, ok := h.idParam(w, r, "id", "contract")
	if !ok {
		return
	}
	var req changeRequestReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		h.writeError(w, r, err)
		return
	}
	c, err := h.service.CreateChangeRequest(r.Context(), principal.PersonID, id, ChangeRequestInput{
		RequestedBy: req.RequestedBy, Description: req.Description, EffortImpact: req.EffortImpact,
		PriceImpactMinor: req.PriceImpactMinor, Currency: req.Currency, DeadlineImpactDays: req.DeadlineImpactDays,
	})
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, changeRequestOf(c, principal.Can("contracts.contract_value.sensitive.read")))
}

// ApproveChangeRequest handles POST /api/v1/change-requests/{id}/approve.
func (h *Handler) ApproveChangeRequest(w http.ResponseWriter, r *http.Request) {
	h.decideChangeRequest(w, r, true)
}

// RejectChangeRequest handles POST /api/v1/change-requests/{id}/reject.
func (h *Handler) RejectChangeRequest(w http.ResponseWriter, r *http.Request) {
	h.decideChangeRequest(w, r, false)
}

func (h *Handler) decideChangeRequest(w http.ResponseWriter, r *http.Request, approve bool) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	id, ok := h.idParam(w, r, "id", "change request")
	if !ok {
		return
	}
	current, err := h.service.getChangeRequest(r.Context(), id)
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
	var cr ChangeRequest
	if approve {
		cr, err = h.service.ApproveChangeRequest(r.Context(), principal.PersonID, id, current.Version, req.ApprovalID)
	} else {
		cr, err = h.service.RejectChangeRequest(r.Context(), principal.PersonID, id, current.Version, req.Reason)
	}
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.SetETag(w, int64(cr.Version))
	httpx.WriteJSON(w, http.StatusOK, changeRequestOf(cr, principal.Can("contracts.contract_value.sensitive.read")))
}

// ── request bodies ──────────────────────────────────────────────────────────

type contractReq struct {
	OrganizationID string          `json:"organization_id"`
	OpportunityID  *string         `json:"opportunity_id"`
	ContractType   string          `json:"contract_type"`
	ValueMinor     *int64          `json:"value_minor"`
	Currency       string          `json:"currency"`
	StartAt        *string         `json:"start_at"`
	EndAt          *string         `json:"end_at"`
	PaymentTerms   string          `json:"payment_terms"`
	RenewalTerms   string          `json:"renewal_terms"`
	GoverningDocs  json.RawMessage `json:"governing_docs"`
}

func toContractInput(req contractReq) (ContractInput, error) {
	org, err := uuid.Parse(strings.TrimSpace(req.OrganizationID))
	if err != nil {
		return ContractInput{}, errInvalidField("organization_id")
	}
	in := ContractInput{
		OrganizationID: org, ContractType: ContractType(strings.TrimSpace(req.ContractType)),
		ValueMinor: req.ValueMinor, Currency: req.Currency,
		PaymentTerms: req.PaymentTerms, RenewalTerms: req.RenewalTerms, GoverningDocs: req.GoverningDocs,
	}
	if in.OpportunityID, err = optUUID(req.OpportunityID, "opportunity_id"); err != nil {
		return in, err
	}
	if in.StartAt, err = optTime(req.StartAt, "start_at"); err != nil {
		return in, err
	}
	if in.EndAt, err = optTime(req.EndAt, "end_at"); err != nil {
		return in, err
	}
	return in, nil
}

type milestoneReq struct {
	Title              string  `json:"title"`
	DueAt              *string `json:"due_at"`
	AmountMinor        *int64  `json:"amount_minor"`
	Currency           string  `json:"currency"`
	ProjectMilestoneID *string `json:"project_milestone_id"`
}

func toMilestoneInput(req milestoneReq) (MilestoneInput, error) {
	in := MilestoneInput{Title: req.Title, AmountMinor: req.AmountMinor, Currency: req.Currency}
	var err error
	if in.DueAt, err = optTime(req.DueAt, "due_at"); err != nil {
		return in, err
	}
	if in.ProjectMilestoneID, err = optUUID(req.ProjectMilestoneID, "project_milestone_id"); err != nil {
		return in, err
	}
	return in, nil
}

type deliverableReq struct {
	Description        string  `json:"description"`
	AcceptanceCriteria string  `json:"acceptance_criteria"`
	MilestoneID        *string `json:"milestone_id"`
}

type obligationReq struct {
	Party         string  `json:"party"`
	Description   string  `json:"description"`
	DueAt         *string `json:"due_at"`
	Recurrence    string  `json:"recurrence"`
	OwnerPersonID *string `json:"owner_person_id"`
}

type slaTermReq struct {
	Metric            string   `json:"metric"`
	Target            string   `json:"target"`
	MeasurementWindow string   `json:"measurement_window"`
	SystemIDs         []string `json:"system_ids"`
}

type changeRequestReq struct {
	RequestedBy        string `json:"requested_by"`
	Description        string `json:"description"`
	EffortImpact       string `json:"effort_impact"`
	PriceImpactMinor   *int64 `json:"price_impact_minor"`
	Currency           string `json:"currency"`
	DeadlineImpactDays *int   `json:"deadline_impact_days"`
}

type signReq struct {
	Name   string `json:"name"`
	Role   string `json:"role"`
	Method string `json:"method"`
}

type reasonReq struct {
	Reason string `json:"reason"`
}

type decideReq struct {
	ApprovalID *uuid.UUID `json:"approval_id"`
	Reason     string     `json:"reason"`
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
	id, err := uuid.Parse(strings.TrimSpace(chi.URLParam(r, param)))
	if err != nil {
		httpx.WriteProblem(w, r, httpx.BadRequest("id is not a valid "+what+" id."))
		return uuid.Nil, false
	}
	return id, true
}

func (h *Handler) writeError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		httpx.WriteProblem(w, r, httpx.NotFound("No such contract record."))
	case errors.Is(err, ErrInvalid):
		httpx.WriteProblem(w, r, httpx.BadRequest(err.Error()))
	case errors.Is(err, ErrForbidden):
		httpx.WriteProblem(w, r, httpx.Forbidden(err.Error()))
	case errors.Is(err, ErrConflict):
		httpx.WriteProblem(w, r, httpx.PreconditionFailed(err.Error()))
	default:
		httpx.WriteError(w, r, h.logger, err)
	}
}

func optUUID(p *string, field string) (*uuid.UUID, error) {
	if p == nil || strings.TrimSpace(*p) == "" {
		return nil, nil
	}
	id, err := uuid.Parse(strings.TrimSpace(*p))
	if err != nil {
		return nil, errInvalidField(field)
	}
	return &id, nil
}

func optTime(p *string, field string) (*time.Time, error) {
	if p == nil || strings.TrimSpace(*p) == "" {
		return nil, nil
	}
	if ts, err := time.Parse(time.RFC3339, strings.TrimSpace(*p)); err == nil {
		ts = ts.UTC()
		return &ts, nil
	}
	if d, err := time.Parse("2006-01-02", strings.TrimSpace(*p)); err == nil {
		return &d, nil
	}
	return nil, errInvalidField(field)
}

func errInvalidField(field string) error {
	return &fieldError{field: field}
}

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
		return nil, errors.New("contracts: malformed cursor")
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
