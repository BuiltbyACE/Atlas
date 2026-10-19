package crm

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/safaristack/atlas/apps/api/internal/audit"
	"github.com/safaristack/atlas/apps/api/internal/outbox"
)

// Entity types used in the audit trail and event aggregate names.
const (
	EntityOrganization = "crm.organization"
	EntityContact      = "crm.contact"
	EntityLead         = "crm.lead"
	EntityOpportunity  = "crm.opportunity"
	EntityInteraction  = "crm.interaction"
	EntityFollowup     = "crm.followup"
	EntityProposal     = "crm.proposal"
)

// DefaultDiscountApprovalPercent is the share of price above which a proposal
// discount needs crm.proposal.discount_approve (§3.3, §8.1). It is a starting
// constant; a typed setting can replace it without changing the call sites.
const DefaultDiscountApprovalPercent = 10

// Service coordinates CRM: validation, state transitions, content rules,
// auditing and events.
type Service struct {
	store  Store
	audit  *audit.Writer
	now    func() time.Time
	logger *slog.Logger
}

// NewService builds the CRM service. aw may be nil in tests, in which case the
// audit write is skipped.
func NewService(store Store, aw *audit.Writer, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{store: store, audit: aw, now: func() time.Time { return time.Now().UTC() }, logger: logger}
}

// ── inputs ──────────────────────────────────────────────────────────────────

// OrganizationInput is the replacement payload for an organization write.
type OrganizationInput struct {
	Name          string
	Industry      string
	Size          string
	Location      string
	KRAPin        string
	Status        OrganizationStatus
	OwnerPersonID *uuid.UUID
}

// ContactInput is the replacement payload for a contact write.
type ContactInput struct {
	OrganizationID   *uuid.UUID
	Name             string
	Role             string
	Phones           []string
	Emails           []string
	WhatsAppNumber   string
	ConsentMarketing bool
	ConsentWhatsApp  bool
	ConsentRecording bool
	ConsentSource    string
}

// LeadInput is the replacement payload for a lead write.
type LeadInput struct {
	SourceKey           string
	SourceDetail        string
	OrganizationID      *uuid.UUID
	ContactID           *uuid.UUID
	ContactName         string
	ContactPhone        string
	ContactEmail        string
	NeedSummary         string
	EstimatedValueMinor *int64
	Currency            string
	Priority            Priority
	OwnerPersonID       *uuid.UUID
	Status              LeadStatus
	NextAction          string
	NextActionDueAt     *time.Time
	QualificationNotes  string
	LostReason          *LostReason
}

// ConvertInput describes the opportunity a qualified lead becomes.
type ConvertInput struct {
	Title           string
	OrganizationID  *uuid.UUID
	ValueMinor      *int64
	Currency        string
	StageKey        string
	Probability     *int
	ExpectedCloseAt *time.Time
	OwnerPersonID   *uuid.UUID
}

// InteractionInput is the payload for recording an interaction.
type InteractionInput struct {
	Type            InteractionType
	Direction       Direction
	ContactID       *uuid.UUID
	LeadID          *uuid.UUID
	OpportunityID   *uuid.UUID
	OccurredAt      *time.Time
	DurationMinutes *int
	Outcome         string
	Summary         string
	NextAction      string
	NextActionDueAt *time.Time
	RecordingFileID *uuid.UUID
	// SkipAdvance suppresses the automatic assigned→contacted move (§8.1) when
	// the first interaction is recorded. The move is on by default.
	SkipAdvance bool
}

// FollowupInput is the payload for scheduling a follow-up.
type FollowupInput struct {
	LeadID        *uuid.UUID
	OpportunityID *uuid.UUID
	OwnerPersonID *uuid.UUID
	DueAt         time.Time
	Channel       FollowupChannel
	Reason        string
}

// ProposalInput is the payload for creating a proposal.
type ProposalInput struct {
	OpportunityID  uuid.UUID
	ScopeSummary   string
	LineItems      []byte
	PriceMinor     int64
	Currency       string
	DiscountMinor  int64
	ValidUntil     *time.Time
	DocumentFileID *uuid.UUID
}

// ── organizations ───────────────────────────────────────────────────────────

// CreateOrganization validates and inserts an organization.
func (s *Service) CreateOrganization(ctx context.Context, actor uuid.UUID, in OrganizationInput) (Organization, error) {
	o, err := validateOrganization(Organization{
		Name: in.Name, Industry: in.Industry, Size: in.Size, Location: in.Location,
		KRAPin: in.KRAPin, Status: in.Status, OwnerPersonID: in.OwnerPersonID, Version: 1,
	})
	if err != nil {
		return Organization{}, err
	}
	if existing, err := s.store.OrganizationByName(ctx, o.NameNormalized); err != nil {
		return Organization{}, err
	} else if existing != nil {
		return Organization{}, fmt.Errorf("%w: an organization named %q already exists", ErrDuplicate, existing.Name)
	}
	id, err := uuid.NewV7()
	if err != nil {
		return Organization{}, fmt.Errorf("crm: new id: %w", err)
	}
	o.ID = id
	now := s.now()
	o.CreatedAt, o.UpdatedAt = now, now
	created, err := s.store.CreateOrganization(ctx, o)
	if err != nil {
		return Organization{}, err
	}
	s.writeAudit(ctx, actor, "crm.organization.created", EntityOrganization, created.ID, nil, organizationSnapshot(created))
	return created, nil
}

// GetOrganization returns an organization or ErrNotFound.
func (s *Service) GetOrganization(ctx context.Context, id uuid.UUID) (Organization, error) {
	o, err := s.store.GetOrganization(ctx, id)
	if err != nil {
		return Organization{}, err
	}
	if o == nil {
		return Organization{}, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	return *o, nil
}

// UpdateOrganization applies a replacement to an existing organization.
func (s *Service) UpdateOrganization(ctx context.Context, actor uuid.UUID, id uuid.UUID, expectedVersion int, in OrganizationInput) (Organization, error) {
	cur, err := s.GetOrganization(ctx, id)
	if err != nil {
		return Organization{}, err
	}
	next, err := validateOrganization(Organization{
		ID: id, Name: in.Name, Industry: in.Industry, Size: in.Size, Location: in.Location,
		KRAPin: in.KRAPin, Status: in.Status, OwnerPersonID: in.OwnerPersonID,
	})
	if err != nil {
		return Organization{}, err
	}
	if next.NameNormalized != cur.NameNormalized {
		if existing, err := s.store.OrganizationByName(ctx, next.NameNormalized); err != nil {
			return Organization{}, err
		} else if existing != nil && existing.ID != id {
			return Organization{}, fmt.Errorf("%w: an organization named %q already exists", ErrDuplicate, existing.Name)
		}
	}
	updated, err := s.store.UpdateOrganization(ctx, next, expectedVersion)
	if err != nil {
		return Organization{}, err
	}
	s.writeAudit(ctx, actor, "crm.organization.updated", EntityOrganization, updated.ID, organizationSnapshot(cur), organizationSnapshot(updated))
	return updated, nil
}

// ListOrganizations returns a page of organizations, newest first.
func (s *Service) ListOrganizations(ctx context.Context, q string, limit int, before *Cursor) ([]Organization, error) {
	if limit <= 0 {
		limit = 50
	}
	return s.store.ListOrganizations(ctx, trim(q), limit, before)
}

// ── contacts ────────────────────────────────────────────────────────────────

// CreateContact validates and inserts a contact, stamping consent timestamps.
func (s *Service) CreateContact(ctx context.Context, actor uuid.UUID, in ContactInput) (Contact, error) {
	c, err := validateContact(Contact{
		OrganizationID: in.OrganizationID, Name: in.Name, Role: in.Role, Phones: in.Phones,
		Emails: in.Emails, WhatsAppNumber: in.WhatsAppNumber,
		ConsentMarketing: in.ConsentMarketing, ConsentWhatsApp: in.ConsentWhatsApp,
		ConsentRecording: in.ConsentRecording, Version: 1,
	})
	if err != nil {
		return Contact{}, err
	}
	id, err := uuid.NewV7()
	if err != nil {
		return Contact{}, fmt.Errorf("crm: new id: %w", err)
	}
	c.ID = id
	now := s.now()
	c.CreatedAt, c.UpdatedAt = now, now
	applyConsent(c, Contact{}, in.ConsentSource, now)
	created, err := s.store.CreateContact(ctx, c)
	if err != nil {
		return Contact{}, err
	}
	s.writeAudit(ctx, actor, "crm.contact.created", EntityContact, created.ID, nil, contactSnapshot(created))
	return created, nil
}

// GetContact returns a contact or ErrNotFound.
func (s *Service) GetContact(ctx context.Context, id uuid.UUID) (Contact, error) {
	c, err := s.store.GetContact(ctx, id)
	if err != nil {
		return Contact{}, err
	}
	if c == nil {
		return Contact{}, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	return *c, nil
}

// UpdateContact applies a replacement to an existing contact.
func (s *Service) UpdateContact(ctx context.Context, actor uuid.UUID, id uuid.UUID, expectedVersion int, in ContactInput) (Contact, error) {
	cur, err := s.GetContact(ctx, id)
	if err != nil {
		return Contact{}, err
	}
	next, err := validateContact(Contact{
		ID: id, OrganizationID: in.OrganizationID, Name: in.Name, Role: in.Role, Phones: in.Phones,
		Emails: in.Emails, WhatsAppNumber: in.WhatsAppNumber,
		ConsentMarketing: in.ConsentMarketing, ConsentWhatsApp: in.ConsentWhatsApp,
		ConsentRecording: in.ConsentRecording,
	})
	if err != nil {
		return Contact{}, err
	}
	// Carry existing consent timestamps, then stamp any newly-granted consent.
	next.ConsentMarketingAt, next.ConsentMarketingSource = cur.ConsentMarketingAt, cur.ConsentMarketingSource
	next.ConsentWhatsAppAt, next.ConsentWhatsAppSource = cur.ConsentWhatsAppAt, cur.ConsentWhatsAppSource
	next.ConsentRecordingAt, next.ConsentRecordingSource = cur.ConsentRecordingAt, cur.ConsentRecordingSource
	applyConsent(next, cur, in.ConsentSource, s.now())
	updated, err := s.store.UpdateContact(ctx, next, expectedVersion)
	if err != nil {
		return Contact{}, err
	}
	s.writeAudit(ctx, actor, "crm.contact.updated", EntityContact, updated.ID, contactSnapshot(cur), contactSnapshot(updated))
	return updated, nil
}

// ListContacts returns a page of contacts, newest first.
func (s *Service) ListContacts(ctx context.Context, organizationID *uuid.UUID, limit int, before *Cursor) ([]Contact, error) {
	if limit <= 0 {
		limit = 50
	}
	return s.store.ListContacts(ctx, organizationID, limit, before)
}

// applyConsent stamps the timestamp and source for any consent flag that is
// newly true, so consent is always dated and attributable (ADR-011).
func applyConsent(next, prev Contact, source string, now time.Time) {
	source = trim(source)
	if next.ConsentMarketing && !prev.ConsentMarketing {
		next.ConsentMarketingAt, next.ConsentMarketingSource = &now, source
	}
	if next.ConsentWhatsApp && !prev.ConsentWhatsApp {
		next.ConsentWhatsAppAt, next.ConsentWhatsAppSource = &now, source
	}
	if next.ConsentRecording && !prev.ConsentRecording {
		next.ConsentRecordingAt, next.ConsentRecordingSource = &now, source
	}
	// A revoked consent drops its timestamp: consent is current, not historical.
	if !next.ConsentMarketing {
		next.ConsentMarketingAt, next.ConsentMarketingSource = nil, ""
	}
	if !next.ConsentWhatsApp {
		next.ConsentWhatsAppAt, next.ConsentWhatsAppSource = nil, ""
	}
	if !next.ConsentRecording {
		next.ConsentRecordingAt, next.ConsentRecordingSource = nil, ""
	}
}

// ── config ──────────────────────────────────────────────────────────────────

// ListLeadSources returns the configured lead sources.
func (s *Service) ListLeadSources(ctx context.Context) ([]LeadSource, error) {
	return s.store.ListLeadSources(ctx)
}

// ListPipelineStages returns the configured pipeline stages.
func (s *Service) ListPipelineStages(ctx context.Context) ([]PipelineStage, error) {
	return s.store.ListPipelineStages(ctx)
}

// ── leads ───────────────────────────────────────────────────────────────────

// CreateLead validates and inserts a lead, emitting crm.lead.created.v1.
func (s *Service) CreateLead(ctx context.Context, actor uuid.UUID, in LeadInput) (Lead, error) {
	l, err := validateLead(Lead{
		SourceKey: in.SourceKey, SourceDetail: in.SourceDetail, OrganizationID: in.OrganizationID,
		ContactID: in.ContactID, ContactName: in.ContactName, ContactPhone: in.ContactPhone,
		ContactEmail: in.ContactEmail, NeedSummary: in.NeedSummary,
		EstimatedValueMinor: in.EstimatedValueMinor, Currency: in.Currency, Priority: in.Priority,
		OwnerPersonID: in.OwnerPersonID, Status: in.Status, NextAction: in.NextAction,
		NextActionDueAt: in.NextActionDueAt, QualificationNotes: in.QualificationNotes,
		LostReason: in.LostReason, Version: 1,
	})
	if err != nil {
		return Lead{}, err
	}
	if ok, err := s.store.LeadSourceExists(ctx, l.SourceKey); err != nil {
		return Lead{}, err
	} else if !ok {
		return Lead{}, fmt.Errorf("%w: unknown lead source %q", ErrInvalid, l.SourceKey)
	}
	id, err := uuid.NewV7()
	if err != nil {
		return Lead{}, fmt.Errorf("crm: new id: %w", err)
	}
	l.ID = id
	now := s.now()
	l.CreatedAt, l.UpdatedAt = now, now

	ownerUser, err := s.accountForPerson(ctx, l.OwnerPersonID)
	if err != nil {
		return Lead{}, err
	}
	created, err := s.store.CreateLead(ctx, l, func(ref string) (outbox.Event, error) {
		return outbox.NewLeadCreated(&actor, outbox.LeadRef{
			ID: l.ID, Ref: ref, Status: string(l.Status),
			OwnerID: l.OwnerPersonID, OwnerUserID: ownerUser,
		})
	})
	if err != nil {
		return Lead{}, err
	}
	s.writeAudit(ctx, actor, "crm.lead.created", EntityLead, created.ID, nil, leadSnapshot(created))
	return created, nil
}

// GetLead returns a lead or ErrNotFound.
func (s *Service) GetLead(ctx context.Context, id uuid.UUID) (Lead, error) {
	l, err := s.store.GetLead(ctx, id)
	if err != nil {
		return Lead{}, err
	}
	if l == nil {
		return Lead{}, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	return *l, nil
}

// ListLeads returns a page of leads, newest first.
func (s *Service) ListLeads(ctx context.Context, q ListLeadsQuery, limit int, before *Cursor) ([]Lead, error) {
	if limit <= 0 {
		limit = 50
	}
	return s.store.ListLeads(ctx, q, limit, before)
}

// UpdateLead applies a replacement to a lead, enforcing the lifecycle and the
// §8.1 open-lead rule, and emits a status-changed event on a move.
func (s *Service) UpdateLead(ctx context.Context, actor uuid.UUID, id uuid.UUID, expectedVersion int, in LeadInput) (Lead, error) {
	cur, err := s.GetLead(ctx, id)
	if err != nil {
		return Lead{}, err
	}
	next, err := validateLead(Lead{
		ID: id, SourceKey: in.SourceKey, SourceDetail: in.SourceDetail, OrganizationID: in.OrganizationID,
		ContactID: in.ContactID, ContactName: in.ContactName, ContactPhone: in.ContactPhone,
		ContactEmail: in.ContactEmail, NeedSummary: in.NeedSummary,
		EstimatedValueMinor: in.EstimatedValueMinor, Currency: in.Currency, Priority: in.Priority,
		OwnerPersonID: in.OwnerPersonID, Status: in.Status, NextAction: in.NextAction,
		NextActionDueAt: in.NextActionDueAt, QualificationNotes: in.QualificationNotes,
		LostReason: in.LostReason,
	})
	if err != nil {
		return Lead{}, err
	}
	// A converted lead is terminal: never re-open it by a plain update.
	if cur.Status == LeadConverted && next.Status != LeadConverted {
		return Lead{}, fmt.Errorf("%w: a converted lead cannot change status", ErrConflict)
	}
	statusChanged := next.Status != cur.Status
	if statusChanged && !allowedLeadTransition(cur.Status, next.Status) {
		return Lead{}, fmt.Errorf("%w: cannot move a lead from %s to %s", ErrConflict, cur.Status, next.Status)
	}
	if !statusChanged {
		next.ConvertedOpportunityID = cur.ConvertedOpportunityID
		next.Ref = cur.Ref
	}
	var ev *outbox.Event
	if statusChanged {
		built, err := outbox.NewLeadStatusChanged(&actor, id, cur.Ref, string(cur.Status), string(next.Status), actor)
		if err != nil {
			return Lead{}, err
		}
		ev = &built
	}
	updated, err := s.store.UpdateLead(ctx, next, expectedVersion, ev)
	if err != nil {
		return Lead{}, err
	}
	action := "crm.lead.updated"
	if statusChanged {
		action = "crm.lead.status_changed"
	}
	s.writeAudit(ctx, actor, action, EntityLead, updated.ID, leadSnapshot(cur), leadSnapshot(updated))
	return updated, nil
}

// AssignLead sets the owner and a dated next action, advancing a new lead to
// assigned (§8.1) and emitting crm.lead.assigned.v1.
func (s *Service) AssignLead(ctx context.Context, actor uuid.UUID, id uuid.UUID, owner uuid.UUID, nextAction string, dueAt time.Time) (Lead, error) {
	cur, err := s.GetLead(ctx, id)
	if err != nil {
		return Lead{}, err
	}
	if cur.Status == LeadConverted || cur.Status == LeadDisqualified {
		return Lead{}, fmt.Errorf("%w: an owned lead cannot be reassigned after it is %s", ErrConflict, cur.Status)
	}
	nextAction = trim(nextAction)
	if nextAction == "" || dueAt.IsZero() {
		return Lead{}, fmt.Errorf("%w: assigning a lead needs a next action and due date", ErrInvalid)
	}
	ownerUser, err := s.accountForPerson(ctx, &owner)
	if err != nil {
		return Lead{}, err
	}
	ev, err := outbox.NewLeadAssigned(&actor, outbox.LeadRef{ID: id, Ref: cur.Ref, Status: string(cur.Status), OwnerID: &owner, OwnerUserID: ownerUser}, actor)
	if err != nil {
		return Lead{}, err
	}
	updated, err := s.store.SetLeadOwner(ctx, id, owner, nextAction, dueAt.UTC(), &ev)
	if err != nil {
		return Lead{}, err
	}
	s.writeAudit(ctx, actor, "crm.lead.assigned", EntityLead, updated.ID, leadSnapshot(cur), leadSnapshot(updated))
	return updated, nil
}

// ConvertLead turns a qualified lead into an opportunity in one transaction.
func (s *Service) ConvertLead(ctx context.Context, actor uuid.UUID, id uuid.UUID, in ConvertInput) (Opportunity, Lead, error) {
	cur, err := s.GetLead(ctx, id)
	if err != nil {
		return Opportunity{}, Lead{}, err
	}
	if cur.Status != LeadQualified {
		return Opportunity{}, Lead{}, fmt.Errorf("%w: only a qualified lead can convert (this one is %s)", ErrConflict, cur.Status)
	}
	stages, err := s.store.ListPipelineStages(ctx)
	if err != nil {
		return Opportunity{}, Lead{}, err
	}
	stageKey := trim(in.StageKey)
	if stageKey == "" {
		for _, st := range stages {
			if !st.IsWon && !st.IsLost {
				stageKey = st.Key
				break
			}
		}
	}
	if stageKey == "" {
		return Opportunity{}, Lead{}, fmt.Errorf("%w: no open pipeline stage is configured", ErrInvalid)
	}
	if st, err := s.store.GetPipelineStage(ctx, stageKey); err != nil {
		return Opportunity{}, Lead{}, err
	} else if st == nil {
		return Opportunity{}, Lead{}, fmt.Errorf("%w: unknown pipeline stage %q", ErrInvalid, stageKey)
	}
	title := trim(in.Title)
	if title == "" {
		title = cur.NeedSummary
	}
	if title == "" {
		title = cur.ContactName
	}
	if title == "" {
		title = "Opportunity from " + cur.Ref
	}
	orgID := in.OrganizationID
	if orgID == nil {
		orgID = cur.OrganizationID
	}
	value := in.ValueMinor
	if value == nil {
		value = cur.EstimatedValueMinor
	}
	currency := in.Currency
	if trim(currency) == "" {
		currency = cur.Currency
	}
	owner := in.OwnerPersonID
	if owner == nil {
		owner = cur.OwnerPersonID
	}
	opp, err := validateOpportunity(Opportunity{
		OrganizationID: orgID, Title: title, ValueMinor: value, Currency: currency,
		Probability: in.Probability, StageKey: stageKey, ExpectedCloseAt: in.ExpectedCloseAt,
		OwnerPersonID: owner, Version: 1,
	})
	if err != nil {
		return Opportunity{}, Lead{}, err
	}
	oppID, err := uuid.NewV7()
	if err != nil {
		return Opportunity{}, Lead{}, fmt.Errorf("crm: new id: %w", err)
	}
	opp.ID = oppID
	now := s.now()
	opp.CreatedAt, opp.UpdatedAt = now, now

	ownerUser, err := s.accountForPerson(ctx, opp.OwnerPersonID)
	if err != nil {
		return Opportunity{}, Lead{}, err
	}
	leadEv, err := outbox.NewLeadConverted(&actor, cur.ID, cur.Ref, opp.ID, actor)
	if err != nil {
		return Opportunity{}, Lead{}, err
	}
	createdOpp, updatedLead, err := s.store.ConvertLead(ctx, id, opp, func(ref string) (outbox.Event, error) {
		return outbox.NewOpportunityCreated(&actor, outbox.OpportunityRef{
			ID: opp.ID, Ref: ref, StageKey: opp.StageKey,
			OwnerID: opp.OwnerPersonID, OwnerUserID: ownerUser,
		})
	}, &leadEv)
	if err != nil {
		return Opportunity{}, Lead{}, err
	}
	s.writeAudit(ctx, actor, "crm.lead.converted", EntityLead, updatedLead.ID, leadSnapshot(cur), leadSnapshot(updatedLead))
	s.writeAudit(ctx, actor, "crm.opportunity.created", EntityOpportunity, createdOpp.ID, nil, opportunitySnapshot(createdOpp))
	return createdOpp, updatedLead, nil
}

// ── opportunities ───────────────────────────────────────────────────────────

// GetOpportunity returns an opportunity or ErrNotFound.
func (s *Service) GetOpportunity(ctx context.Context, id uuid.UUID) (Opportunity, error) {
	o, err := s.store.GetOpportunity(ctx, id)
	if err != nil {
		return Opportunity{}, err
	}
	if o == nil {
		return Opportunity{}, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	return *o, nil
}

// ListOpportunities returns a page of opportunities, newest first.
func (s *Service) ListOpportunities(ctx context.Context, stageKey *string, ownerID *uuid.UUID, limit int, before *Cursor) ([]Opportunity, error) {
	if limit <= 0 {
		limit = 50
	}
	return s.store.ListOpportunities(ctx, stageKey, ownerID, limit, before)
}

// MoveOpportunityStage moves an opportunity to a named stage. Won/Lost stages
// must go through Win/Lose so their extra rules apply.
func (s *Service) MoveOpportunityStage(ctx context.Context, actor uuid.UUID, id uuid.UUID, stageKey string) (Opportunity, error) {
	cur, err := s.GetOpportunity(ctx, id)
	if err != nil {
		return Opportunity{}, err
	}
	st, err := s.store.GetPipelineStage(ctx, stageKey)
	if err != nil {
		return Opportunity{}, err
	}
	if st == nil {
		return Opportunity{}, fmt.Errorf("%w: unknown pipeline stage %q", ErrInvalid, stageKey)
	}
	if st.IsWon || st.IsLost {
		return Opportunity{}, fmt.Errorf("%w: use the win or lose action for a closing stage", ErrInvalid)
	}
	if cur.StageKey == stageKey {
		return cur, nil
	}
	ev, err := outbox.NewOpportunityStageChanged(&actor, id, cur.Ref, cur.StageKey, stageKey, actor)
	if err != nil {
		return Opportunity{}, err
	}
	updated, err := s.store.SetOpportunityStage(ctx, id, stageKey, nil, &ev)
	if err != nil {
		return Opportunity{}, err
	}
	s.writeAudit(ctx, actor, "crm.opportunity.stage_changed", EntityOpportunity, updated.ID, opportunitySnapshot(cur), opportunitySnapshot(updated))
	return updated, nil
}

// WinOpportunity closes a deal as won. §8.1 requires an accepted proposal.
func (s *Service) WinOpportunity(ctx context.Context, actor uuid.UUID, id uuid.UUID) (Opportunity, error) {
	cur, err := s.GetOpportunity(ctx, id)
	if err != nil {
		return Opportunity{}, err
	}
	proposals, err := s.store.ListProposals(ctx, id)
	if err != nil {
		return Opportunity{}, err
	}
	accepted := false
	for _, p := range proposals {
		if p.Status == ProposalAccepted {
			accepted = true
			break
		}
	}
	if !accepted {
		return Opportunity{}, fmt.Errorf("%w: a won opportunity needs an accepted proposal", ErrConflict)
	}
	stage, err := s.closingStage(ctx, true)
	if err != nil {
		return Opportunity{}, err
	}
	ev, err := outbox.NewOpportunityWon(&actor, id, cur.Ref, actor)
	if err != nil {
		return Opportunity{}, err
	}
	updated, err := s.store.SetOpportunityStage(ctx, id, stage, nil, &ev)
	if err != nil {
		return Opportunity{}, err
	}
	s.writeAudit(ctx, actor, "crm.opportunity.won", EntityOpportunity, updated.ID, opportunitySnapshot(cur), opportunitySnapshot(updated))
	return updated, nil
}

// LoseOpportunity closes a deal as lost. A lost reason is required (§8.1).
func (s *Service) LoseOpportunity(ctx context.Context, actor uuid.UUID, id uuid.UUID, reason LostReason) (Opportunity, error) {
	if !validLostReason(reason) {
		return Opportunity{}, fmt.Errorf("%w: a lost opportunity needs a valid lost reason", ErrInvalid)
	}
	cur, err := s.GetOpportunity(ctx, id)
	if err != nil {
		return Opportunity{}, err
	}
	stage, err := s.closingStage(ctx, false)
	if err != nil {
		return Opportunity{}, err
	}
	ev, err := outbox.NewOpportunityLost(&actor, id, cur.Ref, string(reason), actor)
	if err != nil {
		return Opportunity{}, err
	}
	updated, err := s.store.SetOpportunityStage(ctx, id, stage, &reason, &ev)
	if err != nil {
		return Opportunity{}, err
	}
	s.writeAudit(ctx, actor, "crm.opportunity.lost", EntityOpportunity, updated.ID, opportunitySnapshot(cur), opportunitySnapshot(updated))
	return updated, nil
}

func (s *Service) closingStage(ctx context.Context, won bool) (string, error) {
	stages, err := s.store.ListPipelineStages(ctx)
	if err != nil {
		return "", err
	}
	for _, st := range stages {
		if won && st.IsWon {
			return st.Key, nil
		}
		if !won && st.IsLost {
			return st.Key, nil
		}
	}
	return "", fmt.Errorf("%w: no closing pipeline stage is configured", ErrInvalid)
}

// ── interactions ────────────────────────────────────────────────────────────

// RecordInteraction validates and stores an interaction, emitting
// crm.interaction.recorded.v1. When it is the first touch on an assigned lead,
// the lead advances to contacted (§8.1).
func (s *Service) RecordInteraction(ctx context.Context, actor uuid.UUID, in InteractionInput) (Interaction, error) {
	i := Interaction{
		Type: in.Type, Direction: in.Direction, ContactID: in.ContactID, LeadID: in.LeadID,
		OpportunityID: in.OpportunityID, DurationMinutes: in.DurationMinutes, Outcome: in.Outcome,
		Summary: in.Summary, NextAction: in.NextAction, NextActionDueAt: in.NextActionDueAt,
		RecordingFileID: in.RecordingFileID, RecordedBy: actor, Version: 1,
	}
	if in.OccurredAt != nil {
		i.OccurredAt = in.OccurredAt.UTC()
	}
	validated, err := validateInteraction(i)
	if err != nil {
		return Interaction{}, err
	}
	// A recording link is only allowed with the contact's recorded consent
	// (ADR-011).
	if validated.RecordingFileID != nil {
		contactID := validated.ContactID
		if contactID == nil && validated.LeadID != nil {
			if lead, err := s.store.GetLead(ctx, *validated.LeadID); err != nil {
				return Interaction{}, err
			} else if lead != nil {
				contactID = lead.ContactID
			}
		}
		if contactID == nil {
			return Interaction{}, fmt.Errorf("%w: a recording link needs a contact with call-recording consent", ErrInvalid)
		}
		contact, err := s.store.GetContact(ctx, *contactID)
		if err != nil {
			return Interaction{}, err
		}
		if contact == nil || !contact.ConsentRecording {
			return Interaction{}, fmt.Errorf("%w: a recording link needs call-recording consent", ErrForbidden)
		}
	}
	id, err := uuid.NewV7()
	if err != nil {
		return Interaction{}, fmt.Errorf("crm: new id: %w", err)
	}
	validated.ID = id
	now := s.now()
	validated.CreatedAt, validated.UpdatedAt = now, now

	var ownerUser *uuid.UUID
	if validated.LeadID != nil {
		if lead, err := s.store.GetLead(ctx, *validated.LeadID); err != nil {
			return Interaction{}, err
		} else if lead != nil {
			ownerUser, err = s.accountForPerson(ctx, lead.OwnerPersonID)
			if err != nil {
				return Interaction{}, err
			}
		}
	}
	ev, err := outbox.NewInteractionRecorded(actor, outbox.InteractionRef{
		ID: validated.ID, Type: string(validated.Type), ContactID: validated.ContactID,
		LeadID: validated.LeadID, OpportunityID: validated.OpportunityID, OwnerUserID: ownerUser,
	})
	if err != nil {
		return Interaction{}, err
	}
	created, err := s.store.CreateInteraction(ctx, validated, &ev)
	if err != nil {
		return Interaction{}, err
	}
	s.writeAudit(ctx, actor, "crm.interaction.recorded", EntityInteraction, created.ID, nil, interactionSnapshot(created))

	if !in.SkipAdvance && created.LeadID != nil {
		if lead, err := s.store.GetLead(ctx, *created.LeadID); err == nil && lead != nil && lead.Status == LeadAssigned {
			if _, err := s.SetLeadStatus(ctx, actor, lead.ID, LeadContacted, nil); err != nil {
				s.logger.WarnContext(ctx, "crm: advance lead to contacted failed", slog.String("lead_id", lead.ID.String()), slog.String("error", err.Error()))
			}
		}
	}
	return created, nil
}

// GetInteraction returns an interaction or ErrNotFound.
func (s *Service) GetInteraction(ctx context.Context, id uuid.UUID) (Interaction, error) {
	i, err := s.store.GetInteraction(ctx, id)
	if err != nil {
		return Interaction{}, err
	}
	if i == nil {
		return Interaction{}, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	return *i, nil
}

// ListInteractions returns a page of interactions, newest first.
func (s *Service) ListInteractions(ctx context.Context, q ListInteractionsQuery, limit int, before *Cursor) ([]Interaction, error) {
	if limit <= 0 {
		limit = 50
	}
	return s.store.ListInteractions(ctx, q, limit, before)
}

// SetLeadStatus moves a lead along its lifecycle (used by the interaction flow).
func (s *Service) SetLeadStatus(ctx context.Context, actor uuid.UUID, id uuid.UUID, status LeadStatus, reason *LostReason) (Lead, error) {
	cur, err := s.GetLead(ctx, id)
	if err != nil {
		return Lead{}, err
	}
	if !allowedLeadTransition(cur.Status, status) {
		return Lead{}, fmt.Errorf("%w: cannot move a lead from %s to %s", ErrConflict, cur.Status, status)
	}
	if status == LeadDisqualified && (reason == nil || !validLostReason(*reason)) {
		return Lead{}, fmt.Errorf("%w: a disqualified lead needs a lost reason", ErrInvalid)
	}
	ev, err := outbox.NewLeadStatusChanged(&actor, id, cur.Ref, string(cur.Status), string(status), actor)
	if err != nil {
		return Lead{}, err
	}
	updated, err := s.store.SetLeadStatus(ctx, id, status, reason, &ev)
	if err != nil {
		return Lead{}, err
	}
	s.writeAudit(ctx, actor, "crm.lead.status_changed", EntityLead, updated.ID, leadSnapshot(cur), leadSnapshot(updated))
	return updated, nil
}

// ── follow-ups ──────────────────────────────────────────────────────────────

// ScheduleFollowup validates and stores a follow-up, emitting
// crm.followup.scheduled.v1.
func (s *Service) ScheduleFollowup(ctx context.Context, actor uuid.UUID, in FollowupInput) (Followup, error) {
	owner := actor
	if in.OwnerPersonID != nil {
		owner = *in.OwnerPersonID
	}
	f, err := validateFollowup(Followup{
		LeadID: in.LeadID, OpportunityID: in.OpportunityID, OwnerPersonID: owner,
		DueAt: in.DueAt.UTC(), Channel: in.Channel, Reason: in.Reason, Status: FollowupScheduled, Version: 1,
	})
	if err != nil {
		return Followup{}, err
	}
	id, err := uuid.NewV7()
	if err != nil {
		return Followup{}, fmt.Errorf("crm: new id: %w", err)
	}
	f.ID = id
	now := s.now()
	f.CreatedAt, f.UpdatedAt = now, now
	ownerUser, err := s.accountForPerson(ctx, &owner)
	if err != nil {
		return Followup{}, err
	}
	ev, err := outbox.NewFollowupScheduled(&actor, outbox.FollowupRef{
		ID: f.ID, LeadID: f.LeadID, OpportunityID: f.OpportunityID, OwnerID: f.OwnerPersonID,
		OwnerUserID: ownerUser, DueAt: f.DueAt, Status: string(f.Status),
	})
	if err != nil {
		return Followup{}, err
	}
	created, err := s.store.CreateFollowup(ctx, f, &ev)
	if err != nil {
		return Followup{}, err
	}
	s.writeAudit(ctx, actor, "crm.followup.scheduled", EntityFollowup, created.ID, nil, followupSnapshot(created))
	return created, nil
}

// GetFollowup returns a follow-up or ErrNotFound.
func (s *Service) GetFollowup(ctx context.Context, id uuid.UUID) (Followup, error) {
	f, err := s.store.GetFollowup(ctx, id)
	if err != nil {
		return Followup{}, err
	}
	if f == nil {
		return Followup{}, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	return *f, nil
}

// ListFollowups returns a page of follow-ups, soonest due first.
func (s *Service) ListFollowups(ctx context.Context, q ListFollowupsQuery, limit int, before *Cursor) ([]Followup, error) {
	if limit <= 0 {
		limit = 50
	}
	return s.store.ListFollowups(ctx, q, limit, before)
}

// CompleteFollowup marks a scheduled follow-up done with an outcome.
func (s *Service) CompleteFollowup(ctx context.Context, actor uuid.UUID, id uuid.UUID, outcome string) (Followup, error) {
	return s.resolveFollowup(ctx, actor, id, FollowupDone, outcome)
}

// CancelFollowup cancels a scheduled follow-up.
func (s *Service) CancelFollowup(ctx context.Context, actor uuid.UUID, id uuid.UUID, outcome string) (Followup, error) {
	return s.resolveFollowup(ctx, actor, id, FollowupCancelled, outcome)
}

func (s *Service) resolveFollowup(ctx context.Context, actor, id uuid.UUID, status FollowupStatus, outcome string) (Followup, error) {
	cur, err := s.GetFollowup(ctx, id)
	if err != nil {
		return Followup{}, err
	}
	if cur.Status != FollowupScheduled && cur.Status != FollowupMissed {
		return Followup{}, fmt.Errorf("%w: a %s follow-up cannot be resolved", ErrConflict, cur.Status)
	}
	ownerUser, err := s.accountForPerson(ctx, &cur.OwnerPersonID)
	if err != nil {
		return Followup{}, err
	}
	ev, err := outbox.NewFollowupCompleted(&actor, outbox.FollowupRef{
		ID: id, LeadID: cur.LeadID, OpportunityID: cur.OpportunityID, OwnerID: cur.OwnerPersonID,
		OwnerUserID: ownerUser, DueAt: cur.DueAt, Status: string(status),
	})
	if err != nil {
		return Followup{}, err
	}
	updated, err := s.store.ResolveFollowup(ctx, id, status, trim(outcome), &ev)
	if err != nil {
		return Followup{}, err
	}
	s.writeAudit(ctx, actor, "crm.followup."+string(status), EntityFollowup, updated.ID, followupSnapshot(cur), followupSnapshot(updated))
	return updated, nil
}

// SweepMissedFollowups flips overdue scheduled follow-ups to missed and emits
// crm.followup.missed.v1 for each. It is called by the worker sweep (§4.1).
func (s *Service) SweepMissedFollowups(ctx context.Context, limit int) (int, error) {
	if limit <= 0 {
		limit = 200
	}
	now := s.now()
	missed, err := s.store.MissOverdueFollowups(ctx, now, limit, func(f Followup) (outbox.Event, error) {
		ownerUser, err := s.accountForPerson(ctx, &f.OwnerPersonID)
		if err != nil {
			return outbox.Event{}, err
		}
		return outbox.NewFollowupMissed(nil, outbox.FollowupRef{
			ID: f.ID, LeadID: f.LeadID, OpportunityID: f.OpportunityID, OwnerID: f.OwnerPersonID,
			OwnerUserID: ownerUser, DueAt: f.DueAt, Status: string(f.Status),
		})
	})
	if err != nil {
		return 0, err
	}
	for _, f := range missed {
		s.writeAuditAs(ctx, audit.ActorSystem, "system", "crm.followup.missed", EntityFollowup, f.ID, nil, followupSnapshot(f))
	}
	return len(missed), nil
}

// ── proposals ───────────────────────────────────────────────────────────────

// CreateProposal validates and stores a proposal, emitting
// crm.proposal.created.v1. A discount above DefaultDiscountApprovalPercent
// needs the caller to hold crm.proposal.discount_approve (§3.3, §8.1).
func (s *Service) CreateProposal(ctx context.Context, actor uuid.UUID, canApproveDiscount bool, in ProposalInput) (Proposal, error) {
	opp, err := s.GetOpportunity(ctx, in.OpportunityID)
	if err != nil {
		return Proposal{}, err
	}
	p, err := validateProposal(Proposal{
		OpportunityID: in.OpportunityID, ScopeSummary: in.ScopeSummary, LineItems: in.LineItems,
		PriceMinor: in.PriceMinor, Currency: in.Currency, DiscountMinor: in.DiscountMinor,
		ValidUntil: in.ValidUntil, DocumentFileID: in.DocumentFileID, Status: ProposalDraft, Version: 1,
	})
	if err != nil {
		return Proposal{}, err
	}
	if threshold := p.PriceMinor * DefaultDiscountApprovalPercent / 100; p.DiscountMinor > threshold && !canApproveDiscount {
		return Proposal{}, fmt.Errorf("%w: a discount above %d%% needs discount approval", ErrForbidden, DefaultDiscountApprovalPercent)
	}
	id, err := uuid.NewV7()
	if err != nil {
		return Proposal{}, fmt.Errorf("crm: new id: %w", err)
	}
	p.ID = id
	now := s.now()
	p.CreatedAt, p.UpdatedAt = now, now
	ownerUser, err := s.accountForPerson(ctx, opp.OwnerPersonID)
	if err != nil {
		return Proposal{}, err
	}
	created, err := s.store.CreateProposal(ctx, p, func(ref string) (outbox.Event, error) {
		return outbox.NewProposalCreated(actor, outbox.ProposalRef{
			ID: p.ID, Ref: ref, OpportunityID: p.OpportunityID, Status: string(p.Status),
			PriceMinor: p.PriceMinor, Currency: p.Currency,
			OwnerID: opp.OwnerPersonID, OwnerUserID: ownerUser,
		})
	})
	if err != nil {
		return Proposal{}, err
	}
	s.writeAudit(ctx, actor, "crm.proposal.created", EntityProposal, created.ID, nil, proposalSnapshot(created))
	return created, nil
}

// GetProposal returns a proposal or ErrNotFound.
func (s *Service) GetProposal(ctx context.Context, id uuid.UUID) (Proposal, error) {
	p, err := s.store.GetProposal(ctx, id)
	if err != nil {
		return Proposal{}, err
	}
	if p == nil {
		return Proposal{}, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	return *p, nil
}

// ListProposals returns an opportunity's proposals, newest version first.
func (s *Service) ListProposals(ctx context.Context, opportunityID uuid.UUID) ([]Proposal, error) {
	return s.store.ListProposals(ctx, opportunityID)
}

// SendProposal marks a draft proposal sent, emitting crm.proposal.sent.v1.
func (s *Service) SendProposal(ctx context.Context, actor uuid.UUID, id uuid.UUID) (Proposal, error) {
	cur, err := s.GetProposal(ctx, id)
	if err != nil {
		return Proposal{}, err
	}
	if cur.Status != ProposalDraft {
		return Proposal{}, fmt.Errorf("%w: only a draft proposal can be sent", ErrConflict)
	}
	opp, err := s.GetOpportunity(ctx, cur.OpportunityID)
	if err != nil {
		return Proposal{}, err
	}
	ownerUser, err := s.accountForPerson(ctx, opp.OwnerPersonID)
	if err != nil {
		return Proposal{}, err
	}
	ev, err := outbox.NewProposalSent(actor, outbox.ProposalRef{
		ID: cur.ID, Ref: cur.Ref, OpportunityID: cur.OpportunityID, Status: string(ProposalSent),
		PriceMinor: cur.PriceMinor, Currency: cur.Currency, OwnerID: opp.OwnerPersonID, OwnerUserID: ownerUser,
	})
	if err != nil {
		return Proposal{}, err
	}
	updated, err := s.store.SetProposalStatus(ctx, id, ProposalSent, &ev)
	if err != nil {
		return Proposal{}, err
	}
	s.writeAudit(ctx, actor, "crm.proposal.sent", EntityProposal, updated.ID, proposalSnapshot(cur), proposalSnapshot(updated))
	return updated, nil
}

// Pipeline returns the per-stage pipeline totals for the commercial dashboard.
func (s *Service) Pipeline(ctx context.Context) ([]PipelineStageTotal, error) {
	return s.store.PipelineSummary(ctx)
}

// ── helpers ─────────────────────────────────────────────────────────────────

func (s *Service) accountForPerson(ctx context.Context, personID *uuid.UUID) (*uuid.UUID, error) {
	if personID == nil {
		return nil, nil
	}
	return s.store.AccountForPerson(ctx, *personID)
}

func (s *Service) writeAudit(ctx context.Context, actor uuid.UUID, action, entityType string, entityID uuid.UUID, before, after any) {
	s.writeAuditAs(ctx, audit.ActorUser, actor.String(), action, entityType, entityID, before, after)
}

func (s *Service) writeAuditAs(ctx context.Context, actorType, actorID, action, entityType string, entityID uuid.UUID, before, after any) {
	if s.audit == nil {
		return
	}
	id := entityID
	if err := s.audit.Write(ctx, nil, audit.Event{
		ActorType: actorType, ActorID: actorID, Action: action,
		EntityType: entityType, EntityID: &id, Before: before, After: after,
	}); err != nil {
		s.logger.ErrorContext(ctx, "crm: audit write failed", slog.String("action", action), slog.String("error", err.Error()))
	}
}

// ── snapshots (audit payloads) ──────────────────────────────────────────────

func organizationSnapshot(o Organization) map[string]any {
	return map[string]any{"name": o.Name, "status": string(o.Status), "industry": o.Industry, "location": o.Location}
}

func contactSnapshot(c Contact) map[string]any {
	return map[string]any{
		"name": c.Name, "organization_id": idOrEmpty(c.OrganizationID),
		"consent_marketing": c.ConsentMarketing, "consent_whatsapp": c.ConsentWhatsApp,
		"consent_call_recording": c.ConsentRecording,
	}
}

func leadSnapshot(l Lead) map[string]any {
	return map[string]any{
		"ref": l.Ref, "status": string(l.Status), "source_key": l.SourceKey,
		"owner_person_id": idOrEmpty(l.OwnerPersonID), "priority": string(l.Priority),
		"next_action_due_at": l.NextActionDueAt,
	}
}

func opportunitySnapshot(o Opportunity) map[string]any {
	return map[string]any{
		"ref": o.Ref, "stage_key": o.StageKey, "title": o.Title,
		"value_minor": o.ValueMinor, "currency": o.Currency, "owner_person_id": idOrEmpty(o.OwnerPersonID),
	}
}

func interactionSnapshot(i Interaction) map[string]any {
	return map[string]any{
		"type": string(i.Type), "direction": string(i.Direction), "outcome": i.Outcome,
		"lead_id": idOrEmpty(i.LeadID), "opportunity_id": idOrEmpty(i.OpportunityID),
	}
}

func followupSnapshot(f Followup) map[string]any {
	return map[string]any{
		"status": string(f.Status), "channel": string(f.Channel), "due_at": f.DueAt,
		"owner_person_id": f.OwnerPersonID, "lead_id": idOrEmpty(f.LeadID), "opportunity_id": idOrEmpty(f.OpportunityID),
	}
}

func proposalSnapshot(p Proposal) map[string]any {
	return map[string]any{
		"ref": p.Ref, "status": string(p.Status), "price_minor": p.PriceMinor,
		"discount_minor": p.DiscountMinor, "currency": p.Currency,
	}
}

func idOrEmpty(id *uuid.UUID) string {
	if id == nil {
		return ""
	}
	return id.String()
}
