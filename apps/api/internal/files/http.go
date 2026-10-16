package files

import (
	"encoding/base64"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/safaristack/atlas/apps/api/internal/authz"
	"github.com/safaristack/atlas/apps/api/internal/platform/httpx"
)

// Handler serves the files HTTP endpoints (docs/ATLAS.md §6.5, §11).
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

type fileView struct {
	ID             string    `json:"id"`
	Filename       string    `json:"filename"`
	ContentType    string    `json:"content_type"`
	SizeBytes      int64     `json:"size_bytes"`
	ChecksumSHA256 string    `json:"checksum_sha256,omitempty"`
	Classification string    `json:"classification"`
	Status         string    `json:"status"`
	EntityType     string    `json:"entity_type,omitempty"`
	EntityID       string    `json:"entity_id,omitempty"`
	UploadedBy     string    `json:"uploaded_by,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type uploadTicketView struct {
	FileID    string            `json:"file_id"`
	UploadURL string            `json:"upload_url"`
	Method    string            `json:"method"`
	Headers   map[string]string `json:"headers"`
	ExpiresAt time.Time         `json:"expires_at"`
}

type downloadTicketView struct {
	URL       string    `json:"url"`
	ExpiresAt time.Time `json:"expires_at"`
}

type requestUploadReq struct {
	Filename       string  `json:"filename"`
	ContentType    string  `json:"content_type"`
	SizeBytes      int64   `json:"size_bytes"`
	Classification string  `json:"classification,omitempty"`
	EntityType     string  `json:"entity_type,omitempty"`
	EntityID       *string `json:"entity_id,omitempty"`
	ChecksumSHA256 string  `json:"checksum_sha256,omitempty"`
}

// RequestUpload handles POST /api/v1/files. It records a pending file and
// returns a pre-signed PUT URL the client uploads to directly.
func (h *Handler) RequestUpload(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}

	var req requestUploadReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		h.writeError(w, r, err)
		return
	}
	var entityID *uuid.UUID
	if raw := strings.TrimSpace(derefString(req.EntityID)); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			httpx.WriteProblem(w, r, httpx.BadRequest("entity_id is not a valid id."))
			return
		}
		entityID = &id
	}

	f, ticket, err := h.service.RequestUpload(r.Context(), principal.PersonID, UploadRequest{
		Filename:       req.Filename,
		ContentType:    req.ContentType,
		SizeBytes:      req.SizeBytes,
		Classification: Classification(strings.TrimSpace(req.Classification)),
		EntityType:     req.EntityType,
		EntityID:       entityID,
		ChecksumSHA256: req.ChecksumSHA256,
	})
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, map[string]any{
		"file": viewOf(f),
		"upload": uploadTicketView{
			FileID:    ticket.FileID.String(),
			UploadURL: ticket.UploadURL,
			Method:    ticket.Method,
			Headers:   ticket.Headers,
			ExpiresAt: ticket.ExpiresAt,
		},
	})
}

// Complete handles POST /api/v1/files/{id}/complete.
func (h *Handler) Complete(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	id, ok := h.fileID(w, r)
	if !ok {
		return
	}
	f, err := h.service.CompleteUpload(r.Context(), principal.PersonID, id)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, viewOf(f))
}

// Get handles GET /api/v1/files/{id}.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := h.fileID(w, r)
	if !ok {
		return
	}
	f, err := h.service.Get(r.Context(), id)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, viewOf(f))
}

// Download handles GET /api/v1/files/{id}/download, returning a short-lived
// pre-signed URL rather than redirecting, so an XHR client keeps control.
func (h *Handler) Download(w http.ResponseWriter, r *http.Request) {
	id, ok := h.fileID(w, r)
	if !ok {
		return
	}
	ticket, err := h.service.DownloadURL(r.Context(), id)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, downloadTicketView{URL: ticket.URL, ExpiresAt: ticket.ExpiresAt})
}

// List handles GET /api/v1/files.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
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

	q := ListQuery{EntityType: strings.TrimSpace(r.URL.Query().Get("entity_type"))}
	if raw := strings.TrimSpace(r.URL.Query().Get("entity_id")); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			httpx.WriteProblem(w, r, httpx.BadRequest("entity_id is not a valid id."))
			return
		}
		q.EntityID = &id
	}
	if strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("uploaded_by")), "me") {
		principal, ok := authz.PrincipalFrom(r.Context())
		if !ok || !principal.IsUser() {
			httpx.WriteProblem(w, r, httpx.Unauthenticated("Authentication is required."))
			return
		}
		q.UploadedBy = &principal.PersonID
	}
	if s := strings.TrimSpace(r.URL.Query().Get("status")); s != "" {
		q.Status = Status(s)
	}

	items, err := h.service.List(r.Context(), q, limit+1, before)
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
	views := make([]fileView, 0, len(items))
	for _, f := range items {
		views = append(views, viewOf(f))
	}
	httpx.WriteJSON(w, http.StatusOK, httpx.NewPage(views, next, limit))
}

// Archive handles DELETE /api/v1/files/{id}.
func (h *Handler) Archive(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	id, ok := h.fileID(w, r)
	if !ok {
		return
	}
	if err := h.service.Archive(r.Context(), principal.PersonID, id, strings.TrimSpace(r.URL.Query().Get("reason"))); err != nil {
		h.writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) principal(w http.ResponseWriter, r *http.Request) (*authz.Principal, bool) {
	principal, ok := authz.PrincipalFrom(r.Context())
	if !ok || !principal.IsUser() {
		httpx.WriteProblem(w, r, httpx.Unauthenticated("Authentication is required."))
		return nil, false
	}
	return principal, true
}

func (h *Handler) fileID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(strings.TrimSpace(chi.URLParam(r, "id")))
	if err != nil {
		httpx.WriteProblem(w, r, httpx.BadRequest("id is not a valid file id."))
		return uuid.Nil, false
	}
	return id, true
}

func (h *Handler) writeError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		httpx.WriteProblem(w, r, httpx.NotFound("No such file."))
	case errors.Is(err, ErrInvalid):
		httpx.WriteProblem(w, r, httpx.BadRequest(err.Error()))
	case errors.Is(err, ErrForbidden):
		httpx.WriteProblem(w, r, httpx.Forbidden(err.Error()))
	case errors.Is(err, ErrObjectMissing):
		httpx.WriteProblem(w, r, httpx.BadRequest("The uploaded object is not in storage."))
	case errors.Is(err, ErrConflict):
		httpx.WriteProblem(w, r, httpx.Conflict(err.Error()))
	default:
		httpx.WriteError(w, r, h.logger, err)
	}
}

func viewOf(f File) fileView {
	v := fileView{
		ID:             f.ID.String(),
		Filename:       f.Filename,
		ContentType:    f.ContentType,
		SizeBytes:      f.SizeBytes,
		ChecksumSHA256: f.ChecksumSHA256,
		Classification: string(f.Classification),
		Status:         string(f.Status),
		EntityType:     f.EntityType,
		CreatedAt:      f.CreatedAt,
		UpdatedAt:      f.UpdatedAt,
	}
	if f.EntityID != nil {
		v.EntityID = f.EntityID.String()
	}
	if f.UploadedBy != nil {
		v.UploadedBy = f.UploadedBy.String()
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
		return nil, errors.New("files: malformed cursor")
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

func derefString(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
