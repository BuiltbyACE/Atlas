package dashboard

import (
	"log/slog"
	"net/http"
	"time"

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

// DailyBrief handles GET /api/v1/dashboard/executive/daily.
func (h *Handler) DailyBrief(w http.ResponseWriter, r *http.Request) {
	date := time.Now().UTC()
	if raw := r.URL.Query().Get("date"); raw != "" {
		parsed, err := time.Parse("2006-01-02", raw)
		if err != nil {
			httpx.WriteProblem(w, r, httpx.BadRequest("invalid date"))
			return
		}
		date = parsed
	}
	sum, err := h.service.DailyBrief(r.Context(), date)
	if err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, sum)
}
