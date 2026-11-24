// Package chat serves Atlas's messaging platform (§8.5).
package chat

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/safaristack/atlas/apps/api/internal/platform/httpx"
	"github.com/safaristack/atlas/apps/api/internal/platform/routing"
)

// Handler serves the chat surface.
type Handler struct {
	service *Service
	logger  *slog.Logger
}

// NewHandler builds the chat HTTP handler.
func NewHandler(service *Service, logger *slog.Logger) *Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Handler{service: service, logger: logger}
}

// Routes registers the chat routes on the registry.
func (h *Handler) Routes(reg *routing.Registry) {
	reg.Authenticated(http.MethodGet, "/api/v1/chat/conversations",
		"List the caller's conversations", h.ListConversations)
	reg.Authenticated(http.MethodPost, "/api/v1/chat/conversations",
		"Start a 1:1 or group conversation", h.CreateConversation)
	reg.Authenticated(http.MethodGet, "/api/v1/chat/conversations/{id}/messages",
		"List messages in a conversation", h.ListMessages)
	reg.Authenticated(http.MethodPost, "/api/v1/chat/conversations/{id}/messages",
		"Send a message in a conversation", h.SendMessage)
}

// ── request and response shapes ──────────────────────────────

type createConversationRequest struct {
	Type        string   `json:"type"`
	Name        string   `json:"name,omitempty"`
	Participant []uuid.UUID `json:"participant,omitempty"`
}

type createConversationResponse struct {
	ID          string `json:"id"`
	Type        string `json:"type"`
	Name        string `json:"name,omitempty"`
	CreatedAt   string `json:"created_at"`
}

type sendMessageRequest struct {
	Body           string     `json:"body"`
	BodyType       string     `json:"body_type,omitempty"`
	ReplyToMessage *uuid.UUID `json:"reply_to_message,omitempty"`
}

type sendMessageResponse struct {
	ID             string    `json:"id"`
	ConversationID uuid.UUID `json:"conversation_id"`
	SenderID       uuid.UUID `json:"sender_id"`
	CreatedAt      string  `json:"created_at"`
}

// ── handlers ─────────────────────────────────────────────────

// ListConversations handles GET /api/v1/chat/conversations.
func (h *Handler) ListConversations(w http.ResponseWriter, r *http.Request) {
	// TODO: get user ID from principal context
	_ = r
	httpx.WriteError(w, r, h.logger, httpx.BadRequest("User ID from principal not yet wired."))
}

// CreateConversation handles POST /api/v1/chat/conversations.
func (h *Handler) CreateConversation(w http.ResponseWriter, r *http.Request) {
	var req createConversationRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}

	// TODO: get user ID from principal context
	_ = req
	httpx.WriteError(w, r, h.logger, httpx.BadRequest("User ID from principal not yet wired."))
}

// ListMessages handles GET /api/v1/chat/conversations/{id}/messages.
func (h *Handler) ListMessages(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	convID, err := uuid.Parse(idStr)
	if err != nil {
		httpx.WriteError(w, r, h.logger, httpx.BadRequest("Invalid conversation ID."))
		return
	}
	// TODO: get user ID from principal context
	_ = convID
	httpx.WriteError(w, r, h.logger, httpx.BadRequest("User ID from principal not yet wired."))
}

// SendMessage handles POST /api/v1/chat/conversations/{id}/messages.
func (h *Handler) SendMessage(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	convID, err := uuid.Parse(idStr)
	if err != nil {
		httpx.WriteError(w, r, h.logger, httpx.BadRequest("Invalid conversation ID."))
		return
	}
	var req sendMessageRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	// TODO: get user ID from principal context
	_ = convID
	_ = req
	httpx.WriteError(w, r, h.logger, httpx.BadRequest("User ID from principal not yet wired."))
}
