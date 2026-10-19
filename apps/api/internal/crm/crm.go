// Package crm owns Phase 3 client acquisition: organizations, contacts, lead
// sources, leads, pipeline stages, opportunities, interactions, follow-ups and
// proposals (docs/ATLAS.md §8.1). Go is the sole writer (ADR-002); every state
// change is audited and, where the orchestrator or the browser must react, an
// outbox event is written in the same transaction (ADR-005, ADR-009).
//
// Optimistic concurrency (§6.6): every write takes the version the client read
// and the store rejects a stale one with ErrConflict. Listings are keyset-paged.
// Money is integer minor units with a currency (ADR-010).
//
// The WhatsApp and telephony providers are deferred (ADR-011, ADR-012): calls
// and messages are recorded manually through the interaction endpoint, and a
// no-op provider seam (Messenger) is left for a later adapter.
package crm

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Sentinel errors. The HTTP layer maps these onto problem responses.
var (
	// ErrNotFound marks an unknown organization/contact/lead/….
	ErrNotFound = errors.New("crm: not found")
	// ErrInvalid marks a request that fails validation.
	ErrInvalid = errors.New("crm: invalid")
	// ErrConflict marks a stale version or an illegal transition.
	ErrConflict = errors.New("crm: conflict")
	// ErrForbidden marks an action the caller may not take.
	ErrForbidden = errors.New("crm: forbidden")
	// ErrDuplicate marks a record that duplicates an existing one (§8.1).
	ErrDuplicate = errors.New("crm: duplicate")
)

// ── enums ───────────────────────────────────────────────────────────────────

// OrganizationStatus is the lifecycle of a client company (§8.1).
type OrganizationStatus string

const (
	OrgProspect OrganizationStatus = "prospect"
	OrgClient   OrganizationStatus = "client"
	OrgFormer   OrganizationStatus = "former"
)

func validOrgStatus(s OrganizationStatus) bool {
	switch s {
	case OrgProspect, OrgClient, OrgFormer:
		return true
	}
	return false
}

// LeadStatus is the lead lifecycle state (§8.1).
type LeadStatus string

const (
	LeadNew          LeadStatus = "new"
	LeadAssigned     LeadStatus = "assigned"
	LeadContacted    LeadStatus = "contacted"
	LeadQualified    LeadStatus = "qualified"
	LeadNurture      LeadStatus = "nurture"
	LeadDisqualified LeadStatus = "disqualified"
	LeadConverted    LeadStatus = "converted"
)

func validLeadStatus(s LeadStatus) bool {
	switch s {
	case LeadNew, LeadAssigned, LeadContacted, LeadQualified, LeadNurture, LeadDisqualified, LeadConverted:
		return true
	}
	return false
}

// openLead reports whether a lead still needs an owner and a dated next action.
func openLead(s LeadStatus) bool {
	switch s {
	case LeadAssigned, LeadContacted, LeadQualified, LeadNurture:
		return true
	}
	return false
}

// allowedLeadTransition encodes the §8.1 lifecycle diagram.
func allowedLeadTransition(from, to LeadStatus) bool {
	if from == to {
		return true
	}
	switch from {
	case LeadNew:
		return to == LeadAssigned || to == LeadDisqualified
	case LeadAssigned:
		return to == LeadContacted || to == LeadDisqualified
	case LeadContacted:
		return to == LeadQualified || to == LeadNurture || to == LeadDisqualified
	case LeadNurture:
		return to == LeadContacted || to == LeadDisqualified
	case LeadQualified:
		return to == LeadConverted || to == LeadDisqualified
	}
	return false
}

// Priority is a lead's triage priority (§8.1).
type Priority string

const (
	PriorityLow    Priority = "low"
	PriorityNormal Priority = "normal"
	PriorityHigh   Priority = "high"
	PriorityUrgent Priority = "urgent"
)

func validPriority(p Priority) bool {
	switch p {
	case PriorityLow, PriorityNormal, PriorityHigh, PriorityUrgent:
		return true
	}
	return false
}

// LostReason explains a lost lead or opportunity (§8.1). It feeds strategy.
type LostReason string

const (
	LostPrice      LostReason = "price"
	LostTiming     LostReason = "timing"
	LostCompetitor LostReason = "competitor"
	LostNoResponse LostReason = "no_response"
	LostScopeFit   LostReason = "scope_fit"
	LostOther      LostReason = "other"
)

func validLostReason(r LostReason) bool {
	switch r {
	case LostPrice, LostTiming, LostCompetitor, LostNoResponse, LostScopeFit, LostOther:
		return true
	}
	return false
}

// InteractionType is how a touch happened (§8.1).
type InteractionType string

const (
	InteractionCall     InteractionType = "call"
	InteractionWhatsApp InteractionType = "whatsapp"
	InteractionEmail    InteractionType = "email"
	InteractionMeeting  InteractionType = "meeting"
	InteractionNote     InteractionType = "note"
)

func validInteractionType(t InteractionType) bool {
	switch t {
	case InteractionCall, InteractionWhatsApp, InteractionEmail, InteractionMeeting, InteractionNote:
		return true
	}
	return false
}

// Direction is the direction of an interaction.
type Direction string

const (
	DirectionInbound  Direction = "inbound"
	DirectionOutbound Direction = "outbound"
)

func validDirection(d Direction) bool {
	return d == DirectionInbound || d == DirectionOutbound
}

// FollowupChannel is how a follow-up is due to happen (§8.1).
type FollowupChannel string

const (
	ChannelCall     FollowupChannel = "call"
	ChannelWhatsApp FollowupChannel = "whatsapp"
	ChannelEmail    FollowupChannel = "email"
	ChannelMeeting  FollowupChannel = "meeting"
	ChannelOther    FollowupChannel = "other"
)

func validFollowupChannel(c FollowupChannel) bool {
	switch c {
	case ChannelCall, ChannelWhatsApp, ChannelEmail, ChannelMeeting, ChannelOther:
		return true
	}
	return false
}

// FollowupStatus is a follow-up's state (§8.1).
type FollowupStatus string

const (
	FollowupScheduled FollowupStatus = "scheduled"
	FollowupDone      FollowupStatus = "done"
	FollowupMissed    FollowupStatus = "missed"
	FollowupCancelled FollowupStatus = "cancelled"
)

func validFollowupStatus(s FollowupStatus) bool {
	switch s {
	case FollowupScheduled, FollowupDone, FollowupMissed, FollowupCancelled:
		return true
	}
	return false
}

// ProposalStatus is a proposal's state (§8.1).
type ProposalStatus string

const (
	ProposalDraft    ProposalStatus = "draft"
	ProposalSent     ProposalStatus = "sent"
	ProposalAccepted ProposalStatus = "accepted"
	ProposalDeclined ProposalStatus = "declined"
	ProposalExpired  ProposalStatus = "expired"
)

func validProposalStatus(s ProposalStatus) bool {
	switch s {
	case ProposalDraft, ProposalSent, ProposalAccepted, ProposalDeclined, ProposalExpired:
		return true
	}
	return false
}

// ── entities ────────────────────────────────────────────────────────────────

// Organization is a client or prospect company (§8.1).
type Organization struct {
	ID             uuid.UUID
	Name           string
	NameNormalized string
	Industry       string
	Size           string
	Location       string
	KRAPin         string
	Status         OrganizationStatus
	OwnerPersonID  *uuid.UUID
	Version        int
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// Contact is a person at an organization (§8.1). Consent is recorded with a
// timestamp and a source (ADR-011).
type Contact struct {
	ID                     uuid.UUID
	OrganizationID         *uuid.UUID
	Name                   string
	Role                   string
	Phones                 []string
	Emails                 []string
	WhatsAppNumber         string
	ConsentMarketing       bool
	ConsentMarketingAt     *time.Time
	ConsentMarketingSource string
	ConsentWhatsApp        bool
	ConsentWhatsAppAt      *time.Time
	ConsentWhatsAppSource  string
	ConsentRecording       bool
	ConsentRecordingAt     *time.Time
	ConsentRecordingSource string
	Version                int
	CreatedAt              time.Time
	UpdatedAt              time.Time
}

// LeadSource is a configured origin for leads (§8.1 [CONFIG]).
type LeadSource struct {
	Key       string
	Label     string
	SortOrder int
	Active    bool
}

// PipelineStage is a configured opportunity stage (§8.1 [CONFIG]).
type PipelineStage struct {
	Key       string
	Label     string
	SortOrder int
	IsWon     bool
	IsLost    bool
	Active    bool
}

// Lead is an enquiry on its way to qualified (§8.1).
type Lead struct {
	ID                     uuid.UUID
	Ref                    string
	SourceKey              string
	SourceDetail           string
	OrganizationID         *uuid.UUID
	ContactID              *uuid.UUID
	ContactName            string
	ContactPhone           string
	ContactEmail           string
	NeedSummary            string
	EstimatedValueMinor    *int64
	Currency               string
	Priority               Priority
	OwnerPersonID          *uuid.UUID
	Status                 LeadStatus
	NextAction             string
	NextActionDueAt        *time.Time
	QualificationNotes     string
	LostReason             *LostReason
	ConvertedOpportunityID *uuid.UUID
	Version                int
	CreatedAt              time.Time
	UpdatedAt              time.Time
}

// Opportunity is a qualified deal in the pipeline (§8.1).
type Opportunity struct {
	ID              uuid.UUID
	Ref             string
	OrganizationID  *uuid.UUID
	Title           string
	ValueMinor      *int64
	Currency        string
	Probability     *int
	StageKey        string
	ExpectedCloseAt *time.Time
	OwnerPersonID   *uuid.UUID
	LostReason      *LostReason
	Version         int
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// Interaction is a call, message, email, meeting or note (§8.1).
type Interaction struct {
	ID              uuid.UUID
	Type            InteractionType
	Direction       Direction
	ContactID       *uuid.UUID
	LeadID          *uuid.UUID
	OpportunityID   *uuid.UUID
	OccurredAt      time.Time
	DurationMinutes *int
	Outcome         string
	Summary         string
	NextAction      string
	NextActionDueAt *time.Time
	RecordingFileID *uuid.UUID
	RecordedBy      uuid.UUID
	Version         int
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// Followup is a scheduled next touch (§8.1).
type Followup struct {
	ID            uuid.UUID
	LeadID        *uuid.UUID
	OpportunityID *uuid.UUID
	OwnerPersonID uuid.UUID
	DueAt         time.Time
	Channel       FollowupChannel
	Reason        string
	Status        FollowupStatus
	Outcome       string
	CompletedAt   *time.Time
	Version       int
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// Proposal is a priced offer against an opportunity (§8.1).
type Proposal struct {
	ID             uuid.UUID
	Ref            string
	OpportunityID  uuid.UUID
	VersionNo      int
	ScopeSummary   string
	LineItems      []byte
	PriceMinor     int64
	Currency       string
	DiscountMinor  int64
	ValidUntil     *time.Time
	Status         ProposalStatus
	DocumentFileID *uuid.UUID
	ApprovalID     *uuid.UUID
	Version        int
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// ── validation ──────────────────────────────────────────────────────────────

func trim(s string) string { return strings.TrimSpace(s) }

func normalizeName(s string) string { return strings.ToLower(trim(s)) }

// defaultCurrency keeps money in KES unless the caller says otherwise (§6.6).
func defaultCurrency(c string) string {
	c = strings.ToUpper(trim(c))
	if c == "" {
		return "KES"
	}
	return c
}

func validateOrganization(o Organization) (Organization, error) {
	o.Name = trim(o.Name)
	if o.Name == "" {
		return o, fmt.Errorf("%w: organization name is required", ErrInvalid)
	}
	o.NameNormalized = normalizeName(o.Name)
	o.Industry = trim(o.Industry)
	o.Size = trim(o.Size)
	o.Location = trim(o.Location)
	o.KRAPin = trim(o.KRAPin)
	if o.Status == "" {
		o.Status = OrgProspect
	}
	if !validOrgStatus(o.Status) {
		return o, fmt.Errorf("%w: unknown organization status %q", ErrInvalid, o.Status)
	}
	return o, nil
}

func validateContact(c Contact) (Contact, error) {
	c.Name = trim(c.Name)
	if c.Name == "" {
		return c, fmt.Errorf("%w: contact name is required", ErrInvalid)
	}
	c.Role = trim(c.Role)
	c.WhatsAppNumber = trim(c.WhatsAppNumber)
	c.Phones = cleanList(c.Phones)
	c.Emails = cleanList(c.Emails)
	return c, nil
}

func cleanList(in []string) []string {
	if len(in) == 0 {
		return []string{}
	}
	out := make([]string, 0, len(in))
	for _, v := range in {
		if v = trim(v); v != "" {
			out = append(out, v)
		}
	}
	return out
}

// validateLead enforces §8.1: a lead cannot be open without an owner and a
// dated next action; a disqualified lead needs a lost reason.
func validateLead(l Lead) (Lead, error) {
	l.SourceKey = trim(l.SourceKey)
	if l.SourceKey == "" {
		return l, fmt.Errorf("%w: source is required", ErrInvalid)
	}
	l.SourceDetail = trim(l.SourceDetail)
	l.ContactName = trim(l.ContactName)
	l.ContactPhone = trim(l.ContactPhone)
	l.ContactEmail = trim(l.ContactEmail)
	l.NeedSummary = trim(l.NeedSummary)
	l.NextAction = trim(l.NextAction)
	l.QualificationNotes = trim(l.QualificationNotes)
	l.Currency = defaultCurrency(l.Currency)
	if l.Priority == "" {
		l.Priority = PriorityNormal
	}
	if !validPriority(l.Priority) {
		return l, fmt.Errorf("%w: unknown priority %q", ErrInvalid, l.Priority)
	}
	if l.Status == "" {
		l.Status = LeadNew
	}
	if !validLeadStatus(l.Status) {
		return l, fmt.Errorf("%w: unknown lead status %q", ErrInvalid, l.Status)
	}
	if openLead(l.Status) {
		if l.OwnerPersonID == nil {
			return l, fmt.Errorf("%w: an open lead needs an owner", ErrInvalid)
		}
		if l.NextAction == "" || l.NextActionDueAt == nil {
			return l, fmt.Errorf("%w: an open lead needs a next action with a due date", ErrInvalid)
		}
	}
	if l.Status == LeadDisqualified && (l.LostReason == nil || !validLostReason(*l.LostReason)) {
		return l, fmt.Errorf("%w: a disqualified lead needs a lost reason", ErrInvalid)
	}
	if l.EstimatedValueMinor != nil && *l.EstimatedValueMinor < 0 {
		return l, fmt.Errorf("%w: estimated value cannot be negative", ErrInvalid)
	}
	return l, nil
}

func validateOpportunity(o Opportunity) (Opportunity, error) {
	o.Title = trim(o.Title)
	if o.Title == "" {
		return o, fmt.Errorf("%w: opportunity title is required", ErrInvalid)
	}
	o.Currency = defaultCurrency(o.Currency)
	o.StageKey = trim(o.StageKey)
	if o.StageKey == "" {
		return o, fmt.Errorf("%w: pipeline stage is required", ErrInvalid)
	}
	if o.Probability != nil && (*o.Probability < 0 || *o.Probability > 100) {
		return o, fmt.Errorf("%w: probability must be between 0 and 100", ErrInvalid)
	}
	if o.ValueMinor != nil && *o.ValueMinor < 0 {
		return o, fmt.Errorf("%w: opportunity value cannot be negative", ErrInvalid)
	}
	if o.LostReason != nil && !validLostReason(*o.LostReason) {
		return o, fmt.Errorf("%w: unknown lost reason %q", ErrInvalid, *o.LostReason)
	}
	return o, nil
}

func validateInteraction(i Interaction) (Interaction, error) {
	if !validInteractionType(i.Type) {
		return i, fmt.Errorf("%w: unknown interaction type %q", ErrInvalid, i.Type)
	}
	if i.Direction == "" {
		i.Direction = DirectionOutbound
	}
	if !validDirection(i.Direction) {
		return i, fmt.Errorf("%w: unknown direction %q", ErrInvalid, i.Direction)
	}
	i.Outcome = trim(i.Outcome)
	if i.Outcome == "" {
		return i, fmt.Errorf("%w: an interaction needs an outcome", ErrInvalid)
	}
	i.Summary = trim(i.Summary)
	i.NextAction = trim(i.NextAction)
	if i.NextAction != "" && i.NextActionDueAt == nil {
		return i, fmt.Errorf("%w: a next action needs a due date", ErrInvalid)
	}
	if i.ContactID == nil && i.LeadID == nil && i.OpportunityID == nil {
		return i, fmt.Errorf("%w: an interaction must reference a contact, lead or opportunity", ErrInvalid)
	}
	if i.OccurredAt.IsZero() {
		i.OccurredAt = time.Now().UTC()
	}
	if i.DurationMinutes != nil && *i.DurationMinutes < 0 {
		return i, fmt.Errorf("%w: duration cannot be negative", ErrInvalid)
	}
	return i, nil
}

func validateFollowup(f Followup) (Followup, error) {
	if f.LeadID == nil && f.OpportunityID == nil {
		return f, fmt.Errorf("%w: a follow-up must reference a lead or an opportunity", ErrInvalid)
	}
	if f.OwnerPersonID == uuid.Nil {
		return f, fmt.Errorf("%w: a follow-up needs an owner", ErrInvalid)
	}
	if f.DueAt.IsZero() {
		return f, fmt.Errorf("%w: a follow-up needs a due date", ErrInvalid)
	}
	if !validFollowupChannel(f.Channel) {
		return f, fmt.Errorf("%w: unknown channel %q", ErrInvalid, f.Channel)
	}
	f.Reason = trim(f.Reason)
	if f.Reason == "" {
		return f, fmt.Errorf("%w: a follow-up needs a reason", ErrInvalid)
	}
	if f.Status == "" {
		f.Status = FollowupScheduled
	}
	if !validFollowupStatus(f.Status) {
		return f, fmt.Errorf("%w: unknown follow-up status %q", ErrInvalid, f.Status)
	}
	return f, nil
}

func validateProposal(p Proposal) (Proposal, error) {
	if p.PriceMinor < 0 {
		return p, fmt.Errorf("%w: price cannot be negative", ErrInvalid)
	}
	if p.DiscountMinor < 0 {
		return p, fmt.Errorf("%w: discount cannot be negative", ErrInvalid)
	}
	if p.DiscountMinor > p.PriceMinor {
		return p, fmt.Errorf("%w: discount cannot exceed the price", ErrInvalid)
	}
	p.Currency = defaultCurrency(p.Currency)
	p.ScopeSummary = trim(p.ScopeSummary)
	if p.Status == "" {
		p.Status = ProposalDraft
	}
	if !validProposalStatus(p.Status) {
		return p, fmt.Errorf("%w: unknown proposal status %q", ErrInvalid, p.Status)
	}
	if p.VersionNo < 1 {
		p.VersionNo = 1
	}
	if len(p.LineItems) == 0 {
		p.LineItems = []byte("[]")
	}
	return p, nil
}

// lineItemsOrEmpty keeps the jsonb column well-formed.
func lineItemsOrEmpty(b []byte) []byte {
	if len(b) == 0 {
		return []byte("[]")
	}
	return b
}
