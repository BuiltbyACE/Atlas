// Package authz evaluates Atlas permissions (docs/ATLAS.md §6.1, §6.2, §6.4).
//
// The API resolves a Principal once per request (from a session cookie or a
// service token); handlers ask it whether the actor may perform the action.
// Keeping the check a plain method keeps authorisation decisions in one place
// and easy to test.
package authz

import (
	"errors"

	"github.com/google/uuid"
)

// Kind distinguishes human accounts from service identities (§6.1).
type Kind string

const (
	KindUser    Kind = "user"
	KindService Kind = "service"
)

// Scope types for a role grant (§10.3 core.role_grants).
const (
	ScopeCompany      = "company"
	ScopeBusinessUnit = "business_unit"
	ScopeDepartment   = "department"
	ScopeProject      = "project"
	ScopeOwn          = "own"
)

// Grant is a role held by a principal within a scope.
type Grant struct {
	RoleKey   string
	ScopeType string
	ScopeID   uuid.UUID
}

// Principal is the authenticated actor behind a request.
type Principal struct {
	Kind     Kind
	Subject  string // "user:<uuid>" or "svc:<name>"
	UserID   uuid.UUID
	PersonID uuid.UUID
	Email    string
	Name     string

	// Roles lists granted role keys; Grants carries their scopes.
	Roles  []string
	Grants []Grant

	// Permissions is the effective permission set, computed by the store as
	// the union of role-permission mappings over active grants.
	Permissions map[string]struct{}

	// SessionID is the session the principal authenticated with (zero for a
	// service identity).
	SessionID uuid.UUID
}

// Can reports whether the principal holds a permission.
func (p *Principal) Can(permission string) bool {
	if p == nil || permission == "" {
		return false
	}
	_, ok := p.Permissions[permission]
	return ok
}

// HasRole reports whether the principal holds a role key.
func (p *Principal) HasRole(role string) bool {
	for _, r := range p.Roles {
		if r == role {
			return true
		}
	}
	return false
}

// IsUser reports whether the principal is a human account.
func (p *Principal) IsUser() bool { return p != nil && p.Kind == KindUser }

// IsService reports whether the principal is a service identity.
func (p *Principal) IsService() bool { return p != nil && p.Kind == KindService }

// ── separation of duties (§6.4) ──────────────────────────────
//
// The blueprint is explicit: no one approves their own entry, and a payment
// needs two different people. These helpers make those rules testable.

// ErrSelfApproval is returned when an actor would act on their own record.
var ErrSelfApproval = errors.New("separation of duties: the actor cannot act on their own record")

// ErrSameApprover is returned when the same person is required twice.
var ErrSameApprover = errors.New("separation of duties: a different, second person is required")

// FourEyes verifies separation of duties for the four-eyes principle: the
// actor and the subject must be different people. When either id is unknown
// (for example a system-generated record), it passes.
func FourEyes(actor, subject uuid.UUID) error {
	if actor == uuid.Nil || subject == uuid.Nil {
		return nil
	}
	if actor == subject {
		return ErrSelfApproval
	}
	return nil
}

// TwoPersonRule verifies that two approvals came from different people (§6.4).
func TwoPersonRule(first, second uuid.UUID) error {
	if first == uuid.Nil || second == uuid.Nil {
		return errors.New("separation of duties: both approvers are required")
	}
	if first == second {
		return ErrSameApprover
	}
	return nil
}
