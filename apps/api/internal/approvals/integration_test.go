package approvals

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
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
	svc, pool, _ := newTestServiceWithNotifier(t)
	return svc, pool
}

// fakeNotifier records notification intents in memory so fan-out can be
// asserted without an SMTP server or the notifications tables.
type fakeNotifier struct {
	mu   sync.Mutex
	sent []Notification
}

func (f *fakeNotifier) Notify(_ context.Context, n Notification) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, n)
	return nil
}

func (f *fakeNotifier) recipients(template string) []uuid.UUID {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []uuid.UUID
	for _, n := range f.sent {
		if template == "" || n.Template == template {
			out = append(out, n.RecipientPersonID)
		}
	}
	return out
}

func newTestServiceWithNotifier(t *testing.T) (*Service, *pgxpool.Pool, *fakeNotifier) {
	t.Helper()

	pool := testPool(t)
	if _, err := pool.Exec(context.Background(),
		`TRUNCATE core.approval_history, core.approval_decisions, core.approvals,
		 core.approval_policies, outbox.events RESTART IDENTITY CASCADE`,
	); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	notifier := &fakeNotifier{}
	return NewService(NewPGStore(pool), nil, notifier, nil), pool, notifier
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

// ── policy lookup and separation of duties ───────────────────

// insertPolicy writes an approval policy and returns its id.
func insertPolicy(t *testing.T, pool *pgxpool.Pool, decisionType, rule string, currency *string) uuid.UUID {
	t.Helper()
	id := mustNewID(t)
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO core.approval_policies
		  (id, decision_type, min_amount_minor, max_amount_minor, currency, approver_rule, effective_from)
		VALUES ($1,$2,NULL,NULL,$3,$4, now() - interval '1 hour')`,
		id, decisionType, currency, rule); err != nil {
		t.Fatalf("insert policy: %v", err)
	}
	return id
}

// seedPerson inserts a person and returns its id.
func seedPerson(t *testing.T, pool *pgxpool.Pool) uuid.UUID {
	t.Helper()
	id := mustNewID(t)
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO people.persons (id, full_name, work_email, employment_type)
		VALUES ($1,$2,$3,'full_time')`,
		id, "Test Person", fmt.Sprintf("person-%s@example.com", id)); err != nil {
		t.Fatalf("insert person: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM people.persons WHERE id=$1`, id)
	})
	return id
}

// seedPersonWithRole inserts a person with an active user account and a company
// grant of the given role, returning the person id. The caller passes a role
// key unique to the test: fan-out notifies every holder of a role, so a shared
// key (ceo_internal) would make the assertion depend on whatever grants the dev
// database already holds. Everything is removed when the test ends.
func seedPersonWithRole(t *testing.T, pool *pgxpool.Pool, role string) uuid.UUID {
	t.Helper()
	person := seedPerson(t, pool)
	ctx := context.Background()

	if _, err := pool.Exec(ctx, `
		INSERT INTO core.roles (key, name) VALUES ($1,$2)
		ON CONFLICT (key) DO NOTHING`,
		role, "Test Role"); err != nil {
		t.Fatalf("insert role: %v", err)
	}

	account := mustNewID(t)
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO core.user_accounts (id, person_id, email, status)
		VALUES ($1,$2,$3,'active')`,
		account, person, fmt.Sprintf("account-%s@example.com", account)); err != nil {
		t.Fatalf("insert user account: %v", err)
	}
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO core.role_grants (id, user_id, role_key, scope_type, granted_by)
		VALUES ($1,$2,$3,'company',$4)`,
		mustNewID(t), account, role, person); err != nil {
		t.Fatalf("insert role grant: %v", err)
	}

	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM core.role_grants WHERE user_id=$1`, account)
		_, _ = pool.Exec(ctx, `DELETE FROM core.user_accounts WHERE id=$1`, account)
		_, _ = pool.Exec(ctx, `DELETE FROM core.roles WHERE key=$1`, role)
	})
	return person
}

// TestApprovalPolicyResolvesApproverRole proves a policy can supply the
// approver when the caller does not, and that the policy is attached.
func TestApprovalPolicyResolvesApproverRole(t *testing.T) {
	ctx := context.Background()
	svc, pool := newTestService(t)
	policyID := insertPolicy(t, pool, "consulting_expense", "ceo_internal", nil)

	requester := mustNewID(t)
	a, err := svc.Request(ctx, Request{
		EntityType:  "consulting_expense",
		EntityID:    mustNewID(t),
		RequestedBy: requester,
		AmountMinor: 250000,
		Currency:    "KES",
	}, &requester)
	if err != nil {
		t.Fatalf("Request: %v", err)
	}
	if a.ApproverRole != "ceo_internal" {
		t.Fatalf("approver_role = %q, want ceo_internal (from policy)", a.ApproverRole)
	}
	if a.PolicyRef != policyID.String() {
		t.Fatalf("policy_ref = %q, want %s", a.PolicyRef, policyID)
	}
	if a.ApprovalsRequired != 1 {
		t.Fatalf("approvals_required = %d, want 1", a.ApprovalsRequired)
	}
}

// TestApprovalPolicyBothCEOsRequiresTwo checks that a two-person policy makes
// the approval require two distinct approvals.
func TestApprovalPolicyBothCEOsRequiresTwo(t *testing.T) {
	ctx := context.Background()
	svc, pool := newTestService(t)
	insertPolicy(t, pool, "capex", RuleBothCEOs, nil)

	requester := mustNewID(t)
	a, err := svc.Request(ctx, Request{
		EntityType:   "capex",
		EntityID:     mustNewID(t),
		RequestedBy:  requester,
		ApproverRole: "ceo_internal",
	}, &requester)
	if err != nil {
		t.Fatalf("Request: %v", err)
	}
	if a.ApprovalsRequired != 2 {
		t.Fatalf("approvals_required = %d, want 2", a.ApprovalsRequired)
	}
}

// TestApprovalPolicyUnresolvableRuleIsCallerError checks that a rule which does
// not name a single role (both_ceos) without an explicit approver is rejected
// rather than silently defaulted.
func TestApprovalPolicyUnresolvableRuleIsCallerError(t *testing.T) {
	ctx := context.Background()
	svc, pool := newTestService(t)
	insertPolicy(t, pool, "capex", RuleBothCEOs, nil)

	requester := mustNewID(t)
	_, err := svc.Request(ctx, Request{
		EntityType:  "capex",
		EntityID:    mustNewID(t),
		RequestedBy: requester,
	}, &requester)
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("Request with unresolvable rule: err = %v, want ErrInvalid", err)
	}
}

// TestApprovalRejectsSelfApproval is the four-eyes rule: nobody approves their
// own request (§3.3, §6.4).
func TestApprovalRejectsSelfApproval(t *testing.T) {
	ctx := context.Background()
	svc, _ := newTestService(t)

	requester := mustNewID(t)
	a, err := svc.Request(ctx, Request{
		EntityType:   "invoice",
		EntityID:     mustNewID(t),
		RequestedBy:  requester,
		ApproverRole: "ceo_internal",
	}, &requester)
	if err != nil {
		t.Fatalf("Request: %v", err)
	}
	err = svc.Decide(ctx, Decide{ApprovalID: a.ID, ActorID: requester, Approve: true})
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("self-approval: err = %v, want ErrForbidden", err)
	}
}

// TestTwoPersonApprovalNeedsTwoDistinctPeople covers the both-CEO decision: the
// first approval leaves it pending, the same person cannot approve twice and a
// different person completes it.
func TestTwoPersonApprovalNeedsTwoDistinctPeople(t *testing.T) {
	ctx := context.Background()
	svc, pool, _ := newTestServiceWithNotifier(t)

	requester := mustNewID(t)
	first := mustNewID(t)
	second := mustNewID(t)

	a, err := svc.Request(ctx, Request{
		EntityType:        "capex",
		EntityID:          mustNewID(t),
		RequestedBy:       requester,
		ApproverRole:      "ceo_internal",
		ApprovalsRequired: 2,
	}, &requester)
	if err != nil {
		t.Fatalf("Request: %v", err)
	}

	if err := svc.Decide(ctx, Decide{ApprovalID: a.ID, ActorID: first, Approve: true, Reason: "first yes"}); err != nil {
		t.Fatalf("first Decide: %v", err)
	}
	got, err := svc.Get(ctx, a.ID)
	if err != nil {
		t.Fatalf("Get after first: %v", err)
	}
	if got.Status != StatusPending {
		t.Fatalf("after first approval: status = %q, want pending", got.Status)
	}
	if n := countRows(t, pool, `SELECT count(*) FROM outbox.events WHERE type = $1`, outbox.TypeApprovalApprovedV1); n != 0 {
		t.Fatalf("approved events after first = %d, want 0", n)
	}

	if err := svc.Decide(ctx, Decide{ApprovalID: a.ID, ActorID: first, Approve: true}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("same person approving twice: err = %v, want ErrForbidden", err)
	}

	if err := svc.Decide(ctx, Decide{ApprovalID: a.ID, ActorID: second, Approve: true, Reason: "second yes"}); err != nil {
		t.Fatalf("second Decide: %v", err)
	}
	got, err = svc.Get(ctx, a.ID)
	if err != nil {
		t.Fatalf("Get after second: %v", err)
	}
	if got.Status != StatusApproved {
		t.Fatalf("after second approval: status = %q, want approved", got.Status)
	}
	if got.Version != 3 {
		t.Fatalf("after second approval: version = %d, want 3", got.Version)
	}
	if n := countRows(t, pool, `SELECT count(*) FROM core.approval_decisions WHERE approval_id = $1 AND decision = 'approve'`, a.ID); n != 2 {
		t.Fatalf("recorded approvals = %d, want 2", n)
	}
	if n := countRows(t, pool, `SELECT count(*) FROM outbox.events WHERE type = $1`, outbox.TypeApprovalApprovedV1); n != 1 {
		t.Fatalf("approved events = %d, want 1", n)
	}
}

// TestApprovalNotificationsFanOut covers request→approvers and decide→requester.
func TestApprovalNotificationsFanOut(t *testing.T) {
	ctx := context.Background()
	svc, pool, notifier := newTestServiceWithNotifier(t)

	roleKey := "test_role_" + mustNewID(t).String()
	approver := seedPersonWithRole(t, pool, roleKey)
	requester := seedPerson(t, pool)

	a, err := svc.Request(ctx, Request{
		EntityType:   "invoice",
		EntityID:     mustNewID(t),
		RequestedBy:  requester,
		ApproverRole: roleKey,
	}, &requester)
	if err != nil {
		t.Fatalf("Request: %v", err)
	}

	requested := notifier.recipients(TemplateRequested)
	if len(requested) != 1 || requested[0] != approver {
		t.Fatalf("requested notifications = %v, want [%s]", requested, approver)
	}

	decider := mustNewID(t)
	if err := svc.Decide(ctx, Decide{ApprovalID: a.ID, ActorID: decider, Approve: true}); err != nil {
		t.Fatalf("Decide: %v", err)
	}
	approved := notifier.recipients(TemplateApproved)
	if len(approved) != 1 || approved[0] != requester {
		t.Fatalf("approved notifications = %v, want [%s]", approved, requester)
	}
}
