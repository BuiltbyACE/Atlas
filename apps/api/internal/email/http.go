// Package email serves Atlas's two-way email pipeline (§8.4).
//
// The HTTP layer handles compose (outbound), inbox (list threads),
// conversation view (thread + messages), and address management
// (send-as identities). Inbound mail arrives through a webhook handled
// by the internal endpoint.
package email

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/safaristack/atlas/apps/api/internal/authz"
	"github.com/safaristack/atlas/apps/api/internal/platform/httpx"
	"github.com/safaristack/atlas/apps/api/internal/platform/routing"
)

// Handler serves the email surface.
type Handler struct {
	service       *Service
	inboundSecret string
	logger        *slog.Logger
}

// NewHandler builds the email HTTP handler.
func NewHandler(service *Service, logger *slog.Logger) *Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Handler{service: service, logger: logger}
}

// Routes registers the email routes on the registry.
func (h *Handler) Routes(reg *routing.Registry) {
	reg.Gated("core.email.send", http.MethodPost, "/api/v1/email/compose",
		"Send an email", h.Compose)
	reg.Authenticated(http.MethodGet, "/api/v1/email/threads",
		"List email threads (inbox)", h.ListThreads)
	reg.Authenticated(http.MethodGet, "/api/v1/email/threads/{id}",
		"Get an email thread with messages", h.GetThread)
	reg.Authenticated(http.MethodGet, "/api/v1/email/addresses",
		"List the caller's verified send-as addresses", h.ListAddresses)
	reg.Authenticated(http.MethodPost, "/api/v1/email/addresses",
		"Register a send-as email address", h.RegisterAddress)
	reg.Public(http.MethodPost, "/internal/v1/email/inbound",
		"Inbound email webhook from Resend", h.ReceiveInbound)
}

// ── request and response shapes ──────────────────────────────

type composeRequest struct {
	ThreadID       *uuid.UUID `json:"thread_id,omitempty"`
	Subject        string     `json:"subject"`
	BodyText       string     `json:"body_text,omitempty"`
	BodyHTML       string     `json:"body_html,omitempty"`
	FromAddress    string     `json:"from_address"`
	ToAddresses    []string   `json:"to_addresses"`
	CCAddresses    []string   `json:"cc_addresses,omitempty"`
	BCCAddresses   []string   `json:"bcc_addresses,omitempty"`
	InReplyTo      string     `json:"in_reply_to,omitempty"`
	MsgReferences  string     `json:"msg_references,omitempty"`
	IdempotencyKey string     `json:"idempotency_key,omitempty"`
}

type composeResponse struct {
	ID        string `json:"id"`
	ThreadID  string `json:"thread_id"`
	Status    string `json:"status"`
	CreatedAt string `json:"created_at"`
}

type registerAddressRequest struct {
	Address   string `json:"address"`
	IsPrimary bool   `json:"is_primary,omitempty"`
}

type registerAddressResponse struct {
	ID        string `json:"id"`
	Address   string `json:"address"`
	Verified  bool   `json:"verified"`
	IsPrimary bool   `json:"is_primary"`
}

// ── handlers ─────────────────────────────────────────────────

// Compose handles POST /api/v1/email/compose.
func (h *Handler) Compose(w http.ResponseWriter, r *http.Request) {
	var req composeRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}

	actor, ok := h.actor(w, r)
	if !ok {
		return
	}
	msg, err := h.service.Send(r.Context(), actor, SendMessageOptions{
		ThreadID:       req.ThreadID,
		Subject:        req.Subject,
		BodyText:       req.BodyText,
		BodyHTML:       req.BodyHTML,
		FromAddress:    req.FromAddress,
		ToAddresses:    req.ToAddresses,
		CCAddresses:    req.CCAddresses,
		BCCAddresses:   req.BCCAddresses,
		InReplyTo:      req.InReplyTo,
		MsgReferences:  req.MsgReferences,
		IdempotencyKey: req.IdempotencyKey,
	})
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, composeResponse{
		ID:        msg.ID.String(),
		ThreadID:  msg.ThreadID.String(),
		Status:    string(msg.Status),
		CreatedAt: msg.CreatedAt.Format(time.RFC3339),
	})
}

// ListThreads handles GET /api/v1/email/threads.
func (h *Handler) ListThreads(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.actor(w, r)
	if !ok {
		return
	}
	threads, err := h.service.ListThreads(r.Context(), actor, 50, nil)
	if err != nil {
		h.logger.Error("email: list threads failed", slog.String("error", fmt.Sprintf("%+v", err)))
		h.writeError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, threads)
}

// GetThread handles GET /api/v1/email/threads/{id}.
func (h *Handler) GetThread(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		httpx.WriteError(w, r, h.logger, httpx.BadRequest("Invalid thread ID."))
		return
	}

	actor, ok := h.actor(w, r)
	if !ok {
		return
	}
	_, messages, err := h.service.GetThreadWithMessages(r.Context(), actor, id, 100, nil)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, messages)
}

// maxInboundBytes bounds a webhook body. Attachments travel as references, so a
// legitimate payload is small.
const maxInboundBytes = 2 << 20

// WithInboundSecret sets the signing secret the inbound provider uses. Until it
// is set the inbound route answers 503 and stores nothing: an endpoint anyone
// can post to must never be open by default.
func (h *Handler) WithInboundSecret(secret string) *Handler {
	h.inboundSecret = secret
	return h
}

// ReceiveInbound handles POST /internal/v1/email/inbound from the mail provider.
// The route is public at the edge, so the signature is the only gate: the body is
// parsed only after it verifies, and a delivery older than five minutes is refused
// so it cannot be replayed.
func (h *Handler) ReceiveInbound(w http.ResponseWriter, r *http.Request) {
	if strings.TrimSpace(h.inboundSecret) == "" {
		h.logger.Warn("email: inbound webhook refused: ATLAS_EMAIL_INBOUND_SECRET is not set")
		httpx.WriteProblem(w, r, httpx.Unavailable("Inbound email is not configured."))
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxInboundBytes))
	if err != nil {
		httpx.WriteProblem(w, r, httpx.BadRequest("The request body could not be read."))
		return
	}
	if err := verifyWebhook(h.inboundSecret,
		r.Header.Get(headerSvixID), r.Header.Get(headerSvixTimestamp), r.Header.Get(headerSvixSignature),
		body, time.Now()); err != nil {
		h.logger.Warn("email: inbound webhook signature rejected", slog.String("reason", err.Error()))
		httpx.WriteProblem(w, r, httpx.Unauthenticated("Invalid webhook signature."))
		return
	}

	msg, ignored, err := parseInbound(body)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	if ignored {
		w.WriteHeader(http.StatusOK)
		return
	}

	stored, err := h.service.Receive(r.Context(), *msg)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	// A duplicate delivery (already received) is not an error: acknowledge it.
	if stored == nil {
		w.WriteHeader(http.StatusOK)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, map[string]string{
		"id":        stored.ID.String(),
		"thread_id": stored.ThreadID.String(),
		"status":    string(stored.Status),
	})
}

// ListAddresses handles GET /api/v1/email/addresses.
func (h *Handler) ListAddresses(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.actor(w, r)
	if !ok {
		return
	}
	addresses, err := h.service.ListAddresses(r.Context(), actor)
	if err != nil {
		h.logger.Error("email: list addresses failed", slog.String("error", err.Error()))
		h.writeError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, addresses)
}

// RegisterAddress handles POST /api/v1/email/addresses.
func (h *Handler) RegisterAddress(w http.ResponseWriter, r *http.Request) {
	var req registerAddressRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}

	actor, ok := h.actor(w, r)
	if !ok {
		return
	}
	addr, err := h.service.RegisterAddress(r.Context(), actor, req.Address, req.IsPrimary)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, registerAddressResponse{
		ID:        addr.ID.String(),
		Address:   addr.Address,
		Verified:  addr.Verified,
		IsPrimary: addr.IsPrimary,
	})
}

// actor is the signed-in person the request runs as. The route gate has already
// required a principal, but a service identity has no mailbox, so only users pass.
func (h *Handler) actor(w http.ResponseWriter, r *http.Request) (Actor, bool) {
	p, ok := authz.PrincipalFrom(r.Context())
	if !ok || !p.IsUser() {
		httpx.WriteProblem(w, r, httpx.Unauthenticated("Authentication is required."))
		return Actor{}, false
	}
	return Actor{UserID: p.UserID, Email: p.Email}, true
}

// writeError maps the package's sentinel errors onto problem responses; anything
// else is an internal error whose detail stays in the log.
func (h *Handler) writeError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ErrInvalid):
		httpx.WriteProblem(w, r, httpx.BadRequest(strings.TrimPrefix(err.Error(), ErrInvalid.Error()+": ")))
	case errors.Is(err, ErrNotFound):
		httpx.WriteProblem(w, r, httpx.NotFound("Not found."))
	case errors.Is(err, ErrForbidden):
		httpx.WriteProblem(w, r, httpx.Forbidden("You do not have access to this mailbox."))
	case errors.Is(err, ErrNotConfigured):
		httpx.WriteProblem(w, r, httpx.Unavailable("Outbound email is not configured on this server."))
	case errors.Is(err, ErrDelivery):
		httpx.WriteProblem(w, r, httpx.New(http.StatusBadGateway, httpx.CodeUnavailable,
			strings.TrimPrefix(err.Error(), ErrDelivery.Error()+": ")))
	case errors.Is(err, errDuplicateAddress):
		httpx.WriteProblem(w, r, httpx.Conflict("That address is already registered."))
	default:
		httpx.WriteError(w, r, h.logger, err)
	}
}
