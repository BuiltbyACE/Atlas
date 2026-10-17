package work

import (
	"context"
	"testing"
)

// TestWorkSchemaGrants guards the provisioning gap migration 00015 fixes:
// migration 00011 created the `work` schema without granting it to the runtime
// roles, so every work route and the escalation sweep failed with "permission
// denied for schema work" while the Go tests — which connect as the schema
// owner — stayed green. Asserting the privileges is the only way a test can
// catch a missing GRANT.
//
// The roles only exist where the platform provisions them (locally by
// infra/compose/postgres/initdb.d/01-roles.sh), so a database without them
// skips rather than fails.
func TestWorkSchemaGrants(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	for _, role := range []string{"atlas_api", "atlas_worker"} {
		var present bool
		if err := pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = $1)`, role).Scan(&present); err != nil {
			t.Fatalf("look up role %s: %v", role, err)
		}
		if !present {
			t.Logf("role %s is not present in this database; skipping", role)
			continue
		}
		var usage bool
		if err := pool.QueryRow(ctx, `SELECT has_schema_privilege($1, 'work', 'USAGE')`, role).Scan(&usage); err != nil {
			t.Fatalf("has_schema_privilege(%s, work): %v", role, err)
		}
		if !usage {
			t.Fatalf("%s lacks USAGE on the work schema (see migration 00015)", role)
		}
		for _, table := range []string{"tasks", "escalations", "time_entries"} {
			for _, priv := range []string{"SELECT", "INSERT", "UPDATE", "DELETE"} {
				var ok bool
				if err := pool.QueryRow(ctx, `SELECT has_table_privilege($1, $2, $3)`, role, "work."+table, priv).Scan(&ok); err != nil {
					t.Fatalf("has_table_privilege(%s, work.%s, %s): %v", role, table, priv, err)
				}
				if !ok {
					t.Fatalf("%s lacks %s on work.%s (see migration 00015)", role, priv, table)
				}
			}
		}
	}
}
