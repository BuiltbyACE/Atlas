package units

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/safaristack/atlas/apps/api/internal/audit"
)

// Service is the units read model's rules: it validates the filters, resolves
// absence into ErrNotFound, and assembles the unit view. When the store also
// implements Writer, the same service carries the unit and integration
// lifecycle (§8.7).
type Service struct {
	store  Store
	writer Writer
	audit  *audit.Writer
	now    func() time.Time
	logger *slog.Logger
}

// NewService builds the units service over a store. A store that implements
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

func (s *Service) writeAudit(ctx context.Context, actor uuid.UUID, action, entityType string, entityID uuid.UUID, before, after any) {
	if s.audit == nil {
		return
	}
	id := entityID
	if err := s.audit.Write(ctx, nil, audit.Event{
		ActorType: audit.ActorUser, ActorID: actor.String(), Action: action,
		EntityType: entityType, EntityID: &id, Before: before, After: after,
	}); err != nil {
		s.logger.ErrorContext(ctx, "units: audit write failed",
			slog.String("action", action), slog.String("error", err.Error()))
	}
}

// asOf is the instant a read is dated from, truncated to the day.
func (s *Service) asOf() time.Time {
	n := s.now()
	return time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, time.UTC)
}

// Units lists business units.
func (s *Service) Units(ctx context.Context, q UnitQuery, limit int, before *Cursor) ([]BusinessUnit, error) {
	if err := validateUnitQuery(q); err != nil {
		return nil, err
	}
	return s.store.ListUnits(ctx, q, limit, before)
}

// Unit returns one unit, or ErrNotFound.
func (s *Service) Unit(ctx context.Context, id uuid.UUID) (BusinessUnit, error) {
	u, err := s.store.GetUnit(ctx, id)
	if err != nil {
		return BusinessUnit{}, err
	}
	if u == nil {
		return BusinessUnit{}, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	return *u, nil
}

// Integrations lists a unit's integrations.
func (s *Service) Integrations(ctx context.Context, q IntegrationQuery, limit int, before *Cursor) ([]UnitIntegration, error) {
	if err := validateIntegrationQuery(q); err != nil {
		return nil, err
	}
	return s.store.ListIntegrations(ctx, q, limit, before)
}

// Integration returns one integration, or ErrNotFound.
func (s *Service) Integration(ctx context.Context, id uuid.UUID) (UnitIntegration, error) {
	i, err := s.store.GetIntegration(ctx, id)
	if err != nil {
		return UnitIntegration{}, err
	}
	if i == nil {
		return UnitIntegration{}, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	return *i, nil
}

// IndicatorDefinitions lists a unit's indicator definitions.
func (s *Service) IndicatorDefinitions(ctx context.Context, q IndicatorDefinitionQuery, limit int, before *Cursor) ([]UnitIndicatorDefinition, error) {
	if err := validateIndicatorDefinitionQuery(q); err != nil {
		return nil, err
	}
	return s.store.ListIndicatorDefinitions(ctx, q, limit, before)
}

// IndicatorDefinition returns one indicator definition, or ErrNotFound.
func (s *Service) IndicatorDefinition(ctx context.Context, id uuid.UUID) (UnitIndicatorDefinition, error) {
	d, err := s.store.GetIndicatorDefinition(ctx, id)
	if err != nil {
		return UnitIndicatorDefinition{}, err
	}
	if d == nil {
		return UnitIndicatorDefinition{}, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	return *d, nil
}

// Snapshots lists indicator snapshots.
func (s *Service) Snapshots(ctx context.Context, q SnapshotQuery, limit int, before *Cursor) ([]UnitSnapshot, error) {
	if err := validateSnapshotQuery(q); err != nil {
		return nil, err
	}
	out, err := s.store.ListSnapshots(ctx, q, limit, before)
	if err != nil {
		return nil, err
	}
	sortSnapshots(out)
	return out, nil
}

// Snapshot returns one snapshot, or ErrNotFound.
func (s *Service) Snapshot(ctx context.Context, id uuid.UUID) (UnitSnapshot, error) {
	sp, err := s.store.GetSnapshot(ctx, id)
	if err != nil {
		return UnitSnapshot{}, err
	}
	if sp == nil {
		return UnitSnapshot{}, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	return *sp, nil
}

// FinanceSummaries lists finance summaries.
func (s *Service) FinanceSummaries(ctx context.Context, q FinanceSummaryQuery, limit int, before *Cursor) ([]UnitFinanceSummary, error) {
	if err := validateFinanceSummaryQuery(q); err != nil {
		return nil, err
	}
	return s.store.ListFinanceSummaries(ctx, q, limit, before)
}

// FinanceSummary returns one finance summary, or ErrNotFound.
func (s *Service) FinanceSummary(ctx context.Context, id uuid.UUID) (UnitFinanceSummary, error) {
	f, err := s.store.GetFinanceSummary(ctx, id)
	if err != nil {
		return UnitFinanceSummary{}, err
	}
	if f == nil {
		return UnitFinanceSummary{}, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	return *f, nil
}

// ── write lifecycle ─────────────────────────────────────────────────────────

// CreateUnit registers a business unit (§8.7). `units.unit.manage`.
func (s *Service) CreateUnit(ctx context.Context, actor uuid.UUID, in UnitInput) (BusinessUnit, error) {
	u := BusinessUnit{
		Name:          trim(in.Name),
		Code:          trim(in.Code),
		Kind:          UnitKind(trim(in.Kind)),
		OwnerPersonID: in.OwnerPersonID,
		Description:   trim(in.Description),
		Status:        UnitActive,
	}
	if err := validateUnitInput(u); err != nil {
		return BusinessUnit{}, err
	}
	created, err := s.writer.CreateUnit(ctx, u)
	if err != nil {
		return BusinessUnit{}, err
	}
	if s.audit != nil {
		s.writeAudit(ctx, actor, "units.unit.created", "units.business_unit", created.ID,
			nil, map[string]any{"ref": created.Ref, "name": created.Name, "code": created.Code})
	}
	return created, nil
}

// UpdateUnit edits a business unit (§8.7). `units.unit.manage`. Refused if
// the unit is retired or the version is stale.
func (s *Service) UpdateUnit(ctx context.Context, actor uuid.UUID, id uuid.UUID, expectedVersion int, in UnitInput) (BusinessUnit, error) {
	current, err := s.Unit(ctx, id)
	if err != nil {
		return BusinessUnit{}, err
	}
	u := BusinessUnit{
		ID:            id,
		Ref:           current.Ref,
		Code:          current.Code,
		Name:          trim(in.Name),
		Kind:          UnitKind(trim(in.Kind)),
		OwnerPersonID: in.OwnerPersonID,
		Description:   trim(in.Description),
		Status:        UnitStatus(trim(in.Status)),
		Version:       expectedVersion,
	}
	if err := validateUnitInput(u); err != nil {
		return BusinessUnit{}, err
	}
	updated, err := s.writer.UpdateUnit(ctx, u, expectedVersion)
	if err != nil {
		return BusinessUnit{}, err
	}
	if s.audit != nil {
		s.writeAudit(ctx, actor, "units.unit.updated", "units.business_unit", updated.ID,
			map[string]any{"ref": updated.Ref, "name": updated.Name}, nil)
	}
	return updated, nil
}

// CreateIntegration records a unit integration (§8.7). `units.unit.manage`.
func (s *Service) CreateIntegration(ctx context.Context, actor uuid.UUID, in IntegrationInput) (UnitIntegration, error) {
	i := UnitIntegration{
		UnitID:        in.UnitID,
		Name:          trim(in.Name),
		Kind:          IntegrationKind(trim(in.Kind)),
		Endpoint:      trim(in.Endpoint),
		CredentialRef: trim(in.CredentialRef),
		Schedule:      trim(in.Schedule),
		Active:        true,
	}
	if err := validateIntegrationInput(i); err != nil {
		return UnitIntegration{}, err
	}
	created, err := s.writer.CreateIntegration(ctx, i)
	if err != nil {
		return UnitIntegration{}, err
	}
	if s.audit != nil {
		s.writeAudit(ctx, actor, "units.integration.created", "units.unit_integration", created.ID,
			nil, map[string]any{"unit_id": created.UnitID, "name": created.Name, "kind": created.Kind})
	}
	return created, nil
}

// UpdateIntegration edits a unit integration (§8.7). `units.unit.manage`.
func (s *Service) UpdateIntegration(ctx context.Context, actor uuid.UUID, id uuid.UUID, expectedVersion int, in IntegrationInput) (UnitIntegration, error) {
	current, err := s.Integration(ctx, id)
	if err != nil {
		return UnitIntegration{}, err
	}
	i := UnitIntegration{
		ID:            id,
		UnitID:        current.UnitID,
		Name:          trim(in.Name),
		Kind:          IntegrationKind(trim(in.Kind)),
		Endpoint:      trim(in.Endpoint),
		CredentialRef: trim(in.CredentialRef),
		Schedule:      trim(in.Schedule),
		Active:        in.Active,
		Version:       expectedVersion,
	}
	if err := validateIntegrationInput(i); err != nil {
		return UnitIntegration{}, err
	}
	updated, err := s.writer.UpdateIntegration(ctx, i, expectedVersion)
	if err != nil {
		return UnitIntegration{}, err
	}
	if s.audit != nil {
		s.writeAudit(ctx, actor, "units.integration.updated", "units.unit_integration", updated.ID,
			map[string]any{"unit_id": updated.UnitID, "name": updated.Name}, nil)
	}
	return updated, nil
}

// CreateIndicatorDefinition records a unit indicator definition (§8.7).
// `units.unit.manage`.
func (s *Service) CreateIndicatorDefinition(ctx context.Context, actor uuid.UUID, in IndicatorDefinitionInput) (UnitIndicatorDefinition, error) {
	d := UnitIndicatorDefinition{
		UnitID:            in.UnitID,
		Name:              trim(in.Name),
		Kind:              IndicatorKind(trim(in.Kind)),
		SourceField:       trim(in.SourceField),
		Formula:           trim(in.Formula),
		Target:            trim(in.Target),
		WarningThreshold:  trim(in.WarningThreshold),
		CriticalThreshold: trim(in.CriticalThreshold),
		Active:            true,
	}
	if err := validateIndicatorDefinitionInput(d); err != nil {
		return UnitIndicatorDefinition{}, err
	}
	created, err := s.writer.CreateIndicatorDefinition(ctx, d)
	if err != nil {
		return UnitIndicatorDefinition{}, err
	}
	if s.audit != nil {
		s.writeAudit(ctx, actor, "units.indicator_definition.created", "units.unit_indicator_definition", created.ID,
			nil, map[string]any{"unit_id": created.UnitID, "name": created.Name, "kind": created.Kind})
	}
	return created, nil
}

// UpdateIndicatorDefinition edits a unit indicator definition (§8.7).
// `units.unit.manage`.
func (s *Service) UpdateIndicatorDefinition(ctx context.Context, actor uuid.UUID, id uuid.UUID, expectedVersion int, in IndicatorDefinitionInput) (UnitIndicatorDefinition, error) {
	current, err := s.IndicatorDefinition(ctx, id)
	if err != nil {
		return UnitIndicatorDefinition{}, err
	}
	d := UnitIndicatorDefinition{
		ID:                id,
		UnitID:            current.UnitID,
		Name:              trim(in.Name),
		Kind:              IndicatorKind(trim(in.Kind)),
		SourceField:       trim(in.SourceField),
		Formula:           trim(in.Formula),
		Target:            trim(in.Target),
		WarningThreshold:  trim(in.WarningThreshold),
		CriticalThreshold: trim(in.CriticalThreshold),
		Active:            in.Active,
		Version:           expectedVersion,
	}
	if err := validateIndicatorDefinitionInput(d); err != nil {
		return UnitIndicatorDefinition{}, err
	}
	updated, err := s.writer.UpdateIndicatorDefinition(ctx, d, expectedVersion)
	if err != nil {
		return UnitIndicatorDefinition{}, err
	}
	if s.audit != nil {
		s.writeAudit(ctx, actor, "units.indicator_definition.updated", "units.unit_indicator_definition", updated.ID,
			map[string]any{"unit_id": updated.UnitID, "name": updated.Name}, nil)
	}
	return updated, nil
}

// ── write input types ───────────────────────────────────────────────────────

// UnitInput is the create/update payload for a business unit.
type UnitInput struct {
	Name          string
	Code          string
	Kind          string
	OwnerPersonID *uuid.UUID
	Description   string
	Status        string
}

// IntegrationInput is the create/update payload for a unit integration.
type IntegrationInput struct {
	UnitID        uuid.UUID
	Name          string
	Kind          string
	Endpoint      string
	CredentialRef string
	Schedule      string
	Active        bool
}

// IndicatorDefinitionInput is the create/update payload for an indicator definition.
type IndicatorDefinitionInput struct {
	UnitID            uuid.UUID
	Name              string
	Kind              string
	SourceField       string
	Formula           string
	Target            string
	WarningThreshold  string
	CriticalThreshold string
	Active            bool
}

// ── write input validation ─────────────────────────────────────────────────

func validateUnitInput(u BusinessUnit) error {
	if trim(u.Name) == "" {
		return errInvalidField("name")
	}
	if !validUnitCode(u.Code) {
		return errInvalidField("code")
	}
	if !validUnitKind(u.Kind) {
		return errInvalidField("kind")
	}
	return nil
}

func validateIntegrationInput(i UnitIntegration) error {
	if trim(i.Name) == "" {
		return errInvalidField("name")
	}
	if !validIntegrationKind(i.Kind) {
		return errInvalidField("kind")
	}
	if !validEndpoint(i.Endpoint) {
		return errInvalidField("endpoint")
	}
	return nil
}

func validateIndicatorDefinitionInput(d UnitIndicatorDefinition) error {
	if trim(d.Name) == "" {
		return errInvalidField("name")
	}
	if !validIndicatorKind(d.Kind) {
		return errInvalidField("kind")
	}
	return nil
}

// ── filter validation ───────────────────────────────────────────────────────

func validateUnitQuery(q UnitQuery) error {
	if q.Kind != nil && !validUnitKind(*q.Kind) {
		return fmt.Errorf("%w: unknown unit kind", ErrInvalid)
	}
	if q.Status != nil && !validUnitStatus(*q.Status) {
		return fmt.Errorf("%w: unknown unit status", ErrInvalid)
	}
	return nil
}

func validateIntegrationQuery(q IntegrationQuery) error {
	if q.Kind != nil && !validIntegrationKind(*q.Kind) {
		return fmt.Errorf("%w: unknown integration kind", ErrInvalid)
	}
	if q.Status != nil && !validIntegrationStatus(*q.Status) {
		return fmt.Errorf("%w: unknown integration status", ErrInvalid)
	}
	return nil
}

func validateIndicatorDefinitionQuery(q IndicatorDefinitionQuery) error {
	if q.Kind != nil && !validIndicatorKind(*q.Kind) {
		return fmt.Errorf("%w: unknown indicator kind", ErrInvalid)
	}
	return nil
}

func validateSnapshotQuery(q SnapshotQuery) error {
	if q.From != nil && q.To != nil && q.To.Before(*q.From) {
		return fmt.Errorf("%w: from must be before to", ErrInvalid)
	}
	return nil
}

func validateFinanceSummaryQuery(q FinanceSummaryQuery) error {
	if q.From != nil && q.To != nil && q.To.Before(*q.From) {
		return fmt.Errorf("%w: from must be before to", ErrInvalid)
	}
	return nil
}
