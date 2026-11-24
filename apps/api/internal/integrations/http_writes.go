package integrations

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/safaristack/atlas/apps/api/internal/authz"
	"github.com/safaristack/atlas/apps/api/internal/platform/httpx"
)

// ListIntegrations handles GET /api/v1/integrations.
func (h *Handler) ListIntegrations(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.principal(w, r); !ok {
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
	var q IntegrationQuery
	var ok bool
	if raw := trim(r.URL.Query().Get("kind")); raw != "" {
		k := IntegrationKind(raw)
		q.Kind = &k
	}
	if raw := trim(r.URL.Query().Get("direction")); raw != "" {
		d := Direction(raw)
		q.Direction = &d
	}
	if raw := trim(r.URL.Query().Get("status")); raw != "" {
		s := IntegrationStatus(raw)
		q.Status = &s
	}
	if q.OwnerID, ok = h.uuidParam(w, r, "owner_person_id"); !ok {
		return
	}
	q.Search = trim(r.URL.Query().Get("q"))

	items, err := h.service.Integrations(r.Context(), q, limit+1, before)
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
	views := make([]integrationView, 0, len(items))
	for _, i := range items {
		views = append(views, integrationOf(i))
	}
	httpx.WriteJSON(w, http.StatusOK, httpx.NewPage(views, next, limit))
}

// GetIntegration handles GET /api/v1/integrations/{id}.
func (h *Handler) GetIntegration(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.principal(w, r); !ok {
		return
	}
	id, ok := h.idParam(w, r, "id", "integration")
	if !ok {
		return
	}
	i, err := h.service.Integration(r.Context(), id)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.SetETag(w, int64(i.Version))
	httpx.WriteJSON(w, http.StatusOK, integrationOf(i))
}

// ListWebhookInbox handles GET /api/v1/integrations/webhook-inbox.
func (h *Handler) ListWebhookInbox(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.principal(w, r); !ok {
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
	var q WebhookInboxQuery
	var ok bool
	if raw := trim(r.URL.Query().Get("provider")); raw != "" {
		q.Provider = &raw
	}
	if raw := trim(r.URL.Query().Get("event_type")); raw != "" {
		q.EventType = &raw
	}
	if raw := trim(r.URL.Query().Get("delivery_id")); raw != "" {
		q.DeliveryID = &raw
	}
	if raw := trim(r.URL.Query().Get("processing_status")); raw != "" {
		ps := ProcessingStatus(raw)
		q.ProcessingStatus = &ps
	}
	if q.From, ok = h.timeParam(w, r, "from"); !ok {
		return
	}
	if q.To, ok = h.timeParam(w, r, "to"); !ok {
		return
	}
	q.Search = trim(r.URL.Query().Get("q"))

	items, err := h.service.WebhookInbox(r.Context(), q, limit+1, before)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	next := ""
	if len(items) > limit {
		items = items[:limit]
		last := items[len(items)-1]
		next = encodeCursor(last.ReceivedAt, last.ID)
	}
	views := make([]webhookInboxView, 0, len(items))
	for _, entry := range items {
		views = append(views, webhookInboxOf(entry))
	}
	httpx.WriteJSON(w, http.StatusOK, httpx.NewPage(views, next, limit))
}

// GetWebhookInbox handles GET /api/v1/integrations/webhook-inbox/{id}.
func (h *Handler) GetWebhookInbox(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.principal(w, r); !ok {
		return
	}
	id, ok := h.idParam(w, r, "id", "webhook inbox entry")
	if !ok {
		return
	}
	entry, err := h.service.WebhookInboxEntry(r.Context(), id)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, webhookInboxOf(entry))
}

// ListOutboundLog handles GET /api/v1/integrations/outbound-log.
func (h *Handler) ListOutboundLog(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.principal(w, r); !ok {
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
	var q OutboundLogQuery
	var ok bool
	if raw := trim(r.URL.Query().Get("provider")); raw != "" {
		q.Provider = &raw
	}
	if raw := trim(r.URL.Query().Get("endpoint")); raw != "" {
		q.Endpoint = &raw
	}
	if raw := trim(r.URL.Query().Get("circuit_state")); raw != "" {
		cs := CircuitState(raw)
		q.CircuitState = &cs
	}
	if q.From, ok = h.timeParam(w, r, "from"); !ok {
		return
	}
	if q.To, ok = h.timeParam(w, r, "to"); !ok {
		return
	}
	q.Search = trim(r.URL.Query().Get("q"))

	items, err := h.service.OutboundLog(r.Context(), q, limit+1, before)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	next := ""
	if len(items) > limit {
		items = items[:limit]
		last := items[len(items)-1]
		next = encodeCursor(last.LastAttemptAt, last.ID)
	}
	views := make([]outboundLogView, 0, len(items))
	for _, o := range items {
		views = append(views, outboundLogOf(o))
	}
	httpx.WriteJSON(w, http.StatusOK, httpx.NewPage(views, next, limit))
}

// GetOutboundLog handles GET /api/v1/integrations/outbound-log/{id}.
func (h *Handler) GetOutboundLog(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.principal(w, r); !ok {
		return
	}
	id, ok := h.idParam(w, r, "id", "outbound log entry")
	if !ok {
		return
	}
	o, err := h.service.OutboundLogEntry(r.Context(), id)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, outboundLogOf(o))
}

// ── write handlers ──────────────────────────────────────────────────────────

// CreateIntegration handles POST /api/v1/integrations.
func (h *Handler) CreateIntegration(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	var req integrationReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		h.writeError(w, r, err)
		return
	}
	in, err := req.integrationInput()
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	created, err := h.service.CreateIntegration(r.Context(), principal.PersonID, in)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.SetETag(w, int64(created.Version))
	httpx.WriteJSON(w, http.StatusCreated, integrationOf(created))
}

// UpdateIntegration handles PUT /api/v1/integrations/{id}.
func (h *Handler) UpdateIntegration(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	id, ok := h.idParam(w, r, "id", "integration")
	if !ok {
		return
	}
	current, err := h.service.Integration(r.Context(), id)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	if err := httpx.RequireIfMatch(r, int64(current.Version)); err != nil {
		h.writeError(w, r, err)
		return
	}
	var req integrationReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		h.writeError(w, r, err)
		return
	}
	in, err := req.integrationInput()
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	updated, err := h.service.UpdateIntegration(r.Context(), principal.PersonID, id, current.Version, in)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.SetETag(w, int64(updated.Version))
	httpx.WriteJSON(w, http.StatusOK, integrationOf(updated))
}

// ── request types and parsing ───────────────────────────────────────────────

type integrationReq struct {
	Name          *string  `json:"name"`
	Kind          string   `json:"kind"`
	Direction     string   `json:"direction"`
	OwnerPersonID *string  `json:"owner_person_id"`
	CredentialRef string   `json:"credential_ref"`
	Scopes        []string `json:"scopes"`
	Status        string   `json:"status"`
}

func (r integrationReq) integrationInput() (IntegrationInput, error) {
	in := IntegrationInput{
		Name:          trim(strOr(r.Name, "")),
		Kind:          trim(r.Kind),
		Direction:     trim(r.Direction),
		CredentialRef: trim(r.CredentialRef),
		Status:        trim(r.Status),
	}
	if r.OwnerPersonID != nil {
		id, err := uuid.Parse(trim(*r.OwnerPersonID))
		if err != nil {
			return in, errInvalidField("owner_person_id")
		}
		in.OwnerPersonID = &id
	}
	in.Scopes = append(in.Scopes, r.Scopes...)
	return in, nil
}

// ── helpers ────────────────────────────────────────────────────────────────

func rfc(t time.Time) string { return t.UTC().Format(time.RFC3339) }

func rfcPtr(t *time.Time) string {
	if t == nil {
		return ""
	}
	return rfc(*t)
}

func personRef(id *uuid.UUID, name string) *personRefView {
	if id == nil {
		return nil
	}
	return &personRefView{ID: id.String(), Name: name}
}

func strOr(s *string, fallback string) string {
	if s == nil {
		return fallback
	}
	return trim(*s)
}

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

func (h *Handler) writeError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		httpx.WriteProblem(w, r, httpx.NotFound("No such integrations record."))
	case errors.Is(err, ErrInvalid):
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

type integrationView struct {
	ID            string         `json:"id"`
	Ref           string         `json:"ref"`
	Name          string         `json:"name"`
	Kind          string         `json:"kind"`
	Direction     string         `json:"direction"`
	Owner         *personRefView `json:"owner,omitempty"`
	CredentialRef string         `json:"credential_ref"`
	Scopes        []string       `json:"scopes"`
	Status        string         `json:"status"`
	LastSuccess   string         `json:"last_success,omitempty"`
	ErrorRate     float64        `json:"error_rate"`
	Version       int            `json:"version"`
	CreatedAt     string         `json:"created_at"`
	UpdatedAt     string         `json:"updated_at"`
}

type personRefView struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type webhookInboxView struct {
	ID               string `json:"id"`
	Provider         string `json:"provider"`
	EventType        string `json:"event_type"`
	DeliveryID       string `json:"delivery_id"`
	ReceivedAt       string `json:"received_at"`
	ProcessedAt      string `json:"processed_at,omitempty"`
	ProcessingStatus string `json:"processing_status"`
	ErrorMessage     string `json:"error_message,omitempty"`
}

type outboundLogView struct {
	ID            string `json:"id"`
	Provider      string `json:"provider"`
	Endpoint      string `json:"endpoint"`
	Method        string `json:"method"`
	StatusCode    *int   `json:"status_code,omitempty"`
	ErrorMessage  string `json:"error_message,omitempty"`
	Attempt       int    `json:"attempt"`
	MaxAttempts   int    `json:"max_attempts"`
	LastAttemptAt string `json:"last_attempt_at"`
	CircuitState  string `json:"circuit_state"`
	CreatedAt     string `json:"created_at"`
}

func integrationOf(i Integration) integrationView {
	return integrationView{
		ID:            i.ID.String(),
		Ref:           i.Ref,
		Name:          i.Name,
		Kind:          string(i.Kind),
		Direction:     string(i.Direction),
		Owner:         personRef(i.OwnerPersonID, i.OwnerName),
		CredentialRef: i.CredentialRef,
		Scopes:        i.Scopes,
		Status:        string(i.Status),
		LastSuccess:   rfcPtr(i.LastSuccess),
		ErrorRate:     i.ErrorRate,
		Version:       i.Version,
		CreatedAt:     rfc(i.CreatedAt),
		UpdatedAt:     rfc(i.UpdatedAt),
	}
}

func webhookInboxOf(w WebhookInboxEntry) webhookInboxView {
	return webhookInboxView{
		ID:               w.ID.String(),
		Provider:         w.Provider,
		EventType:        w.EventType,
		DeliveryID:       w.DeliveryID,
		ReceivedAt:       w.ReceivedAt.UTC().Format(time.RFC3339),
		ProcessedAt:      rfcPtr(w.ProcessedAt),
		ProcessingStatus: string(w.ProcessingStatus),
		ErrorMessage:     w.ErrorMessage,
	}
}

func outboundLogOf(o OutboundLogEntry) outboundLogView {
	return outboundLogView{
		ID:            o.ID.String(),
		Provider:      o.Provider,
		Endpoint:      o.Endpoint,
		Method:        o.Method,
		StatusCode:    o.StatusCode,
		ErrorMessage:  o.ErrorMessage,
		Attempt:       o.Attempt,
		MaxAttempts:   o.MaxAttempts,
		LastAttemptAt: o.LastAttemptAt.UTC().Format(time.RFC3339),
		CircuitState:  string(o.CircuitState),
		CreatedAt:     rfc(o.CreatedAt),
	}
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
		return nil, errors.New("integrations: malformed cursor")
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
