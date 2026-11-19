package auth

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/safaristack/atlas/apps/api/internal/platform/config"
	"github.com/safaristack/atlas/apps/api/internal/seed"
)

func TestValidateE2EProvision(t *testing.T) {
	valid := E2EProvisionInput{
		Env:      config.EnvLocal,
		Enabled:  true,
		Email:    "E2E@Example.Test",
		FullName: "Atlas E2E",
		Password: "E2eTestPassword123!",
	}

	tests := []struct {
		name   string
		mutate func(*E2EProvisionInput)
		want   string
	}{
		{
			name:   "requires explicit opt-in",
			mutate: func(in *E2EProvisionInput) { in.Enabled = false },
		},
		{
			name:   "rejects production",
			mutate: func(in *E2EProvisionInput) { in.Env = config.EnvProduction },
		},
		{
			name:   "rejects staging",
			mutate: func(in *E2EProvisionInput) { in.Env = config.EnvStaging },
		},
		{
			name:   "rejects a real email domain",
			mutate: func(in *E2EProvisionInput) { in.Email = "person@safaristack.com" },
		},
		{
			name:   "rejects a missing name",
			mutate: func(in *E2EProvisionInput) { in.FullName = "  " },
		},
		{
			name:   "rejects a short password",
			mutate: func(in *E2EProvisionInput) { in.Password = "short" },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := valid
			tt.mutate(&in)
			if _, err := ValidateE2EProvision(in); err == nil {
				t.Fatal("ValidateE2EProvision() = nil error, want refusal")
			}
		})
	}

	got, err := ValidateE2EProvision(valid)
	if err != nil {
		t.Fatalf("ValidateE2EProvision(valid) = %v", err)
	}
	if got != "e2e@example.test" {
		t.Fatalf("normalised email = %q, want %q", got, "e2e@example.test")
	}
}

// TestE2EPermissionsAreMinimal pins the exact set the Playwright suite is
// allowed. Adding a capability to the test identity has to be a deliberate edit
// here, and every key must exist in the seeded catalogue.
func TestE2EPermissionsAreMinimal(t *testing.T) {
	want := []string{
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
	if len(E2EPermissions) != len(want) {
		t.Fatalf("E2EPermissions has %d keys, want exactly %d", len(E2EPermissions), len(want))
	}
	got := make(map[string]bool, len(E2EPermissions))
	for _, key := range E2EPermissions {
		if got[key] {
			t.Errorf("duplicate E2E permission %q", key)
		}
		got[key] = true
	}
	for _, key := range want {
		if !got[key] {
			t.Errorf("E2E permission set is missing %q", key)
		}
	}

	known := make(map[string]bool, len(seed.Permissions))
	for _, p := range seed.Permissions {
		known[p.Key] = true
	}
	for _, key := range E2EPermissions {
		if !known[key] {
			t.Errorf("E2E permission %q does not exist in the seeded catalogue", key)
		}
	}

	for _, privileged := range []string{
		"core.user.manage", "core.role.manage", "core.settings.manage",
		"core.audit.read", "finance.expense.approve", "finance.amount.sensitive.read",
		"contracts.contract_value.sensitive.read", "people.person.manage",
	} {
		if got[privileged] {
			t.Errorf("E2E identity must not hold %q", privileged)
		}
	}
}

// TestProvisionE2E exercises the real SQL against a disposable database. It is
// skipped when ATLAS_TEST_DATABASE_URL is not set, the same contract the other
// auth store integration tests use.
func TestProvisionE2E(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)

	suffix := uuid.NewString()[:8]
	in := E2EProvisionInput{
		Env:      config.EnvLocal,
		Enabled:  true,
		Email:    "e2e-" + suffix + "@example.test",
		FullName: "Atlas E2E " + suffix,
		Password: "E2eTestPassword123!",
	}

	account, err := ProvisionE2E(ctx, pool, in)
	if err != nil {
		t.Fatalf("ProvisionE2E: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM core.role_grants WHERE user_id = $1`, account.AccountID)
		_, _ = pool.Exec(ctx, `DELETE FROM core.user_accounts WHERE id = $1`, account.AccountID)
		_, _ = pool.Exec(ctx, `DELETE FROM people.persons WHERE id = $1`, account.PersonID)
		_, _ = pool.Exec(ctx, `DELETE FROM core.role_permissions WHERE role_key = $1`, E2ERoleKey)
		_, _ = pool.Exec(ctx, `DELETE FROM core.roles WHERE key = $1`, E2ERoleKey)
	})

	var mfaRequired bool
	var status string
	if err := pool.QueryRow(ctx,
		`SELECT mfa_required, status FROM core.user_accounts WHERE id = $1`, account.AccountID).
		Scan(&mfaRequired, &status); err != nil {
		t.Fatalf("read account: %v", err)
	}
	if mfaRequired {
		t.Error("E2E account must not require MFA; the browser has no TOTP secret")
	}
	if status != "active" {
		t.Errorf("E2E account status = %q, want active", status)
	}

	principal, err := NewPGStore(pool).LoadPrincipal(ctx, account.AccountID, uuid.Nil)
	if err != nil {
		t.Fatalf("LoadPrincipal: %v", err)
	}
	if len(principal.Roles) != 1 || principal.Roles[0] != E2ERoleKey {
		t.Fatalf("roles = %v, want only %q", principal.Roles, E2ERoleKey)
	}
	if len(principal.Permissions) != len(E2EPermissions) {
		t.Fatalf("effective permissions = %d, want %d", len(principal.Permissions), len(E2EPermissions))
	}
	for _, key := range E2EPermissions {
		if _, ok := principal.Permissions[key]; !ok {
			t.Errorf("missing effective permission %q", key)
		}
	}

	// Re-running is part of the CI contract: it refreshes the same account and
	// replaces the role's permission set rather than creating a second identity.
	again, err := ProvisionE2E(ctx, pool, in)
	if err != nil {
		t.Fatalf("ProvisionE2E (rerun): %v", err)
	}
	if again.AccountID != account.AccountID {
		t.Fatalf("rerun created account %s, want %s", again.AccountID, account.AccountID)
	}

	// A real account on a reserved domain with another role must never be
	// repurposed. This proves the guard rather than only its happy path.
	personID := uuid.Must(uuid.NewV7())
	otherID := uuid.Must(uuid.NewV7())
	collisionEmail := "collision-" + suffix + "@example.test"
	if _, err := pool.Exec(ctx, `
		INSERT INTO people.persons (id, full_name, employment_type) VALUES ($1, 'Collision Person', 'full_time')`,
		personID); err != nil {
		t.Fatalf("insert collision person: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO core.user_accounts (id, person_id, email, status) VALUES ($1, $2, $3, 'active')`,
		otherID, personID, collisionEmail); err != nil {
		t.Fatalf("insert collision account: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO core.role_grants (id, user_id, role_key, scope_type, granted_by)
		VALUES ($1, $2, 'staff', 'company', $2)`, uuid.Must(uuid.NewV7()), otherID); err != nil {
		t.Fatalf("insert collision grant: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM core.role_grants WHERE user_id = $1`, otherID)
		_, _ = pool.Exec(ctx, `DELETE FROM core.user_accounts WHERE id = $1`, otherID)
		_, _ = pool.Exec(ctx, `DELETE FROM people.persons WHERE id = $1`, personID)
	})

	collision := in
	collision.Email = collisionEmail
	if _, err := ProvisionE2E(ctx, pool, collision); err == nil || !strings.Contains(err.Error(), "already holds role") {
		t.Fatalf("ProvisionE2E on an account with another role = %v, want refusal", err)
	}
}
