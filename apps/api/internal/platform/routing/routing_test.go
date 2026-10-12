package routing

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
)

// fakeGate records which routes it was asked to protect.
type fakeGate struct {
	protected []Route
}

func (g *fakeGate) Protect(route Route, next http.Handler) http.Handler {
	g.protected = append(g.protected, route)
	return next
}

func TestPublicRoutesAreNotGated(t *testing.T) {
	gate := &fakeGate{}
	reg := New()
	reg.Public(http.MethodGet, "/api/v1/health", "liveness", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	r := chi.NewRouter()
	reg.Mount(r, gate)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/health", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if len(gate.protected) != 1 || gate.protected[0].Access != Public {
		t.Fatalf("gate saw %+v, want one public route", gate.protected)
	}
}

func TestPermissionRouteCarriesPermission(t *testing.T) {
	reg := New()
	reg.Gated("core.user.manage", http.MethodPost, "/api/v1/auth/invites", "invite", func(http.ResponseWriter, *http.Request) {})

	routes := reg.Routes()
	if len(routes) != 1 {
		t.Fatalf("routes = %d, want 1", len(routes))
	}
	got := routes[0]
	if got.Access != Permission || got.Permission != "core.user.manage" {
		t.Errorf("route = %+v, want a permission route for core.user.manage", got)
	}
}

func TestAuthenticatedRouteAccess(t *testing.T) {
	reg := New()
	reg.Authenticated(http.MethodGet, "/api/v1/auth/me", "me", func(http.ResponseWriter, *http.Request) {})

	got := reg.Routes()[0]
	if got.Access != Authenticated {
		t.Errorf("access = %q, want %q", got.Access, Authenticated)
	}
}

func TestRoutesReturnsCopy(t *testing.T) {
	reg := New()
	reg.Public(http.MethodGet, "/one", "one", func(http.ResponseWriter, *http.Request) {})

	snapshot := reg.Routes()
	snapshot[0].Pattern = "/mutated"

	if reg.Routes()[0].Pattern != "/one" {
		t.Error("Routes must return a copy, not the backing slice")
	}
}

func TestMountWithNilGateStillRoutes(t *testing.T) {
	reg := New()
	reg.Public(http.MethodGet, "/ping", "ping", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	r := chi.NewRouter()
	reg.Mount(r, nil)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/ping", nil))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
}
