package contracts

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func testURL(t *testing.T) string {
	t.Helper()
	url := os.Getenv("ATLAS_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("ATLAS_TEST_DATABASE_URL is not set; skipping contracts integration tests")
	}
	return url
}

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), testURL(t))
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	if err := pool.Ping(context.Background()); err != nil {
		t.Fatalf("ping: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func newUUID(t *testing.T) uuid.UUID {
	t.Helper()
	id, err := uuid.NewV7()
	if err != nil {
		t.Fatalf("new uuid: %v", err)
	}
	return id
}

// mustOrganization inserts a crm.organizations row: a contract is always drafted
// against a client company (§8.1), and the contracts schema has a foreign key to
// it.
func mustOrganization(t *testing.T, pool *pgxpool.Pool) uuid.UUID {
	t.Helper()
	id := newUUID(t)
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO crm.organizations (id, name, name_normalized, status) VALUES ($1, 'Test Co', 'test co', 'prospect')`,
		id); err != nil {
		t.Fatalf("insert organization: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM crm.organizations WHERE id = $1`, id)
	})
	return id
}

// countEvents counts outbox rows of a type for an aggregate.
func countEvents(t *testing.T, pool *pgxpool.Pool, aggregate uuid.UUID, eventType string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM outbox.events WHERE aggregate_id = $1 AND type = $2`,
		aggregate, eventType).Scan(&n); err != nil {
		t.Fatalf("count events: %v", err)
	}
	return n
}

// TestContractLifecycle walks a contract through the whole §8.3 flow: draft →
// review → approved → sent → signed → active, its child records, and the
// change-request versioning that amends a signed agreement.
func TestContractLifecycle(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	svc := NewService(NewPGStore(pool), nil, nil)
	actor := newUUID(t)
	org := mustOrganization(t, pool)

	value := int64(1_500_000)
	created, err := svc.CreateContract(ctx, actor, ContractInput{
		OrganizationID: org, ContractType: TypeFixedPriceProject,
		ValueMinor: &value, Currency: "KES",
	})
	if err != nil {
		t.Fatalf("CreateContract: %v", err)
	}
	// Every aggregate the test writes events for; cleaned up in one place.
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM outbox.events WHERE aggregate_id IN ($1)`, created.ID)
		_, _ = pool.Exec(ctx, `DELETE FROM contracts.change_requests WHERE contract_id = $1`, created.ID)
		_, _ = pool.Exec(ctx, `DELETE FROM contracts.sla_terms WHERE contract_id = $1`, created.ID)
		_, _ = pool.Exec(ctx, `DELETE FROM contracts.obligations WHERE contract_id = $1`, created.ID)
		_, _ = pool.Exec(ctx, `DELETE FROM contracts.deliverables WHERE contract_id = $1`, created.ID)
		_, _ = pool.Exec(ctx, `DELETE FROM contracts.milestones WHERE contract_id = $1`, created.ID)
		_, _ = pool.Exec(ctx, `DELETE FROM contracts.contracts WHERE id = $1`, created.ID)
	})

	if created.Status != StatusDraft {
		t.Fatalf("new contract status = %q, want draft", created.Status)
	}
	if !strings.HasPrefix(created.Ref, "CTR-") {
		t.Fatalf("contract ref = %q, want CTR-…", created.Ref)
	}
	if created.Version != 1 {
		t.Fatalf("new contract version = %d, want 1", created.Version)
	}
	if n := countEvents(t, pool, created.ID, "contracts.contract.created.v1"); n != 1 {
		t.Fatalf("created events = %d, want 1", n)
	}

	// Lifecycle: every step bumps the version and emits a status change.
	cur := created
	for _, step := range []struct {
		name string
		fn   func(uuid.UUID, int) (Contract, error)
		want ContractStatus
	}{
		{"submit", func(id uuid.UUID, v int) (Contract, error) { return svc.Submit(ctx, actor, id, v) }, StatusInternalReview},
		{"approve", func(id uuid.UUID, v int) (Contract, error) { return svc.Approve(ctx, actor, id, v) }, StatusApproved},
		{"send", func(id uuid.UUID, v int) (Contract, error) { return svc.Send(ctx, actor, id, v) }, StatusSent},
		{"activate", func(id uuid.UUID, v int) (Contract, error) { return svc.Activate(ctx, actor, id, v) }, StatusActive},
	} {
		next, err := step.fn(cur.ID, cur.Version)
		if err != nil {
			t.Fatalf("%s: %v", step.name, err)
		}
		if next.Status != step.want {
			t.Fatalf("%s status = %q, want %q", step.name, next.Status, step.want)
		}
		if next.Version != cur.Version+1 {
			t.Fatalf("%s version = %d, want %d", step.name, next.Version, cur.Version+1)
		}
		cur = next
	}
	if n := countEvents(t, pool, created.ID, "contracts.contract.status_changed.v1"); n != 3 {
		t.Fatalf("status change events = %d, want 3", n)
	}

	// Stale versions are rejected with ErrConflict.
	if _, err := svc.UpdateContract(ctx, actor, created.ID, 1, ContractInput{
		OrganizationID: org, ContractType: TypeFixedPriceProject, ValueMinor: &value, Currency: "KES",
	}); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale update: err = %v, want ErrConflict", err)
	}
	// An active contract is frozen: later changes go through a change request.
	active, err := svc.GetContract(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetContract: %v", err)
	}
	if _, err := svc.UpdateContract(ctx, actor, created.ID, active.Version, ContractInput{
		OrganizationID: org, ContractType: TypeFixedPriceProject, ValueMinor: &value, Currency: "KES",
	}); !errors.Is(err, ErrConflict) {
		t.Fatalf("update active: err = %v, want ErrConflict", err)
	}

	// Child records.
	m, err := svc.CreateMilestone(ctx, actor, created.ID, MilestoneInput{Title: "Kickoff", AmountMinor: &value})
	if err != nil {
		t.Fatalf("CreateMilestone: %v", err)
	}
	if _, err := svc.CreateDeliverable(ctx, actor, created.ID, DeliverableInput{
		Description: "Architecture document", MilestoneID: &m.ID,
	}); err != nil {
		t.Fatalf("CreateDeliverable: %v", err)
	}
	if _, err := svc.CreateObligation(ctx, actor, created.ID, ObligationInput{
		Party: PartySafariStack, Description: "Monthly status report",
	}); err != nil {
		t.Fatalf("CreateObligation: %v", err)
	}
	if _, err := svc.CreateSLATerm(ctx, actor, created.ID, SLATermInput{
		Metric: "uptime", Target: "99.9%", MeasurementWindow: "month",
	}); err != nil {
		t.Fatalf("CreateSLATerm: %v", err)
	}
	if ms, err := svc.ListMilestones(ctx, created.ID); err != nil || len(ms) != 1 {
		t.Fatalf("ListMilestones = %d, %v, want 1", len(ms), err)
	}
	if ds, err := svc.ListDeliverables(ctx, created.ID); err != nil || len(ds) != 1 {
		t.Fatalf("ListDeliverables = %d, %v, want 1", len(ds), err)
	}
	if obligations, err := svc.ListObligations(ctx, created.ID); err != nil || len(obligations) != 1 {
		t.Fatalf("ListObligations = %d, %v, want 1", len(obligations), err)
	}
	if ts, err := svc.ListSLATerms(ctx, created.ID); err != nil || len(ts) != 1 {
		t.Fatalf("ListSLATerms = %d, %v, want 1", len(ts), err)
	}

	// Change requests: raising one records it, approving it appends a new
	// contract version (§8.3), and a rejection is recorded without versioning.
	cr, err := svc.CreateChangeRequest(ctx, actor, created.ID, ChangeRequestInput{
		RequestedBy: "client", Description: "Add a second environment", PriceImpactMinor: &value,
	})
	if err != nil {
		t.Fatalf("CreateChangeRequest: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM outbox.events WHERE aggregate_id IN ($1)`, cr.ID)
		_, _ = pool.Exec(ctx, `DELETE FROM contracts.change_requests WHERE id = $1`, cr.ID)
	})
	if cr.Status != CRSubmitted || !strings.HasPrefix(cr.Ref, "CR-") {
		t.Fatalf("change request = %+v, want submitted CR-…", cr)
	}
	if n := countEvents(t, pool, cr.ID, "contracts.change_request.created.v1"); n != 1 {
		t.Fatalf("change request created events = %d, want 1", n)
	}

	// A second request to exercise rejection.
	cr2, err := svc.CreateChangeRequest(ctx, actor, created.ID, ChangeRequestInput{
		RequestedBy: "client", Description: "Not this time",
	})
	if err != nil {
		t.Fatalf("CreateChangeRequest 2: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM outbox.events WHERE aggregate_id IN ($1)`, cr2.ID)
		_, _ = pool.Exec(ctx, `DELETE FROM contracts.change_requests WHERE id = $1`, cr2.ID)
	})

	versionsBefore := contractVersionCount(t, pool, created.ID)
	approved, err := svc.ApproveChangeRequest(ctx, actor, cr.ID, cr.Version, nil)
	if err != nil {
		t.Fatalf("ApproveChangeRequest: %v", err)
	}
	if approved.Status != CRApproved {
		t.Fatalf("change request status = %q, want approved", approved.Status)
	}
	if n := contractVersionCount(t, pool, created.ID); n != versionsBefore+1 {
		t.Fatalf("contract versions = %d, want %d (an approved change adds one)", n, versionsBefore+1)
	}
	if n := countEvents(t, pool, cr.ID, "contracts.change_request.approved.v1"); n != 1 {
		t.Fatalf("change request approved events = %d, want 1", n)
	}

	rejected, err := svc.RejectChangeRequest(ctx, actor, cr2.ID, cr2.Version, "out of scope")
	if err != nil {
		t.Fatalf("RejectChangeRequest: %v", err)
	}
	if rejected.Status != CRRejected {
		t.Fatalf("change request status = %q, want rejected", rejected.Status)
	}
	if n := countEvents(t, pool, cr2.ID, "contracts.change_request.rejected.v1"); n != 1 {
		t.Fatalf("change request rejected events = %d, want 1", n)
	}

	// The approved change summary carries the change request reference.
	var summary string
	if err := pool.QueryRow(ctx,
		`SELECT change_summary FROM contracts.contract_versions WHERE contract_id = $1 ORDER BY version_no DESC LIMIT 1`,
		created.ID).Scan(&summary); err != nil {
		t.Fatalf("read version summary: %v", err)
	}
	if !strings.Contains(summary, cr.Ref) {
		t.Fatalf("version summary %q does not mention the change request %q", summary, cr.Ref)
	}
}

// TestSignRecordsSignature proves signing is reachable only from `sent`, appends
// the signature to the stored signatories and emits the handover event.
func TestSignRecordsSignature(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	svc := NewService(NewPGStore(pool), nil, nil)
	actor := newUUID(t)
	org := mustOrganization(t, pool)

	c, err := svc.CreateContract(ctx, actor, ContractInput{OrganizationID: org, ContractType: TypeRetainer})
	if err != nil {
		t.Fatalf("CreateContract: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM outbox.events WHERE aggregate_id = $1`, c.ID)
		_, _ = pool.Exec(ctx, `DELETE FROM contracts.contracts WHERE id = $1`, c.ID)
	})

	// A draft cannot be signed.
	if _, err := svc.Sign(ctx, actor, c.ID, c.Version, SignInput{Name: "Jane"}); !errors.Is(err, ErrConflict) {
		t.Fatalf("sign a draft: err = %v, want ErrConflict", err)
	}

	cur, err := svc.Submit(ctx, actor, c.ID, c.Version)
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	cur, err = svc.Approve(ctx, actor, cur.ID, cur.Version)
	if err != nil {
		t.Fatalf("Approve: %v", err)
	}
	cur, err = svc.Send(ctx, actor, cur.ID, cur.Version)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}

	signed, err := svc.Sign(ctx, actor, cur.ID, cur.Version, SignInput{Name: "Jane", Role: "CTO"})
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	if signed.Status != StatusSigned || signed.SignedAt == nil {
		t.Fatalf("signed = %+v, want signed with a timestamp", signed)
	}
	var signatories []Signatory
	if err := json.Unmarshal(signed.Signatories, &signatories); err != nil {
		t.Fatalf("decode signatories: %v", err)
	}
	if len(signatories) != 1 || signatories[0].Name != "Jane" {
		t.Fatalf("signatories = %+v, want one entry named Jane", signatories)
	}
	if n := countEvents(t, pool, c.ID, "contracts.contract.signed.v1"); n != 1 {
		t.Fatalf("signed events = %d, want 1", n)
	}
}

func contractVersionCount(t *testing.T, pool *pgxpool.Pool, contractID uuid.UUID) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM contracts.contract_versions WHERE contract_id = $1`,
		contractID).Scan(&n); err != nil {
		t.Fatalf("count contract versions: %v", err)
	}
	return n
}
