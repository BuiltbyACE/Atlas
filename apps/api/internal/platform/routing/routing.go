// Package routing is Atlas's HTTP route registry (docs/ATLAS.md §6.2, §11).
//
// Every route the Go API serves is declared once, together with its access
// rule: public, authenticated, or gated by a permission from the §6.2
// catalogue. Declaring routes in code — next to the handler — makes the
// security surface reviewable: a test walks the mounted router and fails the
// build if a route is missing from the registry or a gated route names a
// permission that does not exist.
package routing

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

// Access describes how a route is protected.
type Access string

const (
	// Public routes need no credential (health, login, JWKS).
	Public Access = "public"
	// Authenticated routes need a valid principal (session or service token).
	Authenticated Access = "authenticated"
	// Permission routes need a principal holding Permission.
	Permission Access = "permission"
)

// Route is one declared endpoint.
type Route struct {
	Method      string
	Pattern     string
	Access      Access
	Permission  string
	Description string
	Handler     http.HandlerFunc
}

// Gate enforces a route's access rule. It is supplied by the API package so
// this package stays free of authentication concerns.
type Gate interface {
	Protect(route Route, next http.Handler) http.Handler
}

// Registry collects routes and mounts them on a chi router.
type Registry struct {
	routes []Route
}

// New returns an empty registry.
func New() *Registry { return &Registry{} }

// Add registers a fully-specified route.
func (reg *Registry) Add(rt Route) *Registry {
	reg.routes = append(reg.routes, rt)
	return reg
}

// Public registers an unauthenticated route.
func (reg *Registry) Public(method, pattern, description string, h http.HandlerFunc) *Registry {
	return reg.Add(Route{Method: method, Pattern: pattern, Access: Public, Description: description, Handler: h})
}

// Authenticated registers a route that requires any signed-in principal.
func (reg *Registry) Authenticated(method, pattern, description string, h http.HandlerFunc) *Registry {
	return reg.Add(Route{Method: method, Pattern: pattern, Access: Authenticated, Description: description, Handler: h})
}

// Gated registers a route that requires a named permission (§6.2).
func (reg *Registry) Gated(permission, method, pattern, description string, h http.HandlerFunc) *Registry {
	return reg.Add(Route{Method: method, Pattern: pattern, Access: Permission, Permission: permission, Description: description, Handler: h})
}

// Routes returns a copy of the declared routes.
func (reg *Registry) Routes() []Route {
	out := make([]Route, len(reg.routes))
	copy(out, reg.routes)
	return out
}

// Mount registers every route on r, wrapping each handler with gate.
func (reg *Registry) Mount(r chi.Router, gate Gate) {
	for _, rt := range reg.routes {
		var h http.Handler = rt.Handler
		if gate != nil {
			h = gate.Protect(rt, h)
		}
		r.Method(rt.Method, rt.Pattern, h)
	}
}
