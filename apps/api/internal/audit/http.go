package audit

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/safaristack/atlas/apps/api/internal/platform/httpx"
)

// Handler serves the audit read endpoints (docs/ATLAS.md §6.3, §11). Both are
// gated by core.audit.read, which the auditor role and both CEOs hold, so the
// trail is global for exactly those principals.
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

type entryView struct {
	ID         string          `json:"id"`
	OccurredAt time.Time       `json:"occurred_at"`
	ActorType  string          `json:"actor_type"`
	ActorID    string          `json:"actor_id"`
	OnBehalfOf string          `json:"on_behalf_of,omitempty"`
	Action     string          `json:"action"`
	EntityType string          `json:"entity_type"`
	EntityID   string          `json:"entity_id,omitempty"`
	Before     json.RawMessage `json:"before,omitempty"`
	After      json.RawMessage `json:"after,omitempty"`
	Reason     string          `json:"reason,omitempty"`
	RequestID  string          `json:"request_id,omitempty"`
	IP         string          `json:"ip,omitempty"`
	UserAgent  string          `json:"user_agent,omitempty"`
}

// List handles GET /api/v1/audit.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	limit, cursor, err := httpx.ParsePage(r)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	before, ok := h.cursor(w, r, cursor)
	if !ok {
		return
	}
	q, ok := h.query(w, r)
	if !ok {
		return
	}

	entries, err := h.service.List(r.Context(), q, limit+1, before)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	h.writePage(w, limit, entries)
}

// History handles GET /api/v1/audit/{entity_type}/{entity_id} — the "History"
// tab for one record.
func (h *Handler) History(w http.ResponseWriter, r *http.Request) {
	limit, cursor, err := httpx.ParsePage(r)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	before, ok := h.cursor(w, r, cursor)
	if !ok {
		return
	}

	entityType := strings.TrimSpace(chi.URLParam(r, "entity_type"))
	entityID, err := uuid.Parse(strings.TrimSpace(chi.URLParam(r, "entity_id")))
	if err != nil {
		httpx.WriteProblem(w, r, httpx.BadRequest("entity_id is not a valid id."))
		return
	}

	entries, err := h.service.History(r.Context(), entityType, entityID, limit+1, before)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	h.writePage(w, limit, entries)
}

func (h *Handler) writePage(w http.ResponseWriter, limit int, entries []Entry) {
	next := ""
	if len(entries) > limit {
		entries = entries[:limit]
		last := entries[len(entries)-1]
		next = encodeCursor(last.OccurredAt, last.ID)
	}
	views := make([]entryView, 0, len(entries))
	for _, e := range entries {
		views = append(views, viewOf(e))
	}
	httpx.WriteJSON(w, http.StatusOK, httpx.NewPage(views, next, limit))
}

func (h *Handler) cursor(w http.ResponseWriter, r *http.Request, cursor string) (*Cursor, bool) {
	if cursor == "" {
		return nil, true
	}
	before, err := decodeCursor(cursor)
	if err != nil {
		httpx.WriteProblem(w, r, httpx.BadRequest("cursor is not valid."))
		return nil, false
	}
	return before, true
}

func (h *Handler) query(w http.ResponseWriter, r *http.Request) (Query, bool) {
	values := r.URL.Query()
	q := Query{
		EntityType: strings.TrimSpace(values.Get("entity_type")),
		ActorType:  strings.TrimSpace(values.Get("actor_type")),
		ActorID:    strings.TrimSpace(values.Get("actor_id")),
		Action:     strings.TrimSpace(values.Get("action")),
	}
	if raw := strings.TrimSpace(values.Get("entity_id")); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			httpx.WriteProblem(w, r, httpx.BadRequest("entity_id is not a valid id."))
			return q, false
		}
		q.EntityID = &id
	}
	if raw := strings.TrimSpace(values.Get("since")); raw != "" {
		ts, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			httpx.WriteProblem(w, r, httpx.BadRequest("since must be an RFC 3339 timestamp."))
			return q, false
		}
		uts := ts.UTC()
		q.Since = &uts
	}
	if raw := strings.TrimSpace(values.Get("until")); raw != "" {
		ts, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			httpx.WriteProblem(w, r, httpx.BadRequest("until must be an RFC 3339 timestamp."))
			return q, false
		}
		uts := ts.UTC()
		q.Until = &uts
	}
	return q, true
}

func (h *Handler) writeError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ErrInvalid):
		httpx.WriteProblem(w, r, httpx.BadRequest(err.Error()))
	default:
		httpx.WriteError(w, r, h.logger, err)
	}
}

func viewOf(e Entry) entryView {
	v := entryView{
		ID:         e.ID.String(),
		OccurredAt: e.OccurredAt,
		ActorType:  e.ActorType,
		ActorID:    e.ActorID,
		Action:     e.Action,
		EntityType: e.EntityType,
		Before:     e.Before,
		After:      e.After,
		Reason:     e.Reason,
		RequestID:  e.RequestID,
		IP:         e.IP,
		UserAgent:  e.UserAgent,
	}
	if e.OnBehalfOf != nil {
		v.OnBehalfOf = e.OnBehalfOf.String()
	}
	if e.EntityID != nil {
		v.EntityID = e.EntityID.String()
	}
	return v
}

func encodeCursor(at time.Time, id uuid.UUID) string {
	raw := at.UTC().Format(time.RFC3339Nano) + "|" + id.String()
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

func decodeCursor(cursor string) (*Cursor, error) {
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return nil, err
	}
	at, id, ok := strings.Cut(string(raw), "|")
	if !ok {
		return nil, errors.New("audit: malformed cursor")
	}
	ts, err := time.Parse(time.RFC3339Nano, at)
	if err != nil {
		return nil, err
	}
	parsed, err := uuid.Parse(id)
	if err != nil {
		return nil, err
	}
	return &Cursor{At: ts.UTC(), ID: parsed}, nil
}
