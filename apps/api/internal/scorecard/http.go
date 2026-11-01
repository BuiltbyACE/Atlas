package scorecard

import (
	"log/slog"
	"net/http"

	"github.com/safaristack/atlas/apps/api/internal/platform/httpx"
)

// Handler serves the company scorecard (§8.9). The single route is gated by
// intel.dashboard.read, held by both CEOs.
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

// Scorecard handles GET /api/v1/dashboard/scorecard.
func (h *Handler) Scorecard(w http.ResponseWriter, r *http.Request) {
	sc, err := h.service.Scorecard(r.Context())
	if err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, sc)
}
