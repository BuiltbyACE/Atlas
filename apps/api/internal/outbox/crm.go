package outbox

import (
	"time"

	"github.com/google/uuid"
)

// CRM event types (§7.4). The orchestrator reacts to these to nudge owners and
// invalidate the commercial views (ADR-009); Go records the fact.
const (
	TypeLeadCreatedV1        = "crm.lead.created.v1"
	TypeLeadAssignedV1       = "crm.lead.assigned.v1"
	TypeLeadStatusChangedV1  = "crm.lead.status_changed.v1"
	TypeLeadConvertedV1      = "crm.lead.converted.v1"
	TypeOpportunityCreatedV1 = "crm.opportunity.created.v1"
	TypeOpportunityStageV1   = "crm.opportunity.stage_changed.v1"
	TypeOpportunityWonV1     = "crm.opportunity.won.v1"
	TypeOpportunityLostV1    = "crm.opportunity.lost.v1"
	TypeInteractionRecorded  = "crm.interaction.recorded.v1"
	TypeFollowupScheduledV1  = "crm.followup.scheduled.v1"
	TypeFollowupCompletedV1  = "crm.followup.completed.v1"
	TypeFollowupMissedV1     = "crm.followup.missed.v1"
	TypeProposalCreatedV1    = "crm.proposal.created.v1"
	TypeProposalSentV1       = "crm.proposal.sent.v1"
)

// Aggregate types for CRM events.
const (
	AggregateOrganization = "organization"
	AggregateContact      = "contact"
	AggregateLead         = "lead"
	AggregateOpportunity  = "opportunity"
	AggregateInteraction  = "interaction"
	AggregateFollowup     = "followup"
	AggregateProposal     = "proposal"
)

// LeadPayload is emitted when a lead is created or assigned. OwnerUserID is the
// owner's account (core.user_accounts) so the realtime bus can address them
// (§7.5); nil when unknown.
type LeadPayload struct {
	LeadID      uuid.UUID  `json:"lead_id"`
	Ref         string     `json:"ref"`
	Status      string     `json:"status"`
	OwnerID     *uuid.UUID `json:"owner_person_id,omitempty"`
	OwnerUserID *uuid.UUID `json:"owner_user_id,omitempty"`
	ActorID     *uuid.UUID `json:"actor_id,omitempty"`
	OccurredAt  time.Time  `json:"occurred_at"`
}

// LeadStatusChangedPayload is emitted on a lead status transition.
type LeadStatusChangedPayload struct {
	LeadID     uuid.UUID `json:"lead_id"`
	Ref        string    `json:"ref"`
	From       string    `json:"from"`
	To         string    `json:"to"`
	ChangedBy  uuid.UUID `json:"changed_by,omitempty"`
	OccurredAt time.Time `json:"occurred_at"`
}

// LeadConvertedPayload is emitted when a qualified lead becomes an opportunity.
type LeadConvertedPayload struct {
	LeadID        uuid.UUID `json:"lead_id"`
	Ref           string    `json:"ref"`
	OpportunityID uuid.UUID `json:"opportunity_id"`
	ConvertedBy   uuid.UUID `json:"converted_by,omitempty"`
	OccurredAt    time.Time `json:"occurred_at"`
}

// OpportunityPayload is emitted when an opportunity is created or changes stage.
type OpportunityPayload struct {
	OpportunityID uuid.UUID  `json:"opportunity_id"`
	Ref           string     `json:"ref"`
	StageKey      string     `json:"stage_key"`
	OwnerID       *uuid.UUID `json:"owner_person_id,omitempty"`
	OwnerUserID   *uuid.UUID `json:"owner_user_id,omitempty"`
	ActorID       *uuid.UUID `json:"actor_id,omitempty"`
	OccurredAt    time.Time  `json:"occurred_at"`
}

// OpportunityStageChangedPayload is emitted on a stage move.
type OpportunityStageChangedPayload struct {
	OpportunityID uuid.UUID `json:"opportunity_id"`
	Ref           string    `json:"ref"`
	From          string    `json:"from"`
	To            string    `json:"to"`
	ChangedBy     uuid.UUID `json:"changed_by,omitempty"`
	OccurredAt    time.Time `json:"occurred_at"`
}

// OpportunityLostPayload carries the reason, so strategy is fed (§8.1).
type OpportunityLostPayload struct {
	OpportunityID uuid.UUID `json:"opportunity_id"`
	Ref           string    `json:"ref"`
	LostReason    string    `json:"lost_reason"`
	ActorID       uuid.UUID `json:"actor_id,omitempty"`
	OccurredAt    time.Time `json:"occurred_at"`
}

// OpportunityWonPayload is emitted when an opportunity is won. It starts the
// contract workflow (§8.1, §9.1).
type OpportunityWonPayload struct {
	OpportunityID uuid.UUID `json:"opportunity_id"`
	Ref           string    `json:"ref"`
	ActorID       uuid.UUID `json:"actor_id,omitempty"`
	OccurredAt    time.Time `json:"occurred_at"`
}

// InteractionRecordedPayload is emitted when a touch is logged.
type InteractionRecordedPayload struct {
	InteractionID uuid.UUID  `json:"interaction_id"`
	Type          string     `json:"interaction_type"`
	LeadID        *uuid.UUID `json:"lead_id,omitempty"`
	OpportunityID *uuid.UUID `json:"opportunity_id,omitempty"`
	ContactID     *uuid.UUID `json:"contact_id,omitempty"`
	OwnerUserID   *uuid.UUID `json:"owner_user_id,omitempty"`
	ActorID       uuid.UUID  `json:"actor_id,omitempty"`
	OccurredAt    time.Time  `json:"occurred_at"`
}

// FollowupPayload is emitted when a follow-up is scheduled or resolved.
type FollowupPayload struct {
	FollowupID    uuid.UUID  `json:"followup_id"`
	LeadID        *uuid.UUID `json:"lead_id,omitempty"`
	OpportunityID *uuid.UUID `json:"opportunity_id,omitempty"`
	OwnerID       uuid.UUID  `json:"owner_person_id"`
	OwnerUserID   *uuid.UUID `json:"owner_user_id,omitempty"`
	DueAt         time.Time  `json:"due_at"`
	Status        string     `json:"status"`
	OccurredAt    time.Time  `json:"occurred_at"`
}

// ProposalPayload is emitted when a proposal is created or sent.
type ProposalPayload struct {
	ProposalID    uuid.UUID  `json:"proposal_id"`
	Ref           string     `json:"ref"`
	OpportunityID uuid.UUID  `json:"opportunity_id"`
	Status        string     `json:"status"`
	PriceMinor    int64      `json:"price_minor"`
	Currency      string     `json:"currency"`
	OwnerID       *uuid.UUID `json:"owner_person_id,omitempty"`
	OwnerUserID   *uuid.UUID `json:"owner_user_id,omitempty"`
	ActorID       uuid.UUID  `json:"actor_id,omitempty"`
	OccurredAt    time.Time  `json:"occurred_at"`
}

// LeadRef is the minimal lead view the event builders need.
type LeadRef struct {
	ID          uuid.UUID
	Ref         string
	Status      string
	OwnerID     *uuid.UUID
	OwnerUserID *uuid.UUID
}

// NewLeadCreated builds a crm.lead.created.v1 event.
func NewLeadCreated(actorID *uuid.UUID, l LeadRef) (Event, error) {
	return buildEvent(TypeLeadCreatedV1, AggregateLead, l.ID, actorID, nil, LeadPayload{
		LeadID: l.ID, Ref: l.Ref, Status: l.Status,
		OwnerID: l.OwnerID, OwnerUserID: l.OwnerUserID, ActorID: actorID, OccurredAt: time.Now().UTC(),
	})
}

// NewLeadAssigned builds a crm.lead.assigned.v1 event.
func NewLeadAssigned(actorID *uuid.UUID, l LeadRef, assignedBy uuid.UUID) (Event, error) {
	return buildEvent(TypeLeadAssignedV1, AggregateLead, l.ID, actorID, nil, LeadPayload{
		LeadID: l.ID, Ref: l.Ref, Status: l.Status,
		OwnerID: l.OwnerID, OwnerUserID: l.OwnerUserID, ActorID: &assignedBy, OccurredAt: time.Now().UTC(),
	})
}

// NewLeadStatusChanged builds a crm.lead.status_changed.v1 event.
func NewLeadStatusChanged(actorID *uuid.UUID, id uuid.UUID, ref, from, to string, changedBy uuid.UUID) (Event, error) {
	return buildEvent(TypeLeadStatusChangedV1, AggregateLead, id, actorID, nil, LeadStatusChangedPayload{
		LeadID: id, Ref: ref, From: from, To: to, ChangedBy: changedBy, OccurredAt: time.Now().UTC(),
	})
}

// NewLeadConverted builds a crm.lead.converted.v1 event.
func NewLeadConverted(actorID *uuid.UUID, leadID uuid.UUID, ref string, opportunityID uuid.UUID, convertedBy uuid.UUID) (Event, error) {
	return buildEvent(TypeLeadConvertedV1, AggregateLead, leadID, actorID, nil, LeadConvertedPayload{
		LeadID: leadID, Ref: ref, OpportunityID: opportunityID, ConvertedBy: convertedBy, OccurredAt: time.Now().UTC(),
	})
}

// NewOpportunityCreated builds a crm.opportunity.created.v1 event.
func NewOpportunityCreated(actorID *uuid.UUID, o OpportunityRef) (Event, error) {
	return buildEvent(TypeOpportunityCreatedV1, AggregateOpportunity, o.ID, actorID, nil, OpportunityPayload{
		OpportunityID: o.ID, Ref: o.Ref, StageKey: o.StageKey,
		OwnerID: o.OwnerID, OwnerUserID: o.OwnerUserID, ActorID: actorID, OccurredAt: time.Now().UTC(),
	})
}

// NewOpportunityStageChanged builds a crm.opportunity.stage_changed.v1 event.
func NewOpportunityStageChanged(actorID *uuid.UUID, id uuid.UUID, ref, from, to string, changedBy uuid.UUID) (Event, error) {
	return buildEvent(TypeOpportunityStageV1, AggregateOpportunity, id, actorID, nil, OpportunityStageChangedPayload{
		OpportunityID: id, Ref: ref, From: from, To: to, ChangedBy: changedBy, OccurredAt: time.Now().UTC(),
	})
}

// NewOpportunityWon builds a crm.opportunity.won.v1 event.
func NewOpportunityWon(actorID *uuid.UUID, id uuid.UUID, ref string, wonBy uuid.UUID) (Event, error) {
	return buildEvent(TypeOpportunityWonV1, AggregateOpportunity, id, actorID, nil, OpportunityWonPayload{
		OpportunityID: id, Ref: ref, ActorID: wonBy, OccurredAt: time.Now().UTC(),
	})
}

// NewOpportunityLost builds a crm.opportunity.lost.v1 event.
func NewOpportunityLost(actorID *uuid.UUID, id uuid.UUID, ref, reason string, lostBy uuid.UUID) (Event, error) {
	return buildEvent(TypeOpportunityLostV1, AggregateOpportunity, id, actorID, nil, OpportunityLostPayload{
		OpportunityID: id, Ref: ref, LostReason: reason, ActorID: lostBy, OccurredAt: time.Now().UTC(),
	})
}

// OpportunityRef is the minimal opportunity view the builders need.
type OpportunityRef struct {
	ID          uuid.UUID
	Ref         string
	StageKey    string
	OwnerID     *uuid.UUID
	OwnerUserID *uuid.UUID
}

// InteractionRef is the minimal interaction view the builder needs.
type InteractionRef struct {
	ID            uuid.UUID
	Type          string
	ContactID     *uuid.UUID
	LeadID        *uuid.UUID
	OpportunityID *uuid.UUID
	OwnerUserID   *uuid.UUID
}

// NewInteractionRecorded builds a crm.interaction.recorded.v1 event.
func NewInteractionRecorded(actorID uuid.UUID, i InteractionRef) (Event, error) {
	return buildEvent(TypeInteractionRecorded, AggregateInteraction, i.ID, &actorID, nil, InteractionRecordedPayload{
		InteractionID: i.ID, Type: i.Type,
		ContactID: i.ContactID, LeadID: i.LeadID, OpportunityID: i.OpportunityID,
		OwnerUserID: i.OwnerUserID, ActorID: actorID, OccurredAt: time.Now().UTC(),
	})
}

// FollowupRef is the minimal follow-up view the builders need.
type FollowupRef struct {
	ID            uuid.UUID
	LeadID        *uuid.UUID
	OpportunityID *uuid.UUID
	OwnerID       uuid.UUID
	OwnerUserID   *uuid.UUID
	DueAt         time.Time
	Status        string
}

// NewFollowupScheduled builds a crm.followup.scheduled.v1 event.
func NewFollowupScheduled(actorID *uuid.UUID, f FollowupRef) (Event, error) {
	return buildEvent(TypeFollowupScheduledV1, AggregateFollowup, f.ID, actorID, nil, FollowupPayload{
		FollowupID: f.ID, LeadID: f.LeadID, OpportunityID: f.OpportunityID,
		OwnerID: f.OwnerID, OwnerUserID: f.OwnerUserID, DueAt: f.DueAt, Status: f.Status, OccurredAt: time.Now().UTC(),
	})
}

// NewFollowupCompleted builds a crm.followup.completed.v1 event.
func NewFollowupCompleted(actorID *uuid.UUID, f FollowupRef) (Event, error) {
	return buildEvent(TypeFollowupCompletedV1, AggregateFollowup, f.ID, actorID, nil, FollowupPayload{
		FollowupID: f.ID, LeadID: f.LeadID, OpportunityID: f.OpportunityID,
		OwnerID: f.OwnerID, OwnerUserID: f.OwnerUserID, DueAt: f.DueAt, Status: f.Status, OccurredAt: time.Now().UTC(),
	})
}

// NewFollowupMissed builds a crm.followup.missed.v1 event, emitted by the sweep.
func NewFollowupMissed(actorID *uuid.UUID, f FollowupRef) (Event, error) {
	return buildEvent(TypeFollowupMissedV1, AggregateFollowup, f.ID, actorID, nil, FollowupPayload{
		FollowupID: f.ID, LeadID: f.LeadID, OpportunityID: f.OpportunityID,
		OwnerID: f.OwnerID, OwnerUserID: f.OwnerUserID, DueAt: f.DueAt, Status: f.Status, OccurredAt: time.Now().UTC(),
	})
}

// ProposalRef is the minimal proposal view the builders need.
type ProposalRef struct {
	ID            uuid.UUID
	Ref           string
	OpportunityID uuid.UUID
	Status        string
	PriceMinor    int64
	Currency      string
	OwnerID       *uuid.UUID
	OwnerUserID   *uuid.UUID
}

// NewProposalCreated builds a crm.proposal.created.v1 event.
func NewProposalCreated(actorID uuid.UUID, p ProposalRef) (Event, error) {
	return buildEvent(TypeProposalCreatedV1, AggregateProposal, p.ID, &actorID, nil, ProposalPayload{
		ProposalID: p.ID, Ref: p.Ref, OpportunityID: p.OpportunityID, Status: p.Status,
		PriceMinor: p.PriceMinor, Currency: p.Currency,
		OwnerID: p.OwnerID, OwnerUserID: p.OwnerUserID, ActorID: actorID, OccurredAt: time.Now().UTC(),
	})
}

// NewProposalSent builds a crm.proposal.sent.v1 event.
func NewProposalSent(actorID uuid.UUID, p ProposalRef) (Event, error) {
	return buildEvent(TypeProposalSentV1, AggregateProposal, p.ID, &actorID, nil, ProposalPayload{
		ProposalID: p.ID, Ref: p.Ref, OpportunityID: p.OpportunityID, Status: p.Status,
		PriceMinor: p.PriceMinor, Currency: p.Currency,
		OwnerID: p.OwnerID, OwnerUserID: p.OwnerUserID, ActorID: actorID, OccurredAt: time.Now().UTC(),
	})
}
