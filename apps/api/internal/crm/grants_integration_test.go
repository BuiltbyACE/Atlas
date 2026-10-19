package crm

import (
	"context"
	"testing"
)

// TestCRMSchemaGrants guards the same provisioning gap migration 00015 fixed for
// the work schema: a schema created without GRANTs to the runtime roles makes
// every route fail with "permission denied for schema crm" while the Go tests —
// which connect as the schema owner — stay green. Asserting the privileges is
// the only way a test can catch a missing GRANT.
//
// The roles only exist where the platform provisions them (locally by
// infra/compose/postgres/initdb.d/01-roles.sh), so a database without them
// skips rather than fails.
func TestCRMSchemaGrants(t *testing.T) {
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
		if err := pool.QueryRow(ctx, `SELECT has_schema_privilege($1, 'crm', 'USAGE')`, role).Scan(&usage); err != nil {
			t.Fatalf("has_schema_privilege(%s, crm): %v", role, err)
		}
		if !usage {
			t.Fatalf("%s lacks USAGE on the crm schema (see migration 00020)", role)
		}
		for _, table := range []string{"organizations", "contacts", "leads", "opportunities", "interactions", "followups", "proposals"} {
			for _, priv := range []string{"SELECT", "INSERT", "UPDATE", "DELETE"} {
				var ok bool
				if err := pool.QueryRow(ctx, `SELECT has_table_privilege($1, $2, $3)`, role, "crm."+table, priv).Scan(&ok); err != nil {
					t.Fatalf("has_table_privilege(%s, crm.%s, %s): %v", role, table, priv, err)
				}
				if !ok {
					t.Fatalf("%s lacks %s on crm.%s (see migration 00020)", role, priv, table)
				}
			}
		}
	}
}
