package integrations

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/google/uuid"

	"github.com/safaristack/atlas/apps/api/internal/platform/httpx"
)

// maxWebhookBytes caps a webhook body. GitHub pull_request payloads are far
// smaller; a larger body is a mistake or an attack, and either way it is
// rejected before it is read into memory.
const maxWebhookBytes = 1 << 20

// Handler serves the inbound webhook and installation management surface (§11, §12).
type Handler struct {
	service       *Service
	secret        string
	appSlug       string
	publicBaseURL string
	logger        *slog.Logger
}

// NewHandler builds the inbound webhook handler. secret is the shared GitHub
// webhook secret; when it is empty the endpoint answers 503 rather than
// trusting an unsigned body (configuration, not a fail-open).
// appSlug is the GitHub App's URL-friendly name, used to build the installation
// URL. publicBaseURL is the externally visible base URL for OAuth callbacks.
func NewHandler(service *Service, secret, appSlug, publicBaseURL string, logger *slog.Logger) *Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Handler{service: service, secret: secret, appSlug: appSlug, publicBaseURL: publicBaseURL, logger: logger}
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

// InstallGitHub initiates the GitHub App installation flow. It redirects the
// user to GitHub to install the App on their personal account. The person_id
// query parameter identifies the SCOS employee installing the App.
func (h *Handler) InstallGitHub(w http.ResponseWriter, r *http.Request) {
	personID := r.URL.Query().Get("person_id")
	if personID == "" {
		httpx.WriteProblem(w, r, httpx.BadRequest("person_id is required"))
		return
	}
	if h.appSlug == "" {
		httpx.WriteProblem(w, r, httpx.Unavailable("GitHub App is not configured."))
		return
	}
	// Build the GitHub App installation URL with a state parameter that
	// encodes the person_id for the callback to consume.
	state := url.QueryEscape(personID)
	installURL := fmt.Sprintf("https://github.com/apps/%s/installations/new?state=%s", h.appSlug, state)
	http.Redirect(w, r, installURL, http.StatusFound)
}

// GitHubInstallCallback handles GitHub's redirect back after installation.
// GitHub appends ?installation_id=...&setup_action=...&state=...
func (h *Handler) GitHubInstallCallback(w http.ResponseWriter, r *http.Request) {
	installationID := r.URL.Query().Get("installation_id")
	setupAction := r.URL.Query().Get("setup_action")
	state := r.URL.Query().Get("state")

	if installationID == "" {
		httpx.WriteProblem(w, r, httpx.BadRequest("installation_id is missing"))
		return
	}
	if state == "" {
		httpx.WriteProblem(w, r, httpx.BadRequest("state is missing"))
		return
	}

	personID, err := url.QueryUnescape(state)
	if err != nil {
		httpx.WriteProblem(w, r, httpx.BadRequest("invalid state parameter"))
		return
	}

	// TODO: Persist the installation mapping (person_id, installation_id,
	// account_type, account_login) to the database. This requires fetching
	// the installation details from GitHub's API using an installation access
	// token, which needs the GitHub App private key (V2).
	//
	// For V1, we acknowledge the callback and instruct the user to complete
	// setup via the SCOS settings page.

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"status":         "received",
		"installation_id": installationID,
		"setup_action":    setupAction,
		"person_id":       personID,
		"message":         "Installation received. Please complete mapping in SCOS settings.",
	})
}

// GetGitHubStatus returns the current GitHub connection status for the
// signed-in person. Used by the frontend Connect GitHub button to decide
// whether to show the connect prompt or the connected state.
func (h *Handler) GetGitHubStatus(w http.ResponseWriter, r *http.Request) {
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"connected": false,
		"message":   "GitHub connection status endpoint. Full installation mapping requires GitHub App private key (V2).",
	})
}

func optionalUUID(id uuid.UUID) string {
	if id == uuid.Nil {
		return ""
	}
	return id.String()
}
