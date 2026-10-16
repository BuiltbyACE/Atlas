package approvals

import (
	"encoding/base64"
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

// Handler serves approvals HTTP endpoints.
type Handler struct {
	service *Service
	logger  *slog.Logger
}

// NewHandler builds the handler.
func NewHandler(service *Service, logger *slog.Logger) *Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Handler{service: service, logger: logger}
}

type requestApprovalReq struct {
	EntityType        string     `json:"entity_type"`
	EntityID          string     `json:"entity_id"`
	AmountMinor       int64      `json:"amount_minor"`
	Currency          string     `json:"currency"`
	ApproverRole      string     `json:"approver_role"`
	ApproverID        string     `json:"approver_id,omitempty"`
	ApprovalsRequired int        `json:"approvals_required,omitempty"`
	PolicyRef         string     `json:"policy_ref"`
	DueAt             *time.Time `json:"due_at"`
}

type decideReq struct {
	Approve bool   `json:"approve"`
	Reason  string `json:"reason"`
}

type escalateReq struct {
	ToRole     string `json:"to_role"`
	ToPersonID string `json:"to_person_id"`
	Reason     string `json:"reason"`
}

type cancelReq struct {
	Reason string `json:"reason"`
}

type approvalView struct {
	ID                string     `json:"id"`
	Ref               string     `json:"ref"`
	EntityType        string     `json:"entity_type"`
	EntityID          string     `json:"entity_id"`
	RequestedBy       string     `json:"requested_by"`
	AmountMinor       int64      `json:"amount_minor"`
	Currency          string     `json:"currency"`
	ApproverRole      string     `json:"approver_role"`
	ApproverID        string     `json:"approver_id,omitempty"`
	ApprovalsRequired int        `json:"approvals_required"`
	Status            string     `json:"status"`
	PolicyRef         string     `json:"policy_ref,omitempty"`
	DueAt             *time.Time `json:"due_at,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
}

type policyView struct {
	ID             string     `json:"id"`
	DecisionType   string     `json:"decision_type"`
	MinAmountMinor *int64     `json:"min_amount_minor,omitempty"`
	MaxAmountMinor *int64     `json:"max_amount_minor,omitempty"`
	Currency       string     `json:"currency,omitempty"`
	ApproverRule   string     `json:"approver_rule"`
	EffectiveFrom  time.Time  `json:"effective_from"`
	EffectiveTo    *time.Time `json:"effective_to,omitempty"`
}

func viewOfPolicy(p Policy) policyView {
	return policyView{
		ID:             p.ID.String(),
		DecisionType:   p.DecisionType,
		MinAmountMinor: p.MinAmountMinor,
		MaxAmountMinor: p.MaxAmountMinor,
		Currency:       p.Currency,
		ApproverRule:   p.ApproverRule,
		EffectiveFrom:  p.EffectiveFrom,
		EffectiveTo:    p.EffectiveTo,
	}
}

// RequestApproval handles POST /api/v1/approvals.
func (h *Handler) RequestApproval(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	var req requestApprovalReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		h.writeError(w, r, err)
		return
	}
	entityID, err := uuid.Parse(strings.TrimSpace(req.EntityID))
	if err != nil {
		httpx.WriteProblem(w, r, httpx.BadRequest("entity_id must be a UUID."))
		return
	}
	var approverID *uuid.UUID
	if s := strings.TrimSpace(req.ApproverID); s != "" {
		id, err := uuid.Parse(s)
		if err != nil {
			httpx.WriteProblem(w, r, httpx.BadRequest("approver_id must be a UUID."))
			return
		}
		approverID = &id
	}
	if principal.PersonID == uuid.Nil {
		httpx.WriteProblem(w, r, httpx.Unauthenticated("Authentication is required."))
		return
	}
	in := Request{
		EntityType:        strings.TrimSpace(req.EntityType),
		EntityID:          entityID,
		RequestedBy:       principal.PersonID,
		AmountMinor:       req.AmountMinor,
		Currency:          strings.ToUpper(strings.TrimSpace(req.Currency)),
		ApproverRole:      strings.TrimSpace(req.ApproverRole),
		ApproverID:        approverID,
		ApprovalsRequired: req.ApprovalsRequired,
		PolicyRef:         strings.TrimSpace(req.PolicyRef),
		DueAt:             req.DueAt,
	}
	a, err := h.service.Request(r.Context(), in, &principal.PersonID)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, viewOf(*a))
}

// Get handles GET /api/v1/approvals/{id}.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	_, ok := h.principal(w, r)
	if !ok {
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.WriteProblem(w, r, httpx.BadRequest("id must be a UUID."))
		return
	}
	a, err := h.service.Get(r.Context(), id)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, viewOf(*a))
}

// PendingForRole handles GET /api/v1/approvals/pending?role=...
func (h *Handler) PendingForRole(w http.ResponseWriter, r *http.Request) {
	_, ok := h.principal(w, r)
	if !ok {
		return
	}
	role := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("role")))
	if role == "" {
		httpx.WriteProblem(w, r, httpx.BadRequest("role is required."))
		return
	}
	limit, cursor, err := httpx.ParsePage(r)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	var before *Cursor
	if cursor != "" {
		before, err = decodeCursor(cursor)
		if err != nil {
			httpx.WriteProblem(w, r, httpx.BadRequest("cursor is not valid."))
			return
		}
	}
	items, err := h.service.ListPendingForRole(r.Context(), role, limit+1, before)
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
	views := make([]approvalView, 0, len(items))
	for _, a := range items {
		views = append(views, viewOf(a))
	}
	httpx.WriteJSON(w, http.StatusOK, httpx.NewPage(views, next, limit))
}

// MyRequested handles GET /api/v1/approvals/mine.
func (h *Handler) MyRequested(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	limit, cursor, err := httpx.ParsePage(r)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	var before *Cursor
	if cursor != "" {
		before, err = decodeCursor(cursor)
		if err != nil {
			httpx.WriteProblem(w, r, httpx.BadRequest("cursor is not valid."))
			return
		}
	}
	items, err := h.service.ListRequestedBy(r.Context(), principal.PersonID, limit+1, before)
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
	views := make([]approvalView, 0, len(items))
	for _, a := range items {
		views = append(views, viewOf(a))
	}
	httpx.WriteJSON(w, http.StatusOK, httpx.NewPage(views, next, limit))
}

// ListPolicies handles GET /api/v1/approval-policies (§3.3). Policies are
// configuration; a change to them is itself an approval, so this is read-only.
func (h *Handler) ListPolicies(w http.ResponseWriter, r *http.Request) {
	limit, cursor, err := httpx.ParsePage(r)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	var before *Cursor
	if cursor != "" {
		before, err = decodeCursor(cursor)
		if err != nil {
			httpx.WriteProblem(w, r, httpx.BadRequest("cursor is not valid."))
			return
		}
	}
	items, err := h.service.ListPolicies(r.Context(), limit+1, before)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	next := ""
	if len(items) > limit {
		items = items[:limit]
		last := items[len(items)-1]
		next = encodeCursor(last.EffectiveFrom, last.ID)
	}
	views := make([]policyView, 0, len(items))
	for _, p := range items {
		views = append(views, viewOfPolicy(p))
	}
	httpx.WriteJSON(w, http.StatusOK, httpx.NewPage(views, next, limit))
}

// Approve handles POST /api/v1/approvals/{id}/approve.
func (h *Handler) Approve(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.WriteProblem(w, r, httpx.BadRequest("id must be a UUID."))
		return
	}
	var req decideReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		h.writeError(w, r, err)
		return
	}
	if err := h.service.Decide(r.Context(), Decide{ApprovalID: id, ActorID: principal.PersonID, Approve: true, Reason: req.Reason}); err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "approved"})
}

// Decline handles POST /api/v1/approvals/{id}/decline.
func (h *Handler) Decline(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.WriteProblem(w, r, httpx.BadRequest("id must be a UUID."))
		return
	}
	var req decideReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		h.writeError(w, r, err)
		return
	}
	if err := h.service.Decide(r.Context(), Decide{ApprovalID: id, ActorID: principal.PersonID, Approve: false, Reason: req.Reason}); err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "declined"})
}

// Escalate handles POST /api/v1/approvals/{id}/escalate.
func (h *Handler) Escalate(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.WriteProblem(w, r, httpx.BadRequest("id must be a UUID."))
		return
	}
	var req escalateReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		h.writeError(w, r, err)
		return
	}
	var toPerson *uuid.UUID
	if s := strings.TrimSpace(req.ToPersonID); s != "" {
		pid, err := uuid.Parse(s)
		if err != nil {
			httpx.WriteProblem(w, r, httpx.BadRequest("to_person_id must be a UUID."))
			return
		}
		toPerson = &pid
	}
	if err := h.service.Escalate(r.Context(), Escalate{ApprovalID: id, ActorID: principal.PersonID, ToRole: req.ToRole, ToPersonID: toPerson, Reason: req.Reason}); err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "escalated"})
}

// Cancel handles POST /api/v1/approvals/{id}/cancel.
func (h *Handler) Cancel(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.WriteProblem(w, r, httpx.BadRequest("id must be a UUID."))
		return
	}
	var req cancelReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		h.writeError(w, r, err)
		return
	}
	if err := h.service.Cancel(r.Context(), Cancel{ApprovalID: id, ActorID: principal.PersonID, Reason: req.Reason}); err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "cancelled"})
}

func (h *Handler) principal(w http.ResponseWriter, r *http.Request) (*authz.Principal, bool) {
	principal, ok := authz.PrincipalFrom(r.Context())
	if !ok || !principal.IsUser() {
		httpx.WriteProblem(w, r, httpx.Unauthenticated("Authentication is required."))
		return nil, false
	}
	return principal, true
}

func (h *Handler) writeError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		httpx.WriteProblem(w, r, httpx.NotFound("The approval does not exist."))
	case errors.Is(err, ErrInvalid):
		httpx.WriteProblem(w, r, httpx.BadRequest(err.Error()))
	case errors.Is(err, ErrForbidden):
		httpx.WriteProblem(w, r, httpx.Forbidden(err.Error()))
	default:
		httpx.WriteError(w, r, h.logger, err)
	}
}

func viewOf(a Approval) approvalView {
	v := approvalView{
		ID:                a.ID.String(),
		Ref:               a.Ref,
		EntityType:        a.EntityType,
		EntityID:          a.EntityID.String(),
		RequestedBy:       a.RequestedBy.String(),
		AmountMinor:       a.AmountMinor,
		Currency:          a.Currency,
		ApproverRole:      a.ApproverRole,
		ApprovalsRequired: a.ApprovalsRequired,
		Status:            string(a.Status),
		PolicyRef:         a.PolicyRef,
		DueAt:             a.DueAt,
		CreatedAt:         a.CreatedAt,
	}
	if a.ApproverID != nil {
		v.ApproverID = a.ApproverID.String()
	}
	return v
}

func encodeCursor(at time.Time, id uuid.UUID) string {
	raw := strconv.FormatInt(at.UTC().UnixNano(), 10) + ":" + id.String()
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

func decodeCursor(raw string) (*Cursor, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return nil, err
	}
	parts := strings.SplitN(string(decoded), ":", 2)
	if len(parts) != 2 {
		return nil, errors.New("malformed cursor")
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
