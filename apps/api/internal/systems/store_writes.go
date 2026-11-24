package systems

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/safaristack/atlas/apps/api/internal/outbox"
)

// Writer is the systems write store (§8.6). It is deliberately separate from
// Store: a read-only deployment can serve the register without ever being able
// to change it, and the HTTP surface decides which one a route uses.
//
// Every method that changes a row takes the caller's expected version and
// refuses a stale one, the same optimistic-concurrency contract the rest of
// Atlas uses (§11). Methods that emit an event take it as an argument and write
// it in the same transaction as the change (ADR-005), so the event and the fact
// commit or roll back together.
type Writer interface {
	// AllocateRef reserves the next human reference for a year. A rolled-back
	// create leaves a gap, which is harmless: a reference is a label, not a
	// count.
	AllocateRef(ctx context.Context, refType string, year int) (string, error)

	// Systems.
	CreateSystem(ctx context.Context, s System) (System, error)
	UpdateSystem(ctx context.Context, s System, expectedVersion int) (System, error)
	SetSystemStatus(ctx context.Context, id uuid.UUID, expectedVersion int, status SystemStatus) (System, error)

	// Checks.
	CreateCheck(ctx context.Context, c Check) (Check, error)
	UpdateCheck(ctx context.Context, c Check, expectedVersion int) (Check, error)
	SetCheckActive(ctx context.Context, id uuid.UUID, expectedVersion int, active bool) (Check, error)

	// Incidents. A transition writes the incident, its timeline entry and its
	// event in one transaction.
	CreateIncident(ctx context.Context, in Incident, systemIDs []uuid.UUID, entry TimelineEntry, ev outbox.Event) (Incident, error)
	AcknowledgeIncident(ctx context.Context, id uuid.UUID, expectedVersion int, at time.Time, entry TimelineEntry, ev outbox.Event) (Incident, error)
	MitigateIncident(ctx context.Context, id uuid.UUID, expectedVersion int, at time.Time, entry TimelineEntry, ev outbox.Event) (Incident, error)
	ResolveIncident(ctx context.Context, id uuid.UUID, expectedVersion int, at time.Time, reviewRequired bool, entry TimelineEntry, ev outbox.Event) (Incident, error)
	CloseIncident(ctx context.Context, id uuid.UUID, expectedVersion int, at time.Time, entry TimelineEntry, ev outbox.Event) (Incident, error)
	RecordIncidentReview(ctx context.Context, id uuid.UUID, expectedVersion int, at time.Time,
		rootCause, summary string, entry TimelineEntry, ev outbox.Event) (Incident, error)
	AddTimelineEntry(ctx context.Context, entry TimelineEntry) (TimelineEntry, error)
	CreateAction(ctx context.Context, a IncidentAction, entry *TimelineEntry) (IncidentAction, error)
	UpdateAction(ctx context.Context, a IncidentAction, expectedVersion int) (IncidentAction, error)

	// Domains, certificates and credential records.
	CreateDomain(ctx context.Context, d Domain) (Domain, error)
	UpdateDomain(ctx context.Context, d Domain, expectedVersion int) (Domain, error)
	CreateCertificate(ctx context.Context, c Certificate) (Certificate, error)
	UpdateCertificate(ctx context.Context, c Certificate, expectedVersion int) (Certificate, error)
	CreateCredential(ctx context.Context, c CredentialRecord) (CredentialRecord, error)
	UpdateCredential(ctx context.Context, c CredentialRecord, expectedVersion int) (CredentialRecord, error)
}

// Compile-time proof that the Postgres store implements both halves.
var _ Writer = (*PGStore)(nil)

// AllocateRef reserves the next reference from core.sequences.
func (s *PGStore) AllocateRef(ctx context.Context, refType string, year int) (string, error) {
	var ref string
	if err := s.db.QueryRow(ctx, `SELECT core.next_reference($1, $2)`, refType, year).Scan(&ref); err != nil {
		return "", fmt.Errorf("systems: allocate ref %q: %w", refType, err)
	}
	return ref, nil
}

// nullText stores an optional string as NULL rather than "".
func nullText(v string) any {
	if v == "" {
		return nil
	}
	return v
}

// ── systems ─────────────────────────────────────────────────────────────────

func (s *PGStore) CreateSystem(ctx context.Context, sys System) (System, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return System{}, fmt.Errorf("systems: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `
		INSERT INTO systems.systems (
			id, ref, name, system_type, owner_person_id, organization_id, environment,
			criticality, contract_id, runbook_url, repository_url, description, status,
			version, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)`,
		sys.ID, sys.Ref, sys.Name, string(sys.SystemType), sys.OwnerPersonID, sys.OrganizationID,
		string(sys.Environment), string(sys.Criticality), sys.ContractID, nullText(sys.RunbookURL),
		nullText(sys.RepositoryURL), nullText(sys.Description), string(sys.Status), sys.Version,
		sys.CreatedAt, sys.UpdatedAt); err != nil {
		return System{}, writeErr("create system", err)
	}

	created, err := systemTx(ctx, tx, sys.ID)
	if err != nil {
		return System{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return System{}, fmt.Errorf("systems: commit: %w", err)
	}
	return created, nil
}

func (s *PGStore) UpdateSystem(ctx context.Context, sys System, expectedVersion int) (System, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return System{}, fmt.Errorf("systems: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var updated uuid.UUID
	err = tx.QueryRow(ctx, `
		UPDATE systems.systems SET
			name = $2, system_type = $3, owner_person_id = $4, organization_id = $5,
			environment = $6, criticality = $7, contract_id = $8, runbook_url = $9,
			repository_url = $10, description = $11, version = version + 1
		WHERE id = $1 AND version = $12
		RETURNING id`,
		sys.ID, sys.Name, string(sys.SystemType), sys.OwnerPersonID, sys.OrganizationID,
		string(sys.Environment), string(sys.Criticality), sys.ContractID, nullText(sys.RunbookURL),
		nullText(sys.RepositoryURL), nullText(sys.Description), expectedVersion).Scan(&updated)
	if errors.Is(err, pgx.ErrNoRows) {
		_ = tx.Rollback(ctx)
		return System{}, s.versionConflict(ctx, "systems.systems", sys.ID, expectedVersion)
	}
	if err != nil {
		return System{}, writeErr("update system", err)
	}

	updatedSys, err := systemTx(ctx, tx, sys.ID)
	if err != nil {
		return System{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return System{}, fmt.Errorf("systems: commit: %w", err)
	}
	return updatedSys, nil
}

func (s *PGStore) SetSystemStatus(ctx context.Context, id uuid.UUID, expectedVersion int, status SystemStatus) (System, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return System{}, fmt.Errorf("systems: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var updated uuid.UUID
	err = tx.QueryRow(ctx, `UPDATE systems.systems SET status = $2, version = version + 1
		WHERE id = $1 AND version = $3 RETURNING id`, id, string(status), expectedVersion).Scan(&updated)
	if errors.Is(err, pgx.ErrNoRows) {
		_ = tx.Rollback(ctx)
		return System{}, s.versionConflict(ctx, "systems.systems", id, expectedVersion)
	}
	if err != nil {
		return System{}, writeErr("set system status", err)
	}

	sys, err := systemTx(ctx, tx, id)
	if err != nil {
		return System{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return System{}, fmt.Errorf("systems: commit: %w", err)
	}
	return sys, nil
}

// systemTx reads a system's full projection inside a transaction, so a write
// returns the same shape a read would.
func systemTx(ctx context.Context, tx pgx.Tx, id uuid.UUID) (System, error) {
	args := []any{time.Now().UTC().Add(-AvailabilityWindow), id}
	joins := strings.Replace(systemJoins, availSincePlaceholder, "$1", 1)
	row := tx.QueryRow(ctx, `SELECT `+systemCols+` FROM systems.systems s`+joins+` WHERE s.id = $2`, args...)
	sys, err := scanSystem(row)
	if err != nil {
		return System{}, fmt.Errorf("systems: read system: %w", err)
	}
	return sys, nil
}

// ── checks ──────────────────────────────────────────────────────────────────

func (s *PGStore) CreateCheck(ctx context.Context, c Check) (Check, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Check{}, fmt.Errorf("systems: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `
		INSERT INTO systems.checks (
			id, system_id, name, check_type, target, expect, interval_seconds, timeout_seconds,
			regions, warn_ms, fail_ms, failure_threshold, heartbeat_window_seconds, active,
			version, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)`,
		c.ID, c.SystemID, c.Name, string(c.CheckType), c.Target, nullText(c.Expect),
		c.IntervalSeconds, c.TimeoutSeconds, c.Regions, c.WarnMS, c.FailMS, c.FailureThreshold,
		c.HeartbeatWindowSeconds, c.Active, c.Version, c.CreatedAt, c.UpdatedAt); err != nil {
		return Check{}, writeErr("create check", err)
	}

	created, err := checkTx(ctx, tx, c.ID)
	if err != nil {
		return Check{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Check{}, fmt.Errorf("systems: commit: %w", err)
	}
	return created, nil
}

func (s *PGStore) UpdateCheck(ctx context.Context, c Check, expectedVersion int) (Check, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Check{}, fmt.Errorf("systems: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var updated uuid.UUID
	err = tx.QueryRow(ctx, `
		UPDATE systems.checks SET
			name = $2, check_type = $3, target = $4, expect = $5, interval_seconds = $6,
			timeout_seconds = $7, regions = $8, warn_ms = $9, fail_ms = $10,
			failure_threshold = $11, heartbeat_window_seconds = $12, active = $13,
			version = version + 1
		WHERE id = $1 AND version = $14
		RETURNING id`,
		c.ID, c.Name, string(c.CheckType), c.Target, nullText(c.Expect), c.IntervalSeconds,
		c.TimeoutSeconds, c.Regions, c.WarnMS, c.FailMS, c.FailureThreshold,
		c.HeartbeatWindowSeconds, c.Active, expectedVersion).Scan(&updated)
	if errors.Is(err, pgx.ErrNoRows) {
		_ = tx.Rollback(ctx)
		return Check{}, s.versionConflict(ctx, "systems.checks", c.ID, expectedVersion)
	}
	if err != nil {
		return Check{}, writeErr("update check", err)
	}

	updatedCheck, err := checkTx(ctx, tx, c.ID)
	if err != nil {
		return Check{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Check{}, fmt.Errorf("systems: commit: %w", err)
	}
	return updatedCheck, nil
}

func (s *PGStore) SetCheckActive(ctx context.Context, id uuid.UUID, expectedVersion int, active bool) (Check, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Check{}, fmt.Errorf("systems: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var updated uuid.UUID
	err = tx.QueryRow(ctx, `UPDATE systems.checks SET active = $2, version = version + 1
		WHERE id = $1 AND version = $3 RETURNING id`, id, active, expectedVersion).Scan(&updated)
	if errors.Is(err, pgx.ErrNoRows) {
		_ = tx.Rollback(ctx)
		return Check{}, s.versionConflict(ctx, "systems.checks", id, expectedVersion)
	}
	if err != nil {
		return Check{}, writeErr("set check active", err)
	}

	c, err := checkTx(ctx, tx, id)
	if err != nil {
		return Check{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Check{}, fmt.Errorf("systems: commit: %w", err)
	}
	return c, nil
}

func checkTx(ctx context.Context, tx pgx.Tx, id uuid.UUID) (Check, error) {
	c, err := scanCheck(tx.QueryRow(ctx, `SELECT `+checkCols+` FROM systems.checks ck`+checkJoins+
		` WHERE ck.id = $1`, id))
	if err != nil {
		return Check{}, fmt.Errorf("systems: read check: %w", err)
	}
	return c, nil
}

// ── incidents ───────────────────────────────────────────────────────────────

func (s *PGStore) CreateIncident(ctx context.Context, in Incident, systemIDs []uuid.UUID,
	entry TimelineEntry, ev outbox.Event) (Incident, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Incident{}, fmt.Errorf("systems: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `
		INSERT INTO systems.incidents (
			id, ref, title, severity, status, detected_at, detected_source,
			commander_person_id, client_impact, version, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,
		in.ID, in.Ref, in.Title, string(in.Severity), string(in.Status), in.DetectedAt,
		string(in.DetectedSource), in.CommanderPersonID, nullText(in.ClientImpact),
		in.Version, in.CreatedAt, in.UpdatedAt); err != nil {
		return Incident{}, writeErr("open incident", err)
	}

	for _, systemID := range systemIDs {
		if _, err := tx.Exec(ctx, `INSERT INTO systems.incident_systems (incident_id, system_id)
			VALUES ($1, $2)`, in.ID, systemID); err != nil {
			return Incident{}, writeErr("link incident system", err)
		}
	}

	if err := insertTimelineTx(ctx, tx, entry); err != nil {
		return Incident{}, err
	}
	if _, err := outbox.Write(ctx, tx, ev); err != nil {
		return Incident{}, err
	}

	created, err := incidentTx(ctx, tx, in.ID)
	if err != nil {
		return Incident{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Incident{}, fmt.Errorf("systems: commit: %w", err)
	}
	return created, nil
}

func (s *PGStore) AcknowledgeIncident(ctx context.Context, id uuid.UUID, expectedVersion int,
	at time.Time, entry TimelineEntry, ev outbox.Event) (Incident, error) {
	return s.transitionIncident(ctx, id, expectedVersion,
		`status = 'acknowledged', acknowledged_at = $2`, []any{at}, entry, ev)
}

func (s *PGStore) MitigateIncident(ctx context.Context, id uuid.UUID, expectedVersion int,
	at time.Time, entry TimelineEntry, ev outbox.Event) (Incident, error) {
	return s.transitionIncident(ctx, id, expectedVersion,
		`status = 'mitigated', mitigated_at = $2, acknowledged_at = COALESCE(acknowledged_at, $2)`,
		[]any{at}, entry, ev)
}

func (s *PGStore) ResolveIncident(ctx context.Context, id uuid.UUID, expectedVersion int,
	at time.Time, reviewRequired bool, entry TimelineEntry, ev outbox.Event) (Incident, error) {
	return s.transitionIncident(ctx, id, expectedVersion,
		`status = 'resolved', resolved_at = $2, review_required = $3,
		 acknowledged_at = COALESCE(acknowledged_at, $2),
		 mitigated_at = COALESCE(mitigated_at, $2)`,
		[]any{at, reviewRequired}, entry, ev)
}

func (s *PGStore) CloseIncident(ctx context.Context, id uuid.UUID, expectedVersion int,
	at time.Time, entry TimelineEntry, ev outbox.Event) (Incident, error) {
	return s.transitionIncident(ctx, id, expectedVersion,
		`status = 'closed', closed_at = $2`, []any{at}, entry, ev)
}

func (s *PGStore) RecordIncidentReview(ctx context.Context, id uuid.UUID, expectedVersion int,
	at time.Time, rootCause, summary string, entry TimelineEntry, ev outbox.Event) (Incident, error) {
	return s.transitionIncident(ctx, id, expectedVersion,
		`root_cause = $2, summary = $3, review_at = $4`,
		[]any{rootCause, summary, at}, entry, ev)
}

// transitionIncident is the shared write: the row change, the timeline entry it
// produces and the event another module reacts to, in one transaction.
func (s *PGStore) transitionIncident(ctx context.Context, id uuid.UUID, expectedVersion int,
	set string, args []any, entry TimelineEntry, ev outbox.Event) (Incident, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Incident{}, fmt.Errorf("systems: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	args = append([]any{id}, args...)
	args = append(args, expectedVersion)
	query := `UPDATE systems.incidents AS i SET ` + set + `, version = version + 1
		WHERE i.id = $1 AND i.version = $` + strconv.Itoa(len(args)) + ` RETURNING i.id`
	var updated uuid.UUID
	err = tx.QueryRow(ctx, query, args...).Scan(&updated)
	if errors.Is(err, pgx.ErrNoRows) {
		_ = tx.Rollback(ctx)
		return Incident{}, s.versionConflict(ctx, "systems.incidents", id, expectedVersion)
	}
	if err != nil {
		return Incident{}, writeErr("transition incident", err)
	}

	if err := insertTimelineTx(ctx, tx, entry); err != nil {
		return Incident{}, err
	}
	if _, err := outbox.Write(ctx, tx, ev); err != nil {
		return Incident{}, err
	}

	in, err := incidentTx(ctx, tx, id)
	if err != nil {
		return Incident{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Incident{}, fmt.Errorf("systems: commit: %w", err)
	}
	return in, nil
}

func incidentTx(ctx context.Context, tx pgx.Tx, id uuid.UUID) (Incident, error) {
	in, err := scanIncident(tx.QueryRow(ctx, `SELECT `+incidentCols+` FROM systems.incidents i`+
		incidentJoins+` WHERE i.id = $1`, id))
	if err != nil {
		return Incident{}, fmt.Errorf("systems: read incident: %w", err)
	}
	return in, nil
}

func insertTimelineTx(ctx context.Context, tx pgx.Tx, entry TimelineEntry) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO systems.incident_timeline (id, incident_id, at, actor_person_id, entry_type, note)
		VALUES ($1,$2,$3,$4,$5,$6)`,
		entry.ID, entry.IncidentID, entry.At, entry.ActorPersonID, string(entry.EntryType), entry.Note)
	if err != nil {
		return writeErr("add timeline entry", err)
	}
	return nil
}

func (s *PGStore) AddTimelineEntry(ctx context.Context, entry TimelineEntry) (TimelineEntry, error) {
	if _, err := s.db.Exec(ctx, `
		INSERT INTO systems.incident_timeline (id, incident_id, at, actor_person_id, entry_type, note)
		VALUES ($1,$2,$3,$4,$5,$6)`,
		entry.ID, entry.IncidentID, entry.At, entry.ActorPersonID, string(entry.EntryType), entry.Note); err != nil {
		return TimelineEntry{}, writeErr("add timeline entry", err)
	}
	return entry, nil
}

// ── incident actions ────────────────────────────────────────────────────────

const actionCols = `a.id, a.incident_id, a.title, a.owner_person_id, COALESCE(p.full_name, ''),
	a.due_on, a.status, a.task_id, a.version, a.created_at, a.updated_at`

func scanAction(row rowScanner) (IncidentAction, error) {
	var a IncidentAction
	err := row.Scan(&a.ID, &a.IncidentID, &a.Title, &a.OwnerPersonID, &a.OwnerName,
		&a.DueOn, &a.Status, &a.TaskID, &a.Version, &a.CreatedAt, &a.UpdatedAt)
	return a, err
}

func (s *PGStore) CreateAction(ctx context.Context, a IncidentAction, entry *TimelineEntry) (IncidentAction, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return IncidentAction{}, fmt.Errorf("systems: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `
		INSERT INTO systems.incident_actions (
			id, incident_id, title, owner_person_id, due_on, status, version, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		a.ID, a.IncidentID, a.Title, a.OwnerPersonID, a.DueOn, string(a.Status),
		a.Version, a.CreatedAt, a.UpdatedAt); err != nil {
		return IncidentAction{}, writeErr("create incident action", err)
	}
	if entry != nil {
		if err := insertTimelineTx(ctx, tx, *entry); err != nil {
			return IncidentAction{}, err
		}
	}

	created, err := actionTx(ctx, tx, a.ID)
	if err != nil {
		return IncidentAction{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return IncidentAction{}, fmt.Errorf("systems: commit: %w", err)
	}
	return created, nil
}

func (s *PGStore) UpdateAction(ctx context.Context, a IncidentAction, expectedVersion int) (IncidentAction, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return IncidentAction{}, fmt.Errorf("systems: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var updated uuid.UUID
	err = tx.QueryRow(ctx, `
		UPDATE systems.incident_actions AS a SET
			title = $2, owner_person_id = $3, due_on = $4, status = $5, version = version + 1
		WHERE a.id = $1 AND a.incident_id = $6 AND a.version = $7
		RETURNING a.id`,
		a.ID, a.Title, a.OwnerPersonID, a.DueOn, string(a.Status), a.IncidentID,
		expectedVersion).Scan(&updated)
	if errors.Is(err, pgx.ErrNoRows) {
		_ = tx.Rollback(ctx)
		return IncidentAction{}, s.versionConflict(ctx, "systems.incident_actions", a.ID, expectedVersion)
	}
	if err != nil {
		return IncidentAction{}, writeErr("update incident action", err)
	}

	action, err := actionTx(ctx, tx, a.ID)
	if err != nil {
		return IncidentAction{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return IncidentAction{}, fmt.Errorf("systems: commit: %w", err)
	}
	return action, nil
}

func actionTx(ctx context.Context, tx pgx.Tx, id uuid.UUID) (IncidentAction, error) {
	a, err := scanAction(tx.QueryRow(ctx, `SELECT `+actionCols+` FROM systems.incident_actions a
		LEFT JOIN people.persons p ON p.id = a.owner_person_id WHERE a.id = $1`, id))
	if err != nil {
		return IncidentAction{}, fmt.Errorf("systems: read incident action: %w", err)
	}
	return a, nil
}

// ── domains and certificates ────────────────────────────────────────────────

func (s *PGStore) CreateDomain(ctx context.Context, d Domain) (Domain, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Domain{}, fmt.Errorf("systems: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `
		INSERT INTO systems.domains (
			id, name, system_id, registrar, registered_on, expires_on, auto_renew,
			owner_person_id, notes, version, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,
		d.ID, d.Name, d.SystemID, nullText(d.Registrar), d.RegisteredOn, d.ExpiresOn,
		d.AutoRenew, d.OwnerPersonID, nullText(d.Notes), d.Version, d.CreatedAt, d.UpdatedAt); err != nil {
		return Domain{}, writeErr("create domain", err)
	}

	created, err := domainTx(ctx, tx, d.ID)
	if err != nil {
		return Domain{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Domain{}, fmt.Errorf("systems: commit: %w", err)
	}
	return created, nil
}

func (s *PGStore) UpdateDomain(ctx context.Context, d Domain, expectedVersion int) (Domain, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Domain{}, fmt.Errorf("systems: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var updated uuid.UUID
	err = tx.QueryRow(ctx, `
		UPDATE systems.domains SET
			name = $2, system_id = $3, registrar = $4, registered_on = $5, expires_on = $6,
			auto_renew = $7, owner_person_id = $8, notes = $9, version = version + 1
		WHERE id = $1 AND version = $10
		RETURNING id`,
		d.ID, d.Name, d.SystemID, nullText(d.Registrar), d.RegisteredOn, d.ExpiresOn,
		d.AutoRenew, d.OwnerPersonID, nullText(d.Notes), expectedVersion).Scan(&updated)
	if errors.Is(err, pgx.ErrNoRows) {
		_ = tx.Rollback(ctx)
		return Domain{}, s.versionConflict(ctx, "systems.domains", d.ID, expectedVersion)
	}
	if err != nil {
		return Domain{}, writeErr("update domain", err)
	}

	updatedDomain, err := domainTx(ctx, tx, d.ID)
	if err != nil {
		return Domain{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Domain{}, fmt.Errorf("systems: commit: %w", err)
	}
	return updatedDomain, nil
}

func domainTx(ctx context.Context, tx pgx.Tx, id uuid.UUID) (Domain, error) {
	d, err := scanDomain(tx.QueryRow(ctx, `SELECT `+domainCols+` FROM systems.domains d`+
		domainJoins+` WHERE d.id = $1`, id))
	if err != nil {
		return Domain{}, fmt.Errorf("systems: read domain: %w", err)
	}
	return d, nil
}

func (s *PGStore) CreateCertificate(ctx context.Context, c Certificate) (Certificate, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Certificate{}, fmt.Errorf("systems: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `
		INSERT INTO systems.certificates (
			id, hostname, system_id, issuer, issued_on, expires_on, auto_renew,
			owner_person_id, notes, version, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,
		c.ID, c.Hostname, c.SystemID, nullText(c.Issuer), c.IssuedOn, c.ExpiresOn,
		c.AutoRenew, c.OwnerPersonID, nullText(c.Notes), c.Version, c.CreatedAt, c.UpdatedAt); err != nil {
		return Certificate{}, writeErr("create certificate", err)
	}

	created, err := certificateTx(ctx, tx, c.ID)
	if err != nil {
		return Certificate{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Certificate{}, fmt.Errorf("systems: commit: %w", err)
	}
	return created, nil
}

func (s *PGStore) UpdateCertificate(ctx context.Context, c Certificate, expectedVersion int) (Certificate, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Certificate{}, fmt.Errorf("systems: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var updated uuid.UUID
	err = tx.QueryRow(ctx, `
		UPDATE systems.certificates SET
			hostname = $2, system_id = $3, issuer = $4, issued_on = $5, expires_on = $6,
			auto_renew = $7, owner_person_id = $8, notes = $9, version = version + 1
		WHERE id = $1 AND version = $10
		RETURNING id`,
		c.ID, c.Hostname, c.SystemID, nullText(c.Issuer), c.IssuedOn, c.ExpiresOn,
		c.AutoRenew, c.OwnerPersonID, nullText(c.Notes), expectedVersion).Scan(&updated)
	if errors.Is(err, pgx.ErrNoRows) {
		_ = tx.Rollback(ctx)
		return Certificate{}, s.versionConflict(ctx, "systems.certificates", c.ID, expectedVersion)
	}
	if err != nil {
		return Certificate{}, writeErr("update certificate", err)
	}

	updatedCert, err := certificateTx(ctx, tx, c.ID)
	if err != nil {
		return Certificate{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Certificate{}, fmt.Errorf("systems: commit: %w", err)
	}
	return updatedCert, nil
}

func certificateTx(ctx context.Context, tx pgx.Tx, id uuid.UUID) (Certificate, error) {
	c, err := scanCertificate(tx.QueryRow(ctx, `SELECT `+certificateCols+` FROM systems.certificates c`+
		certificateJoins+` WHERE c.id = $1`, id))
	if err != nil {
		return Certificate{}, fmt.Errorf("systems: read certificate: %w", err)
	}
	return c, nil
}

// ── credential records ──────────────────────────────────────────────────────

func (s *PGStore) CreateCredential(ctx context.Context, c CredentialRecord) (CredentialRecord, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return CredentialRecord{}, fmt.Errorf("systems: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `
		INSERT INTO systems.credentials (
			id, system_id, label, credential_kind, store_path, owner_person_id,
			rotated_on, next_rotation_on, notes, active, version, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`,
		c.ID, c.SystemID, c.Label, c.CredentialKind, c.StorePath, c.OwnerPersonID,
		c.RotatedOn, c.NextRotationOn, nullText(c.Notes), c.Active, c.Version,
		c.CreatedAt, c.UpdatedAt); err != nil {
		return CredentialRecord{}, writeErr("create credential", err)
	}

	created, err := credentialTx(ctx, tx, c.ID)
	if err != nil {
		return CredentialRecord{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return CredentialRecord{}, fmt.Errorf("systems: commit: %w", err)
	}
	return created, nil
}

func (s *PGStore) UpdateCredential(ctx context.Context, c CredentialRecord, expectedVersion int) (CredentialRecord, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return CredentialRecord{}, fmt.Errorf("systems: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var updated uuid.UUID
	err = tx.QueryRow(ctx, `
		UPDATE systems.credentials SET
			label = $2, credential_kind = $3, store_path = $4, owner_person_id = $5,
			rotated_on = $6, next_rotation_on = $7, notes = $8, active = $9,
			version = version + 1
		WHERE id = $1 AND version = $10
		RETURNING id`,
		c.ID, c.Label, c.CredentialKind, c.StorePath, c.OwnerPersonID, c.RotatedOn,
		c.NextRotationOn, nullText(c.Notes), c.Active, expectedVersion).Scan(&updated)
	if errors.Is(err, pgx.ErrNoRows) {
		_ = tx.Rollback(ctx)
		return CredentialRecord{}, s.versionConflict(ctx, "systems.credentials", c.ID, expectedVersion)
	}
	if err != nil {
		return CredentialRecord{}, writeErr("update credential", err)
	}

	updatedCred, err := credentialTx(ctx, tx, c.ID)
	if err != nil {
		return CredentialRecord{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return CredentialRecord{}, fmt.Errorf("systems: commit: %w", err)
	}
	return updatedCred, nil
}

func credentialTx(ctx context.Context, tx pgx.Tx, id uuid.UUID) (CredentialRecord, error) {
	c, err := scanCredential(tx.QueryRow(ctx, `SELECT `+credentialCols+` FROM systems.credentials cr
		LEFT JOIN people.persons owner ON owner.id = cr.owner_person_id WHERE cr.id = $1`, id))
	if err != nil {
		return CredentialRecord{}, fmt.Errorf("systems: read credential: %w", err)
	}
	return c, nil
}

// versionConflict turns an empty UPDATE into the precise error: not found, or a
// stale version (§6.6).
func (s *PGStore) versionConflict(ctx context.Context, table string, id uuid.UUID, expected int) error {
	var cur int
	err := s.db.QueryRow(ctx, `SELECT version FROM `+table+` WHERE id = $1`, id).Scan(&cur)
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	if err != nil {
		return fmt.Errorf("systems: read version: %w", err)
	}
	return fmt.Errorf("%w: version %d is stale (current %d)", ErrConflict, expected, cur)
}

// writeErr classifies a database error a client caused: an unknown reference or
// a duplicate domain name is a validation problem, not a 500.
func writeErr(op string, err error) error {
	var pg *pgconn.PgError
	if errors.As(err, &pg) {
		switch pg.Code {
		case "23503":
			return fmt.Errorf("%w: %s: a referenced record does not exist", ErrInvalid, op)
		case "23505":
			return fmt.Errorf("%w: %s: a record with that value already exists", ErrDuplicate, op)
		case "23514":
			return fmt.Errorf("%w: %s: a value is outside the allowed range", ErrInvalid, op)
		}
	}
	return fmt.Errorf("systems: %s: %w", op, err)
}
