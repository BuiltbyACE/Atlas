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
	return s
}

// SetAudit attaches the audit writer that records who changed what (§6.6).
func (s *Service) SetAudit(aw *audit.Writer) { s.audit = aw }

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
