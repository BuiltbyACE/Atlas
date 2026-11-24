package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/safaristack/atlas/apps/api/internal/platform/config"
)

// E2ERoleKey is the only role the E2E provisioner will ever grant. It is not
// part of the seeded production catalogue: a deployment that never runs the
// provisioner never has this role at all.
const E2ERoleKey = "e2e_tester"

// E2EPermissions is the exact permission set the Playwright suite needs: enough
// to create and read the work, CRM, contracts and finance records the specs
// exercise, and nothing that administers the system. It deliberately excludes
// every privileged role's capabilities (users, roles, settings, audit, payroll,
// sensitive amounts and approvals).
var E2EPermissions = []string{
	"people.person.read",
	"people.daily_report.read", "people.daily_report.submit",

	"work.project.read", "work.project.manage",
	"work.task.read", "work.task.write",

	"crm.organization.read", "crm.organization.write",
	"crm.lead.read", "crm.lead.write",
	"crm.opportunity.read", "crm.pipeline.read",

	"contracts.contract.read", "contracts.contract.write",

	"finance.report.read", "finance.expense.read", "finance.expense.write",
}

// e2eEmailDomains are the reserved documentation/test domains (RFC 2606). The
// provisioner refuses any other domain so it can never be pointed at a real
// person's work address.
var e2eEmailDomains = []string{
	"@example.com",
	"@example.net",
	"@example.org",
	"@example.test",
	"@example.invalid",
}

// e2eRoleDescription records why the role exists, so an operator who finds it
// in a disposable database knows it is test scaffolding rather than a real seat.
const e2eRoleDescription = "Disposable CI identity created by `atlas-cli e2e provision`; never use outside a throwaway database."

// E2EProvisionInput describes the test-only account a CI job may create.
//
// Env and Enabled are part of the input rather than ambient environment reads so
// the guard is explicit at every call site: the provisioner runs only when the
// caller has separately confirmed local development and opted in.
type E2EProvisionInput struct {
	Env      string
	Enabled  bool
	Email    string
	FullName string
	Password string
}

// ValidateE2EProvision checks the guard and the account details without touching
// the database. It is exported so the CLI can fail before opening a connection.
func ValidateE2EProvision(in E2EProvisionInput) (string, error) {
	if !in.Enabled {
		return "", errors.New("e2e provision: refusing to run without ATLAS_E2E_PROVISION=1")
	}
	if in.Env != config.EnvLocal {
		return "", fmt.Errorf("e2e provision: refusing to run with ATLAS_ENV=%q; only local is allowed", in.Env)
	}

	email := strings.ToLower(strings.TrimSpace(in.Email))
	if !strings.Contains(email, "@") {
		return "", fmt.Errorf("e2e provision: %q is not an email address", in.Email)
	}
	allowed := false
	for _, suffix := range e2eEmailDomains {
		if strings.HasSuffix(email, suffix) {
			allowed = true
			break
		}
	}
	if !allowed {
		return "", fmt.Errorf("e2e provision: %q is not on a reserved example domain (%s)", in.Email, strings.Join(e2eEmailDomains, ", "))
	}
	if strings.TrimSpace(in.FullName) == "" {
		return "", errors.New("e2e provision: a full name is required")
	}
	if len(in.Password) < minPasswordLength {
		return "", ErrPasswordTooShort
	}
	return email, nil
}

// ProvisionE2E creates (or refreshes) the disposable Playwright identity: one
// reserved-domain account, one E2E-only role grant, and the exact permission
// set in E2EPermissions.
//
// It is deliberately not an operator-facing invitation path. The account is
// always non-privileged (mfa_required = false) because privileged roles force
// TOTP enrolment on first sign-in, and a browser test cannot complete an
// enrolment it has no shared secret for. The guard in ValidateE2EProvision
// keeps every other environment out.
//
// Re-running is safe on a disposable database: the role and its permissions are
// replaced wholesale, and the account's password and active status are reset.
// An existing account on the same reserved email is refused if it carries any
// other role, so this can never quietly widen a real account's access.
func ProvisionE2E(ctx context.Context, db *pgxpool.Pool, in E2EProvisionInput) (*AcceptedAccount, error) {
	email, err := ValidateE2EProvision(in)
	if err != nil {
		return nil, err
	}
	name := strings.TrimSpace(in.FullName)

	hash, err := HashPassword(in.Password, DefaultArgon2Params())
	if err != nil {
		return nil, err
	}

	tx, err := db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("e2e provision begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// The catalogue is data, written by `atlas-cli seed`. Fail with the missing
	// keys rather than granting a role that silently references nothing.
	rows, err := tx.Query(ctx, `SELECT key FROM core.permissions WHERE key = ANY($1)`, E2EPermissions)
	if err != nil {
		return nil, fmt.Errorf("e2e provision read permissions: %w", err)
	}
	known := make(map[string]struct{}, len(E2EPermissions))
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			rows.Close()
			return nil, fmt.Errorf("e2e provision scan permission: %w", err)
		}
		known[key] = struct{}{}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("e2e provision read permissions: %w", err)
	}
	var missing []string
	for _, key := range E2EPermissions {
		if _, ok := known[key]; !ok {
			missing = append(missing, key)
		}
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("e2e provision: permissions missing from the catalogue (run `atlas-cli seed` first): %s", strings.Join(missing, ", "))
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO core.roles (key, name, description)
		VALUES ($1, $2, $3)
		ON CONFLICT (key) DO UPDATE
		  SET name = EXCLUDED.name, description = EXCLUDED.description`,
		E2ERoleKey, "E2E test runner", e2eRoleDescription); err != nil {
		return nil, fmt.Errorf("e2e provision role: %w", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM core.role_permissions WHERE role_key = $1`, E2ERoleKey); err != nil {
		return nil, fmt.Errorf("e2e provision clear role: %w", err)
	}
	for _, key := range E2EPermissions {
		if _, err := tx.Exec(ctx,
			`INSERT INTO core.role_permissions (role_key, permission_key) VALUES ($1, $2)`,
			E2ERoleKey, key); err != nil {
			return nil, fmt.Errorf("e2e provision grant %s: %w", key, err)
		}
	}

	var accountID, personID uuid.UUID
	err = tx.QueryRow(ctx,
		`SELECT id, person_id FROM core.user_accounts WHERE email = $1`, email).
		Scan(&accountID, &personID)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		if accountID, personID, err = createE2EAccount(ctx, tx, email, name, hash); err != nil {
			return nil, err
		}
	case err != nil:
		return nil, fmt.Errorf("e2e provision find account: %w", err)
	default:
		if err := assertOnlyE2EGrant(ctx, tx, accountID); err != nil {
			return nil, err
		}
		if _, err := tx.Exec(ctx, `
			UPDATE core.user_accounts
			SET password_hash = $2, mfa_required = false, status = 'active'
			WHERE id = $1`, accountID, hash); err != nil {
			return nil, fmt.Errorf("e2e provision refresh account: %w", err)
		}
		grantID, err := uuid.NewV7()
		if err != nil {
			return nil, fmt.Errorf("e2e provision grant id: %w", err)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO core.role_grants (id, user_id, role_key, scope_type, scope_id, granted_by)
			SELECT $1, $2, $3, 'company', NULL, $2
			WHERE NOT EXISTS (
				SELECT 1 FROM core.role_grants
				WHERE user_id = $2 AND role_key = $3 AND revoked_at IS NULL
			)`, grantID, accountID, E2ERoleKey); err != nil {
			return nil, fmt.Errorf("e2e provision ensure grant: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("e2e provision commit: %w", err)
	}
	return &AcceptedAccount{AccountID: accountID, PersonID: personID, Email: email}, nil
}

// createE2EAccount mirrors Bootstrap's three inserts but forces the E2E role and
// a non-MFA account. It runs inside the provisioner's transaction.
func createE2EAccount(ctx context.Context, tx pgx.Tx, email, name, hash string) (uuid.UUID, uuid.UUID, error) {
	personID, err := uuid.NewV7()
	if err != nil {
		return uuid.Nil, uuid.Nil, fmt.Errorf("e2e provision person id: %w", err)
	}
	accountID, err := uuid.NewV7()
	if err != nil {
		return uuid.Nil, uuid.Nil, fmt.Errorf("e2e provision account id: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO people.persons (id, full_name, work_email, employment_type, status)
		VALUES ($1, $2, $3, 'full_time', 'active')`,
		personID, name, email); err != nil {
		return uuid.Nil, uuid.Nil, fmt.Errorf("e2e provision person: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO core.user_accounts (id, person_id, email, password_hash, mfa_required, status)
		VALUES ($1, $2, $3, $4, false, 'active')`,
		accountID, personID, email, hash); err != nil {
		return uuid.Nil, uuid.Nil, fmt.Errorf("e2e provision account: %w", err)
	}
	grantID, err := uuid.NewV7()
	if err != nil {
		return uuid.Nil, uuid.Nil, fmt.Errorf("e2e provision grant id: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO core.role_grants (id, user_id, role_key, scope_type, scope_id, granted_by)
		VALUES ($1, $2, $3, 'company', NULL, $2)`,
		grantID, accountID, E2ERoleKey); err != nil {
		return uuid.Nil, uuid.Nil, fmt.Errorf("e2e provision grant role: %w", err)
	}
	return accountID, personID, nil
}

// assertOnlyE2EGrant refuses to repurpose an existing account that holds any
// other live role. The reserved email domain makes a collision unlikely; this
// makes it impossible for the provisioner to widen a real account.
func assertOnlyE2EGrant(ctx context.Context, tx pgx.Tx, accountID uuid.UUID) error {
	rows, err := tx.Query(ctx, `
		SELECT role_key FROM core.role_grants
		WHERE user_id = $1 AND revoked_at IS NULL`, accountID)
	if err != nil {
		return fmt.Errorf("e2e provision read grants: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var role string
		if err := rows.Scan(&role); err != nil {
			return fmt.Errorf("e2e provision scan grant: %w", err)
		}
		if role != E2ERoleKey {
			return fmt.Errorf("e2e provision: account %s already holds role %q; refusing to repurpose it", accountID, role)
		}
	}
	return rows.Err()
}
