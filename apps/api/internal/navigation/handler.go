package navigation

import (
	"log/slog"
	"net/http"

	"github.com/safaristack/atlas/apps/api/internal/authz"
	"github.com/safaristack/atlas/apps/api/internal/platform/httpx"
)

// Handler serves the workspace map for the signed-in person (§4, §6.2).
type Handler struct {
	registry Registry
	logger   *slog.Logger
}

// NewHandler builds the handler. A nil registry uses Default.
func NewHandler(registry Registry, logger *slog.Logger) *Handler {
	if logger == nil {
		logger = slog.Default()
	}
	if registry == nil {
		registry = Default
	}
	return &Handler{registry: registry, logger: logger}
}

// Get handles GET /api/v1/me/navigation. It answers with the destinations the
// caller's effective permissions unlock; a service identity has no workspace,
// so only a person receives a map.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	principal, ok := authz.PrincipalFrom(r.Context())
	if !ok || !principal.IsUser() {
		httpx.WriteProblem(w, r, httpx.Unauthenticated("Authentication is required."))
		return
	}
	httpx.WriteJSON(w, http.StatusOK, h.registry.Build(principal))
}
