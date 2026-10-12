package auth

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/safaristack/atlas/apps/api/internal/authz"
	"github.com/safaristack/atlas/apps/api/internal/platform/httpx"
	"github.com/safaristack/atlas/apps/api/internal/platform/routing"
)

// Gate enforces the access rule each route declares (§6.2, §11). It is the
// single place that turns a credential into a principal, so handlers can
// assume one is present on authenticated and gated routes.
type Gate struct {
	service *Service
	cookie  string
	logger  *slog.Logger
}

// NewGate builds a Gate. cookieName is the session cookie to read.
func NewGate(service *Service, cookieName string, logger *slog.Logger) *Gate {
	if logger == nil {
		logger = slog.Default()
	}
	return &Gate{service: service, cookie: cookieName, logger: logger}
}

// Protect wraps a handler according to its route access rule.
func (g *Gate) Protect(route routing.Route, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if route.Access == routing.Public {
			next.ServeHTTP(w, r)
			return
		}

		principal, err := g.authenticate(r)
		if err != nil {
			httpx.WriteProblem(w, r, httpx.Unauthenticated("Your session is missing or has expired."))
			return
		}
		if principal == nil {
			httpx.WriteProblem(w, r, httpx.Unauthenticated("Authentication is required."))
			return
		}

		if route.Access == routing.Permission && !principal.Can(route.Permission) {
			httpx.WriteProblem(w, r, httpx.Forbidden("You do not have permission to perform this action."))
			return
		}

		next.ServeHTTP(w, r.WithContext(authz.WithPrincipal(r.Context(), principal)))
	})
}

// authenticate resolves a principal from a service bearer token, then from the
// session cookie. A missing credential is not an error; a malformed one is.
func (g *Gate) authenticate(r *http.Request) (*authz.Principal, error) {
	if header := r.Header.Get("Authorization"); strings.HasPrefix(strings.ToLower(header), "bearer ") {
		token := strings.TrimSpace(header[len("Bearer "):])
		return g.service.ResolveServiceToken(r.Context(), token)
	}

	cookie, err := r.Cookie(g.cookie)
	if err != nil || strings.TrimSpace(cookie.Value) == "" {
		return nil, nil
	}
	_, principal, err := g.service.ResolveSession(r.Context(), cookie.Value)
	if err != nil {
		return nil, err
	}
	return principal, nil
}
