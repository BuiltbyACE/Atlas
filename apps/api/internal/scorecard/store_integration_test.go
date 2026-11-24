package scorecard

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// TestScorecardQueriesRunAgainstTheRealSchema executes every roll-up query on a
// migrated database. In production the commercial tile failed on 12–19 Nov with
// SQLSTATE 42703 because it read a column (estimated_value_minor) that exists on
// crm.leads but not on crm.opportunities; only a real database catches that.
func TestScorecardQueriesRunAgainstTheRealSchema(t *testing.T) {
	url := os.Getenv("ATLAS_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("ATLAS_TEST_DATABASE_URL is not set; skipping scorecard integration test")
	}
	pool, err := pgxpool.New(context.Background(), url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)

	if _, err := NewPGStore(pool).Scorecard(context.Background(), time.Now().UTC()); err != nil {
		t.Fatalf("Scorecard: %v", err)
	}
}
