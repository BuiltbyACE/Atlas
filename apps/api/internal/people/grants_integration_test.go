package people

import (
	"context"
	"testing"
)

// TestPeopleSchemaGrants guards the provisioning gap migration 00016 fixes:
// migration 00013 moved daily_reports and leave_requests from `work` to
// `people` with ALTER TABLE ... SET SCHEMA, which carries a table's ACLs across
// unchanged. They were created in `work` (00011) before work was ever granted,
// so after the move they had none — and the people module failed at runtime
// with "permission denied for table daily_reports" while the Go tests, which
// connect as the schema owner, stayed green. Asserting the privileges is the
// only way a test can catch a missing GRANT.
//
// The roles only exist where the platform provisions them (locally by
// infra/compose/postgres/initdb.d/01-roles.sh), so a database without them
// skips rather than fails.
func TestPeopleSchemaGrants(t *testing.T) {
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
		if err := pool.QueryRow(ctx, `SELECT has_schema_privilege($1, 'people', 'USAGE')`, role).Scan(&usage); err != nil {
			t.Fatalf("has_schema_privilege(%s, people): %v", role, err)
		}
		if !usage {
			t.Fatalf("%s lacks USAGE on the people schema (see migration 00016)", role)
		}
		for _, table := range []string{"daily_reports", "leave_requests"} {
			for _, priv := range []string{"SELECT", "INSERT", "UPDATE", "DELETE"} {
				var ok bool
				if err := pool.QueryRow(ctx, `SELECT has_table_privilege($1, $2, $3)`, role, "people."+table, priv).Scan(&ok); err != nil {
					t.Fatalf("has_table_privilege(%s, people.%s, %s): %v", role, table, priv, err)
				}
				if !ok {
					t.Fatalf("%s lacks %s on people.%s (see migration 00016)", role, priv, table)
				}
			}
		}
	}
}
