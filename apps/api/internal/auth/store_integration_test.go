package auth

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("ATLAS_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("ATLAS_TEST_DATABASE_URL is not set; skipping auth store integration tests")
	}
	pool, err := pgxpool.New(context.Background(), url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	if err := pool.Ping(context.Background()); err != nil {
		t.Fatalf("ping: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func mustExec(t *testing.T, pool *pgxpool.Pool, sql string, args ...any) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), sql, args...); err != nil {
		t.Fatalf("exec %q: %v", sql, err)
	}
}

// TestPGStoreLoadPrincipal guards the fields the API depends on: the person id
// (used as the actor on every user-facing write), the granted roles, and the
// effective permission set resolved from those roles.
func TestPGStoreLoadPrincipal(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	store := NewPGStore(pool)

	personID := uuid.Must(uuid.NewV7())
	userID := uuid.Must(uuid.NewV7())
	grantID := uuid.Must(uuid.NewV7())
	suffix := uuid.NewString()[:8]
	roleKey := "test_role_" + suffix
	permKey := "test.permission." + suffix

	mustExec(t, pool,
		`INSERT INTO people.persons (id, full_name, employment_type) VALUES ($1, $2, 'full_time')`,
		personID, "Principal Person")
	mustExec(t, pool, `
		INSERT INTO core.user_accounts (id, person_id, email, status)
		VALUES ($1, $2, $3, 'active')`, userID, personID, "principal-"+suffix+"@example.com")
	mustExec(t, pool, `INSERT INTO core.roles (key, name) VALUES ($1, $2)`, roleKey, "Test Role")
	mustExec(t, pool, `INSERT INTO core.permissions (key, description) VALUES ($1, $2)`, permKey, "Test permission")
	mustExec(t, pool, `INSERT INTO core.role_permissions (role_key, permission_key) VALUES ($1, $2)`, roleKey, permKey)
	mustExec(t, pool, `
		INSERT INTO core.role_grants (id, user_id, role_key, scope_type, granted_by)
		VALUES ($1, $2, $3, 'company', $2)`, grantID, userID, roleKey)

	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM core.role_grants WHERE id = $1`, grantID)
		_, _ = pool.Exec(ctx, `DELETE FROM core.role_permissions WHERE role_key = $1`, roleKey)
		_, _ = pool.Exec(ctx, `DELETE FROM core.roles WHERE key = $1`, roleKey)
		_, _ = pool.Exec(ctx, `DELETE FROM core.permissions WHERE key = $1`, permKey)
		_, _ = pool.Exec(ctx, `DELETE FROM core.user_accounts WHERE id = $1`, userID)
		_, _ = pool.Exec(ctx, `DELETE FROM people.persons WHERE id = $1`, personID)
	})

	p, err := store.LoadPrincipal(ctx, userID, uuid.Nil)
	if err != nil {
		t.Fatalf("LoadPrincipal: %v", err)
	}
	if p.PersonID != personID {
		t.Fatalf("person id = %s, want %s", p.PersonID, personID)
	}
	if p.UserID != userID {
		t.Fatalf("user id = %s, want %s", p.UserID, userID)
	}
	if !p.Can(permKey) {
		t.Fatalf("permissions = %v, want %q", p.Permissions, permKey)
	}
	if !p.HasRole(roleKey) {
		t.Fatalf("roles = %v, want %q", p.Roles, roleKey)
	}
}
