package dashboard

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/safaristack/atlas/apps/api/internal/audit"
)

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("ATLAS_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("ATLAS_TEST_DATABASE_URL is not set; skipping dashboard integration tests")
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

func insertPerson(t *testing.T, pool *pgxpool.Pool, status string) uuid.UUID {
	t.Helper()
	id := uuid.New()
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO people.persons (id, full_name, employment_type, status)
		 VALUES ($1, $2, 'full_time', $3)`,
		id, "Dashboard Test "+id.String(), status); err != nil {
		t.Fatalf("insert person: %v", err)
	}
	return id
}

func insertApproval(t *testing.T, pool *pgxpool.Pool, status, role string, dueAt *time.Time) uuid.UUID {
	t.Helper()
	id := uuid.New()
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO core.approvals (id, entity_type, entity_id, requested_by, status, approver_role, due_at)
		 VALUES ($1, 'dashboard_test', $2, $3, $4, $5, $6)`,
		id, uuid.New(), uuid.New(), status, role, dueAt); err != nil {
		t.Fatalf("insert approval: %v", err)
	}
	return id
}

// TestSummaryReadsRealAggregates inserts people and approvals and checks every
// tile moves.
//
// It is deliberately isolated from the other packages' DB tests:
//
//   - approval role names are unique per run, so the approvals package's role
//     inbox tests never see these rows;
//   - the overdue fixture's due_at is in the future relative to the wall clock,
//     so the global expiry sweep (worker) ignores it, while a later "as at"
//     clock — the dashboard read is a point-in-time snapshot — sees it as
//     overdue;
//   - it never writes or deletes audit rows: audit.events is append-only and its
//     tests toggle the guard trigger, so a second toggler would race them.
func TestSummaryReadsRealAggregates(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	svc := NewService(NewPGStore(pool), audit.NewPGStore(pool), nil)

	realNow := time.Now().UTC()
	fakeNow := realNow.Add(2 * time.Hour)
	svc.now = func() time.Time { return fakeNow }

	before, err := svc.Summary(ctx)
	if err != nil {
		t.Fatalf("before: %v", err)
	}

	roleA := "dashboard_test_a_" + uuid.NewString()
	roleB := "dashboard_test_b_" + uuid.NewString()

	personIDs := []uuid.UUID{
		insertPerson(t, pool, "active"),
		insertPerson(t, pool, "active"),
		insertPerson(t, pool, "on_leave"),
	}
	// Future to the wall clock (invisible to the sweep), overdue to fakeNow.
	futureDue := realNow.Add(time.Hour)
	approvalIDs := []uuid.UUID{
		insertApproval(t, pool, "pending", roleA, nil),
		insertApproval(t, pool, "requested", roleA, &futureDue),
		insertApproval(t, pool, "escalated", roleB, nil),
		insertApproval(t, pool, "approved", roleA, nil), // decided: not open
	}

	t.Cleanup(func() {
		for _, id := range personIDs {
			_, _ = pool.Exec(ctx, `DELETE FROM people.persons WHERE id = $1`, id)
		}
		for _, id := range approvalIDs {
			_, _ = pool.Exec(ctx, `DELETE FROM core.approvals WHERE id = $1`, id)
		}
	})

	after, err := svc.Summary(ctx)
	if err != nil {
		t.Fatalf("after: %v", err)
	}

	if after.People.Total < before.People.Total+3 {
		t.Fatalf("people total = %d, want at least %d", after.People.Total, before.People.Total+3)
	}
	if after.People.ByStatus["active"] < before.People.ByStatus["active"]+2 {
		t.Fatalf("active = %d, want at least %d", after.People.ByStatus["active"], before.People.ByStatus["active"]+2)
	}
	if after.People.ByStatus["on_leave"] < before.People.ByStatus["on_leave"]+1 {
		t.Fatalf("on_leave = %d, want at least %d", after.People.ByStatus["on_leave"], before.People.ByStatus["on_leave"]+1)
	}
	if after.Approvals.Pending < before.Approvals.Pending+2 {
		t.Fatalf("pending = %d, want at least %d", after.Approvals.Pending, before.Approvals.Pending+2)
	}
	if after.Approvals.Escalated < before.Approvals.Escalated+1 {
		t.Fatalf("escalated = %d, want at least %d", after.Approvals.Escalated, before.Approvals.Escalated+1)
	}
	if after.Approvals.Overdue < before.Approvals.Overdue+1 {
		t.Fatalf("overdue = %d, want at least %d", after.Approvals.Overdue, before.Approvals.Overdue+1)
	}
	if got := after.Approvals.PendingByRole[roleA]; got != 2 {
		t.Fatalf("pending_by_role[%s] = %d, want 2", roleA, got)
	}
	if _, ok := after.Approvals.PendingByRole[roleB]; ok {
		t.Fatalf("escalated approval leaked into pending_by_role: %+v", after.Approvals.PendingByRole)
	}

	// The audit read is wired and bounded; the audit package covers its content.
	if after.RecentAudit == nil {
		t.Fatalf("recent_audit is nil, want an empty slice when there is nothing to show")
	}
	if len(after.RecentAudit) > defaultAuditHighlights {
		t.Fatalf("recent_audit = %d entries, want at most %d", len(after.RecentAudit), defaultAuditHighlights)
	}
}
