package approvals

import "testing"

func TestPolicyApprovalsRequired(t *testing.T) {
	cases := []struct {
		rule string
		want int
	}{
		{"ceo_internal", 1},
		{RuleDomainCEO, 1},
		{RuleLineManager, 1},
		{RuleBothCEOs, 2},
		{"", 1},
	}
	for _, c := range cases {
		if got := (Policy{ApproverRule: c.rule}).ApprovalsRequired(); got != c.want {
			t.Errorf("ApprovalsRequired(%q) = %d, want %d", c.rule, got, c.want)
		}
	}
}

func TestPolicyRole(t *testing.T) {
	cases := []struct {
		rule string
		want string
	}{
		{"ceo_internal", "ceo_internal"},
		{"finance_officer", "finance_officer"},
		{RuleDomainCEO, ""},
		{RuleLineManager, ""},
		{RuleBothCEOs, ""},
		{"", ""},
	}
	for _, c := range cases {
		if got := (Policy{ApproverRule: c.rule}).Role(); got != c.want {
			t.Errorf("Role(%q) = %q, want %q", c.rule, got, c.want)
		}
	}
}
