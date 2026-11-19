package httpserver

import (
	"testing"

	"github.com/safaristack/atlas/apps/api/internal/navigation"
	"github.com/safaristack/atlas/apps/api/internal/platform/routing"
	"github.com/safaristack/atlas/apps/api/internal/seed"
)

// TestNavigationViewPermissionsAreEnforcedByRoutes keeps the workspace map
// honest: every destination's view permission must be enforced by a declared
// route. A destination may never appear because of a permission Go does not
// check, and a nav entry can never be the only thing guarding a page.
//
// The route may use any method: a page that is a pure form (compose, onboard)
// enforces its view permission on the mutation it performs rather than on a
// GET. The point is that the key is live, not that a particular verb carries it.
func TestNavigationViewPermissionsAreEnforcedByRoutes(t *testing.T) {
	_, reg := build(testDeps(t))
	routes := reg.Routes()

	for _, section := range navigation.Default {
		for _, item := range section.Items {
			for _, permission := range item.View {
				if !anyRouteEnforces(routes, permission) {
					t.Errorf("navigation item %s (%s) requires %q, but no route enforces it",
						item.Key, item.Href, permission)
				}
			}
		}
	}
}

// TestApprovalReadRoutesRequireApprovalRead pins the approvals decision: the
// queue and an individual approval are readable only with core.approval.read,
// while the caller's own requests stay self-scoped.
func TestApprovalReadRoutesRequireApprovalRead(t *testing.T) {
	_, reg := build(testDeps(t))

	want := map[string]routing.Access{
		"GET /api/v1/approvals/pending": routing.Permission,
		"GET /api/v1/approvals/{id}":    routing.Permission,
		"GET /api/v1/approvals/mine":    routing.Authenticated,
	}
	for _, rt := range reg.Routes() {
		key := rt.Method + " " + rt.Pattern
		access, ok := want[key]
		if !ok {
			continue
		}
		delete(want, key)
		if rt.Access != access {
			t.Errorf("%s is %q, want %q", key, rt.Access, access)
		}
		if access == routing.Permission && rt.Permission != "core.approval.read" {
			t.Errorf("%s requires %q, want core.approval.read", key, rt.Permission)
		}
	}
	for key := range want {
		t.Errorf("%s is not declared", key)
	}
}

// TestApproversKeepApprovalRead is the stranding guard. Gating the queue on
// core.approval.read is only safe while every role that may decide an approval
// also holds it, so a later mapping edit cannot quietly lock an approver out.
func TestApproversKeepApprovalRead(t *testing.T) {
	for role, permissions := range seed.RolePermissions {
		canDecide := false
		canRead := false
		for _, permission := range permissions {
			switch permission {
			case "core.approval.decide":
				canDecide = true
			case "core.approval.read":
				canRead = true
			}
		}
		if canDecide && !canRead {
			t.Errorf("role %q may decide an approval but cannot read the queue", role)
		}
	}
}

func anyRouteEnforces(routes []routing.Route, permission string) bool {
	for _, rt := range routes {
		if rt.Access == routing.Permission && rt.Permission == permission {
			return true
		}
	}
	return false
}
