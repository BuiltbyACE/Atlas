package contracts

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/safaristack/atlas/apps/api/internal/audit"
	"github.com/safaristack/atlas/apps/api/internal/outbox"
)

// Entity types used in the audit trail and event aggregate names.
const (
	EntityContract      = "contracts.contract"
	EntityMilestone     = "contracts.milestone"
	EntityDeliverable   = "contracts.deliverable"
	EntityObligation    = "contracts.obligation"
	EntitySLATerm       = "contracts.sla_term"
	EntityChangeRequest = "contracts.change_request"
)

// Signatory is one entry in a contract's signatories list.
type Signatory struct {
	Name     string `json:"name"`
	Role     string `json:"role,omitempty"`
	SignedAt string `json:"signed_at,omitempty"`
	Method   string `json:"method,omitempty"`
}

// Service coordinates contracts: validation, lifecycle transitions, versioning,
// auditing and events.
type Service struct {
	store  Store
	audit  *audit.Writer
	now    func() time.Time
	logger *slog.Logger
}

// NewService builds the contracts service. aw may be nil in tests.
func NewService(store Store, aw *audit.Writer, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{store: store, audit: aw, now: func() time.Time { return time.Now().UTC() }, logger: logger}
}

// ── inputs ──────────────────────────────────────────────────────────────────

// ContractInput is the replacement payload for a contract write.
type ContractInput struct {
	OrganizationID uuid.UUID
	OpportunityID  *uuid.UUID
	ContractType   ContractType
	ValueMinor     *int64
	Currency       string
	StartAt        *time.Time
	EndAt          *time.Time
	PaymentTerms   string
	RenewalTerms   string
	GoverningDocs  json.RawMessage
	Signatories    json.RawMessage
}

// MilestoneInput is the payload for a billing milestone.
type MilestoneInput struct {
	Title              string
	DueAt              *time.Time
	AmountMinor        *int64
	Currency           string
	ProjectMilestoneID *uuid.UUID
}

// DeliverableInput is the payload for a deliverable.
type DeliverableInput struct {
	Description        string
	AcceptanceCriteria string
	MilestoneID        *uuid.UUID
}

// ObligationInput is the payload for an obligation.
type ObligationInput struct {
	Party         ObligationParty
	Description   string
	DueAt         *time.Time
	Recurrence    string
	OwnerPersonID *uuid.UUID
}

// SLATermInput is the payload for an SLA term.
type SLATermInput struct {
	Metric            string
	Target            string
	MeasurementWindow string
	SystemIDs         []uuid.UUID
}

// ChangeRequestInput is the payload for raising a change request.
type ChangeRequestInput struct {
	RequestedBy        string
	Description        string
	EffortImpact       string
	PriceImpactMinor   *int64
	Currency           string
	DeadlineImpactDays *int
}

// SignInput is the payload for recording a signature.
type SignInput struct {
	Name   string
	Role   string
	Method string
}

// ── contracts ───────────────────────────────────────────────────────────────

// CreateContract drafts a contract. It requires a known client company (§8.1).
func (s *Service) CreateContract(ctx context.Context, actor uuid.UUID, in ContractInput) (Contract, error) {
	if err := s.validateContract(in, false); err != nil {
		return Contract{}, err
	}
	exists, err := s.store.OrganizationExists(ctx, in.OrganizationID)
	if err != nil {
		return Contract{}, err
	}
	if !exists {
		return Contract{}, fmt.Errorf("%w: organization does not exist", ErrInvalid)
	}
	now := s.now()
	c := Contract{
		ID: uuid.Must(uuid.NewV7()), OrganizationID: in.OrganizationID, OpportunityID: in.OpportunityID,
		ContractType: in.ContractType, Status: StatusDraft, ValueMinor: in.ValueMinor,
		Currency: normalizeCurrency(in.Currency), StartAt: in.StartAt, EndAt: in.EndAt,
		PaymentTerms: trim(in.PaymentTerms), RenewalTerms: trim(in.RenewalTerms),
		GoverningDocs: docsOrEmpty(in.GoverningDocs), Signatories: json.RawMessage("[]"),
		Version: 1, CreatedAt: now, UpdatedAt: now,
	}
	created, err := s.store.CreateContract(ctx, c, func(ref string) (outbox.Event, error) {
		return outbox.NewContractCreated(&actor, outbox.ContractRef{
			ID: c.ID, Ref: ref, Status: string(StatusDraft), OrganizationID: c.OrganizationID,
			ContractType: string(c.ContractType), ValueMinor: c.ValueMinor, Currency: c.Currency,
		})
	})
	if err != nil {
		return Contract{}, err
	}
	s.writeAudit(ctx, actor, "contracts.contract.created", EntityContract, created.ID, nil, contractSnapshot(created))
	return created, nil
}

// GetContract returns a contract, or ErrNotFound.
func (s *Service) GetContract(ctx context.Context, id uuid.UUID) (Contract, error) {
	c, err := s.store.GetContract(ctx, id)
	if err != nil {
		return Contract{}, err
	}
	if c == nil {
		return Contract{}, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	return *c, nil
}

// ListContracts lists contracts, keyset-paged.
func (s *Service) ListContracts(ctx context.Context, status *ContractStatus, orgID *uuid.UUID, limit int, before *Cursor) ([]Contract, error) {
	return s.store.ListContracts(ctx, status, orgID, limit, before)
}

// UpdateContract replaces a draft or in-review contract. Once approved, the
// document is frozen; later changes go through a change request (§8.3).
func (s *Service) UpdateContract(ctx context.Context, actor, id uuid.UUID, expectedVersion int, in ContractInput) (Contract, error) {
	cur, err := s.GetContract(ctx, id)
	if err != nil {
		return Contract{}, err
	}
	if cur.Status != StatusDraft && cur.Status != StatusInternalReview {
		return Contract{}, fmt.Errorf("%w: a %s contract is changed with a change request", ErrConflict, cur.Status)
	}
	if err := s.validateContract(in, false); err != nil {
		return Contract{}, err
	}
	if cur.Status == StatusDraft {
		exists, err := s.store.OrganizationExists(ctx, in.OrganizationID)
		if err != nil {
			return Contract{}, err
		}
		if !exists {
			return Contract{}, fmt.Errorf("%w: organization does not exist", ErrInvalid)
		}
	}
	cur.OrganizationID = in.OrganizationID
	cur.OpportunityID = in.OpportunityID
	cur.ContractType = in.ContractType
	cur.ValueMinor = in.ValueMinor
	cur.Currency = normalizeCurrency(in.Currency)
	cur.StartAt = in.StartAt
	cur.EndAt = in.EndAt
	cur.PaymentTerms = trim(in.PaymentTerms)
	cur.RenewalTerms = trim(in.RenewalTerms)
	if len(in.GoverningDocs) > 0 {
		cur.GoverningDocs = in.GoverningDocs
	}
	updated, err := s.store.UpdateContract(ctx, cur, expectedVersion)
	if err != nil {
		return Contract{}, err
	}
	s.writeAudit(ctx, actor, "contracts.contract.updated", EntityContract, updated.ID, contractSnapshot(cur), contractSnapshot(updated))
	return updated, nil
}

// Submit moves a draft to internal review.
func (s *Service) Submit(ctx context.Context, actor, id uuid.UUID, expectedVersion int) (Contract, error) {
	return s.transition(ctx, actor, id, expectedVersion, StatusInternalReview, "contracts.contract.submitted")
}

// Approve moves a contract in internal review to approved.
func (s *Service) Approve(ctx context.Context, actor, id uuid.UUID, expectedVersion int) (Contract, error) {
	return s.transition(ctx, actor, id, expectedVersion, StatusApproved, "contracts.contract.approved")
}

// Send marks an approved contract as sent to the client.
func (s *Service) Send(ctx context.Context, actor, id uuid.UUID, expectedVersion int) (Contract, error) {
	return s.transition(ctx, actor, id, expectedVersion, StatusSent, "contracts.contract.sent")
}

// Activate moves a signed contract to active, unlocking delivery and billing.
func (s *Service) Activate(ctx context.Context, actor, id uuid.UUID, expectedVersion int) (Contract, error) {
	return s.transition(ctx, actor, id, expectedVersion, StatusActive, "contracts.contract.activated")
}

// Terminate ends a contract early.
func (s *Service) Terminate(ctx context.Context, actor, id uuid.UUID, expectedVersion int, reason string) (Contract, error) {
	cur, err := s.GetContract(ctx, id)
	if err != nil {
		return Contract{}, err
	}
	if !allowedContractTransition(cur.Status, StatusTerminated) {
		return Contract{}, fmt.Errorf("%w: cannot terminate a %s contract", ErrConflict, cur.Status)
	}
	ev, err := outbox.NewContractStatusChanged(&actor, id, cur.Ref, string(cur.Status), string(StatusTerminated), actor)
	if err != nil {
		return Contract{}, err
	}
	updated, err := s.store.SetContractStatus(ctx, id, expectedVersion, StatusTerminated, &ev)
	if err != nil {
		return Contract{}, err
	}
	s.writeAudit(ctx, actor, "contracts.contract.terminated", EntityContract, id,
		contractSnapshot(cur), map[string]any{"status": string(StatusTerminated), "reason": trim(reason)})
	return updated, nil
}

// Sign records a signature and moves the contract to signed.
func (s *Service) Sign(ctx context.Context, actor, id uuid.UUID, expectedVersion int, in SignInput) (Contract, error) {
	if trim(in.Name) == "" {
		return Contract{}, fmt.Errorf("%w: a signatory name is required", ErrInvalid)
	}
	cur, err := s.GetContract(ctx, id)
	if err != nil {
		return Contract{}, err
	}
	if cur.Status != StatusSent {
		return Contract{}, fmt.Errorf("%w: only a sent contract can be signed (it is %s)", ErrConflict, cur.Status)
	}
	signedAt := s.now()
	var existing []Signatory
	if len(cur.Signatories) > 0 {
		_ = json.Unmarshal(cur.Signatories, &existing)
	}
	existing = append(existing, Signatory{
		Name: trim(in.Name), Role: trim(in.Role), Method: trim(in.Method),
		SignedAt: signedAt.Format(time.RFC3339),
	})
	sigs, err := json.Marshal(existing)
	if err != nil {
		return Contract{}, fmt.Errorf("contracts: marshal signatories: %w", err)
	}
	ev, err := outbox.NewContractSigned(&actor, id, cur.Ref, trim(in.Name), signedAt)
	if err != nil {
		return Contract{}, err
	}
	updated, err := s.store.SignContract(ctx, id, expectedVersion, sigs, signedAt, &ev)
	if err != nil {
		return Contract{}, err
	}
	s.writeAudit(ctx, actor, "contracts.contract.signed", EntityContract, id,
		contractSnapshot(cur), map[string]any{"status": string(StatusSigned), "signed_by": trim(in.Name)})
	return updated, nil
}

// transition is the shared lifecycle move for the non-signing statuses.
func (s *Service) transition(ctx context.Context, actor, id uuid.UUID, expectedVersion int, to ContractStatus, action string) (Contract, error) {
	cur, err := s.GetContract(ctx, id)
	if err != nil {
		return Contract{}, err
	}
	if !allowedContractTransition(cur.Status, to) {
		return Contract{}, fmt.Errorf("%w: cannot move a %s contract to %s", ErrConflict, cur.Status, to)
	}
	ev, err := outbox.NewContractStatusChanged(&actor, id, cur.Ref, string(cur.Status), string(to), actor)
	if err != nil {
		return Contract{}, err
	}
	updated, err := s.store.SetContractStatus(ctx, id, expectedVersion, to, &ev)
	if err != nil {
		return Contract{}, err
	}
	s.writeAudit(ctx, actor, action, EntityContract, id, contractSnapshot(cur), contractSnapshot(updated))
	return updated, nil
}

func (s *Service) validateContract(in ContractInput, requireOrg bool) error {
	if !validContractType(in.ContractType) {
		return fmt.Errorf("%w: unknown contract type", ErrInvalid)
	}
	if requireOrg && in.OrganizationID == uuid.Nil {
		return fmt.Errorf("%w: an organization is required", ErrInvalid)
	}
	if err := validateCurrency(normalizeCurrency(in.Currency)); err != nil {
		return err
	}
	if err := validateAmountMinor(in.ValueMinor); err != nil {
		return err
	}
	if in.StartAt != nil && in.EndAt != nil && in.EndAt.Before(*in.StartAt) {
		return fmt.Errorf("%w: end date cannot precede the start date", ErrInvalid)
	}
	return nil
}

// ── milestones, deliverables, obligations, SLA terms ────────────────────────

// CreateMilestone adds a billing milestone to a contract.
func (s *Service) CreateMilestone(ctx context.Context, actor, contractID uuid.UUID, in MilestoneInput) (Milestone, error) {
	if _, err := s.GetContract(ctx, contractID); err != nil {
		return Milestone{}, err
	}
	if trim(in.Title) == "" {
		return Milestone{}, fmt.Errorf("%w: a milestone title is required", ErrInvalid)
	}
	if err := validateAmountMinor(in.AmountMinor); err != nil {
		return Milestone{}, err
	}
	now := s.now()
	m := Milestone{
		ID: uuid.Must(uuid.NewV7()), ContractID: contractID, Title: trim(in.Title), DueAt: in.DueAt,
		AmountMinor: in.AmountMinor, Currency: normalizeCurrency(in.Currency),
		ProjectMilestoneID: in.ProjectMilestoneID, Version: 1, CreatedAt: now, UpdatedAt: now,
	}
	created, err := s.store.CreateMilestone(ctx, m)
	if err != nil {
		return Milestone{}, err
	}
	s.writeAudit(ctx, actor, "contracts.milestone.created", EntityMilestone, created.ID, nil, milestoneSnapshot(created))
	return created, nil
}

// ListMilestones lists a contract's billing milestones.
func (s *Service) ListMilestones(ctx context.Context, contractID uuid.UUID) ([]Milestone, error) {
	return s.store.ListMilestones(ctx, contractID)
}

// CreateDeliverable adds a deliverable to a contract.
func (s *Service) CreateDeliverable(ctx context.Context, actor, contractID uuid.UUID, in DeliverableInput) (Deliverable, error) {
	if _, err := s.GetContract(ctx, contractID); err != nil {
		return Deliverable{}, err
	}
	if trim(in.Description) == "" {
		return Deliverable{}, fmt.Errorf("%w: a deliverable description is required", ErrInvalid)
	}
	if in.MilestoneID != nil {
		m, err := s.store.GetMilestone(ctx, *in.MilestoneID)
		if err != nil {
			return Deliverable{}, err
		}
		if m == nil || m.ContractID != contractID {
			return Deliverable{}, fmt.Errorf("%w: milestone does not belong to this contract", ErrInvalid)
		}
	}
	now := s.now()
	d := Deliverable{
		ID: uuid.Must(uuid.NewV7()), ContractID: contractID, Description: trim(in.Description),
		AcceptanceCriteria: trim(in.AcceptanceCriteria), MilestoneID: in.MilestoneID,
		Version: 1, CreatedAt: now, UpdatedAt: now,
	}
	created, err := s.store.CreateDeliverable(ctx, d)
	if err != nil {
		return Deliverable{}, err
	}
	s.writeAudit(ctx, actor, "contracts.deliverable.created", EntityDeliverable, created.ID, nil, deliverableSnapshot(created))
	return created, nil
}

// ListDeliverables lists a contract's deliverables.
func (s *Service) ListDeliverables(ctx context.Context, contractID uuid.UUID) ([]Deliverable, error) {
	return s.store.ListDeliverables(ctx, contractID)
}

// CreateObligation adds an obligation to a contract.
func (s *Service) CreateObligation(ctx context.Context, actor, contractID uuid.UUID, in ObligationInput) (Obligation, error) {
	if _, err := s.GetContract(ctx, contractID); err != nil {
		return Obligation{}, err
	}
	if in.Party != PartySafariStack && in.Party != PartyClient {
		return Obligation{}, fmt.Errorf("%w: party must be safaristack or client", ErrInvalid)
	}
	if trim(in.Description) == "" {
		return Obligation{}, fmt.Errorf("%w: an obligation description is required", ErrInvalid)
	}
	now := s.now()
	o := Obligation{
		ID: uuid.Must(uuid.NewV7()), ContractID: contractID, Party: in.Party,
		Description: trim(in.Description), DueAt: in.DueAt, Recurrence: trim(in.Recurrence),
		Status: ObligationOpen, OwnerPersonID: in.OwnerPersonID, Version: 1, CreatedAt: now, UpdatedAt: now,
	}
	created, err := s.store.CreateObligation(ctx, o)
	if err != nil {
		return Obligation{}, err
	}
	s.writeAudit(ctx, actor, "contracts.obligation.created", EntityObligation, created.ID, nil, obligationSnapshot(created))
	return created, nil
}

// ListObligations lists a contract's obligations.
func (s *Service) ListObligations(ctx context.Context, contractID uuid.UUID) ([]Obligation, error) {
	return s.store.ListObligations(ctx, contractID)
}

// CreateSLATerm adds a measurable service promise to a contract.
func (s *Service) CreateSLATerm(ctx context.Context, actor, contractID uuid.UUID, in SLATermInput) (SLATerm, error) {
	if _, err := s.GetContract(ctx, contractID); err != nil {
		return SLATerm{}, err
	}
	if trim(in.Metric) == "" || trim(in.Target) == "" {
		return SLATerm{}, fmt.Errorf("%w: a metric and target are required", ErrInvalid)
	}
	now := s.now()
	t := SLATerm{
		ID: uuid.Must(uuid.NewV7()), ContractID: contractID, Metric: trim(in.Metric),
		Target: trim(in.Target), MeasurementWindow: trim(in.MeasurementWindow),
		SystemIDs: in.SystemIDs, Version: 1, CreatedAt: now, UpdatedAt: now,
	}
	created, err := s.store.CreateSLATerm(ctx, t)
	if err != nil {
		return SLATerm{}, err
	}
	s.writeAudit(ctx, actor, "contracts.sla_term.created", EntitySLATerm, created.ID, nil, slaTermSnapshot(created))
	return created, nil
}

// ListSLATerms lists a contract's SLA terms.
func (s *Service) ListSLATerms(ctx context.Context, contractID uuid.UUID) ([]SLATerm, error) {
	return s.store.ListSLATerms(ctx, contractID)
}

// ── change requests ─────────────────────────────────────────────────────────

// CreateChangeRequest raises a change request against a contract.
func (s *Service) CreateChangeRequest(ctx context.Context, actor, contractID uuid.UUID, in ChangeRequestInput) (ChangeRequest, error) {
	contract, err := s.GetContract(ctx, contractID)
	if err != nil {
		return ChangeRequest{}, err
	}
	if contract.Status == StatusTerminated || contract.Status == StatusCompleted {
		return ChangeRequest{}, fmt.Errorf("%w: a %s contract cannot be changed", ErrConflict, contract.Status)
	}
	if trim(in.Description) == "" {
		return ChangeRequest{}, fmt.Errorf("%w: a change request description is required", ErrInvalid)
	}
	if err := validateAmountMinor(in.PriceImpactMinor); err != nil {
		return ChangeRequest{}, err
	}
	now := s.now()
	cr := ChangeRequest{
		ID: uuid.Must(uuid.NewV7()), ContractID: contractID, RequestedBy: trim(in.RequestedBy),
		Description: trim(in.Description), EffortImpact: trim(in.EffortImpact),
		PriceImpactMinor: in.PriceImpactMinor, Currency: normalizeCurrency(in.Currency),
		DeadlineImpactDays: in.DeadlineImpactDays, Status: CRSubmitted,
		Version: 1, CreatedAt: now, UpdatedAt: now,
	}
	created, err := s.store.CreateChangeRequest(ctx, cr, func(ref string) (outbox.Event, error) {
		return outbox.NewChangeRequestCreated(actor, outbox.ChangeRequestRef{
			ID: cr.ID, Ref: ref, ContractID: cr.ContractID, Status: string(CRSubmitted),
		})
	})
	if err != nil {
		return ChangeRequest{}, err
	}
	s.writeAudit(ctx, actor, "contracts.change_request.created", EntityChangeRequest, created.ID, nil, changeRequestSnapshot(created))
	return created, nil
}

// ListChangeRequests lists a contract's change requests.
func (s *Service) ListChangeRequests(ctx context.Context, contractID uuid.UUID) ([]ChangeRequest, error) {
	return s.store.ListChangeRequests(ctx, contractID)
}

// ApproveChangeRequest approves a submitted change request and appends a new
// contract version recording the change (§8.3).
func (s *Service) ApproveChangeRequest(ctx context.Context, actor, id uuid.UUID, expectedVersion int, approvalID *uuid.UUID) (ChangeRequest, error) {
	cur, err := s.getChangeRequest(ctx, id)
	if err != nil {
		return ChangeRequest{}, err
	}
	if cur.Status != CRSubmitted {
		return ChangeRequest{}, fmt.Errorf("%w: only a submitted change request can be approved (it is %s)", ErrConflict, cur.Status)
	}
	ev, err := outbox.NewChangeRequestApproved(actor, outbox.ChangeRequestRef{
		ID: cur.ID, Ref: cur.Ref, ContractID: cur.ContractID, Status: string(CRApproved),
	})
	if err != nil {
		return ChangeRequest{}, err
	}
	updated, err := s.store.SetChangeRequestStatus(ctx, id, expectedVersion, CRApproved, approvalID, &ev)
	if err != nil {
		return ChangeRequest{}, err
	}
	// A new contract version records the approved change. This is a second write
	// (its own transaction); a failure leaves the change approved but unversioned
	// and is logged for an operator to reconcile.
	if err := s.appendContractVersion(ctx, actor, cur); err != nil {
		s.logger.ErrorContext(ctx, "contracts: append version after change approval failed",
			slog.String("change_request_id", cur.ID.String()), slog.String("error", err.Error()))
	}
	s.writeAudit(ctx, actor, "contracts.change_request.approved", EntityChangeRequest, id,
		changeRequestSnapshot(cur), changeRequestSnapshot(updated))
	return updated, nil
}

// RejectChangeRequest rejects a submitted change request.
func (s *Service) RejectChangeRequest(ctx context.Context, actor, id uuid.UUID, expectedVersion int, reason string) (ChangeRequest, error) {
	cur, err := s.getChangeRequest(ctx, id)
	if err != nil {
		return ChangeRequest{}, err
	}
	if cur.Status != CRSubmitted {
		return ChangeRequest{}, fmt.Errorf("%w: only a submitted change request can be rejected (it is %s)", ErrConflict, cur.Status)
	}
	ev, err := outbox.NewChangeRequestRejected(actor, outbox.ChangeRequestRef{
		ID: cur.ID, Ref: cur.Ref, ContractID: cur.ContractID, Status: string(CRRejected),
	})
	if err != nil {
		return ChangeRequest{}, err
	}
	updated, err := s.store.SetChangeRequestStatus(ctx, id, expectedVersion, CRRejected, nil, &ev)
	if err != nil {
		return ChangeRequest{}, err
	}
	s.writeAudit(ctx, actor, "contracts.change_request.rejected", EntityChangeRequest, id,
		map[string]any{"status": string(cur.Status), "reason": trim(reason)},
		map[string]any{"status": string(CRRejected)})
	return updated, nil
}

func (s *Service) getChangeRequest(ctx context.Context, id uuid.UUID) (ChangeRequest, error) {
	cr, err := s.store.GetChangeRequest(ctx, id)
	if err != nil {
		return ChangeRequest{}, err
	}
	if cr == nil {
		return ChangeRequest{}, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	return *cr, nil
}

func (s *Service) appendContractVersion(ctx context.Context, actor uuid.UUID, cr ChangeRequest) error {
	no, err := s.store.NextContractVersionNo(ctx, cr.ContractID)
	if err != nil {
		return err
	}
	v := ContractVersion{
		ID: uuid.Must(uuid.NewV7()), ContractID: cr.ContractID, VersionNo: no,
		ChangeSummary: fmt.Sprintf("Change request %s: %s", cr.Ref, cr.Description),
		CreatedBy:     &actor, CreatedAt: s.now(),
	}
	return s.store.AddContractVersion(ctx, v, nil)
}

// ── audit & snapshots ───────────────────────────────────────────────────────

func (s *Service) writeAudit(ctx context.Context, actor uuid.UUID, action, entityType string, entityID uuid.UUID, before, after any) {
	if s.audit == nil {
		return
	}
	id := entityID
	if err := s.audit.Write(ctx, nil, audit.Event{
		ActorType: audit.ActorUser, ActorID: actor.String(), Action: action,
		EntityType: entityType, EntityID: &id, Before: before, After: after,
	}); err != nil {
		s.logger.ErrorContext(ctx, "contracts: audit write failed", slog.String("action", action), slog.String("error", err.Error()))
	}
}

func docsOrEmpty(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return json.RawMessage("[]")
	}
	return raw
}

func contractSnapshot(c Contract) map[string]any {
	return map[string]any{
		"ref": c.Ref, "status": string(c.Status), "contract_type": string(c.ContractType),
		"organization_id": c.OrganizationID.String(), "value_minor": c.ValueMinor, "currency": c.Currency,
	}
}

func milestoneSnapshot(m Milestone) map[string]any {
	return map[string]any{"contract_id": m.ContractID.String(), "title": m.Title, "amount_minor": m.AmountMinor, "currency": m.Currency}
}

func deliverableSnapshot(d Deliverable) map[string]any {
	return map[string]any{"contract_id": d.ContractID.String(), "description": d.Description}
}

func obligationSnapshot(o Obligation) map[string]any {
	return map[string]any{"contract_id": o.ContractID.String(), "party": string(o.Party), "description": o.Description, "status": string(o.Status)}
}

func slaTermSnapshot(t SLATerm) map[string]any {
	return map[string]any{"contract_id": t.ContractID.String(), "metric": t.Metric, "target": t.Target}
}

func changeRequestSnapshot(c ChangeRequest) map[string]any {
	return map[string]any{
		"ref": c.Ref, "contract_id": c.ContractID.String(), "status": string(c.Status),
		"price_impact_minor": c.PriceImpactMinor, "deadline_impact_days": c.DeadlineImpactDays,
	}
}
