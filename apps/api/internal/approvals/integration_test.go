package approvals

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/safaristack/atlas/apps/api/internal/outbox"
)

// These are database integration tests: they run the engine's real SQL against
// a real Postgres. They are the guard that was missing when core.approvals
// drifted from this package — `go build` and unit tests never touch the
// database, so the mismatch only showed up at runtime.
//
// CI exports ATLAS_TEST_DATABASE_URL and applies the migrations first
// (.github/workflows/ci.yml). Without the variable the tests skip, so a local
// `go test ./...` still passes without a database. The connecting role must be
// allowed to TRUNCATE the approvals and outbox tables — the database owner (as
// CI uses), not the atlas_api application role, which deliberately cannot.

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	dsn := os.Getenv("ATLAS_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("ATLAS_TEST_DATABASE_URL is not set; skipping approvals integration test")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect to test database: %v", err)
	}
	t.Cleanup(pool.Close)

	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("ping test database: %v", err)
	}
	return pool
}

func newTestService(t *testing.T) (*Service, *pgxpool.Pool) {
	t.Helper()

	pool := testPool(t)
	if _, err := pool.Exec(context.Background(),
		`TRUNCATE core.approval_history, core.approval_decisions, core.approvals, outbox.events RESTART IDENTITY CASCADE`,
	); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	return NewService(NewPGStore(pool), nil, nil), pool
}

func mustNewID(t *testing.T) uuid.UUID {
	t.Helper()
	id, err := uuid.NewV7()
	if err != nil {
		t.Fatalf("generate uuid: %v", err)
	}
	return id
}

func countRows(t *testing.T, pool *pgxpool.Pool, query string, args ...any) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), query, args...).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	return n
}

// TestApprovalLifecyclePersistsAndEmitsEvents covers the happy path end to end:
// the insert (every column the engine reads), the outbox event and the history
// row, then a decision and its own event and history row.
func TestApprovalLifecyclePersistsAndEmitsEvents(t *testing.T) {
	ctx := context.Background()
	svc, pool := newTestService(t)

	actor := mustNewID(t)
	entity := mustNewID(t)
	due := time.Now().UTC().Add(time.Hour).Truncate(time.Microsecond)

	a, err := svc.Request(ctx, Request{
		EntityType:   "invoice",
		EntityID:     entity,
		RequestedBy:  actor,
		AmountMinor:  150000,
		Currency:     "KES",
		ApproverRole: "ceo_internal",
		PolicyRef:    "policy/payments",
		DueAt:        &due,
	}, &actor)
	if err != nil {
		t.Fatalf("Request: %v", err)
	}
	if a.Ref == "" {
		t.Fatal("Request: empty ref")
	}
	if a.Status != StatusRequested {
		t.Fatalf("Request: status = %q, want %q", a.Status, StatusRequested)
	}
	if a.Version != 1 {
		t.Fatalf("Request: version = %d, want 1", a.Version)
	}

	got, err := svc.Get(ctx, a.ID)
	if err != nil {
		t.Fatalf("Get after Request: %v", err)
	}
	if got.AmountMinor != 150000 || got.Currency != "KES" || got.ApproverRole != "ceo_internal" {
		t.Fatalf("Get after Request: fields did not round-trip: %+v", got)
	}
	if got.DueAt == nil || !got.DueAt.Equal(due) {
		t.Fatalf("Get after Request: due_at = %v, want %v", got.DueAt, due)
	}

	byRef, err := NewPGStore(pool).GetByRef(ctx, a.Ref)
	if err != nil {
		t.Fatalf("GetByRef: %v", err)
	}
	if byRef == nil || byRef.ID != a.ID {
		t.Fatalf("GetByRef: got %+v, want approval %s", byRef, a.ID)
	}

	decider := mustNewID(t)
	if err := svc.Decide(ctx, Decide{ApprovalID: a.ID, ActorID: decider, Approve: true, Reason: "approved by test"}); err != nil {
		t.Fatalf("Decide: %v", err)
	}

	got, err = svc.Get(ctx, a.ID)
	if err != nil {
		t.Fatalf("Get after Decide: %v", err)
	}
	if got.Status != StatusApproved {
		t.Fatalf("after Decide: status = %q, want %q", got.Status, StatusApproved)
	}
	if got.Version != 2 {
		t.Fatalf("after Decide: version = %d, want 2", got.Version)
	}
	if got.DecidedBy == nil || *got.DecidedBy != decider {
		t.Fatalf("after Decide: decided_by = %v, want %s", got.DecidedBy, decider)
	}
	if got.DecidedAt == nil {
		t.Fatal("after Decide: decided_at is nil")
	}
	if got.Reason != "approved by test" {
		t.Fatalf("after Decide: reason = %q", got.Reason)
	}

	if n := countRows(t, pool, `SELECT count(*) FROM outbox.events WHERE aggregate_id = $1`, a.ID); n != 2 {
		t.Fatalf("outbox events for approval = %d, want 2", n)
	}
	if n := countRows(t, pool, `SELECT count(*) FROM outbox.events WHERE type = $1`, outbox.TypeApprovalRequestedV1); n != 1 {
		t.Fatalf("requested events = %d, want 1", n)
	}
	if n := countRows(t, pool, `SELECT count(*) FROM outbox.events WHERE type = $1`, outbox.TypeApprovalApprovedV1); n != 1 {
		t.Fatalf("approved events = %d, want 1", n)
	}
	if n := countRows(t, pool, `SELECT count(*) FROM core.approval_history WHERE approval_id = $1`, a.ID); n != 2 {
		t.Fatalf("history rows = %d, want 2", n)
	}
}

// TestApprovalListPendingAndEscalate covers the role inbox and escalation:
// the pending lookup must find the row by approver_role, and escalation must
// move it to the new role.
func TestApprovalListPendingAndEscalate(t *testing.T) {
	ctx := context.Background()
	svc, _ := newTestService(t)

	actor := mustNewID(t)
	a, err := svc.Request(ctx, Request{
		EntityType:   "purchase_order",
		EntityID:     mustNewID(t),
		RequestedBy:  actor,
		ApproverRole: "finance_lead",
	}, &actor)
	if err != nil {
		t.Fatalf("Request: %v", err)
	}

	pending, err := svc.ListPendingForRole(ctx, "finance_lead", 10, nil)
	if err != nil {
		t.Fatalf("ListPendingForRole: %v", err)
	}
	if len(pending) != 1 || pending[0].ID != a.ID {
		t.Fatalf("ListPendingForRole: got %d rows, want the new approval", len(pending))
	}

	if err := svc.Escalate(ctx, Escalate{ApprovalID: a.ID, ActorID: mustNewID(t), ToRole: "ceo_finance", Reason: "overdue"}); err != nil {
		t.Fatalf("Escalate: %v", err)
	}
	got, err := svc.Get(ctx, a.ID)
	if err != nil {
		t.Fatalf("Get after Escalate: %v", err)
	}
	if got.Status != StatusEscalated || got.ApproverRole != "ceo_finance" {
		t.Fatalf("after Escalate: status=%q role=%q, want escalated/ceo_finance", got.Status, got.ApproverRole)
	}
}

// TestApprovalExpirySweep covers the worker path: an overdue approval is
// claimed, then expired with an event.
func TestApprovalExpirySweep(t *testing.T) {
	ctx := context.Background()
	svc, pool := newTestService(t)

	actor := mustNewID(t)
	past := time.Now().UTC().Add(-time.Hour).Truncate(time.Microsecond)

	a, err := svc.Request(ctx, Request{
		EntityType:   "expense_claim",
		EntityID:     mustNewID(t),
		RequestedBy:  actor,
		ApproverRole: "ceo_internal",
		DueAt:        &past,
	}, &actor)
	if err != nil {
		t.Fatalf("Request: %v", err)
	}

	claimed, err := NewPGStore(pool).ClaimExpired(ctx, 10)
	if err != nil {
		t.Fatalf("ClaimExpired: %v", err)
	}
	if len(claimed) != 1 || claimed[0].ID != a.ID {
		t.Fatalf("ClaimExpired: got %d rows, want the overdue approval", len(claimed))
	}

	ev, err := outbox.NewApprovalExpired(nil, a.ID, a.Ref)
	if err != nil {
		t.Fatalf("build expired event: %v", err)
	}
	if err := NewPGStore(pool).ExpireWithEvent(ctx, a.ID, ev); err != nil {
		t.Fatalf("ExpireWithEvent: %v", err)
	}

	got, err := svc.Get(ctx, a.ID)
	if err != nil {
		t.Fatalf("Get after Expire: %v", err)
	}
	if got.Status != StatusExpired {
		t.Fatalf("after Expire: status = %q, want expired", got.Status)
	}
}
