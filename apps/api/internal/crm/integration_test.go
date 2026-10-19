package crm

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func testURL(t *testing.T) string {
	t.Helper()
	url := os.Getenv("ATLAS_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("ATLAS_TEST_DATABASE_URL is not set; skipping crm integration tests")
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

// mustPerson inserts a people.persons row: crm tables have foreign keys to it.
func mustPerson(t *testing.T, pool *pgxpool.Pool) uuid.UUID {
	t.Helper()
	id := newUUID(t)
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO people.persons (id, full_name, employment_type) VALUES ($1, 'Test Person', 'full_time')`,
		id); err != nil {
		t.Fatalf("insert person: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM people.persons WHERE id = $1`, id)
	})
	return id
}

// ensureConfig makes the reference rows a lead and an opportunity depend on.
// They live in crm.lead_sources and crm.pipeline_stages and are normally seeded
// (ON CONFLICT DO NOTHING), so a database that already has them is untouched.
func ensureConfig(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO crm.lead_sources (key, label, sort_order) VALUES ('website_form', 'Website form', 1)
		 ON CONFLICT (key) DO NOTHING`); err != nil {
		t.Fatalf("insert lead source: %v", err)
	}
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO crm.pipeline_stages (key, label, sort_order) VALUES
		 ('discovery', 'Discovery', 10), ('proposal_sent', 'Proposal sent', 30), ('won', 'Won', 100)
		 ON CONFLICT (key) DO NOTHING`); err != nil {
		t.Fatalf("insert pipeline stages: %v", err)
	}
}

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

// TestSalesLifecycle walks a lead to a won opportunity: capture, assignment,
// qualification, conversion, an interaction, a follow-up, a proposal and the win
// — asserting the outbox events each step emits and the state each step leaves.
func TestSalesLifecycle(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	ensureConfig(t, pool)
	svc := NewService(NewPGStore(pool), nil, nil)
	actor := mustPerson(t, pool)
	owner := mustPerson(t, pool)

	// A client company and a contact at it.
	org, err := svc.CreateOrganization(ctx, actor, OrganizationInput{Name: "Apollo Labs"})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM crm.organizations WHERE id = $1`, org.ID)
	})
	contact, err := svc.CreateContact(ctx, actor, ContactInput{OrganizationID: &org.ID, Name: "Jane Wanjiru"})
	if err != nil {
		t.Fatalf("CreateContact: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM crm.contacts WHERE id = $1`, contact.ID)
	})

	// Capture a lead from the website form.
	lead, err := svc.CreateLead(ctx, actor, LeadInput{
		SourceKey: "website_form", OrganizationID: &org.ID, ContactID: &contact.ID,
		ContactName: "Jane Wanjiru", NeedSummary: "Needs an ERP pilot", Status: LeadNew,
	})
	if err != nil {
		t.Fatalf("CreateLead: %v", err)
	}
	// t.Cleanup is LIFO: the lead's children go before the lead itself, and the
	// lead goes before the company it points at.
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM crm.followups WHERE lead_id = $1`, lead.ID)
		_, _ = pool.Exec(ctx, `DELETE FROM crm.interactions WHERE lead_id = $1`, lead.ID)
		_, _ = pool.Exec(ctx, `DELETE FROM crm.leads WHERE id = $1`, lead.ID)
		_, _ = pool.Exec(ctx, `DELETE FROM outbox.events WHERE aggregate_id = $1`, lead.ID)
	})

	if lead.Status != LeadNew || !strings.HasPrefix(lead.Ref, "LEAD-") {
		t.Fatalf("lead = %+v, want a new LEAD-…", lead)
	}
	if n := countEvents(t, pool, lead.ID, "crm.lead.created.v1"); n != 1 {
		t.Fatalf("lead created events = %d, want 1", n)
	}

	// Assignment moves it to `assigned` and stamps the next action.
	due := time.Now().UTC().Add(48 * time.Hour)
	assigned, err := svc.AssignLead(ctx, actor, lead.ID, owner, "Call back about the pilot", due)
	if err != nil {
		t.Fatalf("AssignLead: %v", err)
	}
	if assigned.Status != LeadAssigned || assigned.OwnerPersonID == nil || *assigned.OwnerPersonID != owner {
		t.Fatalf("assigned = %+v, want status assigned owned by %s", assigned, owner)
	}
	if n := countEvents(t, pool, lead.ID, "crm.lead.assigned.v1"); n != 1 {
		t.Fatalf("lead assigned events = %d, want 1", n)
	}

	// The first interaction advances assigned → contacted automatically (§8.1).
	if _, err := svc.RecordInteraction(ctx, actor, InteractionInput{
		Type: InteractionCall, LeadID: &lead.ID, Outcome: "Jane confirmed budget for Q1",
	}); err != nil {
		t.Fatalf("RecordInteraction: %v", err)
	}
	afterInteraction, err := svc.GetLead(ctx, lead.ID)
	if err != nil {
		t.Fatalf("GetLead: %v", err)
	}
	if afterInteraction.Status != LeadContacted {
		t.Fatalf("status after interaction = %q, want contacted", afterInteraction.Status)
	}

	// Qualify, then convert into an opportunity.
	qualified, err := svc.SetLeadStatus(ctx, actor, lead.ID, LeadQualified, nil)
	if err != nil {
		t.Fatalf("SetLeadStatus(qualified): %v", err)
	}
	if qualified.Status != LeadQualified {
		t.Fatalf("qualified status = %q", qualified.Status)
	}

	opp, converted, err := svc.ConvertLead(ctx, actor, lead.ID, ConvertInput{Title: "Apollo ERP pilot"})
	if err != nil {
		t.Fatalf("ConvertLead: %v", err)
	}
	if converted.Status != LeadConverted || converted.ConvertedOpportunityID == nil {
		t.Fatalf("converted lead = %+v", converted)
	}
	if !strings.HasPrefix(opp.Ref, "OPP-") || opp.StageKey == "" {
		t.Fatalf("opportunity = %+v, want OPP-… in an open stage", opp)
	}
	if n := countEvents(t, pool, lead.ID, "crm.lead.converted.v1"); n != 1 {
		t.Fatalf("lead converted events = %d, want 1", n)
	}
	if n := countEvents(t, pool, opp.ID, "crm.opportunity.created.v1"); n != 1 {
		t.Fatalf("opportunity created events = %d, want 1", n)
	}
	// The lead points at this opportunity, so its reference is cleared before the
	// row goes; proposals for it are removed too.
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `UPDATE crm.leads SET converted_opportunity_id = NULL WHERE converted_opportunity_id = $1`, opp.ID)
		_, _ = pool.Exec(ctx, `DELETE FROM crm.proposals WHERE opportunity_id = $1`, opp.ID)
		_, _ = pool.Exec(ctx, `DELETE FROM crm.opportunities WHERE id = $1`, opp.ID)
		_, _ = pool.Exec(ctx, `DELETE FROM outbox.events WHERE aggregate_id = $1`, opp.ID)
	})

	// A follow-up is scheduled against the opportunity and completed.
	fu, err := svc.ScheduleFollowup(ctx, actor, FollowupInput{
		OpportunityID: &opp.ID, OwnerPersonID: &owner, DueAt: due, Channel: ChannelCall,
		Reason: "Confirm the pilot scope",
	})
	if err != nil {
		t.Fatalf("ScheduleFollowup: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM crm.followups WHERE id = $1`, fu.ID)
	})
	if fu.Status != FollowupScheduled {
		t.Fatalf("follow-up status = %q, want scheduled", fu.Status)
	}
	completed, err := svc.CompleteFollowup(ctx, actor, fu.ID, "Scope confirmed")
	if err != nil {
		t.Fatalf("CompleteFollowup: %v", err)
	}
	if completed.Status != FollowupDone || completed.CompletedAt == nil {
		t.Fatalf("completed follow-up = %+v", completed)
	}

	// A discount above the threshold needs the approval permission (§3.3).
	price := int64(100_000)
	if _, err := svc.CreateProposal(ctx, actor, false, ProposalInput{
		OpportunityID: opp.ID, PriceMinor: price, DiscountMinor: 20_000,
	}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("unapproved discount: err = %v, want ErrForbidden", err)
	}

	// A normal proposal is drafted, then sent.
	proposal, err := svc.CreateProposal(ctx, actor, false, ProposalInput{
		OpportunityID: opp.ID, PriceMinor: price, ScopeSummary: "Pilot for one department",
	})
	if err != nil {
		t.Fatalf("CreateProposal: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM crm.proposals WHERE id = $1`, proposal.ID)
	})
	if proposal.Status != ProposalDraft || !strings.HasPrefix(proposal.Ref, "PROP-") {
		t.Fatalf("proposal = %+v, want a draft PROP-…", proposal)
	}
	if n := countEvents(t, pool, proposal.ID, "crm.proposal.created.v1"); n != 1 {
		t.Fatalf("proposal created events = %d, want 1", n)
	}
	sent, err := svc.SendProposal(ctx, actor, proposal.ID)
	if err != nil {
		t.Fatalf("SendProposal: %v", err)
	}
	if sent.Status != ProposalSent {
		t.Fatalf("proposal status = %q, want sent", sent.Status)
	}
	if n := countEvents(t, pool, proposal.ID, "crm.proposal.sent.v1"); n != 1 {
		t.Fatalf("proposal sent events = %d, want 1", n)
	}

	// Winning needs an accepted proposal; before that it is refused.
	if _, err := svc.WinOpportunity(ctx, actor, opp.ID); !errors.Is(err, ErrConflict) {
		t.Fatalf("win without an accepted proposal: err = %v, want ErrConflict", err)
	}
	// Acceptance arrives from the client (an inbound channel, later); record it
	// directly so the win path can be exercised here.
	if _, err := pool.Exec(ctx, `UPDATE crm.proposals SET status = 'accepted' WHERE id = $1`, proposal.ID); err != nil {
		t.Fatalf("accept proposal: %v", err)
	}
	won, err := svc.WinOpportunity(ctx, actor, opp.ID)
	if err != nil {
		t.Fatalf("WinOpportunity: %v", err)
	}
	var isWon bool
	if err := pool.QueryRow(ctx, `SELECT is_won FROM crm.pipeline_stages WHERE key = $1`, won.StageKey).Scan(&isWon); err != nil {
		t.Fatalf("read stage: %v", err)
	}
	if !isWon {
		t.Fatalf("won opportunity sits in stage %q, which is not a won stage", won.StageKey)
	}
	if n := countEvents(t, pool, opp.ID, "crm.opportunity.won.v1"); n != 1 {
		t.Fatalf("opportunity won events = %d, want 1", n)
	}

	// The pipeline summary reports the won stage.
	pipeline, err := svc.Pipeline(ctx)
	if err != nil {
		t.Fatalf("Pipeline: %v", err)
	}
	found := false
	for _, row := range pipeline {
		if row.StageKey == won.StageKey {
			found = true
		}
	}
	if !found {
		t.Fatalf("pipeline summary is missing the won stage %q", won.StageKey)
	}
}

// TestLeadSourceAndStageConfig proves the reference lists the sales screens read
// come from the seeded configuration rather than being hardcoded in Go.
func TestLeadSourceAndStageConfig(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	ensureConfig(t, pool)
	svc := NewService(NewPGStore(pool), nil, nil)

	sources, err := svc.ListLeadSources(ctx)
	if err != nil {
		t.Fatalf("ListLeadSources: %v", err)
	}
	if len(sources) == 0 {
		t.Fatal("no lead sources are configured")
	}
	stages, err := svc.ListPipelineStages(ctx)
	if err != nil {
		t.Fatalf("ListPipelineStages: %v", err)
	}
	if len(stages) == 0 {
		t.Fatal("no pipeline stages are configured")
	}
}
