package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrAccountExists is returned when bootstrapping an email that is already in use.
var ErrAccountExists = errors.New("auth: an account with that email already exists")

// bootstrapScopeTypes mirrors the core.role_grants CHECK constraint (§10.3).
var bootstrapScopeTypes = map[string]bool{
	"company":       true,
	"business_unit": true,
	"department":    true,
	"project":       true,
	"own":           true,
}

// privilegedRoles is the set of roles for which TOTP is mandatory from day one
// (ATLAS.md §13.1, ADR-008): the two CEOs, the finance officer, the system
// administrator and the auditor. Creating or inviting one of these sets
// mfa_required, so the first sign-in is an enrolment.
var privilegedRoles = map[string]bool{
	"ceo_commercial":  true,
	"ceo_internal":    true,
	"finance_officer": true,
	"system_admin":    true,
	"auditor":         true,
}

// RoleRequiresMFA reports whether holding the given role forces multi-factor
// authentication (ATLAS.md §13.1, ADR-008).
func RoleRequiresMFA(roleKey string) bool {
	return privilegedRoles[roleKey]
}

// inviteRequiresMFA reports whether any of an invitation's role grants is
// privileged, so the account it creates must enrol TOTP.
func inviteRequiresMFA(inv *Invite) bool {
	if inv == nil {
		return false
	}
	for _, grant := range inv.RoleGrants {
		if privilegedRoles[grant.RoleKey] {
			return true
		}
	}
	return false
}

// BootstrapInput describes the first account an operator creates by hand.
type BootstrapInput struct {
	Email     string
	Phone     string
	FullName  string
	Password  string
	RoleKey   string
	ScopeType string
}

// Bootstrap creates the first Atlas account: a person, an active user account
// and one role grant, in a single transaction.
//
// It exists because Atlas has no self-service sign-up (§6.1): every ordinary
// account starts from an invitation, and an invitation can only be created by an
// already-authenticated user with `core.user.manage`. The very first account
// therefore has to be created by an operator, deliberately, with real details —
// this is not mock data, it is how the system comes into existence.
//
// It is idempotent in the safe sense: it refuses to touch an email that already
// exists (ErrAccountExists) rather than overwriting a password. Privileged roles
// get mfa_required = true, so sign-in leads to TOTP enrolment.
func Bootstrap(ctx context.Context, db *pgxpool.Pool, in BootstrapInput) (*AcceptedAccount, error) {
	email := strings.ToLower(strings.TrimSpace(in.Email))
	if !strings.Contains(email, "@") {
		return nil, fmt.Errorf("auth: bootstrap: %q is not an email address", in.Email)
	}
	name := strings.TrimSpace(in.FullName)
	if name == "" {
		return nil, errors.New("auth: bootstrap: a full name is required")
	}
	if len(in.Password) < minPasswordLength {
		return nil, ErrPasswordTooShort
	}
	role := strings.TrimSpace(in.RoleKey)
	if role == "" {
		return nil, errors.New("auth: bootstrap: a role is required")
	}

	scope := strings.TrimSpace(in.ScopeType)
	if scope == "" {
		scope = "company"
	}
	if !bootstrapScopeTypes[scope] {
		return nil, fmt.Errorf("auth: bootstrap: unknown scope %q", scope)
	}

	phone := ""
	if p := strings.TrimSpace(in.Phone); p != "" {
		normalized, err := NormalizePhone(p)
		if err != nil {
			return nil, err
		}
		phone = normalized
	}

	hash, err := HashPassword(in.Password, DefaultArgon2Params())
	if err != nil {
		return nil, err
	}

	personID, err := uuid.NewV7()
	if err != nil {
		return nil, fmt.Errorf("auth: bootstrap person id: %w", err)
	}
	accountID, err := uuid.NewV7()
	if err != nil {
		return nil, fmt.Errorf("auth: bootstrap account id: %w", err)
	}

	tx, err := db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("auth: bootstrap begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var exists bool
	if err := tx.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM core.user_accounts WHERE email = $1)`, email).Scan(&exists); err != nil {
		return nil, fmt.Errorf("auth: bootstrap check: %w", err)
	}
	if exists {
		return nil, ErrAccountExists
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO people.persons (id, full_name, work_email, phone_e164, employment_type, status)
		VALUES ($1, $2, $3, $4, 'full_time', 'active')`,
		personID, name, email, nullIfEmpty(phone)); err != nil {
		return nil, fmt.Errorf("auth: bootstrap person: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO core.user_accounts (id, person_id, email, phone_e164, password_hash, mfa_required, status)
		VALUES ($1, $2, $3, $4, $5, $6, 'active')`,
		accountID, personID, email, nullIfEmpty(phone), hash, privilegedRoles[role]); err != nil {
		return nil, fmt.Errorf("auth: bootstrap account: %w", err)
	}

	grantID, err := uuid.NewV7()
	if err != nil {
		return nil, fmt.Errorf("auth: bootstrap grant id: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO core.role_grants (id, user_id, role_key, scope_type, scope_id, granted_by)
		VALUES ($1, $2, $3, $4, NULL, $5)`,
		grantID, accountID, role, scope, accountID); err != nil {
		return nil, fmt.Errorf("auth: bootstrap assign role %q: %w", role, err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("auth: bootstrap commit: %w", err)
	}
	return &AcceptedAccount{AccountID: accountID, PersonID: personID, Email: email}, nil
}
