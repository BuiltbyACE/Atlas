package navigation

import (
	"encoding/json"
	"sort"
	"strings"
	"testing"

	"github.com/safaristack/atlas/apps/api/internal/authz"
	"github.com/safaristack/atlas/apps/api/internal/seed"
)

// principalWith builds a user principal holding exactly the named permissions.
func principalWith(permissions ...string) *authz.Principal {
	set := make(map[string]struct{}, len(permissions))
	for _, permission := range permissions {
		set[permission] = struct{}{}
	}
	return &authz.Principal{Kind: authz.KindUser, Permissions: set}
}

func catalogue() map[string]bool {
	keys := make(map[string]bool, len(seed.Permissions))
	for _, p := range seed.Permissions {
		keys[p.Key] = true
	}
	return keys
}

// TestRegistryPermissionsExistInCatalogue is the contract the finance bug broke:
// a destination may only name permissions that exist in the §6.2 catalogue, so a
// capability can never drift from what Go actually enforces.
func TestRegistryPermissionsExistInCatalogue(t *testing.T) {
	keys := catalogue()
	for _, section := range Default {
		for _, item := range section.Items {
			for _, permission := range append(append([]string{}, item.View...), item.Actions...) {
				if !keys[permission] {
					t.Errorf("%s (%s) names %q, which is not in the permission catalogue", item.Key, item.Href, permission)
				}
			}
		}
	}
}

// TestRegistryKeysAreUnique keeps the response stable for pinned/recent links
// and for the web app's icon and keyboard handling.
func TestRegistryKeysAreUnique(t *testing.T) {
	keys := map[string]bool{}
	hrefs := map[string]bool{}
	for _, section := range Default {
		if section.Key == "" {
			t.Errorf("section %q has no key", section.Title)
		}
		for _, item := range section.Items {
			if keys[item.Key] {
				t.Errorf("duplicate item key %q", item.Key)
			}
			keys[item.Key] = true
			if hrefs[item.Href] {
				t.Errorf("duplicate href %q", item.Href)
			}
			hrefs[item.Href] = true
			if !strings.HasPrefix(item.Href, "/") {
				t.Errorf("%s has href %q, which is not an absolute web path", item.Key, item.Href)
			}
			if item.Icon == "" {
				t.Errorf("%s has no icon", item.Key)
			}
		}
	}
}

// TestBuildHidesDestinationsWithoutPermission proves the response never names a
// destination the caller cannot open: no disabled entries, no existence leaks.
func TestBuildHidesDestinationsWithoutPermission(t *testing.T) {
	response := Default.Build(principalWith())

	for _, section := range response.Sections {
		for _, item := range section.Items {
			for _, permission := range viewPermissionsOf(t, item.Key) {
				if item.Actions[permission] {
					t.Errorf("%s is visible without its view permission %q", item.Key, permission)
				}
			}
		}
	}
	if got := hrefs(response); !equalStrings(got, []string{"/dashboard", "/dashboard/my-work", "/email/addresses", "/email/inbox", "/integrations/github", "/settings"}) {
		t.Errorf("a person with no permissions saw %v", got)
	}
}

// TestBuildReportsOnlyHeldActions proves each action boolean is the caller's
// effective permission: holding it reports true, losing it reports false.
func TestBuildReportsOnlyHeldActions(t *testing.T) {
	response := Default.Build(principalWith("work.project.read"))

	item := findItem(t, response, "projects")
	if !item.Actions["work.project.read"] {
		t.Error("projects did not report the held view permission")
	}
	if item.Actions["work.project.manage"] {
		t.Error("projects reported work.project.manage, which the principal does not hold")
	}
}

// TestRevokingPermissionRemovesExactlyOneDestination is the revocation guard:
// removing a grant removes the destination it guarded and leaves the rest of
// the map untouched, so nothing survives as an authorization shortcut.
func TestRevokingPermissionRemovesExactlyOneDestination(t *testing.T) {
	before := principalWith(seed.RolePermissions["ceo_commercial"]...)
	held := append([]string{}, seed.RolePermissions["ceo_commercial"]...)

	without := make([]string, 0, len(held))
	for _, permission := range held {
		if permission != "core.approval.read" {
			without = append(without, permission)
		}
	}
	after := principalWith(without...)

	beforeHrefs := hrefs(Default.Build(before))
	afterHrefs := hrefs(Default.Build(after))

	removed := difference(beforeHrefs, afterHrefs)
	if !equalStrings(removed, []string{"/approvals"}) {
		t.Errorf("revoking core.approval.read removed %v, want only /approvals", removed)
	}
	if added := difference(afterHrefs, beforeHrefs); len(added) != 0 {
		t.Errorf("revoking a permission added destinations %v", added)
	}
}

// TestRoleSnapshots records, role by role, what the workspace map becomes. It is
// the reviewable answer to "what does an engineer actually see?" and it fails
// when a permission mapping or a destination changes without a deliberate edit.
func TestRoleSnapshots(t *testing.T) {
	want := map[string][]string{
		"ceo_commercial": {
			"/contracts", "/crm", "/dashboard", "/dashboard/commercial", "/dashboard/daily",
			"/dashboard/my-work", "/dashboard/scorecard", "/email/addresses", "/email/inbox",
			"/finance", "/finance/my-expenses", "/integrations/github", "/intel", "/people",
			"/people/activity", "/daily-reports", "/people/departments", "/settings",
			"/approvals", "/units", "/work", "/work/tasks", "/work/workload", "/work/escalations",
			"/crm/interactions", "/crm/followups", "/crm/proposals",
		},
		"ceo_internal": {
			"/approvals", "/contracts", "/crm", "/crm/followups", "/dashboard",
			"/dashboard/commercial", "/dashboard/daily", "/dashboard/my-work", "/dashboard/scorecard",
			"/daily-reports", "/email/addresses", "/email/inbox", "/finance", "/finance/my-expenses",
			"/integrations/github", "/intel", "/people", "/people/activity", "/people/departments",
			"/people/reviews", "/settings", "/systems", "/units", "/work", "/work/escalations",
			"/work/tasks", "/work/time-entries", "/work/workload",
		},
		"finance_officer": {
			"/approvals", "/contracts", "/dashboard", "/dashboard/my-work", "/email/addresses",
			"/email/inbox", "/finance", "/finance/my-expenses", "/integrations/github", "/people",
			"/people/departments", "/settings", "/units", "/work", "/work/escalations", "/work/tasks",
			"/work/workload",
		},
		"sales_rep": {
			"/contracts", "/crm", "/crm/followups", "/crm/interactions", "/crm/proposals",
			"/dashboard", "/dashboard/my-work", "/email/addresses", "/email/inbox",
			"/integrations/github", "/settings", "/units",
		},
		"project_lead": {
			"/contracts", "/daily-reports", "/dashboard", "/dashboard/my-work", "/email/addresses",
			"/email/inbox", "/finance/my-expenses", "/integrations/github", "/people",
			"/people/departments", "/settings", "/units", "/work", "/work/escalations", "/work/tasks",
			"/work/time-entries", "/work/workload",
		},
		"engineer": {
			"/dashboard", "/dashboard/my-work", "/email/addresses", "/email/inbox",
			"/finance/my-expenses", "/integrations/github", "/people", "/people/departments",
			"/settings", "/systems", "/units", "/work", "/work/escalations", "/work/tasks",
			"/work/workload",
		},
		"staff": {
			"/dashboard", "/dashboard/my-work", "/email/addresses", "/email/inbox",
			"/finance/my-expenses", "/integrations/github", "/people", "/people/departments",
			"/settings", "/units", "/work", "/work/escalations", "/work/tasks", "/work/workload",
		},
		"unit_manager": {
			"/crm/followups", "/daily-reports", "/dashboard", "/dashboard/my-work",
			"/email/addresses", "/email/inbox", "/finance", "/finance/my-expenses",
			"/integrations/github", "/people", "/people/departments", "/settings", "/units", "/work",
			"/work/escalations", "/work/tasks", "/work/workload",
		},
		"auditor": {
			"/approvals", "/contracts", "/dashboard", "/dashboard/my-work", "/email/addresses",
			"/email/inbox", "/finance", "/finance/my-expenses", "/integrations/github", "/people",
			"/people/departments", "/settings",
		},
		"system_admin": {
			"/dashboard", "/dashboard/my-work", "/email/addresses", "/email/inbox",
			"/integrations/github", "/people", "/people/activity", "/people/departments",
			"/people/onboard", "/settings", "/systems",
		},
	}

	roles := make([]string, 0, len(want))
	for role := range want {
		roles = append(roles, role)
	}
	sort.Strings(roles)
	for _, role := range roles {
		t.Run(role, func(t *testing.T) {
			permissions, ok := seed.RolePermissions[role]
			if !ok {
				t.Fatalf("no permission mapping for role %q", role)
			}
			got := hrefs(Default.Build(principalWith(permissions...)))
			wantSorted := append([]string{}, want[role]...)
			sort.Strings(wantSorted)
			if !equalStrings(got, wantSorted) {
				t.Errorf("%s sees\n  %v\nwant\n  %v\nmissing: %v\nextra: %v",
					role, got, wantSorted, difference(wantSorted, got), difference(got, wantSorted))
			}
		})
	}
}

// TestBothCEOsCanRunFinanceFromNavigation is the finance bug's regression test:
// both CEOs hold the finance write permissions, so every control the Finance
// page offers must be reported as available to them.
func TestBothCEOsCanRunFinanceFromNavigation(t *testing.T) {
	required := []string{
		"finance.expense.write",
		"finance.expense.approve",
		"finance.invoice.write",
		"finance.invoice.issue",
		"finance.payment.record",
		"finance.budget.manage",
		"finance.account.manage",
		"finance.recurring_cost.manage",
	}
	for _, role := range []string{"ceo_commercial", "ceo_internal"} {
		response := Default.Build(principalWith(seed.RolePermissions[role]...))
		item := findItem(t, response, "finance")
		for _, permission := range required {
			if !item.Actions[permission] {
				t.Errorf("%s is missing finance action %q", role, permission)
			}
		}
	}

	response := Default.Build(principalWith(seed.RolePermissions["auditor"]...))
	item := findItem(t, response, "finance")
	if item.Actions["finance.expense.write"] || item.Actions["finance.expense.approve"] {
		t.Error("the read-only auditor was offered a finance write action")
	}
	if !item.Actions["finance.expense.read"] {
		t.Error("the auditor was not offered the finance read they hold")
	}
}

// TestResponseNeverCarriesScopeIdentifiers guards the approved decision that the
// navigation response exposes no scope ids or personal data: only keys, labels,
// routes and permission names may appear.
func TestResponseNeverCarriesScopeIdentifiers(t *testing.T) {
	response := Default.Build(principalWith(seed.RolePermissions["ceo_commercial"]...))
	encoded, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}

	var generic map[string]any
	if err := json.Unmarshal(encoded, &generic); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	allowed := map[string]bool{
		"sections": true, "permissions": true, "key": true, "title": true, "items": true,
		"label": true, "href": true, "icon": true, "section": true, "keywords": true,
		"badge": true, "actions": true,
	}
	var walk func(value any, insideActions bool)
	walk = func(value any, insideActions bool) {
		switch typed := value.(type) {
		case map[string]any:
			for key, child := range typed {
				// Keys of an item's actions map are permission names, not
				// field names: the response is keyed by the catalogue, and
				// that is the contract under test.
				if !insideActions && !allowed[key] {
					t.Errorf("navigation response carries unexpected field %q", key)
				}
				walk(child, !insideActions && key == "actions")
			}
		case []any:
			for _, child := range typed {
				walk(child, false)
			}
		}
	}
	walk(generic, false)
}

// ── helpers ──────────────────────────────────────────────────

func viewPermissionsOf(t *testing.T, key string) []string {
	t.Helper()
	for _, section := range Default {
		for _, item := range section.Items {
			if item.Key == key {
				return item.View
			}
		}
	}
	t.Fatalf("no registry item %q", key)
	return nil
}

func findItem(t *testing.T, response Response, key string) ItemView {
	t.Helper()
	for _, section := range response.Sections {
		for _, item := range section.Items {
			if item.Key == key {
				return item
			}
		}
	}
	t.Fatalf("item %q is not in the response", key)
	return ItemView{}
}

func hrefs(response Response) []string {
	out := []string{}
	for _, section := range response.Sections {
		for _, item := range section.Items {
			out = append(out, item.Href)
		}
	}
	sort.Strings(out)
	return out
}

func difference(a, b []string) []string {
	set := make(map[string]bool, len(b))
	for _, value := range b {
		set[value] = true
	}
	out := []string{}
	for _, value := range a {
		if !set[value] {
			out = append(out, value)
		}
	}
	sort.Strings(out)
	return out
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
