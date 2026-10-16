package identity

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
		t.Skip("ATLAS_TEST_DATABASE_URL is not set; skipping identity integration tests")
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

func mustPerson(t *testing.T, pool *pgxpool.Pool) uuid.UUID {
	t.Helper()
	id, err := uuid.NewV7()
	if err != nil {
		t.Fatalf("new id: %v", err)
	}
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO people.persons (id, full_name, employment_type) VALUES ($1, $2, 'full_time')`,
		id, "Identity Person"); err != nil {
		t.Fatalf("insert person: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM people.persons WHERE id = $1`, id)
	})
	return id
}

// TestAccountForPerson pins the bridge between the domain person and the login
// account: a linked account resolves, an accountless person answers nil (not an
// error), and the nil person is no address at all.
func TestAccountForPerson(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)

	// A person exists independently of a login.
	lonely := mustPerson(t, pool)
	got, err := AccountForPerson(ctx, pool, lonely)
	if err != nil {
		t.Fatalf("AccountForPerson(no account): %v", err)
	}
	if got != nil {
		t.Fatalf("account for accountless person = %s, want nil", got)
	}

	// A person with an account resolves to that account.
	linked := mustPerson(t, pool)
	accountID, err := uuid.NewV7()
	if err != nil {
		t.Fatalf("new id: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO core.user_accounts (id, person_id, email, status) VALUES ($1, $2, $3, 'active')`,
		accountID, linked, "identity-"+accountID.String()+"@example.test"); err != nil {
		t.Fatalf("insert account: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM core.user_accounts WHERE id = $1`, accountID)
	})

	got, err = AccountForPerson(ctx, pool, linked)
	if err != nil {
		t.Fatalf("AccountForPerson: %v", err)
	}
	if got == nil || *got != accountID {
		t.Fatalf("account = %v, want %s", got, accountID)
	}

	// The nil person is simply no address.
	got, err = AccountForPerson(ctx, pool, uuid.Nil)
	if err != nil || got != nil {
		t.Fatalf("AccountForPerson(nil) = (%v, %v), want (nil, nil)", got, err)
	}
}
