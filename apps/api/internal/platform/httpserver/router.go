// Package httpserver wires the Atlas HTTP surface: middleware, routing, and
// the health endpoints (docs/ATLAS.md §11, §14).
//
// Every route is declared in a routing.Registry, together with its access
// rule. `build` returns both the handler and the registry so a test can assert
// that the two agree — a route that is not in the registry fails the build
// (§6.2).
package httpserver

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/safaristack/atlas/apps/api/internal/auth"
	"github.com/safaristack/atlas/apps/api/internal/platform/config"
	"github.com/safaristack/atlas/apps/api/internal/platform/health"
	"github.com/safaristack/atlas/apps/api/internal/platform/httpx"
	"github.com/safaristack/atlas/apps/api/internal/platform/routing"
)

// Deps are the collaborators the router needs. Passing them explicitly keeps
// the router testable without a global container.
type Deps struct {
	Cfg     *config.Config
	Logger  *slog.Logger
	DB      *pgxpool.Pool
	Version string
	// Auth is optional. When it is nil the authentication routes are not
	// mounted, which keeps the health-only router simple to test.
	Auth *auth.Service
}

// NewRouter builds the root http.Handler for atlas-api.
func NewRouter(d Deps) http.Handler {
	handler, _ := build(d)
	return handler
}

// build assembles the router and returns its route registry.
func build(d Deps) (http.Handler, *routing.Registry) {
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

	reg := routing.New()
	registerBase(reg, d)

	var gate routing.Gate
	if d.Auth != nil {
		handler := auth.NewHandler(d.Auth, auth.HTTPConfig{
			CookieName:    d.Cfg.SessionCookieName,
			SecureCookies: d.Cfg.Env != config.EnvLocal,
		}, d.Logger)
		registerAuth(reg, handler)
		gate = auth.NewGate(d.Auth, d.Cfg.SessionCookieName, d.Logger)
	}

	reg.Mount(r, gate)
	return r, reg
}

// registerBase declares the always-present routes.
func registerBase(reg *routing.Registry, d Deps) {
	checker := health.NewChecker(d.DB)

	reg.Public(http.MethodGet, "/api/v1/health", "Liveness: is the process up?",
		health.LiveHandler(checker))
	reg.Public(http.MethodGet, "/api/v1/health/ready", "Readiness: can it serve traffic?",
		health.ReadyHandler(checker))
	reg.Public(http.MethodGet, "/api/v1/", "Service name and version",
		func(w http.ResponseWriter, r *http.Request) {
			httpx.WriteJSON(w, http.StatusOK, map[string]string{"service": "atlas-api", "version": d.Version})
		})
}

// registerAuth declares the §6.1 identity routes.
func registerAuth(reg *routing.Registry, h *auth.Handler) {
	reg.Public(http.MethodPost, "/api/v1/auth/login", "Sign in with email/phone and password", h.Login)
	reg.Public(http.MethodPost, "/api/v1/auth/mfa/verify", "Complete sign-in with a TOTP or recovery code", h.MFAVerify)
	reg.Public(http.MethodPost, "/api/v1/auth/mfa/enroll", "Begin MFA enrolment (challenge token)", h.MFAEnroll)
	reg.Public(http.MethodPost, "/api/v1/auth/mfa/activate", "Confirm MFA enrolment (challenge token)", h.MFAActivate)
	reg.Public(http.MethodPost, "/api/v1/auth/invites/accept", "Accept an invitation and set a password", h.AcceptInvite)

	reg.Authenticated(http.MethodPost, "/api/v1/auth/logout", "Revoke the current session", h.Logout)
	reg.Authenticated(http.MethodGet, "/api/v1/auth/me", "The current principal", h.Me)
	reg.Authenticated(http.MethodGet, "/api/v1/auth/sessions", "List your active sessions", h.Sessions)
	reg.Authenticated(http.MethodDelete, "/api/v1/auth/sessions/{id}", "Revoke one of your sessions", h.RevokeSession)
	reg.Authenticated(http.MethodPost, "/api/v1/auth/sessions/revoke-all", "Revoke every session you hold", h.RevokeAllSessions)
	reg.Authenticated(http.MethodPost, "/api/v1/auth/realtime-token", "Mint a short-lived realtime token", h.RealtimeToken)

	reg.Gated("core.user.manage", http.MethodPost, "/api/v1/auth/invites",
		"Invite a person to create an account", h.CreateInvite)

	reg.Public(http.MethodGet, "/internal/v1/.well-known/jwks.json",
		"Public signing keys for realtime token verification", h.JWKS)
}
