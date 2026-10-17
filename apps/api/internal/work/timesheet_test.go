package work

import (
	"context"
	"testing"
)

// TestTimesheetPolicyRequired pins D7's decided rule (ADR-019): only the
// configured project types require timesheets, compared case-insensitively.
func TestTimesheetPolicyRequired(t *testing.T) {
	def := DefaultTimesheetPolicy()
	cases := []struct {
		projectType string
		want        bool
	}{
		{"client_delivery", true},
		{"Client_Delivery", true},
		{" client_delivery ", true},
		{"internal_product", false},
		{"maintenance_support", false},
		{"business_unit", false},
		{"", false},
	}
	for _, tc := range cases {
		if got := def.Required(tc.projectType); got != tc.want {
			t.Errorf("DefaultTimesheetPolicy().Required(%q) = %v, want %v", tc.projectType, got, tc.want)
		}
	}

	// An empty list requires nothing; a multi-value list requires each.
	if (TimesheetPolicy{}).Required("client_delivery") {
		t.Error("empty policy should require no project type")
	}
	multi := TimesheetPolicy{RequiredProjectTypes: []string{"client_delivery", "maintenance_support"}}
	if !multi.Required("maintenance_support") || multi.Required("internal_product") {
		t.Error("multi-type policy matched the wrong project types")
	}
}

// TestTimesheetPolicyNormalize checks the normalisation the settings reader
// applies: trim, lowercase and de-duplicate.
func TestTimesheetPolicyNormalize(t *testing.T) {
	p := TimesheetPolicy{RequiredProjectTypes: []string{" Client_Delivery ", "client_delivery", "", "  "}}.normalize()
	if len(p.RequiredProjectTypes) != 1 || p.RequiredProjectTypes[0] != "client_delivery" {
		t.Fatalf("normalize = %#v, want [client_delivery]", p.RequiredProjectTypes)
	}
}

func TestParseProjectTypes(t *testing.T) {
	got := parseProjectTypes(" client_delivery , maintenance_support ,, ")
	want := []string{"client_delivery", "maintenance_support"}
	if len(got) != len(want) {
		t.Fatalf("parseProjectTypes = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("parseProjectTypes = %#v, want %#v", got, want)
		}
	}
	if got := parseProjectTypes(""); len(got) != 0 {
		t.Fatalf("parseProjectTypes(\"\") = %#v, want empty", got)
	}
}

// TestStaticTimesheetPolicy checks the test-only reader normalises too.
func TestStaticTimesheetPolicy(t *testing.T) {
	r := StaticTimesheetPolicy{Value: TimesheetPolicy{RequiredProjectTypes: []string{" Client_Delivery "}}}
	p, err := r.TimesheetPolicy(context.Background())
	if err != nil {
		t.Fatalf("TimesheetPolicy: %v", err)
	}
	if !p.Required("client_delivery") {
		t.Fatal("static policy lost its required type")
	}
}
