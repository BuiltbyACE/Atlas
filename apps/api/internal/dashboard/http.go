package dashboard

import (
	"log/slog"
	"net/http"

	"github.com/safaristack/atlas/apps/api/internal/platform/httpx"
)

// Handler serves the executive dashboard (docs/ATLAS.md §8.8, §11). The single
// route is gated by intel.dashboard.read, held by the two CEOs.
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

// Summary handles GET /api/v1/dashboard/executive.
func (h *Handler) Summary(w http.ResponseWriter, r *http.Request) {
	sum, err := h.service.Summary(r.Context())
	if err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, sum)
}
