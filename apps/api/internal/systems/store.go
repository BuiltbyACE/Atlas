package systems

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// rowScanner is what pgx's Row and Rows have in common.
type rowScanner interface {
	Scan(dest ...any) error
}

// Store is the systems read model (§8.6). Writes go through Writer, added when
// the register and incident lifecycle land.
type Store interface {
	// RegisterTotals counts what is registered and how it is currently doing.
	RegisterTotals(ctx context.Context) (RegisterTotals, error)

	// Incidents.
	ListIncidents(ctx context.Context, q IncidentQuery, limit int, before *Cursor) ([]Incident, error)
	GetIncident(ctx context.Context, id uuid.UUID) (*Incident, error)
	IncidentSystems(ctx context.Context, incidentID uuid.UUID) ([]IncidentSystem, error)
	IncidentTimeline(ctx context.Context, incidentID uuid.UUID) ([]TimelineEntry, error)
	IncidentActions(ctx context.Context, incidentID uuid.UUID) ([]IncidentAction, error)

	// Systems.
	ListSystems(ctx context.Context, q SystemQuery, limit int, before *Cursor) ([]System, error)
	GetSystem(ctx context.Context, id uuid.UUID) (*System, error)
	ListCredentials(ctx context.Context, systemID uuid.UUID) ([]CredentialRecord, error)
	GetCredential(ctx context.Context, id uuid.UUID) (*CredentialRecord, error)
	ListAllCredentials(ctx context.Context, q CredentialQuery, limit int, before *Cursor) ([]CredentialRecord, error)

	// Domains and certificates.
	ListDomains(ctx context.Context, q DomainQuery, limit int, before *Cursor) ([]Domain, error)
	GetDomain(ctx context.Context, id uuid.UUID) (*Domain, error)
	ListCertificates(ctx context.Context, q CertificateQuery, limit int, before *Cursor) ([]Certificate, error)
	GetCertificate(ctx context.Context, id uuid.UUID) (*Certificate, error)

	// Checks.
	ListChecks(ctx context.Context, q CheckQuery, limit int, before *Cursor) ([]Check, error)
	GetCheck(ctx context.Context, id uuid.UUID) (*Check, error)
	ListCheckResults(ctx context.Context, checkID uuid.UUID, limit int, before *Cursor) ([]CheckResult, error)

	// Metrics.
	IncidentMetrics(ctx context.Context, asOf time.Time) (IncidentMetrics, error)
	IncidentSeverityCounts(ctx context.Context) ([]SeverityCount, error)
	Availability(ctx context.Context, since time.Time) ([]SystemAvailability, error)
	Expiring(ctx context.Context, asOf time.Time, withinDays int) ([]Expiry, error)
}

// RegisterTotals is the register's headline counts.
type RegisterTotals struct {
	SystemsTotal   int
	SystemsTier1   int
	SystemsTier2   int
	SystemsTier3   int
	ChecksTotal    int
	ChecksFailing  int
	ChecksWarning  int
	ChecksUnprobed int
}

// IncidentMetrics is the incident scoreboard over the metrics window.
type IncidentMetrics struct {
	OpenIncidents       int
	OpenSev1            int
	OpenSev2            int
	Incidents30D        int
	DetectedByAtlas30D  int
	DetectedByClient30D int
	MTTASeconds         *int64
	MTTRSeconds         *int64
	ReviewsOutstanding  int
}

// PGStore is the Postgres-backed read model.
type PGStore struct{ db *pgxpool.Pool }

// NewPGStore builds the read model over a pool.
func NewPGStore(db *pgxpool.Pool) *PGStore { return &PGStore{db: db} }

// ── systems ─────────────────────────────────────────────────────────────────

// systemCols is the register projection with its derived health. The counts are
// lateral so a system with no checks or no incidents still reads as a row.
const systemCols = `
	s.id, s.ref, s.name, s.system_type, s.owner_person_id, COALESCE(owner.full_name, ''),
	s.organization_id, COALESCE(org.name, ''), s.environment, s.criticality,
	s.contract_id, COALESCE(ct.ref, ''), COALESCE(s.runbook_url, ''), COALESCE(s.repository_url, ''),
	COALESCE(s.description, ''), s.status, s.version, s.created_at, s.updated_at,
	COALESCE(chk.total, 0), COALESCE(chk.failing, 0), COALESCE(chk.warning, 0),
	COALESCE(inc.open, 0), avail.bp`

// systemJoins carries the register's left joins, including the two derived
// aggregates: a check's health is its latest result, and availability is the
// pass ratio over the window.
const systemJoins = `
	LEFT JOIN people.persons owner ON owner.id = s.owner_person_id
	LEFT JOIN crm.organizations org ON org.id = s.organization_id
	LEFT JOIN contracts.contracts ct ON ct.id = s.contract_id
	LEFT JOIN LATERAL (
		SELECT count(*) AS total,
		       count(*) FILTER (WHERE lr.status = 'fail') AS failing,
		       count(*) FILTER (WHERE lr.status = 'warn') AS warning
		FROM systems.checks ck
		LEFT JOIN LATERAL (
			SELECT status FROM systems.check_results r WHERE r.check_id = ck.id ORDER BY r.at DESC LIMIT 1
		) lr ON true
		WHERE ck.system_id = s.id AND ck.active
	) chk ON true
	LEFT JOIN LATERAL (
		SELECT count(*) AS open
		FROM systems.incident_systems isx
		JOIN systems.incidents i ON i.id = isx.incident_id
		WHERE isx.system_id = s.id AND i.status NOT IN ('resolved','closed')
	) inc ON true
	LEFT JOIN LATERAL (
		SELECT (10000 * count(*) FILTER (WHERE r.status = 'pass')) / NULLIF(count(*), 0) AS bp
		FROM systems.check_results r
		JOIN systems.checks ck ON ck.id = r.check_id
		WHERE ck.system_id = s.id AND ck.active AND r.at >= $AVAIL_SINCE
	) avail ON true`

// availSincePlaceholder is replaced with the availability window's parameter
// number when the join is spliced into a query. Availability is the one join
// with a bound value, so it keeps its own placeholder rather than a literal.
const availSincePlaceholder = "$AVAIL_SINCE"

func scanSystem(row rowScanner) (System, error) {
	var s System
	var availability *int
	err := row.Scan(&s.ID, &s.Ref, &s.Name, &s.SystemType, &s.OwnerPersonID, &s.OwnerName,
		&s.OrganizationID, &s.OrganizationName, &s.Environment, &s.Criticality,
		&s.ContractID, &s.ContractRef, &s.RunbookURL, &s.RepositoryURL,
		&s.Description, &s.Status, &s.Version, &s.CreatedAt, &s.UpdatedAt,
		&s.ChecksTotal, &s.ChecksFailing, &s.ChecksWarning, &s.OpenIncidents, &availability)
	if err != nil {
		return System{}, err
	}
	if availability != nil && validAvailabilityBP(*availability) {
		s.AvailabilityBP = availability
	}
	return s, nil
}

func (s *PGStore) ListSystems(ctx context.Context, q SystemQuery, limit int, before *Cursor) ([]System, error) {
	args := []any{}
	where := "TRUE"
	add := func(cond string, v any) {
		args = append(args, v)
		where += fmt.Sprintf(" AND "+cond, len(args))
	}
	if q.Type != nil {
		add("s.system_type = $%d", string(*q.Type))
	}
	if q.Environment != nil {
		add("s.environment = $%d", string(*q.Environment))
	}
	if q.Criticality != nil {
		add("s.criticality = $%d", string(*q.Criticality))
	}
	if q.Status != nil {
		add("s.status = $%d", string(*q.Status))
	}
	if q.OwnerID != nil {
		add("s.owner_person_id = $%d", *q.OwnerID)
	}
	if q.OrganizationID != nil {
		add("s.organization_id = $%d", *q.OrganizationID)
	}
	if search := trim(q.Search); search != "" {
		add("(s.name ILIKE '%%' || $%[1]d || '%%' OR s.ref ILIKE '%%' || $%[1]d || '%%')", search)
	}
	if q.UnhealthyOnly {
		where += " AND (COALESCE(chk.failing, 0) > 0 OR COALESCE(inc.open, 0) > 0)"
	}
	if before != nil {
		args = append(args, before.At, before.ID)
		where += fmt.Sprintf(" AND (s.created_at, s.id) < ($%d, $%d)", len(args)-1, len(args))
	}
	// Availability needs a bound window; it goes last so its placeholder is the
	// highest-numbered parameter in the statement.
	args = append(args, time.Now().UTC().Add(-AvailabilityWindow))
	avail := len(args)
	args = append(args, limit)
	rows, err := s.db.Query(ctx, `SELECT `+systemCols+` FROM systems.systems s`+
		strings.Replace(systemJoins, availSincePlaceholder, fmt.Sprintf("$%d", avail), 1)+
		` WHERE `+where+` ORDER BY s.created_at DESC, s.id DESC LIMIT $`+fmt.Sprint(len(args)), args...)
	if err != nil {
		return nil, fmt.Errorf("systems: list systems: %w", err)
	}
	defer rows.Close()
	out := []System{}
	for rows.Next() {
		sys, err := scanSystem(rows)
		if err != nil {
			return nil, fmt.Errorf("systems: scan system: %w", err)
		}
		out = append(out, sys)
	}
	return out, rows.Err()
}

func (s *PGStore) GetSystem(ctx context.Context, id uuid.UUID) (*System, error) {
	args := []any{time.Now().UTC().Add(-AvailabilityWindow), id}
	row := s.db.QueryRow(ctx, `SELECT `+systemCols+` FROM systems.systems s`+
		strings.Replace(systemJoins, availSincePlaceholder, "$1", 1)+` WHERE s.id = $2`, args...)
	sys, err := scanSystem(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("systems: get system: %w", err)
	}
	return &sys, nil
}

func scanCredential(row rowScanner) (CredentialRecord, error) {
	var c CredentialRecord
	err := row.Scan(&c.ID, &c.SystemID, &c.Label, &c.CredentialKind, &c.StorePath,
		&c.OwnerPersonID, &c.OwnerName, &c.RotatedOn, &c.NextRotationOn, &c.Notes,
		&c.Active, &c.Version, &c.CreatedAt, &c.UpdatedAt)
	return c, err
}

const credentialCols = `cr.id, cr.system_id, cr.label, cr.credential_kind, cr.store_path,
	cr.owner_person_id, COALESCE(owner.full_name, ''), cr.rotated_on, cr.next_rotation_on,
	COALESCE(cr.notes, ''), cr.active, cr.version, cr.created_at, cr.updated_at`

func (s *PGStore) ListCredentials(ctx context.Context, systemID uuid.UUID) ([]CredentialRecord, error) {
	rows, err := s.db.Query(ctx, `SELECT `+credentialCols+` FROM systems.credentials cr
		LEFT JOIN people.persons owner ON owner.id = cr.owner_person_id
		WHERE cr.system_id = $1 ORDER BY cr.label`, systemID)
	if err != nil {
		return nil, fmt.Errorf("systems: list credentials: %w", err)
	}
	defer rows.Close()
	out := []CredentialRecord{}
	for rows.Next() {
		c, err := scanCredential(rows)
		if err != nil {
			return nil, fmt.Errorf("systems: scan credential: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// ── checks ──────────────────────────────────────────────────────────────────

// consecutiveFails counts the run of failures at the head of a check's recent
// results. It reports what flap protection needs: whether this is the Nth
// consecutive failure, so an incident opens on the Nth and not the first
// (§8.6). Non-failing results reset the run.
const consecutiveFails = `(
	SELECT COALESCE(count(*), 0) FROM (
		SELECT status,
		       sum(CASE WHEN status <> 'fail' THEN 1 ELSE 0 END)
		         OVER (ORDER BY at DESC ROWS BETWEEN UNBOUNDED PRECEDING AND CURRENT ROW) AS grp
		FROM (SELECT status, at FROM systems.check_results r
		      WHERE r.check_id = ck.id ORDER BY r.at DESC LIMIT 50) recent
	) w WHERE w.grp = 0 AND w.status = 'fail'
)`

const checkCols = `ck.id, ck.system_id, s.name, s.criticality, ck.name, ck.check_type, ck.target,
	COALESCE(ck.expect, ''), ck.interval_seconds, ck.timeout_seconds, ck.regions,
	ck.warn_ms, ck.fail_ms, ck.failure_threshold, ck.heartbeat_window_seconds,
	ck.active, ck.version, ck.created_at, ck.updated_at,
	lr.status, lr.at, lr.latency_ms, ` + consecutiveFails

func scanCheck(row rowScanner) (Check, error) {
	var c Check
	err := row.Scan(&c.ID, &c.SystemID, &c.SystemName, &c.SystemCriticality, &c.Name, &c.CheckType,
		&c.Target, &c.Expect, &c.IntervalSeconds, &c.TimeoutSeconds, &c.Regions,
		&c.WarnMS, &c.FailMS, &c.FailureThreshold, &c.HeartbeatWindowSeconds,
		&c.Active, &c.Version, &c.CreatedAt, &c.UpdatedAt,
		&c.LastStatus, &c.LastAt, &c.LastLatencyMS, &c.ConsecutiveFailures)
	return c, err
}

// checkJoins resolves each check's latest result once, so both the status
// filter and the projection read the same row.
const checkJoins = `
	JOIN systems.systems s ON s.id = ck.system_id
	LEFT JOIN LATERAL (
		SELECT status, at, latency_ms FROM systems.check_results r
		WHERE r.check_id = ck.id ORDER BY r.at DESC LIMIT 1
	) lr ON true`

func (s *PGStore) ListChecks(ctx context.Context, q CheckQuery, limit int, before *Cursor) ([]Check, error) {
	args := []any{}
	where := "TRUE"
	add := func(cond string, v any) {
		args = append(args, v)
		where += fmt.Sprintf(" AND "+cond, len(args))
	}
	if q.SystemID != nil {
		add("ck.system_id = $%d", *q.SystemID)
	}
	if q.Type != nil {
		add("ck.check_type = $%d", string(*q.Type))
	}
	if q.Active != nil {
		add("ck.active = $%d", *q.Active)
	}
	if q.Status != nil {
		add("lr.status = $%d", string(*q.Status))
	}
	if search := trim(q.Search); search != "" {
		add("(ck.name ILIKE '%%' || $%[1]d || '%%' OR ck.target ILIKE '%%' || $%[1]d || '%%')", search)
	}
	if before != nil {
		args = append(args, before.At, before.ID)
		where += fmt.Sprintf(" AND (ck.created_at, ck.id) < ($%d, $%d)", len(args)-1, len(args))
	}
	args = append(args, limit)
	rows, err := s.db.Query(ctx, `SELECT `+checkCols+` FROM systems.checks ck`+checkJoins+
		` WHERE `+where+` ORDER BY ck.created_at DESC, ck.id DESC LIMIT $`+fmt.Sprint(len(args)), args...)
	if err != nil {
		return nil, fmt.Errorf("systems: list checks: %w", err)
	}
	defer rows.Close()
	out := []Check{}
	for rows.Next() {
		c, err := scanCheck(rows)
		if err != nil {
			return nil, fmt.Errorf("systems: scan check: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *PGStore) GetCheck(ctx context.Context, id uuid.UUID) (*Check, error) {
	c, err := scanCheck(s.db.QueryRow(ctx, `SELECT `+checkCols+` FROM systems.checks ck`+checkJoins+
		` WHERE ck.id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("systems: get check: %w", err)
	}
	return &c, nil
}

func (s *PGStore) ListCheckResults(ctx context.Context, checkID uuid.UUID, limit int, before *Cursor) ([]CheckResult, error) {
	args := []any{checkID}
	where := "check_id = $1"
	if before != nil {
		args = append(args, before.At, before.ID)
		where += fmt.Sprintf(" AND (at, id) < ($%d, $%d)", len(args)-1, len(args))
	}
	args = append(args, limit)
	rows, err := s.db.Query(ctx, `SELECT id, check_id, at, status, latency_ms, COALESCE(detail, ''),
		source, COALESCE(region, '') FROM systems.check_results
		WHERE `+where+` ORDER BY at DESC, id DESC LIMIT $`+fmt.Sprint(len(args)), args...)
	if err != nil {
		return nil, fmt.Errorf("systems: list check results: %w", err)
	}
	defer rows.Close()
	out := []CheckResult{}
	for rows.Next() {
		var r CheckResult
		if err := rows.Scan(&r.ID, &r.CheckID, &r.At, &r.Status, &r.LatencyMS, &r.Detail,
			&r.Source, &r.Region); err != nil {
			return nil, fmt.Errorf("systems: scan check result: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ── incidents ───────────────────────────────────────────────────────────────

// incidentCols projects an incident with its commander and its systems. The
// systems are aggregated as names for the list; the detail read loads them in
// full.
const incidentCols = `i.id, i.ref, i.title, i.severity, i.status, i.detected_at, i.detected_source,
	i.acknowledged_at, i.mitigated_at, i.resolved_at, i.closed_at,
	i.commander_person_id, COALESCE(cmd.full_name, ''), COALESCE(i.client_impact, ''),
	COALESCE(i.root_cause, ''), COALESCE(i.summary, ''), i.review_required, i.review_at,
	i.version, i.created_at, i.updated_at,
	COALESCE((SELECT array_agg(DISTINCT s.name) FROM systems.incident_systems isx
	          JOIN systems.systems s ON s.id = isx.system_id WHERE isx.incident_id = i.id), '{}')`

func scanIncident(row rowScanner) (Incident, error) {
	var in Incident
	err := row.Scan(&in.ID, &in.Ref, &in.Title, &in.Severity, &in.Status, &in.DetectedAt,
		&in.DetectedSource, &in.AcknowledgedAt, &in.MitigatedAt, &in.ResolvedAt, &in.ClosedAt,
		&in.CommanderPersonID, &in.CommanderName, &in.ClientImpact, &in.RootCause, &in.Summary,
		&in.ReviewRequired, &in.ReviewAt, &in.Version, &in.CreatedAt, &in.UpdatedAt, &in.SystemNames)
	return in, err
}

const incidentJoins = `
	LEFT JOIN people.persons cmd ON cmd.id = i.commander_person_id`

func (s *PGStore) ListIncidents(ctx context.Context, q IncidentQuery, limit int, before *Cursor) ([]Incident, error) {
	args := []any{}
	where := "TRUE"
	add := func(cond string, v any) {
		args = append(args, v)
		where += fmt.Sprintf(" AND "+cond, len(args))
	}
	if q.Severity != nil {
		add("i.severity = $%d", string(*q.Severity))
	}
	if q.Status != nil {
		add("i.status = $%d", string(*q.Status))
	}
	if q.OpenOnly {
		where += " AND i.status NOT IN ('resolved','closed')"
	}
	if q.SystemID != nil {
		add("EXISTS (SELECT 1 FROM systems.incident_systems isx WHERE isx.incident_id = i.id AND isx.system_id = $%d)", *q.SystemID)
	}
	if q.CommanderID != nil {
		add("i.commander_person_id = $%d", *q.CommanderID)
	}
	if q.ReviewRequired != nil {
		add("i.review_required = $%d", *q.ReviewRequired)
	}
	if q.From != nil {
		add("i.detected_at >= $%d", *q.From)
	}
	if q.To != nil {
		add("i.detected_at <= $%d", *q.To)
	}
	if search := trim(q.Search); search != "" {
		add("(i.title ILIKE '%%' || $%[1]d || '%%' OR i.ref ILIKE '%%' || $%[1]d || '%%')", search)
	}
	if before != nil {
		args = append(args, before.At, before.ID)
		where += fmt.Sprintf(" AND (i.detected_at, i.id) < ($%d, $%d)", len(args)-1, len(args))
	}
	args = append(args, limit)
	rows, err := s.db.Query(ctx, `SELECT `+incidentCols+` FROM systems.incidents i`+incidentJoins+
		` WHERE `+where+` ORDER BY i.detected_at DESC, i.id DESC LIMIT $`+fmt.Sprint(len(args)), args...)
	if err != nil {
		return nil, fmt.Errorf("systems: list incidents: %w", err)
	}
	defer rows.Close()
	out := []Incident{}
	for rows.Next() {
		in, err := scanIncident(rows)
		if err != nil {
			return nil, fmt.Errorf("systems: scan incident: %w", err)
		}
		out = append(out, in)
	}
	return out, rows.Err()
}

func (s *PGStore) GetIncident(ctx context.Context, id uuid.UUID) (*Incident, error) {
	in, err := scanIncident(s.db.QueryRow(ctx, `SELECT `+incidentCols+` FROM systems.incidents i`+
		incidentJoins+` WHERE i.id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("systems: get incident: %w", err)
	}
	return &in, nil
}

func (s *PGStore) IncidentSystems(ctx context.Context, incidentID uuid.UUID) ([]IncidentSystem, error) {
	rows, err := s.db.Query(ctx, `SELECT s.id, s.ref, s.name, s.criticality
		FROM systems.incident_systems isx JOIN systems.systems s ON s.id = isx.system_id
		WHERE isx.incident_id = $1 ORDER BY s.criticality, s.name`, incidentID)
	if err != nil {
		return nil, fmt.Errorf("systems: incident systems: %w", err)
	}
	defer rows.Close()
	out := []IncidentSystem{}
	for rows.Next() {
		var is IncidentSystem
		if err := rows.Scan(&is.SystemID, &is.Ref, &is.Name, &is.Criticality); err != nil {
			return nil, fmt.Errorf("systems: scan incident system: %w", err)
		}
		out = append(out, is)
	}
	return out, rows.Err()
}

func (s *PGStore) IncidentTimeline(ctx context.Context, incidentID uuid.UUID) ([]TimelineEntry, error) {
	rows, err := s.db.Query(ctx, `SELECT t.id, t.incident_id, t.at, t.actor_person_id,
		COALESCE(p.full_name, ''), t.entry_type, t.note
		FROM systems.incident_timeline t
		LEFT JOIN people.persons p ON p.id = t.actor_person_id
		WHERE t.incident_id = $1 ORDER BY t.at, t.id`, incidentID)
	if err != nil {
		return nil, fmt.Errorf("systems: incident timeline: %w", err)
	}
	defer rows.Close()
	out := []TimelineEntry{}
	for rows.Next() {
		var e TimelineEntry
		if err := rows.Scan(&e.ID, &e.IncidentID, &e.At, &e.ActorPersonID, &e.ActorName,
			&e.EntryType, &e.Note); err != nil {
			return nil, fmt.Errorf("systems: scan timeline entry: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *PGStore) IncidentActions(ctx context.Context, incidentID uuid.UUID) ([]IncidentAction, error) {
	rows, err := s.db.Query(ctx, `SELECT a.id, a.incident_id, a.title, a.owner_person_id,
		COALESCE(p.full_name, ''), a.due_on, a.status, a.task_id, a.version, a.created_at, a.updated_at
		FROM systems.incident_actions a
		LEFT JOIN people.persons p ON p.id = a.owner_person_id
		WHERE a.incident_id = $1 ORDER BY a.due_on NULLS LAST, a.created_at`, incidentID)
	if err != nil {
		return nil, fmt.Errorf("systems: incident actions: %w", err)
	}
	defer rows.Close()
	out := []IncidentAction{}
	for rows.Next() {
		var a IncidentAction
		if err := rows.Scan(&a.ID, &a.IncidentID, &a.Title, &a.OwnerPersonID, &a.OwnerName,
			&a.DueOn, &a.Status, &a.TaskID, &a.Version, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, fmt.Errorf("systems: scan incident action: %w", err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// ── metrics ─────────────────────────────────────────────────────────────────

// latestCheckStatuses picks each active check's newest result. DISTINCT ON uses
// the (check_id, at DESC) index, so it reads one row per check, not the table.
const latestCheckStatuses = `
	SELECT DISTINCT ON (r.check_id) r.check_id, r.status
	FROM systems.check_results r
	ORDER BY r.check_id, r.at DESC`

func (s *PGStore) RegisterTotals(ctx context.Context) (RegisterTotals, error) {
	var t RegisterTotals
	err := s.db.QueryRow(ctx, `
		SELECT
		  (SELECT count(*) FROM systems.systems WHERE status = 'active'),
		  (SELECT count(*) FROM systems.systems WHERE status = 'active' AND criticality = 'tier1'),
		  (SELECT count(*) FROM systems.systems WHERE status = 'active' AND criticality = 'tier2'),
		  (SELECT count(*) FROM systems.systems WHERE status = 'active' AND criticality = 'tier3'),
		  (SELECT count(*) FROM systems.checks WHERE active),
		  (SELECT count(*) FROM systems.checks ck WHERE ck.active
		     AND NOT EXISTS (SELECT 1 FROM systems.check_results r WHERE r.check_id = ck.id)),
		  (SELECT count(*) FROM (`+latestCheckStatuses+`) lr
		     JOIN systems.checks ck ON ck.id = lr.check_id WHERE ck.active AND lr.status = 'fail'),
		  (SELECT count(*) FROM (`+latestCheckStatuses+`) lr
		     JOIN systems.checks ck ON ck.id = lr.check_id WHERE ck.active AND lr.status = 'warn')`).
		Scan(&t.SystemsTotal, &t.SystemsTier1, &t.SystemsTier2, &t.SystemsTier3,
			&t.ChecksTotal, &t.ChecksUnprobed, &t.ChecksFailing, &t.ChecksWarning)
	if err != nil {
		return RegisterTotals{}, fmt.Errorf("systems: register totals: %w", err)
	}
	return t, nil
}

// metricsWindow is how far back MTTA/MTTR look; a rolling quarter keeps the
// numbers responsive to how we work now rather than ever.
const metricsWindow = 90 * 24 * time.Hour

func (s *PGStore) IncidentMetrics(ctx context.Context, asOf time.Time) (IncidentMetrics, error) {
	var m IncidentMetrics
	var mtta, mttr *float64
	err := s.db.QueryRow(ctx, `
		SELECT
		  count(*) FILTER (WHERE status NOT IN ('resolved','closed')),
		  count(*) FILTER (WHERE status NOT IN ('resolved','closed') AND severity = 'sev1'),
		  count(*) FILTER (WHERE status NOT IN ('resolved','closed') AND severity = 'sev2'),
		  count(*) FILTER (WHERE detected_at >= $1),
		  count(*) FILTER (WHERE detected_at >= $1 AND detected_source = 'atlas'),
		  count(*) FILTER (WHERE detected_at >= $1 AND detected_source = 'client_reported'),
		  avg(EXTRACT(EPOCH FROM (acknowledged_at - detected_at)))
		    FILTER (WHERE acknowledged_at IS NOT NULL AND detected_at >= $2 AND severity IN ('sev1','sev2')),
		  avg(EXTRACT(EPOCH FROM (resolved_at - detected_at)))
		    FILTER (WHERE resolved_at IS NOT NULL AND detected_at >= $2 AND severity IN ('sev1','sev2')),
		  count(*) FILTER (WHERE review_required AND review_at IS NULL)
		FROM systems.incidents`,
		asOf.Add(-30*24*time.Hour), asOf.Add(-metricsWindow)).
		Scan(&m.OpenIncidents, &m.OpenSev1, &m.OpenSev2, &m.Incidents30D,
			&m.DetectedByAtlas30D, &m.DetectedByClient30D, &mtta, &mttr, &m.ReviewsOutstanding)
	if err != nil {
		return IncidentMetrics{}, fmt.Errorf("systems: incident metrics: %w", err)
	}
	m.MTTASeconds = secondsOf(mtta)
	m.MTTRSeconds = secondsOf(mttr)
	return m, nil
}

// secondsOf rounds a second count to a whole number, passing through absence.
func secondsOf(v *float64) *int64 {
	if v == nil {
		return nil
	}
	n := int64(*v + 0.5)
	return &n
}

func (s *PGStore) IncidentSeverityCounts(ctx context.Context) ([]SeverityCount, error) {
	rows, err := s.db.Query(ctx, `SELECT severity, count(*) FROM systems.incidents
		WHERE status NOT IN ('resolved','closed') GROUP BY severity`)
	if err != nil {
		return nil, fmt.Errorf("systems: severity counts: %w", err)
	}
	defer rows.Close()
	counts := map[string]int{}
	for rows.Next() {
		var sev string
		var n int
		if err := rows.Scan(&sev, &n); err != nil {
			return nil, fmt.Errorf("systems: scan severity count: %w", err)
		}
		counts[sev] = n
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// Always report every severity, so a chart has no holes.
	out := make([]SeverityCount, 0, 4)
	for _, sev := range []IncidentSeverity{Sev1, Sev2, Sev3, Sev4} {
		out = append(out, SeverityCount{Severity: string(sev), Label: severityLabel(sev), Count: counts[string(sev)]})
	}
	return out, nil
}

func severityLabel(s IncidentSeverity) string {
	switch s {
	case Sev1:
		return "SEV1"
	case Sev2:
		return "SEV2"
	case Sev3:
		return "SEV3"
	default:
		return "SEV4"
	}
}

func (s *PGStore) Availability(ctx context.Context, since time.Time) ([]SystemAvailability, error) {
	rows, err := s.db.Query(ctx, `SELECT s.id, s.ref, s.name, s.criticality,
		  count(*) FILTER (WHERE r.status = 'pass'), count(*)
		FROM systems.systems s
		JOIN systems.checks ck ON ck.system_id = s.id AND ck.active
		JOIN systems.check_results r ON r.check_id = ck.id AND r.at >= $1
		GROUP BY s.id, s.ref, s.name, s.criticality
		ORDER BY s.criticality, s.name`, since)
	if err != nil {
		return nil, fmt.Errorf("systems: availability: %w", err)
	}
	defer rows.Close()
	out := []SystemAvailability{}
	for rows.Next() {
		var a SystemAvailability
		if err := rows.Scan(&a.SystemID, &a.Ref, &a.Name, &a.Criticality, &a.Passed, &a.Total); err != nil {
			return nil, fmt.Errorf("systems: scan availability: %w", err)
		}
		if a.Total > 0 {
			a.AvailabilityBP = 10000 * a.Passed / a.Total
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// Expiring gathers everything about to lapse within withinDays: domain
// registrations, certificates and credential rotations. Each source carries its
// own date, so they are read separately and merged in Go, where the day
// arithmetic is the same one the warnings use.
func (s *PGStore) Expiring(ctx context.Context, asOf time.Time, withinDays int) ([]Expiry, error) {
	until := asOf.AddDate(0, 0, withinDays)
	out := []Expiry{}

	add := func(rows pgx.Rows, kind ExpiryKind) error {
		for rows.Next() {
			var e Expiry
			var expires time.Time
			var autoRenew bool
			var systemID *uuid.UUID
			var systemName string
			if err := rows.Scan(&e.ID, &e.Name, &expires, &autoRenew, &e.OwnerName, &systemID, &systemName); err != nil {
				return fmt.Errorf("systems: scan expiry: %w", err)
			}
			e.Kind, e.ExpiresOn, e.AutoRenew = kind, expires, autoRenew
			e.SystemID, e.SystemName = systemID, systemName
			e.DaysOut = daysBetween(asOf, expires)
			e.WarningBand = warningBand(e.DaysOut)
			out = append(out, e)
		}
		return rows.Err()
	}

	domainRows, err := s.db.Query(ctx, `SELECT d.id, d.name, d.expires_on, d.auto_renew,
		  COALESCE(p.full_name, ''), d.system_id, COALESCE(sys.name, '')
		FROM systems.domains d
		LEFT JOIN people.persons p ON p.id = d.owner_person_id
		LEFT JOIN systems.systems sys ON sys.id = d.system_id
		WHERE d.expires_on IS NOT NULL AND d.expires_on <= $1 AND d.expires_on >= $2
		ORDER BY d.expires_on`, until, asOf.AddDate(0, 0, -365))
	if err != nil {
		return nil, fmt.Errorf("systems: expiring domains: %w", err)
	}
	defer domainRows.Close()
	if err := add(domainRows, ExpiryDomain); err != nil {
		return nil, err
	}

	certRows, err := s.db.Query(ctx, `SELECT c.id, c.hostname, c.expires_on, c.auto_renew,
		  COALESCE(p.full_name, ''), c.system_id, COALESCE(sys.name, '')
		FROM systems.certificates c
		LEFT JOIN people.persons p ON p.id = c.owner_person_id
		LEFT JOIN systems.systems sys ON sys.id = c.system_id
		WHERE c.expires_on <= $1 AND c.expires_on >= $2
		ORDER BY c.expires_on`, until, asOf.AddDate(0, 0, -365))
	if err != nil {
		return nil, fmt.Errorf("systems: expiring certificates: %w", err)
	}
	defer certRows.Close()
	if err := add(certRows, ExpiryCertificate); err != nil {
		return nil, err
	}

	credRows, err := s.db.Query(ctx, `SELECT c.id, c.label, c.next_rotation_on, true,
		  COALESCE(p.full_name, ''), c.system_id, COALESCE(sys.name, '')
		FROM systems.credentials c
		LEFT JOIN people.persons p ON p.id = c.owner_person_id
		LEFT JOIN systems.systems sys ON sys.id = c.system_id
		WHERE c.active AND c.next_rotation_on IS NOT NULL
		  AND c.next_rotation_on <= $1 AND c.next_rotation_on >= $2
		ORDER BY c.next_rotation_on`, until, asOf.AddDate(0, 0, -365))
	if err != nil {
		return nil, fmt.Errorf("systems: expiring credentials: %w", err)
	}
	defer credRows.Close()
	if err := add(credRows, ExpiryCredential); err != nil {
		return nil, err
	}

	sort.SliceStable(out, func(i, j int) bool {
		if out[i].ExpiresOn.Equal(out[j].ExpiresOn) {
			return out[i].Name < out[j].Name
		}
		return out[i].ExpiresOn.Before(out[j].ExpiresOn)
	})
	return out, nil
}
