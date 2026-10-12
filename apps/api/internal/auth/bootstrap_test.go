package auth

import "testing"

func TestRoleRequiresMFA(t *testing.T) {
	privileged := []string{
		"ceo_commercial",
		"ceo_internal",
		"finance_officer",
		"system_admin",
		"auditor",
	}
	for _, role := range privileged {
		if !RoleRequiresMFA(role) {
			t.Errorf("RoleRequiresMFA(%q) = false, want true (ATLAS.md §13.1)", role)
		}
	}

	ordinary := []string{"sales_rep", "project_lead", "engineer", "staff", "unit_manager", ""}
	for _, role := range ordinary {
		if RoleRequiresMFA(role) {
			t.Errorf("RoleRequiresMFA(%q) = true, want false", role)
		}
	}
}

func TestInviteRequiresMFA(t *testing.T) {
	tests := []struct {
		name string
		inv  *Invite
		want bool
	}{
		{name: "nil invite", inv: nil, want: false},
		{name: "no grants", inv: &Invite{}, want: false},
		{
			name: "ordinary roles only",
			inv:  &Invite{RoleGrants: []GrantSpec{{RoleKey: "sales_rep"}, {RoleKey: "staff"}}},
			want: false,
		},
		{
			name: "one privileged role",
			inv:  &Invite{RoleGrants: []GrantSpec{{RoleKey: "sales_rep"}, {RoleKey: "finance_officer"}}},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := inviteRequiresMFA(tt.inv); got != tt.want {
				t.Fatalf("inviteRequiresMFA() = %v, want %v", got, tt.want)
			}
		})
	}
}
