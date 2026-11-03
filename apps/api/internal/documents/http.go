// Package documents serves Atlas's document storage layer (§8.6).
package documents

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/safaristack/atlas/apps/api/internal/platform/httpx"
	"github.com/safaristack/atlas/apps/api/internal/platform/routing"
)

// Handler serves the documents surface.
type Handler struct {
	service *Service
	logger  *slog.Logger
}

// NewHandler builds the documents HTTP handler.
func NewHandler(service *Service, logger *slog.Logger) *Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Handler{service: service, logger: logger}
}

// Routes registers the document routes on the registry.
func (h *Handler) Routes(reg *routing.Registry) {
	reg.Authenticated(http.MethodPost, "/api/v1/documents",
		"Register a document upload (returns presigned PUT URL)", h.PrepareUpload)
	reg.Authenticated(http.MethodGet, "/api/v1/documents",
		"List documents (filter by category)", h.ListDocuments)
	reg.Authenticated(http.MethodGet, "/api/v1/documents/{id}",
		"Get document metadata + presigned download URL", h.GetDocument)
	reg.Authenticated(http.MethodPut, "/api/v1/documents/{id}",
		"Update document metadata", h.UpdateDocument)
	reg.Authenticated(http.MethodDelete, "/api/v1/documents/{id}",
		"Delete a document record", h.DeleteDocument)
	reg.Authenticated(http.MethodPost, "/api/v1/documents/{id}/shares",
		"Grant access to a document", h.ShareDocument)
}

// ── request and response shapes ──────────────────────────────

type prepareUploadRequest struct {
	Filename    string     `json:"filename"`
	ContentType string     `json:"content_type"`
	SizeBytes   int64      `json:"size_bytes"`
	Category    string     `json:"category,omitempty"`
	Description string     `json:"description,omitempty"`
}

type prepareUploadResponse struct {
	Document   *Document          `json:"document"`
	UploadURL  string             `json:"upload_url"`
	Headers    map[string]string  `json:"headers,omitempty"`
}

type prepareDownloadResponse struct {
	Document    *Document `json:"document"`
	DownloadURL string    `json:"download_url"`
}

type listDocumentsResponse struct {
	Documents []Document `json:"documents"`
}

type updateDocumentRequest struct {
	Category   string `json:"category,omitempty"`
	Description string `json:"description,omitempty"`
	IsPublic   bool   `json:"is_public,omitempty"`
}

type shareDocumentRequest struct {
	UserID     uuid.UUID `json:"user_id"`
	Permission string    `json:"permission"`
}

// ── handlers ─────────────────────────────────────────────────

// PrepareUpload handles POST /api/v1/documents.
func (h *Handler) PrepareUpload(w http.ResponseWriter, r *http.Request) {
	var req prepareUploadRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}

	// TODO: get user ID from principal context
	_ = req
	httpx.WriteError(w, r, h.logger, httpx.BadRequest("User ID from principal not yet wired."))
}

// ListDocuments handles GET /api/v1/documents.
func (h *Handler) ListDocuments(w http.ResponseWriter, r *http.Request) {
	// TODO: get user ID from principal context
	_ = r
	httpx.WriteError(w, r, h.logger, httpx.BadRequest("User ID from principal not yet wired."))
}

// GetDocument handles GET /api/v1/documents/{id}.
func (h *Handler) GetDocument(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		httpx.WriteError(w, r, h.logger, httpx.BadRequest("Invalid document ID."))
		return
	}
	// TODO: get user ID from principal context
	_ = id
	httpx.WriteError(w, r, h.logger, httpx.BadRequest("User ID from principal not yet wired."))
}

// UpdateDocument handles PUT /api/v1/documents/{id}.
func (h *Handler) UpdateDocument(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		httpx.WriteError(w, r, h.logger, httpx.BadRequest("Invalid document ID."))
		return
	}
	var req updateDocumentRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	// TODO: get user ID from principal context
	_ = id
	_ = req
	httpx.WriteError(w, r, h.logger, httpx.BadRequest("User ID from principal not yet wired."))
}

// DeleteDocument handles DELETE /api/v1/documents/{id}.
func (h *Handler) DeleteDocument(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		httpx.WriteError(w, r, h.logger, httpx.BadRequest("Invalid document ID."))
		return
	}
	// TODO: get user ID from principal context
	_ = id
	httpx.WriteError(w, r, h.logger, httpx.BadRequest("User ID from principal not yet wired."))
}

// ShareDocument handles POST /api/v1/documents/{id}/shares.
func (h *Handler) ShareDocument(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		httpx.WriteError(w, r, h.logger, httpx.BadRequest("Invalid document ID."))
		return
	}
	var req shareDocumentRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	// TODO: get user ID from principal context
	_ = id
	_ = req
	httpx.WriteError(w, r, h.logger, httpx.BadRequest("User ID from principal not yet wired."))
}
