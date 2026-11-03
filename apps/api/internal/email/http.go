// Package email serves Atlas's two-way email pipeline (§8.4).
//
// The HTTP layer handles compose (outbound), inbox (list threads),
// conversation view (thread + messages), and address management
// (send-as identities). Inbound mail arrives through a webhook handled
// by the internal endpoint.
package email

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/safaristack/atlas/apps/api/internal/platform/httpx"
	"github.com/safaristack/atlas/apps/api/internal/platform/routing"
)

// Handler serves the email surface.
type Handler struct {
	service *Service
	logger  *slog.Logger
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
	reg.Public(http.MethodPost, "/internal/v1/email/inbound",
		"Inbound email webhook from Resend", h.ReceiveInbound)
}

// ── request and response shapes ──────────────────────────────

type composeRequest struct {
	ThreadID      *uuid.UUID `json:"thread_id,omitempty"`
	Subject       string     `json:"subject"`
	BodyText      string     `json:"body_text,omitempty"`
	BodyHTML      string     `json:"body_html,omitempty"`
	FromAddress   string     `json:"from_address"`
	ToAddresses   []string   `json:"to_addresses"`
	CCAddresses   []string   `json:"cc_addresses,omitempty"`
	BCCAddresses  []string   `json:"bcc_addresses,omitempty"`
	InReplyTo     string     `json:"in_reply_to,omitempty"`
	MsgReferences string     `json:"msg_references,omitempty"`
	IdempotencyKey string    `json:"idempotency_key,omitempty"`
}

type composeResponse struct {
	ID        string `json:"id"`
	ThreadID  string `json:"thread_id"`
	Status    string `json:"status"`
	CreatedAt string `json:"created_at"`
}

// ── handlers ─────────────────────────────────────────────────

// Compose handles POST /api/v1/email/compose.
func (h *Handler) Compose(w http.ResponseWriter, r *http.Request) {
	var req composeRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}

	msg, err := h.service.Send(r.Context(), SendMessageOptions{
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
		httpx.WriteError(w, r, h.logger, err)
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
	threads, err := h.service.ListThreads(r.Context(), 50, nil)
	if err != nil {
		httpx.WriteError(w, r, h.logger, err)
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

	_, messages, err := h.service.GetThreadWithMessages(r.Context(), id, 100, nil)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			httpx.WriteError(w, r, h.logger, httpx.NotFound("Thread not found."))
			return
		}
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, messages)
}

// ReceiveInbound handles POST /internal/v1/email/inbound from Resend.
func (h *Handler) ReceiveInbound(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		From        string   `json:"from"`
		To          []string `json:"to"`
		Cc          []string `json:"cc,omitempty"`
		Bcc         []string `json:"bcc,omitempty"`
		Subject     string   `json:"subject"`
		Text        string   `json:"text"`
		Html        string   `json:"html"`
		MessageID   string   `json:"message_id"`
		InReplyTo   string   `json:"in_reply_to,omitempty"`
		References  string   `json:"references,omitempty"`
		Attachments []struct {
			Filename    string `json:"filename"`
			ContentType string `json:"content_type"`
			Size        int64  `json:"size"`
			URL         string `json:"url"`
		} `json:"attachments,omitempty"`
	}
	if err := httpx.DecodeJSON(w, r, &payload); err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}

	attachments := make([]InboundAttachment, 0, len(payload.Attachments))
	for _, a := range payload.Attachments {
		attachments = append(attachments, InboundAttachment{
			Filename:    a.Filename,
			ContentType: a.ContentType,
			SizeBytes:   a.Size,
			StorageKey:  a.URL, // Resend stores the attachment; we keep the URL as reference
		})
	}

	msg, err := h.service.Receive(r.Context(), InboundMessage{
		MessageID:   payload.MessageID,
		InReplyTo:   payload.InReplyTo,
		MsgReferences: payload.References,
		FromAddress: payload.From,
		ToAddresses: payload.To,
		CCAddresses: payload.Cc,
		Subject:     payload.Subject,
		BodyText:    payload.Text,
		BodyHTML:    payload.Html,
		Attachments: attachments,
	})
	if err != nil {
		if errors.Is(err, ErrInvalid) {
			httpx.WriteError(w, r, h.logger, httpx.BadRequest(err.Error()))
			return
		}
		httpx.WriteError(w, r, h.logger, err)
		return
	}

	// Duplicate message (already received) is not an error.
	if msg == nil {
		w.WriteHeader(http.StatusOK)
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, map[string]string{
		"id":        msg.ID.String(),
		"thread_id": msg.ThreadID.String(),
		"status":    string(msg.Status),
	})
}
