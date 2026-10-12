package authz

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

func TestCanUsesEffectivePermissions(t *testing.T) {
	p := &Principal{Permissions: map[string]struct{}{"crm.lead.write": {}}}
	if !p.Can("crm.lead.write") {
		t.Error("expected Can to return true for a granted permission")
	}
	if p.Can("finance.expense.approve") {
		t.Error("a permission not held must be denied")
	}
	if p.Can("") {
		t.Error("empty permission must always be denied")
	}
}

func TestNilPrincipalDenies(t *testing.T) {
	var p *Principal
	if p.Can("anything") {
		t.Error("a nil principal must not hold permissions")
	}
}

func TestHasRole(t *testing.T) {
	p := &Principal{Roles: []string{"engineer", "project_lead"}}
	if !p.HasRole("engineer") {
		t.Error("expected role to be found")
	}
	if p.HasRole("ceo_internal") {
		t.Error("unexpected role found")
	}
}

func TestFourEyes(t *testing.T) {
	a := uuid.New()
	b := uuid.New()

	if err := FourEyes(a, b); err != nil {
		t.Errorf("different people should pass: %v", err)
	}
	if err := FourEyes(a, a); !errors.Is(err, ErrSelfApproval) {
		t.Errorf("self-approval should fail with ErrSelfApproval, got %v", err)
	}
	if err := FourEyes(a, uuid.Nil); err != nil {
		t.Errorf("unknown subject should pass: %v", err)
	}
}

func TestTwoPersonRule(t *testing.T) {
	a := uuid.New()
	b := uuid.New()

	if err := TwoPersonRule(a, b); err != nil {
		t.Errorf("two different approvers should pass: %v", err)
	}
	if err := TwoPersonRule(a, a); !errors.Is(err, ErrSameApprover) {
		t.Errorf("same approver should fail with ErrSameApprover, got %v", err)
	}
	if err := TwoPersonRule(a, uuid.Nil); err == nil {
		t.Error("missing second approver should fail")
	}
}

func TestPrincipalContext(t *testing.T) {
	p := &Principal{Subject: "user:1", Kind: KindUser}
	ctx := WithPrincipal(context.Background(), p)

	got, ok := PrincipalFrom(ctx)
	if !ok || got != p {
		t.Fatalf("PrincipalFrom = (%v, %v)", got, ok)
	}
	if _, ok := PrincipalFrom(context.Background()); ok {
		t.Error("expected no principal in an empty context")
	}
}
