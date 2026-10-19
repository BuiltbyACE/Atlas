package systems

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
)

// Service is the systems read model's rules: it validates the filters, resolves
// absence into ErrNotFound, and assembles the overview from the parts the store
// measures separately.
type Service struct {
	store  Store
	now    func() time.Time
	logger *slog.Logger
}

// NewService builds the systems service over a store.
func NewService(store Store, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{
		store:  store,
		now:    func() time.Time { return time.Now().UTC() },
		logger: logger,
	}
}

// asOf is the instant a read is dated from, truncated to the day so warning
// bands do not shift with the time of day.
func (s *Service) asOf() time.Time {
	n := s.now()
	return time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, time.UTC)
}

// Overview is the systems headline (§8.6 metrics). It is composed from separate
// measurements rather than one wide query, so each figure stands on its own.
func (s *Service) Overview(ctx context.Context) (Overview, error) {
	asOf := s.asOf()
	totals, err := s.store.RegisterTotals(ctx)
	if err != nil {
		return Overview{}, err
	}
	metrics, err := s.store.IncidentMetrics(ctx, asOf)
	if err != nil {
		return Overview{}, err
	}
	bySeverity, err := s.store.IncidentSeverityCounts(ctx)
	if err != nil {
		return Overview{}, err
	}
	availability, err := s.store.Availability(ctx, asOf.Add(-AvailabilityWindow))
	if err != nil {
		return Overview{}, err
	}
	expiring, err := s.store.Expiring(ctx, asOf, ExpiryWarningDays[0])
	if err != nil {
		return Overview{}, err
	}
	rotations := 0
	for _, e := range expiring {
		if e.Kind == ExpiryCredential {
			rotations++
		}
	}

	return Overview{
		AsOf:                asOf,
		SystemsTotal:        totals.SystemsTotal,
		SystemsTier1:        totals.SystemsTier1,
		SystemsTier2:        totals.SystemsTier2,
		SystemsTier3:        totals.SystemsTier3,
		ChecksTotal:         totals.ChecksTotal,
		ChecksFailing:       totals.ChecksFailing,
		ChecksWarning:       totals.ChecksWarning,
		ChecksUnprobed:      totals.ChecksUnprobed,
		OpenIncidents:       metrics.OpenIncidents,
		OpenSev1:            metrics.OpenSev1,
		OpenSev2:            metrics.OpenSev2,
		OpenIncidentsBySev:  bySeverity,
		Incidents30D:        metrics.Incidents30D,
		DetectedByAtlas30D:  metrics.DetectedByAtlas30D,
		DetectedByClient30D: metrics.DetectedByClient30D,
		MTTASeconds:         metrics.MTTASeconds,
		MTTRSeconds:         metrics.MTTRSeconds,
		ReviewsOutstanding:  metrics.ReviewsOutstanding,
		ExpiringCount:       len(expiring),
		RotationsDueCount:   rotations,
		Availability:        availability,
	}, nil
}

// Systems lists the register.
func (s *Service) Systems(ctx context.Context, q SystemQuery, limit int, before *Cursor) ([]System, error) {
	if err := validateSystemQuery(q); err != nil {
		return nil, err
	}
	return s.store.ListSystems(ctx, q, limit, before)
}

// System returns one system, or ErrNotFound.
func (s *Service) System(ctx context.Context, id uuid.UUID) (System, error) {
	sys, err := s.store.GetSystem(ctx, id)
	if err != nil {
		return System{}, err
	}
	if sys == nil {
		return System{}, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	return *sys, nil
}

// Credentials returns a system's credential records — never a secret (§8.6).
func (s *Service) Credentials(ctx context.Context, systemID uuid.UUID) ([]CredentialRecord, error) {
	if _, err := s.System(ctx, systemID); err != nil {
		return nil, err
	}
	return s.store.ListCredentials(ctx, systemID)
}

// Checks lists the probes, each with its latest result.
func (s *Service) Checks(ctx context.Context, q CheckQuery, limit int, before *Cursor) ([]Check, error) {
	if err := validateCheckQuery(q); err != nil {
		return nil, err
	}
	return s.store.ListChecks(ctx, q, limit, before)
}

// CheckResults lists one check's recent results. The check must exist, so a
// typo is a 404 rather than an empty page that looks like health.
func (s *Service) CheckResults(ctx context.Context, checkID uuid.UUID, limit int, before *Cursor) ([]CheckResult, error) {
	c, err := s.store.GetCheck(ctx, checkID)
	if err != nil {
		return nil, err
	}
	if c == nil {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, checkID)
	}
	return s.store.ListCheckResults(ctx, checkID, limit, before)
}

// Incidents lists incidents newest first.
func (s *Service) Incidents(ctx context.Context, q IncidentQuery, limit int, before *Cursor) ([]Incident, error) {
	if err := validateIncidentQuery(q); err != nil {
		return nil, err
	}
	return s.store.ListIncidents(ctx, q, limit, before)
}

// Incident returns one incident with its systems, timeline and follow-up
// actions — the whole record, because that is what an incident is.
func (s *Service) Incident(ctx context.Context, id uuid.UUID) (Incident, error) {
	in, err := s.store.GetIncident(ctx, id)
	if err != nil {
		return Incident{}, err
	}
	if in == nil {
		return Incident{}, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	if in.Systems, err = s.store.IncidentSystems(ctx, id); err != nil {
		return Incident{}, err
	}
	if in.Timeline, err = s.store.IncidentTimeline(ctx, id); err != nil {
		return Incident{}, err
	}
	if in.Actions, err = s.store.IncidentActions(ctx, id); err != nil {
		return Incident{}, err
	}
	return *in, nil
}

// Expiring lists domains, certificates and credential rotations lapsing within
// withinDays, soonest first.
func (s *Service) Expiring(ctx context.Context, withinDays int) ([]Expiry, error) {
	if withinDays < 1 || withinDays > 365 {
		return nil, errInvalidField("within_days")
	}
	return s.store.Expiring(ctx, s.asOf(), withinDays)
}

// ── filter validation ───────────────────────────────────────────────────────

func validateSystemQuery(q SystemQuery) error {
	if q.Type != nil && !validSystemType(*q.Type) {
		return fmt.Errorf("%w: unknown system type", ErrInvalid)
	}
	if q.Environment != nil && !validEnvironment(*q.Environment) {
		return fmt.Errorf("%w: unknown environment", ErrInvalid)
	}
	if q.Criticality != nil && !validCriticality(*q.Criticality) {
		return fmt.Errorf("%w: unknown criticality", ErrInvalid)
	}
	if q.Status != nil && !validSystemStatus(*q.Status) {
		return fmt.Errorf("%w: unknown system status", ErrInvalid)
	}
	return nil
}

func validateCheckQuery(q CheckQuery) error {
	if q.Type != nil && !validCheckType(*q.Type) {
		return fmt.Errorf("%w: unknown check type", ErrInvalid)
	}
	if q.Status != nil && !validCheckStatus(*q.Status) {
		return fmt.Errorf("%w: unknown check status", ErrInvalid)
	}
	return nil
}

func validateIncidentQuery(q IncidentQuery) error {
	if q.Severity != nil && !validIncidentSeverity(*q.Severity) {
		return fmt.Errorf("%w: unknown severity", ErrInvalid)
	}
	if q.Status != nil && !validIncidentStatus(*q.Status) {
		return fmt.Errorf("%w: unknown incident status", ErrInvalid)
	}
	return nil
}
