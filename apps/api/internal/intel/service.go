package intel

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/safaristack/atlas/apps/api/internal/audit"
)

// Service is the intel read model's rules: it validates the filters, resolves
// absence into ErrNotFound. When the store also implements Writer, the same
// service carries the decision/risk/brief lifecycle (§8.8).
type Service struct {
	store  Store
	writer Writer
	audit  *audit.Writer
	now    func() time.Time
	logger *slog.Logger
}

// NewService builds the intel service over a store. A store that implements
// Writer enables the write lifecycle; the read slice still works without one.
func NewService(store Store, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	s := &Service{
		store:  store,
		now:    func() time.Time { return time.Now().UTC() },
		logger: logger,
	}
	if w, ok := store.(Writer); ok {
		s.writer = w
	}
	return s
}

// SetAudit attaches the audit writer that records who changed what (§6.6).
func (s *Service) SetAudit(aw *audit.Writer) { s.audit = aw }

// Decisions lists decisions.
func (s *Service) Decisions(ctx context.Context, q DecisionQuery, limit int, before *Cursor) ([]Decision, error) {
	if err := validateDecisionQuery(q); err != nil {
		return nil, err
	}
	out, err := s.store.ListDecisions(ctx, q, limit, before)
	if err != nil {
		return nil, err
	}
	sortDecisions(out)
	return out, nil
}

// Decision returns one decision, or ErrNotFound.
func (s *Service) Decision(ctx context.Context, id uuid.UUID) (Decision, error) {
	d, err := s.store.GetDecision(ctx, id)
	if err != nil {
		return Decision{}, err
	}
	if d == nil {
		return Decision{}, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	return *d, nil
}

// Risks lists risks.
func (s *Service) Risks(ctx context.Context, q RiskQuery, limit int, before *Cursor) ([]Risk, error) {
	if err := validateRiskQuery(q); err != nil {
		return nil, err
	}
	out, err := s.store.ListRisks(ctx, q, limit, before)
	if err != nil {
		return nil, err
	}
	sortRisks(out)
	return out, nil
}

// Risk returns one risk, or ErrNotFound.
func (s *Service) Risk(ctx context.Context, id uuid.UUID) (Risk, error) {
	r, err := s.store.GetRisk(ctx, id)
	if err != nil {
		return Risk{}, err
	}
	if r == nil {
		return Risk{}, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	return *r, nil
}

// Briefs lists briefs.
func (s *Service) Briefs(ctx context.Context, q BriefQuery, limit int, before *Cursor) ([]Brief, error) {
	if err := validateBriefQuery(q); err != nil {
		return nil, err
	}
	return s.store.ListBriefs(ctx, q, limit, before)
}

// Brief returns one brief, or ErrNotFound.
func (s *Service) Brief(ctx context.Context, id uuid.UUID) (Brief, error) {
	b, err := s.store.GetBrief(ctx, id)
	if err != nil {
		return Brief{}, err
	}
	if b == nil {
		return Brief{}, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	return *b, nil
}

// MetricSnapshots lists metric snapshots.
func (s *Service) MetricSnapshots(ctx context.Context, q MetricSnapshotQuery, limit int, before *Cursor) ([]MetricSnapshot, error) {
	if err := validateMetricSnapshotQuery(q); err != nil {
		return nil, err
	}
	return s.store.ListMetricSnapshots(ctx, q, limit, before)
}

// MetricSnapshot returns one metric snapshot, or ErrNotFound.
func (s *Service) MetricSnapshot(ctx context.Context, id uuid.UUID) (MetricSnapshot, error) {
	m, err := s.store.GetMetricSnapshot(ctx, id)
	if err != nil {
		return MetricSnapshot{}, err
	}
	if m == nil {
		return MetricSnapshot{}, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	return *m, nil
}

// ── write lifecycle ─────────────────────────────────────────────────────────

// CreateDecision records a major decision (§8.8). `intel.decision.manage`.
func (s *Service) CreateDecision(ctx context.Context, actor uuid.UUID, in DecisionInput) (Decision, error) {
	d := Decision{
		Question:        trim(in.Question),
		ContextLinks:    in.ContextLinks,
		Options:         in.Options,
		Recommendation:  trim(in.Recommendation),
		RequestedBy:     in.RequestedBy,
		RequestedByName: trim(in.RequestedByName),
		Deciders:        in.Deciders,
		DueOn:           in.DueOn,
		Outcome:         DecisionOpen,
	}
	if err := validateDecisionInput(d); err != nil {
		return Decision{}, err
	}
	created, err := s.writer.CreateDecision(ctx, d)
	if err != nil {
		return Decision{}, err
	}
	s.writeAudit(ctx, actor, "intel.decision.created", "intel.decision", created.ID,
		nil, map[string]any{"ref": created.Ref, "question": created.Question, "outcome": created.Outcome})
	return created, nil
}

// UpdateDecision edits an open decision (§8.8). `intel.decision.manage`.
// Refused once decided.
func (s *Service) UpdateDecision(ctx context.Context, actor uuid.UUID, id uuid.UUID, expectedVersion int, in DecisionInput) (Decision, error) {
	current, err := s.Decision(ctx, id)
	if err != nil {
		return Decision{}, err
	}
	if !DecisionEditable(current.Outcome) {
		return Decision{}, withCode(ErrForbidden, CodeDecisionAlreadyDecided)
	}
	d := Decision{
		ID:             id,
		Ref:            current.Ref,
		Question:       trim(in.Question),
		ContextLinks:   in.ContextLinks,
		Options:        in.Options,
		Recommendation: trim(in.Recommendation),
		RequestedBy:    in.RequestedBy,
		Deciders:       in.Deciders,
		DueOn:          in.DueOn,
		Outcome:        DecisionOutcome(trim(in.Outcome)),
		Rationale:      trim(in.Rationale),
		Version:        expectedVersion,
	}
	if err := validateDecisionInput(d); err != nil {
		return Decision{}, err
	}
	updated, err := s.writer.UpdateDecision(ctx, d, expectedVersion)
	if err != nil {
		return Decision{}, err
	}
	s.writeAudit(ctx, actor, "intel.decision.updated", "intel.decision", updated.ID,
		map[string]any{"outcome": current.Outcome}, map[string]any{"outcome": updated.Outcome})
	return updated, nil
}

// DecideDecision records the outcome of a decision (§8.8). `intel.decision.manage`.
func (s *Service) DecideDecision(ctx context.Context, actor uuid.UUID, id uuid.UUID, expectedVersion int, outcome DecisionOutcome, rationale string) (Decision, error) {
	current, err := s.Decision(ctx, id)
	if err != nil {
		return Decision{}, err
	}
	if !DecisionEditable(current.Outcome) {
		return Decision{}, withCode(ErrForbidden, CodeDecisionAlreadyDecided)
	}
	if !validDecisionOutcome(outcome) {
		return Decision{}, withCode(errInvalidField("outcome"), CodeInvalidDecisionOutcome)
	}
	d := Decision{
		ID:        id,
		Ref:       current.Ref,
		Outcome:   outcome,
		Rationale: trim(rationale),
		Version:   expectedVersion,
	}
	updated, err := s.writer.DecideDecision(ctx, d, expectedVersion)
	if err != nil {
		return Decision{}, err
	}
	s.writeAudit(ctx, actor, "intel.decision.decided", "intel.decision", updated.ID,
		map[string]any{"outcome": current.Outcome}, map[string]any{"outcome": updated.Outcome, "rationale": updated.Rationale})
	return updated, nil
}

// CreateRisk records a risk (§8.8). `intel.risk.manage`.
func (s *Service) CreateRisk(ctx context.Context, actor uuid.UUID, in RiskInput) (Risk, error) {
	r := Risk{
		Title:         trim(in.Title),
		Kind:          RiskKind(trim(in.Kind)),
		OwnerPersonID: in.OwnerPersonID,
		Likelihood:    RiskLikelihood(trim(in.Likelihood)),
		Impact:        RiskImpact(trim(in.Impact)),
		Mitigation:    trim(in.Mitigation),
		Status:        RiskOpen,
	}
	if err := validateRiskInput(r); err != nil {
		return Risk{}, err
	}
	created, err := s.writer.CreateRisk(ctx, r)
	if err != nil {
		return Risk{}, err
	}
	s.writeAudit(ctx, actor, "intel.risk.created", "intel.risk", created.ID,
		nil, map[string]any{"ref": created.Ref, "title": created.Title, "kind": created.Kind})
	return created, nil
}

// UpdateRisk edits an open or monitoring risk (§8.8). `intel.risk.manage`.
func (s *Service) UpdateRisk(ctx context.Context, actor uuid.UUID, id uuid.UUID, expectedVersion int, in RiskInput) (Risk, error) {
	current, err := s.Risk(ctx, id)
	if err != nil {
		return Risk{}, err
	}
	if !RiskEditable(current.Status) {
		return Risk{}, withCode(ErrForbidden, CodeRiskNotEditable)
	}
	r := Risk{
		ID:            id,
		Ref:           current.Ref,
		Title:         trim(in.Title),
		Kind:          RiskKind(trim(in.Kind)),
		OwnerPersonID: in.OwnerPersonID,
		Likelihood:    RiskLikelihood(trim(in.Likelihood)),
		Impact:        RiskImpact(trim(in.Impact)),
		Mitigation:    trim(in.Mitigation),
		Status:        RiskStatus(trim(in.Status)),
		Version:       expectedVersion,
	}
	if err := validateRiskInput(r); err != nil {
		return Risk{}, err
	}
	updated, err := s.writer.UpdateRisk(ctx, r, expectedVersion)
	if err != nil {
		return Risk{}, err
	}
	s.writeAudit(ctx, actor, "intel.risk.updated", "intel.risk", updated.ID,
		map[string]any{"status": current.Status}, map[string]any{"status": updated.Status})
	return updated, nil
}

// CreateBrief records a daily executive brief (§8.8). `intel.brief.manage`.
func (s *Service) CreateBrief(ctx context.Context, actor uuid.UUID, in BriefInput) (Brief, error) {
	b := Brief{
		ForDate:   in.ForDate,
		Summary:   trim(in.Summary),
		Sections:  in.Sections,
		Delivered: false,
	}
	if err := validateBriefInput(b); err != nil {
		return Brief{}, err
	}
	created, err := s.writer.CreateBrief(ctx, b)
	if err != nil {
		return Brief{}, err
	}
	s.writeAudit(ctx, actor, "intel.brief.created", "intel.brief", created.ID,
		nil, map[string]any{"for_date": created.ForDate, "delivered": created.Delivered})
	return created, nil
}

// UpdateBrief edits a brief (§8.8). `intel.brief.manage`.
func (s *Service) UpdateBrief(ctx context.Context, actor uuid.UUID, id uuid.UUID, expectedVersion int, in BriefInput) (Brief, error) {
	current, err := s.Brief(ctx, id)
	if err != nil {
		return Brief{}, err
	}
	b := Brief{
		ID:        id,
		Ref:       current.Ref,
		ForDate:   in.ForDate,
		Summary:   trim(in.Summary),
		Sections:  in.Sections,
		Delivered: current.Delivered,
		Version:   expectedVersion,
	}
	if err := validateBriefInput(b); err != nil {
		return Brief{}, err
	}
	updated, err := s.writer.UpdateBrief(ctx, b, expectedVersion)
	if err != nil {
		return Brief{}, err
	}
	s.writeAudit(ctx, actor, "intel.brief.updated", "intel.brief", updated.ID,
		map[string]any{"delivered": current.Delivered}, map[string]any{"delivered": updated.Delivered})
	return updated, nil
}

// DeliverBrief marks a brief as delivered (§8.8). `intel.brief.manage`.
func (s *Service) DeliverBrief(ctx context.Context, actor uuid.UUID, id uuid.UUID, expectedVersion int) (Brief, error) {
	current, err := s.Brief(ctx, id)
	if err != nil {
		return Brief{}, err
	}
	if current.Delivered {
		return Brief{}, withCode(ErrConflict, CodeBriefAlreadyDelivered)
	}
	b := Brief{
		ID:          id,
		Ref:         current.Ref,
		ForDate:     current.ForDate,
		Summary:     current.Summary,
		Sections:    current.Sections,
		Delivered:   true,
		DeliveredAt: &time.Time{},
		Version:     expectedVersion,
	}
	now := s.now()
	b.DeliveredAt = &now
	updated, err := s.writer.DeliverBrief(ctx, b, expectedVersion)
	if err != nil {
		return Brief{}, err
	}
	s.writeAudit(ctx, actor, "intel.brief.delivered", "intel.brief", updated.ID,
		map[string]any{"delivered": false}, map[string]any{"delivered": true, "delivered_at": updated.DeliveredAt})
	return updated, nil
}

// CreateMetricSnapshot records a metric snapshot (§8.8). `intel.metric.manage`.
func (s *Service) CreateMetricSnapshot(ctx context.Context, actor uuid.UUID, in MetricSnapshotInput) (MetricSnapshot, error) {
	m := MetricSnapshot{
		MetricName:  trim(in.MetricName),
		MetricValue: trim(in.MetricValue),
		AsOf:        in.AsOf,
		Source:      trim(in.Source),
	}
	if err := validateMetricSnapshotInput(m); err != nil {
		return MetricSnapshot{}, err
	}
	created, err := s.writer.CreateMetricSnapshot(ctx, m)
	if err != nil {
		return MetricSnapshot{}, err
	}
	s.writeAudit(ctx, actor, "intel.metric.created", "intel.metric_snapshot", created.ID,
		nil, map[string]any{"metric_name": created.MetricName, "as_of": created.AsOf})
	return created, nil
}

// ── write input types ───────────────────────────────────────────────────────

// DecisionInput is the create/update payload for a decision.
type DecisionInput struct {
	Question        string
	ContextLinks    []string
	Options         []string
	Recommendation  string
	RequestedBy     *uuid.UUID
	RequestedByName string
	Deciders        []uuid.UUID
	DueOn           *time.Time
	Outcome         string
	Rationale       string
}

// RiskInput is the create/update payload for a risk.
type RiskInput struct {
	Title         string
	Kind          string
	OwnerPersonID *uuid.UUID
	Likelihood    string
	Impact        string
	Mitigation    string
	Status        string
}

// BriefInput is the create/update payload for a brief.
type BriefInput struct {
	ForDate  time.Time
	Summary  string
	Sections map[string]any
}

// MetricSnapshotInput is the create/update payload for a metric snapshot.
type MetricSnapshotInput struct {
	MetricName  string
	MetricValue string
	AsOf        time.Time
	Source      string
}

// ── write input validation ─────────────────────────────────────────────────

func validateDecisionInput(d Decision) error {
	if trim(d.Question) == "" {
		return errInvalidField("question")
	}
	if !validDecisionOutcome(d.Outcome) {
		return errInvalidField("outcome")
	}
	return nil
}

func validateRiskInput(r Risk) error {
	if trim(r.Title) == "" {
		return errInvalidField("title")
	}
	if !validRiskKind(r.Kind) {
		return errInvalidField("kind")
	}
	if !validRiskLikelihood(r.Likelihood) {
		return errInvalidField("likelihood")
	}
	if !validRiskImpact(r.Impact) {
		return errInvalidField("impact")
	}
	return nil
}

func validateBriefInput(b Brief) error {
	if trim(b.Summary) == "" {
		return errInvalidField("summary")
	}
	return nil
}

func validateMetricSnapshotInput(m MetricSnapshot) error {
	if trim(m.MetricName) == "" {
		return errInvalidField("metric_name")
	}
	return nil
}

// ── filter validation ───────────────────────────────────────────────────────

func validateDecisionQuery(q DecisionQuery) error {
	if q.Outcome != nil && !validDecisionOutcome(*q.Outcome) {
		return fmt.Errorf("%w: unknown decision outcome", ErrInvalid)
	}
	return nil
}

func validateRiskQuery(q RiskQuery) error {
	if q.Kind != nil && !validRiskKind(*q.Kind) {
		return fmt.Errorf("%w: unknown risk kind", ErrInvalid)
	}
	if q.Status != nil && !validRiskStatus(*q.Status) {
		return fmt.Errorf("%w: unknown risk status", ErrInvalid)
	}
	return nil
}

func validateBriefQuery(q BriefQuery) error {
	if q.From != nil && q.To != nil && q.To.Before(*q.From) {
		return fmt.Errorf("%w: from must be before to", ErrInvalid)
	}
	return nil
}

func validateMetricSnapshotQuery(q MetricSnapshotQuery) error {
	if q.From != nil && q.To != nil && q.To.Before(*q.From) {
		return fmt.Errorf("%w: from must be before to", ErrInvalid)
	}
	return nil
}

// writeAudit records who changed what (§6.6).
func (s *Service) writeAudit(ctx context.Context, actor uuid.UUID, action, entityType string, entityID uuid.UUID, before, after any) {
	if s.audit == nil {
		return
	}
	id := entityID
	if err := s.audit.Write(ctx, nil, audit.Event{
		ActorType: audit.ActorUser, ActorID: actor.String(), Action: action,
		EntityType: entityType, EntityID: &id, Before: before, After: after,
	}); err != nil {
		s.logger.ErrorContext(ctx, "intel: audit write failed",
			slog.String("action", action), slog.String("error", err.Error()))
	}
}
