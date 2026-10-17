package integrations

import (
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/safaristack/atlas/apps/api/internal/platform/httpx"
)

// maxWebhookBytes caps a webhook body. GitHub pull_request payloads are far
// smaller; a larger body is a mistake or an attack, and either way it is
// rejected before it is read into memory.
const maxWebhookBytes = 1 << 20

// Handler serves the inbound webhook surface (§11, §12).
type Handler struct {
	service *Service
	secret  string
	logger  *slog.Logger
}

// NewHandler builds the inbound webhook handler. secret is the shared GitHub
// webhook secret; when it is empty the endpoint answers 503 rather than
// trusting an unsigned body (configuration, not a fail-open).
func NewHandler(service *Service, secret string, logger *slog.Logger) *Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Handler{service: service, secret: secret, logger: logger}
}

// ReceiveGitHub handles POST /webhooks/github. GitHub has no principal, so the
// route is public and the HMAC signature is the gate: the body is only parsed
// after the signature checks out.
func (h *Handler) ReceiveGitHub(w http.ResponseWriter, r *http.Request) {
	if strings.TrimSpace(h.secret) == "" {
		httpx.WriteProblem(w, r, httpx.Unavailable("GitHub webhooks are not configured."))
		return
	}

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxWebhookBytes))
	if err != nil {
		httpx.WriteProblem(w, r, httpx.BadRequest("Could not read the webhook body."))
		return
	}

	if !VerifySignature(h.secret, body, r.Header.Get(SignatureHeader)) {
		h.logger.Warn("github webhook signature rejected", "delivery", r.Header.Get(DeliveryHeader))
		httpx.WriteProblem(w, r, httpx.Unauthenticated("Invalid webhook signature."))
		return
	}

	outcome, err := h.service.HandleGitHub(r.Context(),
		r.Header.Get(DeliveryHeader), r.Header.Get(EventHeader), body)
	switch {
	case errors.Is(err, ErrInvalid):
		httpx.WriteProblem(w, r, httpx.BadRequest(err.Error()))
		return
	case err != nil:
		h.logger.Error("github webhook not applied", "error", err, "delivery", r.Header.Get(DeliveryHeader))
		httpx.WriteError(w, r, h.logger, err)
		return
	}

	httpx.WriteJSON(w, http.StatusAccepted, map[string]any{
		"status":      outcome.Status,
		"refs":        outcome.Refs,
		"task_id":     optionalUUID(outcome.TaskID),
		"evidence_id": optionalUUID(outcome.EvidenceID),
	})
}

func optionalUUID(id uuid.UUID) string {
	if id == uuid.Nil {
		return ""
	}
	return id.String()
}
