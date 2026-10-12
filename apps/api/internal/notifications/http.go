package notifications

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

// Handler serves the §6.4 notification surface.
type Handler struct {
	service *Service
	logger  *slog.Logger
}

// NewHandler builds the notifications HTTP handler.
func NewHandler(service *Service, logger *slog.Logger) *Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Handler{service: service, logger: logger}
}

// ── shapes ───────────────────────────────────────────────────

type notificationView struct {
	ID         string     `json:"id"`
	Template   string     `json:"template,omitempty"`
	Subject    string     `json:"subject,omitempty"`
	Body       string     `json:"body,omitempty"`
	Critical   bool       `json:"critical"`
	Status     string     `json:"status"`
	EntityType string     `json:"entity_type,omitempty"`
	EntityID   string     `json:"entity_id,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	ReadAt     *time.Time `json:"read_at,omitempty"`
}

type preferenceView struct {
	InApp      bool   `json:"in_app"`
	Email      bool   `json:"email"`
	WhatsApp   bool   `json:"whatsapp"`
	SMS        bool   `json:"sms"`
	QuietStart *int   `json:"quiet_start,omitempty"`
	QuietEnd   *int   `json:"quiet_end,omitempty"`
	Timezone   string `json:"timezone"`
}

type savePreferenceRequest struct {
	InApp      bool   `json:"in_app"`
	Email      bool   `json:"email"`
	WhatsApp   bool   `json:"whatsapp"`
	SMS        bool   `json:"sms"`
	QuietStart *int   `json:"quiet_start"`
	QuietEnd   *int   `json:"quiet_end"`
	Timezone   string `json:"timezone"`
}

type sendRequest struct {
	RecipientPersonID string         `json:"recipient_person_id"`
	Template          string         `json:"template"`
	Subject           string         `json:"subject"`
	Body              string         `json:"body"`
	Payload           map[string]any `json:"payload,omitempty"`
	EntityType        string         `json:"entity_type,omitempty"`
	EntityID          string         `json:"entity_id,omitempty"`
	Critical          bool           `json:"critical,omitempty"`
	IdempotencyKey    string         `json:"idempotency_key,omitempty"`
}

type sendResponse struct {
	NotificationIDs []string `json:"notification_ids"`
}

// ── handlers ─────────────────────────────────────────────────

// List handles GET /api/v1/notifications.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	personID, ok := h.person(w, r)
	if !ok {
		return
	}

	limit, rawCursor, err := httpx.ParsePage(r)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	var before *Cursor
	if rawCursor != "" {
		before, err = decodeCursor(rawCursor)
		if err != nil {
			httpx.WriteProblem(w, r, httpx.BadRequest("cursor is not valid."))
			return
		}
	}

	// Fetch one extra to learn whether another page exists without a count.
	items, err := h.service.List(r.Context(), personID, limit+1, before)
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

	views := make([]notificationView, 0, len(items))
	for _, n := range items {
		views = append(views, viewOf(n))
	}
	httpx.WriteJSON(w, http.StatusOK, httpx.NewPage(views, next, limit))
}

// UnreadCount handles GET /api/v1/notifications/unread-count.
func (h *Handler) UnreadCount(w http.ResponseWriter, r *http.Request) {
	personID, ok := h.person(w, r)
	if !ok {
		return
	}
	count, err := h.service.UnreadCount(r.Context(), personID)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]int{"count": count})
}

// MarkRead handles POST /api/v1/notifications/{id}/read.
func (h *Handler) MarkRead(w http.ResponseWriter, r *http.Request) {
	personID, ok := h.person(w, r)
	if !ok {
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.WriteProblem(w, r, httpx.BadRequest("id must be a UUID."))
		return
	}
	if err := h.service.MarkRead(r.Context(), personID, id); err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "read"})
}

// MarkAllRead handles POST /api/v1/notifications/read-all.
func (h *Handler) MarkAllRead(w http.ResponseWriter, r *http.Request) {
	personID, ok := h.person(w, r)
	if !ok {
		return
	}
	count, err := h.service.MarkAllRead(r.Context(), personID)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]int{"marked": count})
}

// Preferences handles GET /api/v1/notifications/preferences.
func (h *Handler) Preferences(w http.ResponseWriter, r *http.Request) {
	personID, ok := h.person(w, r)
	if !ok {
		return
	}
	p, err := h.service.Preferences(r.Context(), personID)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, preferenceViewOf(p))
}

// SavePreferences handles PUT /api/v1/notifications/preferences.
func (h *Handler) SavePreferences(w http.ResponseWriter, r *http.Request) {
	personID, ok := h.person(w, r)
	if !ok {
		return
	}
	var req savePreferenceRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		h.writeError(w, r, err)
		return
	}
	p, err := h.service.SavePreferences(r.Context(), personID, Preference{
		InApp:      req.InApp,
		Email:      req.Email,
		WhatsApp:   req.WhatsApp,
		SMS:        req.SMS,
		QuietStart: req.QuietStart,
		QuietEnd:   req.QuietEnd,
		Timezone:   req.Timezone,
	})
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, preferenceViewOf(p))
}

// Send handles POST /internal/v1/commands/notifications.send. The Elixir
// orchestrator calls it to ask Go to notify someone (§5.2). It is a
// service-to-service route: the caller must hold core.notification.manage.
func (h *Handler) Send(w http.ResponseWriter, r *http.Request) {
	var req sendRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		h.writeError(w, r, err)
		return
	}

	recipient, err := uuid.Parse(strings.TrimSpace(req.RecipientPersonID))
	if err != nil {
		httpx.WriteProblem(w, r, httpx.BadRequest("recipient_person_id must be a UUID."))
		return
	}
	var entityID *uuid.UUID
	if strings.TrimSpace(req.EntityID) != "" {
		id, perr := uuid.Parse(req.EntityID)
		if perr != nil {
			httpx.WriteProblem(w, r, httpx.BadRequest("entity_id must be a UUID."))
			return
		}
		entityID = &id
	}

	// §11: non-GET internal commands carry an Idempotency-Key header. Prefer it
	// over the body field when present.
	if key := strings.TrimSpace(r.Header.Get("Idempotency-Key")); key != "" {
		req.IdempotencyKey = key
	}

	var actorID *uuid.UUID
	if principal, ok := authz.PrincipalFrom(r.Context()); ok && principal.UserID != uuid.Nil {
		id := principal.UserID
		actorID = &id
	}

	created, err := h.service.Request(r.Context(), Request{
		RecipientPersonID: recipient,
		Template:          req.Template,
		Subject:           req.Subject,
		Body:              req.Body,
		Payload:           req.Payload,
		EntityType:        req.EntityType,
		EntityID:          entityID,
		Critical:          req.Critical,
		IdempotencyKey:    req.IdempotencyKey,
		ActorID:           actorID,
	})
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	ids := make([]string, 0, len(created))
	for _, n := range created {
		ids = append(ids, n.ID.String())
	}
	httpx.WriteJSON(w, http.StatusAccepted, sendResponse{NotificationIDs: ids})
}

// ── helpers ──────────────────────────────────────────────────

func (h *Handler) person(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	principal, ok := authz.PrincipalFrom(r.Context())
	if !ok || !principal.IsUser() || principal.PersonID == uuid.Nil {
		httpx.WriteProblem(w, r, httpx.Unauthenticated("Authentication is required."))
		return uuid.Nil, false
	}
	return principal.PersonID, true
}

func (h *Handler) writeError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		httpx.WriteProblem(w, r, httpx.NotFound("The requested notification does not exist."))
	case errors.Is(err, ErrInvalid):
		httpx.WriteProblem(w, r, httpx.BadRequest(err.Error()))
	default:
		httpx.WriteError(w, r, h.logger, err)
	}
}

func viewOf(n Notification) notificationView {
	v := notificationView{
		ID:         n.ID.String(),
		Template:   n.Template,
		Subject:    n.Subject,
		Body:       n.Body,
		Critical:   n.Critical,
		Status:     string(n.Status),
		EntityType: n.EntityType,
		CreatedAt:  n.CreatedAt,
		ReadAt:     n.ReadAt,
	}
	if n.EntityID != nil {
		v.EntityID = n.EntityID.String()
	}
	return v
}

func preferenceViewOf(p Preference) preferenceView {
	return preferenceView{
		InApp:      p.InApp,
		Email:      p.Email,
		WhatsApp:   p.WhatsApp,
		SMS:        p.SMS,
		QuietStart: p.QuietStart,
		QuietEnd:   p.QuietEnd,
		Timezone:   p.Timezone,
	}
}

// encodeCursor writes a keyset cursor as opaque base64 (created_at, id).
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
