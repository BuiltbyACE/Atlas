package integrations

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
		t.Skip("ATLAS_TEST_DATABASE_URL is not set; skipping integrations integration tests")
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

// TestPGStoreDeliveryLifecycle exercises the replay-protection contract against
// a real database: the first claim wins, a redelivery loses, and a released
// claim (after a processing failure) can be claimed again.
func TestPGStoreDeliveryLifecycle(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	store := NewPGStore(pool)
	id := "test-delivery-" + uuid.NewString()
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM integrations.github_deliveries WHERE delivery_id = $1`, id)
	})

	claimed, err := store.ClaimDelivery(ctx, id, "pull_request")
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if !claimed {
		t.Fatal("first claim = false, want true")
	}

	claimed, err = store.ClaimDelivery(ctx, id, "pull_request")
	if err != nil {
		t.Fatalf("re-claim: %v", err)
	}
	if claimed {
		t.Fatal("redelivery claim = true, want false")
	}

	if err := store.ReleaseDelivery(ctx, id); err != nil {
		t.Fatalf("release: %v", err)
	}
	claimed, err = store.ClaimDelivery(ctx, id, "pull_request")
	if err != nil {
		t.Fatalf("claim after release: %v", err)
	}
	if !claimed {
		t.Fatal("claim after release = false, want true")
	}
}

// TestIntegrationsSchemaGrants mirrors the work grants guard (00015): the API
// role must hold DML on the integrations schema, or every webhook fails with
// "permission denied for schema integrations" while the Go tests, which connect
// as the owner, stay green.
func TestIntegrationsSchemaGrants(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)

	for _, role := range []string{"atlas_api"} {
		var present bool
		if err := pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = $1)`, role).Scan(&present); err != nil {
			t.Fatalf("look up role %s: %v", role, err)
		}
		if !present {
			t.Logf("role %s is not present in this database; skipping", role)
			continue
		}
		var usage bool
		if err := pool.QueryRow(ctx, `SELECT has_schema_privilege($1, 'integrations', 'USAGE')`, role).Scan(&usage); err != nil {
			t.Fatalf("has_schema_privilege(%s, integrations): %v", role, err)
		}
		if !usage {
			t.Fatalf("%s lacks USAGE on the integrations schema (see migration 00019)", role)
		}
		for _, priv := range []string{"SELECT", "INSERT", "UPDATE", "DELETE"} {
			var ok bool
			if err := pool.QueryRow(ctx, `SELECT has_table_privilege($1, 'integrations.github_deliveries', $2)`, role, priv).Scan(&ok); err != nil {
				t.Fatalf("has_table_privilege(%s, %s): %v", role, priv, err)
			}
			if !ok {
				t.Fatalf("%s lacks %s on integrations.github_deliveries (see migration 00019)", role, priv)
			}
		}
	}
}
