package settings

import (
	"encoding/base64"
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

// Handler serves the settings HTTP endpoints (docs/ATLAS.md §6.7, §11).
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

type settingView struct {
	Key         string     `json:"key"`
	Value       any        `json:"value"`
	Type        string     `json:"type"`
	Description string     `json:"description,omitempty"`
	Version     int64      `json:"version"`
	UpdatedBy   string     `json:"updated_by,omitempty"`
	UpdatedAt   *time.Time `json:"updated_at,omitempty"`
}

type changeView struct {
	ID        string    `json:"id"`
	Key       string    `json:"key"`
	Version   int64     `json:"version"`
	OldValue  any       `json:"old_value"`
	NewValue  any       `json:"new_value"`
	ChangedBy string    `json:"changed_by,omitempty"`
	ChangedAt time.Time `json:"changed_at"`
	Reason    string    `json:"reason,omitempty"`
}

type setSettingReq struct {
	Value  json.RawMessage `json:"value"`
	Reason string          `json:"reason,omitempty"`
}

// List handles GET /api/v1/settings.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	items, err := h.service.List(r.Context())
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	views := make([]settingView, 0, len(items))
	for _, st := range items {
		views = append(views, viewOf(st))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": views})
}

// Get handles GET /api/v1/settings/{key}. The current version is the ETag a
// client must echo when it writes.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	st, err := h.service.Get(r.Context(), keyOf(r))
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.SetETag(w, st.Version)
	httpx.WriteJSON(w, http.StatusOK, viewOf(st))
}

// Set handles PUT /api/v1/settings/{key}. The version the client read travels
// in If-Match (§6.6): a missing header is 428, a stale one 412.
func (h *Handler) Set(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}

	current, err := h.service.Get(r.Context(), keyOf(r))
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	if err := httpx.RequireIfMatch(r, current.Version); err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}

	var req setSettingReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		h.writeError(w, r, err)
		return
	}
	if len(req.Value) == 0 {
		httpx.WriteProblem(w, r, httpx.BadRequest("value is required."))
		return
	}
	var value any
	if err := json.Unmarshal(req.Value, &value); err != nil {
		httpx.WriteProblem(w, r, httpx.BadRequest("value is not valid JSON."))
		return
	}

	st, err := h.service.Set(r.Context(), SetInput{
		Key:             current.Key,
		Value:           value,
		ExpectedVersion: current.Version,
		ActorID:         principal.PersonID,
		Reason:          strings.TrimSpace(req.Reason),
	})
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	httpx.SetETag(w, st.Version)
	httpx.WriteJSON(w, http.StatusOK, viewOf(st))
}

// History handles GET /api/v1/settings/{key}/history.
func (h *Handler) History(w http.ResponseWriter, r *http.Request) {
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

	changes, err := h.service.History(r.Context(), keyOf(r), limit+1, before)
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	next := ""
	if len(changes) > limit {
		changes = changes[:limit]
		last := changes[len(changes)-1]
		next = encodeCursor(last.ChangedAt, last.ID)
	}
	views := make([]changeView, 0, len(changes))
	for _, c := range changes {
		views = append(views, changeOf(c))
	}
	httpx.WriteJSON(w, http.StatusOK, httpx.NewPage(views, next, limit))
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
		httpx.WriteProblem(w, r, httpx.NotFound("No such setting."))
	case errors.Is(err, ErrInvalid):
		httpx.WriteProblem(w, r, httpx.BadRequest(err.Error()))
	case errors.Is(err, ErrConflict):
		httpx.WriteProblem(w, r, httpx.PreconditionFailed("The setting changed since you last read it."))
	default:
		httpx.WriteError(w, r, h.logger, err)
	}
}

func keyOf(r *http.Request) string {
	return strings.TrimSpace(chi.URLParam(r, "key"))
}

func viewOf(s Setting) settingView {
	v := settingView{
		Key:         s.Key,
		Value:       s.Value,
		Type:        string(s.Kind),
		Description: s.Description,
		Version:     s.Version,
		UpdatedAt:   s.UpdatedAt,
	}
	if s.UpdatedBy != nil {
		v.UpdatedBy = s.UpdatedBy.String()
	}
	return v
}

func changeOf(c Change) changeView {
	v := changeView{
		ID:        c.ID.String(),
		Key:       c.Key,
		Version:   c.Version,
		OldValue:  c.OldValue,
		NewValue:  c.NewValue,
		ChangedAt: c.ChangedAt,
		Reason:    c.Reason,
	}
	if c.ChangedBy != nil {
		v.ChangedBy = c.ChangedBy.String()
	}
	return v
}

func encodeCursor(at time.Time, id uuid.UUID) string {
	raw := strconv.FormatInt(at.UTC().UnixNano(), 10) + ":" + id.String()
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

func decodeCursor(encoded string) (*Cursor, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(encoded)
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
