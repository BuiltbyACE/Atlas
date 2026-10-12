package seed

import "testing"

func TestCatalogueIsConsistent(t *testing.T) {
	if err := Validate(); err != nil {
		t.Fatalf("catalogue invalid: %v", err)
	}
}

func TestCatalogueCoversBlueprintRoles(t *testing.T) {
	// §3.2 lists ten roles; the seed must define all of them.
	want := []string{
		"ceo_commercial", "ceo_internal", "finance_officer", "sales_rep", "project_lead",
		"engineer", "staff", "unit_manager", "auditor", "system_admin",
	}
	got := make(map[string]bool, len(Roles))
	for _, r := range Roles {
		got[r.Key] = true
	}
	for _, r := range want {
		if !got[r] {
			t.Errorf("role %q from §3.2 missing from catalogue", r)
		}
	}
	if len(Roles) != len(want) {
		t.Errorf("role count = %d, want %d", len(Roles), len(want))
	}
}

func TestSensitivePermissionsAreFlagged(t *testing.T) {
	sensitive := map[string]bool{}
	for _, p := range Permissions {
		if p.Sensitive {
			sensitive[p.Key] = true
		}
	}
	for _, key := range []string{
		"core.audit.read", "people.person.sensitive.read",
		"contracts.contract_value.sensitive.read", "finance.amount.sensitive.read",
	} {
		if !sensitive[key] {
			t.Errorf("permission %q should be sensitive", key)
		}
	}
}
