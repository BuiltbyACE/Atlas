// Package httpserver wires the Atlas HTTP surface: middleware, routing, and
// the health endpoints (docs/ATLAS.md §11, §14).
//
// The router is assembled from explicit pieces so that adding a route is a
// deliberate act and every route inherits the same conventions: request ids,
// structured logging, panic recovery, security headers and RFC 9457 errors.
package httpserver

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/safaristack/atlas/apps/api/internal/platform/config"
	"github.com/safaristack/atlas/apps/api/internal/platform/health"
	"github.com/safaristack/atlas/apps/api/internal/platform/httpx"
)

// Deps are the collaborators the router needs. Passing them explicitly keeps
// the router testable without a global container.
type Deps struct {
	Cfg     *config.Config
	Logger  *slog.Logger
	DB      *pgxpool.Pool
	Version string
}

// NewRouter builds the root http.Handler for atlas-api.
func NewRouter(d Deps) http.Handler {
	r := chi.NewRouter()

	// Order matters: a request id must exist before anything logs or recovers.
	r.Use(chimw.RealIP)
	r.Use(httpx.RequestID)
	r.Use(httpx.Recoverer(d.Logger))
	r.Use(httpx.RequestLogger(d.Logger))
	r.Use(httpx.SecureHeaders)
	r.Use(chimw.Timeout(30 * time.Second))

	// Router-level error responses are Problem Details too (§11).
	r.NotFound(httpx.NotFoundHandler)
	r.MethodNotAllowed(httpx.MethodNotAllowedHandler)

	checker := health.NewChecker(d.DB)

	r.Route("/api/v1", func(r chi.Router) {
		// Liveness: is the process up? Readiness: can it serve? (§11)
		r.Get("/health", health.LiveHandler(checker))
		r.Get("/health/ready", health.ReadyHandler(checker))

		r.Get("/", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			_, _ = w.Write([]byte(`{"service":"atlas-api","version":"` + d.Version + `"}`))
		})

		// Development-only routes. Guarded by ATLAS_ENV=local so they can never
		// be reached in staging or production (§2.10, §13).
		if d.Cfg.IsLocal() {
			mountDevRoutes(r, d)
		}
	})

	return r
}
