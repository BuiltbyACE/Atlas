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

	"github.com/safaristack/atlas/apps/api/internal/approvals"
	"github.com/safaristack/atlas/apps/api/internal/audit"
	"github.com/safaristack/atlas/apps/api/internal/auth"
	"github.com/safaristack/atlas/apps/api/internal/files"
	"github.com/safaristack/atlas/apps/api/internal/notifications"
	"github.com/safaristack/atlas/apps/api/internal/platform/config"
	"github.com/safaristack/atlas/apps/api/internal/platform/health"
	"github.com/safaristack/atlas/apps/api/internal/platform/httpx"
	"github.com/safaristack/atlas/apps/api/internal/platform/routing"
	"github.com/safaristack/atlas/apps/api/internal/settings"
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
	// Notifications is optional, and only mounted alongside Auth (its routes are
	// authenticated). When nil the notification routes are absent.
	Notifications *notifications.Handler
	Approvals     *approvals.Handler
	Settings      *settings.Handler
	// Audit is the read side of the audit trail (§6.3); mounted when present.
	Audit *audit.Handler
	// Files is optional, and only mounted when object storage is configured
	// (local development may run without a bucket).
	Files *files.Handler
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

	if d.Auth != nil && d.Notifications != nil {
		registerNotifications(reg, d.Notifications)
	}

	if d.Approvals != nil {
		registerApprovals(reg, d.Approvals)
	}

	if d.Settings != nil {
		registerSettings(reg, d.Settings)
	}

	if d.Audit != nil {
		registerAudit(reg, d.Audit)
	}

	if d.Files != nil {
		registerFiles(reg, d.Files)
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

// registerNotifications declares the notification surface (§6.4). The user
// routes are authenticated (you only ever see your own); the internal command is
// service-to-service and gated by a catalogue permission.
func registerNotifications(reg *routing.Registry, h *notifications.Handler) {
	reg.Authenticated(http.MethodGet, "/api/v1/notifications",
		"List your in-app notifications", h.List)
	reg.Authenticated(http.MethodGet, "/api/v1/notifications/unread-count",
		"Count your unread notifications", h.UnreadCount)
	reg.Authenticated(http.MethodPost, "/api/v1/notifications/{id}/read",
		"Mark one notification read", h.MarkRead)
	reg.Authenticated(http.MethodPost, "/api/v1/notifications/read-all",
		"Mark every notification read", h.MarkAllRead)
	reg.Authenticated(http.MethodGet, "/api/v1/notifications/preferences",
		"Your notification preferences", h.Preferences)
	reg.Authenticated(http.MethodPut, "/api/v1/notifications/preferences",
		"Update your notification preferences", h.SavePreferences)

	reg.Gated("core.notification.manage", http.MethodPost, "/internal/v1/commands/notifications.send",
		"Ask Go to notify a person (service-to-service)", h.Send)
}

func registerApprovals(reg *routing.Registry, h *approvals.Handler) {
	reg.Authenticated(http.MethodPost, "/api/v1/approvals", "Request an approval", h.RequestApproval)
	reg.Authenticated(http.MethodGet, "/api/v1/approvals/{id}", "Get an approval", h.Get)
	reg.Authenticated(http.MethodGet, "/api/v1/approvals/pending", "List pending approvals for a role", h.PendingForRole)
	reg.Authenticated(http.MethodGet, "/api/v1/approvals/mine", "List approvals requested by you", h.MyRequested)
	reg.Authenticated(http.MethodPost, "/api/v1/approvals/{id}/approve", "Approve an approval", h.Approve)
	reg.Authenticated(http.MethodPost, "/api/v1/approvals/{id}/decline", "Decline an approval", h.Decline)
	reg.Authenticated(http.MethodPost, "/api/v1/approvals/{id}/escalate", "Escalate an approval", h.Escalate)
	reg.Authenticated(http.MethodPost, "/api/v1/approvals/{id}/cancel", "Cancel an approval", h.Cancel)
	reg.Gated("core.approval_policy.read", http.MethodGet, "/api/v1/approval-policies",
		"List approval policies", h.ListPolicies)
}

// registerSettings declares the configuration store (§6.7). Reading needs
// core.settings.read; writing needs core.settings.manage and carries the
// version it read in If-Match.
func registerSettings(reg *routing.Registry, h *settings.Handler) {
	reg.Gated("core.settings.read", http.MethodGet, "/api/v1/settings",
		"List company settings", h.List)
	reg.Gated("core.settings.read", http.MethodGet, "/api/v1/settings/{key}",
		"Get one setting", h.Get)
	reg.Gated("core.settings.read", http.MethodGet, "/api/v1/settings/{key}/history",
		"List a setting's changes", h.History)
	reg.Gated("core.settings.manage", http.MethodPut, "/api/v1/settings/{key}",
		"Change a setting", h.Set)
}

// registerFiles declares the files surface (§6.5). Reading is core.file.read;
// starting and confirming an upload, and deleting your own file, is
// core.file.write. Bytes never pass through these routes: they carry metadata
// and pre-signed URLs only.
func registerFiles(reg *routing.Registry, h *files.Handler) {
	reg.Gated("core.file.read", http.MethodGet, "/api/v1/files",
		"List files", h.List)
	reg.Gated("core.file.read", http.MethodGet, "/api/v1/files/{id}",
		"Get file metadata", h.Get)
	reg.Gated("core.file.read", http.MethodGet, "/api/v1/files/{id}/download",
		"Issue a download URL", h.Download)
	reg.Gated("core.file.write", http.MethodPost, "/api/v1/files",
		"Start an upload", h.RequestUpload)
	reg.Gated("core.file.write", http.MethodPost, "/api/v1/files/{id}/complete",
		"Confirm an upload", h.Complete)
	reg.Gated("core.file.write", http.MethodDelete, "/api/v1/files/{id}",
		"Delete your file", h.Archive)
}

// registerAudit declares the audit read surface (§6.3). Both routes need
// core.audit.read, which the auditor role and both CEOs hold: the trail is
// global for exactly those principals, and viewable per record for them too.
func registerAudit(reg *routing.Registry, h *audit.Handler) {
	reg.Gated("core.audit.read", http.MethodGet, "/api/v1/audit",
		"List audit events", h.List)
	reg.Gated("core.audit.read", http.MethodGet, "/api/v1/audit/{entity_type}/{entity_id}",
		"Get a record's audit history", h.History)
}
