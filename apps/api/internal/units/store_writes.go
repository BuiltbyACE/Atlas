package units

import (
	"context"
	"fmt"
	"time"

	"github.com/safaristack/atlas/apps/api/internal/outbox"
)

// Writer is the units write model (§8.7). Go owns every rule: a retired unit
// is frozen, an integration's credential reference is a location never a
// secret, and an indicator breach is detected at write time.
type Writer interface {
	// AllocateRef allocates a human reference for a new record.
	AllocateRef(ctx context.Context, pType string, pYear int) (string, error)

	// Business units.
	CreateUnit(ctx context.Context, u BusinessUnit, ev outbox.Event) (BusinessUnit, error)
	UpdateUnit(ctx context.Context, u BusinessUnit, expectedVersion int, ev outbox.Event) (BusinessUnit, error)

	// Integrations.
	CreateIntegration(ctx context.Context, i UnitIntegration, ev outbox.Event) (UnitIntegration, error)
	UpdateIntegration(ctx context.Context, i UnitIntegration, expectedVersion int, ev outbox.Event) (UnitIntegration, error)

	// Indicator definitions.
	CreateIndicatorDefinition(ctx context.Context, d UnitIndicatorDefinition, ev outbox.Event) (UnitIndicatorDefinition, error)
	UpdateIndicatorDefinition(ctx context.Context, d UnitIndicatorDefinition, expectedVersion int, ev outbox.Event) (UnitIndicatorDefinition, error)

	// Snapshots and finance summaries are written by background processes,
	// not by API clients.
}

var _ Writer = (*PGStore)(nil)

// AllocateRef reserves the next human reference for a unit year.
func (s *PGStore) AllocateRef(ctx context.Context, refType string, year int) (string, error) {
	var ref string
	if err := s.db.QueryRow(ctx, `SELECT core.next_reference($1, $2)`, refType, year).Scan(&ref); err != nil {
		return "", fmt.Errorf("units: allocate ref %q: %w", refType, err)
	}
	return ref, nil
}

func writeErr(op string, err error) error {
	return fmt.Errorf("units: %s: %w", op, err)
}

func nullText(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// nullTextPtr returns a pointer to the string, or nil when empty.
func nullTextPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// ── business units ──────────────────────────────────────────────────────────

func (s *PGStore) CreateUnit(ctx context.Context, u BusinessUnit, ev outbox.Event) (BusinessUnit, error) {
	if !validUnitCode(u.Code) {
		return BusinessUnit{}, withCode(errInvalidField("code"), CodeDuplicateCode)
	}
	if !validUnitKind(u.Kind) {
		return BusinessUnit{}, withCode(errInvalidField("kind"), CodeInvalidUnitKind)
	}
	if !validUnitStatus(u.Status) {
		u.Status = UnitActive
	}
	if u.Ref == "" {
		ref, err := s.AllocateRef(ctx, "unit", time.Now().UTC().Year())
		if err != nil {
			return BusinessUnit{}, writeErr("allocate ref", err)
		}
		u.Ref = ref
	}
	u.Version = 1
	err := s.db.QueryRow(ctx, `
		INSERT INTO units.business_units (ref, name, code, kind, owner_person_id, description, status, version)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		RETURNING id, created_at, updated_at`,
		u.Ref, u.Name, u.Code, string(u.Kind), u.OwnerPersonID, nullTextPtr(u.Description),
		string(u.Status), u.Version).
		Scan(&u.ID, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return BusinessUnit{}, writeErr("create unit", err)
	}
	if _, err := outbox.Write(ctx, s.db, ev); err != nil {
		return BusinessUnit{}, writeErr("write outbox event", err)
	}
	return u, nil
}

func (s *PGStore) UpdateUnit(ctx context.Context, u BusinessUnit, expectedVersion int, ev outbox.Event) (BusinessUnit, error) {
	current, err := s.GetUnit(ctx, u.ID)
	if err != nil {
		return BusinessUnit{}, err
	}
	if current == nil {
		return BusinessUnit{}, ErrNotFound
	}
	if current.Version != expectedVersion {
		return BusinessUnit{}, withCode(ErrConflict, CodeUnitRetired)
	}
	if !UnitEditable(current.Status) {
		return BusinessUnit{}, withCode(ErrForbidden, CodeUnitNotEditable)
	}
	u.Ref = current.Ref
	u.Code = current.Code
	u.Version = current.Version + 1
	err = s.db.QueryRow(ctx, `
		UPDATE units.business_units
		SET name = $1, kind = $2, owner_person_id = $3, description = $4, status = $5, version = $6
		WHERE id = $7
		RETURNING updated_at`,
		u.Name, string(u.Kind), u.OwnerPersonID, nullTextPtr(u.Description),
		string(u.Status), u.Version, u.ID).
		Scan(&u.UpdatedAt)
	if err != nil {
		return BusinessUnit{}, writeErr("update unit", err)
	}
	if _, err := outbox.Write(ctx, s.db, ev); err != nil {
		return BusinessUnit{}, writeErr("write outbox event", err)
	}
	return u, nil
}

// ── integrations ────────────────────────────────────────────────────────────

func (s *PGStore) CreateIntegration(ctx context.Context, i UnitIntegration, ev outbox.Event) (UnitIntegration, error) {
	if !validIntegrationKind(i.Kind) {
		return UnitIntegration{}, withCode(errInvalidField("kind"), CodeInvalidIntegrationKind)
	}
	if !validEndpoint(i.Endpoint) {
		return UnitIntegration{}, withCode(errInvalidField("endpoint"), CodeInvalidEndpoint)
	}
	i.Version = 1
	err := s.db.QueryRow(ctx, `
		INSERT INTO units.unit_integrations (unit_id, name, kind, endpoint, credential_ref, schedule, active, version)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		RETURNING id, created_at, updated_at`,
		i.UnitID, i.Name, string(i.Kind), i.Endpoint, i.CredentialRef, i.Schedule,
		i.Active, i.Version).
		Scan(&i.ID, &i.CreatedAt, &i.UpdatedAt)
	if err != nil {
		return UnitIntegration{}, writeErr("create integration", err)
	}
	if _, err := outbox.Write(ctx, s.db, ev); err != nil {
		return UnitIntegration{}, writeErr("write outbox event", err)
	}
	return i, nil
}

func (s *PGStore) UpdateIntegration(ctx context.Context, i UnitIntegration, expectedVersion int, ev outbox.Event) (UnitIntegration, error) {
	current, err := s.GetIntegration(ctx, i.ID)
	if err != nil {
		return UnitIntegration{}, err
	}
	if current == nil {
		return UnitIntegration{}, ErrNotFound
	}
	if current.Version != expectedVersion {
		return UnitIntegration{}, withCode(ErrConflict, CodeIntegrationNotEditable)
	}
	i.UnitID = current.UnitID
	i.Version = current.Version + 1
	err = s.db.QueryRow(ctx, `
		UPDATE units.unit_integrations
		SET name = $1, kind = $2, endpoint = $3, credential_ref = $4, schedule = $5, active = $6, version = $7
		WHERE id = $8
		RETURNING updated_at`,
		i.Name, string(i.Kind), i.Endpoint, i.CredentialRef, i.Schedule, i.Active,
		i.Version, i.ID).
		Scan(&i.UpdatedAt)
	if err != nil {
		return UnitIntegration{}, writeErr("update integration", err)
	}
	if _, err := outbox.Write(ctx, s.db, ev); err != nil {
		return UnitIntegration{}, writeErr("write outbox event", err)
	}
	return i, nil
}

// ── indicator definitions ───────────────────────────────────────────────────

func (s *PGStore) CreateIndicatorDefinition(ctx context.Context, d UnitIndicatorDefinition, ev outbox.Event) (UnitIndicatorDefinition, error) {
	if !validIndicatorKind(d.Kind) {
		return UnitIndicatorDefinition{}, withCode(errInvalidField("kind"), CodeInvalidIndicatorKind)
	}
	d.Version = 1
	err := s.db.QueryRow(ctx, `
		INSERT INTO units.unit_indicator_definitions (unit_id, name, kind, source_field, formula, target, warning_threshold, critical_threshold, active, version)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		RETURNING id, created_at, updated_at`,
		d.UnitID, d.Name, string(d.Kind), d.SourceField, d.Formula, d.Target,
		d.WarningThreshold, d.CriticalThreshold, d.Active, d.Version).
		Scan(&d.ID, &d.CreatedAt, &d.UpdatedAt)
	if err != nil {
		return UnitIndicatorDefinition{}, writeErr("create indicator definition", err)
	}
	if _, err := outbox.Write(ctx, s.db, ev); err != nil {
		return UnitIndicatorDefinition{}, writeErr("write outbox event", err)
	}
	return d, nil
}

func (s *PGStore) UpdateIndicatorDefinition(ctx context.Context, d UnitIndicatorDefinition, expectedVersion int, ev outbox.Event) (UnitIndicatorDefinition, error) {
	current, err := s.GetIndicatorDefinition(ctx, d.ID)
	if err != nil {
		return UnitIndicatorDefinition{}, err
	}
	if current == nil {
		return UnitIndicatorDefinition{}, ErrNotFound
	}
	if current.Version != expectedVersion {
		return UnitIndicatorDefinition{}, withCode(ErrConflict, CodeIndicatorNotEditable)
	}
	d.UnitID = current.UnitID
	d.Version = current.Version + 1
	err = s.db.QueryRow(ctx, `
		UPDATE units.unit_indicator_definitions
		SET name = $1, kind = $2, source_field = $3, formula = $4, target = $5,
		    warning_threshold = $6, critical_threshold = $7, active = $8, version = $9
		WHERE id = $10
		RETURNING updated_at`,
		d.Name, string(d.Kind), d.SourceField, d.Formula, d.Target,
		d.WarningThreshold, d.CriticalThreshold, d.Active, d.Version, d.ID).
		Scan(&d.UpdatedAt)
	if err != nil {
		return UnitIndicatorDefinition{}, writeErr("update indicator definition", err)
	}
	if _, err := outbox.Write(ctx, s.db, ev); err != nil {
		return UnitIndicatorDefinition{}, writeErr("write outbox event", err)
	}
	return d, nil
}
