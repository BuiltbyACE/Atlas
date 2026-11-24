package units

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// rowScanner is what pgx's Row and Rows have in common.
type rowScanner interface {
	Scan(dest ...any) error
}

// Store is the units read model (§8.7). Writes go through Writer, added when
// the unit lifecycle lands.
type Store interface {
	// Business units.
	ListUnits(ctx context.Context, q UnitQuery, limit int, before *Cursor) ([]BusinessUnit, error)
	GetUnit(ctx context.Context, id uuid.UUID) (*BusinessUnit, error)

	// Integrations.
	ListIntegrations(ctx context.Context, q IntegrationQuery, limit int, before *Cursor) ([]UnitIntegration, error)
	GetIntegration(ctx context.Context, id uuid.UUID) (*UnitIntegration, error)

	// Indicator definitions.
	ListIndicatorDefinitions(ctx context.Context, q IndicatorDefinitionQuery, limit int, before *Cursor) ([]UnitIndicatorDefinition, error)
	GetIndicatorDefinition(ctx context.Context, id uuid.UUID) (*UnitIndicatorDefinition, error)

	// Snapshots.
	ListSnapshots(ctx context.Context, q SnapshotQuery, limit int, before *Cursor) ([]UnitSnapshot, error)
	GetSnapshot(ctx context.Context, id uuid.UUID) (*UnitSnapshot, error)

	// Finance summaries.
	ListFinanceSummaries(ctx context.Context, q FinanceSummaryQuery, limit int, before *Cursor) ([]UnitFinanceSummary, error)
	GetFinanceSummary(ctx context.Context, id uuid.UUID) (*UnitFinanceSummary, error)
}

// PGStore is the Postgres-backed read model.
type PGStore struct{ db *pgxpool.Pool }

// NewPGStore builds the read model over a pool.
func NewPGStore(db *pgxpool.Pool) *PGStore { return &PGStore{db: db} }

// ── business units ──────────────────────────────────────────────────────────

// unitCols is the unit projection with derived integration/indicator counts.
const unitCols = `
	u.id, u.ref, u.name, u.code, u.kind, u.owner_person_id, COALESCE(p.full_name, ''),
	u.description, CASE WHEN u.active THEN 'active' ELSE 'retired' END AS status, u.version, u.created_at, u.updated_at,
	COALESCE(ig.total, 0), COALESCE(ig.active, 0), COALESCE(ind.count, 0)`

// unitJoins left-joins the derived aggregates.
const unitJoins = `
	LEFT JOIN people.persons p ON p.id = u.owner_person_id
	LEFT JOIN LATERAL (
		SELECT count(*) AS total,
		       count(*) FILTER (WHERE active) AS active
		FROM units.unit_integrations i WHERE i.unit_id = u.id
	) ig ON true
	LEFT JOIN LATERAL (
		SELECT count(*) AS count
		FROM units.unit_indicator_definitions d WHERE d.unit_id = u.id AND d.active
	) ind ON true`

func scanUnit(row rowScanner) (BusinessUnit, error) {
	var u BusinessUnit
	err := row.Scan(&u.ID, &u.Ref, &u.Name, &u.Code, &u.Kind, &u.OwnerPersonID, &u.OwnerName,
		&u.Description, &u.Status, &u.Version, &u.CreatedAt, &u.UpdatedAt,
		&u.IntegrationCount, &u.ActiveIntegrations, &u.IndicatorCount)
	if err != nil {
		return BusinessUnit{}, err
	}
	return u, nil
}

func (s *PGStore) ListUnits(ctx context.Context, q UnitQuery, limit int, before *Cursor) ([]BusinessUnit, error) {
	args := []any{}
	where := "TRUE"
	add := func(cond string, v any) {
		args = append(args, v)
		where += fmt.Sprintf(" AND "+cond, len(args))
	}
	if q.Kind != nil {
		add("u.kind = $%d", string(*q.Kind))
	}
	if q.Status != nil {
		add("u.active = $%d", *q.Status == "active")
	}
	if q.OwnerID != nil {
		add("u.owner_person_id = $%d", *q.OwnerID)
	}
	if q.Active != nil {
		add("u.active = $%d", *q.Active)
	}
	if search := trim(q.Search); search != "" {
		add("(u.name ILIKE '%%' || $%[1]d || '%%' OR u.ref ILIKE '%%' || $%[1]d || '%%' OR u.code ILIKE '%%' || $%[1]d || '%%')", search)
	}
	if before != nil {
		args = append(args, before.At, before.ID)
		where += fmt.Sprintf(" AND (u.created_at, u.id) < ($%d, $%d)", len(args)-1, len(args))
	}
	args = append(args, limit)
	rows, err := s.db.Query(ctx, `SELECT `+unitCols+` FROM units.business_units u`+
		unitJoins+` WHERE `+where+` ORDER BY u.created_at DESC, u.id DESC LIMIT $`+fmt.Sprint(len(args)), args...)
	if err != nil {
		return nil, fmt.Errorf("units: list units: %w", err)
	}
	defer rows.Close()
	out := []BusinessUnit{}
	for rows.Next() {
		u, err := scanUnit(rows)
		if err != nil {
			return nil, fmt.Errorf("units: scan unit: %w", err)
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func (s *PGStore) GetUnit(ctx context.Context, id uuid.UUID) (*BusinessUnit, error) {
	u, err := scanUnit(s.db.QueryRow(ctx, `SELECT `+unitCols+` FROM units.business_units u`+
		unitJoins+` WHERE u.id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("units: get unit: %w", err)
	}
	return &u, nil
}

// ── integrations ────────────────────────────────────────────────────────────

const integrationCols = `
	i.id, i.unit_id, u.name, i.name, i.kind, i.endpoint, i.credential_ref,
	i.schedule, i.last_sync_at, i.last_sync_status, i.active, i.version,
	i.created_at, i.updated_at`

func scanIntegration(row rowScanner) (UnitIntegration, error) {
	var i UnitIntegration
	err := row.Scan(&i.ID, &i.UnitID, &i.UnitName, &i.Name, &i.Kind, &i.Endpoint,
		&i.CredentialRef, &i.Schedule, &i.LastSyncAt, &i.LastSyncStatus,
		&i.Active, &i.Version, &i.CreatedAt, &i.UpdatedAt)
	return i, err
}

const integrationJoins = `
	JOIN units.business_units u ON u.id = i.unit_id`

func (s *PGStore) ListIntegrations(ctx context.Context, q IntegrationQuery, limit int, before *Cursor) ([]UnitIntegration, error) {
	args := []any{}
	where := "TRUE"
	add := func(cond string, v any) {
		args = append(args, v)
		where += fmt.Sprintf(" AND "+cond, len(args))
	}
	if q.UnitID != nil {
		add("i.unit_id = $%d", *q.UnitID)
	}
	if q.Kind != nil {
		add("i.kind = $%d", string(*q.Kind))
	}
	if q.Status != nil {
		add("i.last_sync_status = $%d", string(*q.Status))
	}
	if q.Active != nil {
		add("i.active = $%d", *q.Active)
	}
	if search := trim(q.Search); search != "" {
		add("(i.name ILIKE '%%' || $%[1]d || '%%' OR i.endpoint ILIKE '%%' || $%[1]d || '%%')", search)
	}
	if before != nil {
		args = append(args, before.At, before.ID)
		where += fmt.Sprintf(" AND (i.created_at, i.id) < ($%d, $%d)", len(args)-1, len(args))
	}
	args = append(args, limit)
	rows, err := s.db.Query(ctx, `SELECT `+integrationCols+` FROM units.unit_integrations i`+
		integrationJoins+` WHERE `+where+` ORDER BY i.created_at DESC, i.id DESC LIMIT $`+fmt.Sprint(len(args)), args...)
	if err != nil {
		return nil, fmt.Errorf("units: list integrations: %w", err)
	}
	defer rows.Close()
	out := []UnitIntegration{}
	for rows.Next() {
		i, err := scanIntegration(rows)
		if err != nil {
			return nil, fmt.Errorf("units: scan integration: %w", err)
		}
		out = append(out, i)
	}
	return out, rows.Err()
}

func (s *PGStore) GetIntegration(ctx context.Context, id uuid.UUID) (*UnitIntegration, error) {
	i, err := scanIntegration(s.db.QueryRow(ctx, `SELECT `+integrationCols+` FROM units.unit_integrations i`+
		integrationJoins+` WHERE i.id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("units: get integration: %w", err)
	}
	return &i, nil
}

// ── indicator definitions ───────────────────────────────────────────────────

const indicatorDefCols = `
	d.id, d.unit_id, u.name, d.name, d.kind, d.source_field, d.formula,
	d.target, d.warning_threshold, d.critical_threshold, d.active, d.version,
	d.created_at, d.updated_at`

func scanIndicatorDefinition(row rowScanner) (UnitIndicatorDefinition, error) {
	var d UnitIndicatorDefinition
	err := row.Scan(&d.ID, &d.UnitID, &d.UnitName, &d.Name, &d.Kind, &d.SourceField,
		&d.Formula, &d.Target, &d.WarningThreshold, &d.CriticalThreshold,
		&d.Active, &d.Version, &d.CreatedAt, &d.UpdatedAt)
	return d, err
}

const indicatorDefJoins = `
	JOIN units.business_units u ON u.id = d.unit_id`

func (s *PGStore) ListIndicatorDefinitions(ctx context.Context, q IndicatorDefinitionQuery, limit int, before *Cursor) ([]UnitIndicatorDefinition, error) {
	args := []any{}
	where := "TRUE"
	add := func(cond string, v any) {
		args = append(args, v)
		where += fmt.Sprintf(" AND "+cond, len(args))
	}
	if q.UnitID != nil {
		add("d.unit_id = $%d", *q.UnitID)
	}
	if q.Kind != nil {
		add("d.kind = $%d", string(*q.Kind))
	}
	if q.Active != nil {
		add("d.active = $%d", *q.Active)
	}
	if search := trim(q.Search); search != "" {
		add("(d.name ILIKE '%%' || $%[1]d || '%%' OR d.source_field ILIKE '%%' || $%[1]d || '%%')", search)
	}
	if before != nil {
		args = append(args, before.At, before.ID)
		where += fmt.Sprintf(" AND (d.created_at, d.id) < ($%d, $%d)", len(args)-1, len(args))
	}
	args = append(args, limit)
	rows, err := s.db.Query(ctx, `SELECT `+indicatorDefCols+` FROM units.unit_indicator_definitions d`+
		indicatorDefJoins+` WHERE `+where+` ORDER BY d.created_at DESC, d.id DESC LIMIT $`+fmt.Sprint(len(args)), args...)
	if err != nil {
		return nil, fmt.Errorf("units: list indicator definitions: %w", err)
	}
	defer rows.Close()
	out := []UnitIndicatorDefinition{}
	for rows.Next() {
		d, err := scanIndicatorDefinition(rows)
		if err != nil {
			return nil, fmt.Errorf("units: scan indicator definition: %w", err)
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *PGStore) GetIndicatorDefinition(ctx context.Context, id uuid.UUID) (*UnitIndicatorDefinition, error) {
	d, err := scanIndicatorDefinition(s.db.QueryRow(ctx, `SELECT `+indicatorDefCols+` FROM units.unit_indicator_definitions d`+
		indicatorDefJoins+` WHERE d.id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("units: get indicator definition: %w", err)
	}
	return &d, nil
}

// ── snapshots ───────────────────────────────────────────────────────────────

const snapshotCols = `
	sp.id, sp.unit_id, u.name, sp.on_date, sp.indicator_id, d.name,
	sp.value, sp.met_target, sp.notes, sp.created_at`

func scanSnapshot(row rowScanner) (UnitSnapshot, error) {
	var sp UnitSnapshot
	err := row.Scan(&sp.ID, &sp.UnitID, &sp.UnitName, &sp.OnDate, &sp.IndicatorID,
		&sp.IndicatorName, &sp.Value, &sp.MetTarget, &sp.Notes, &sp.CreatedAt)
	return sp, err
}

const snapshotJoins = `
	JOIN units.business_units u ON u.id = sp.unit_id
	JOIN units.unit_indicator_definitions d ON d.id = sp.indicator_id`

func (s *PGStore) ListSnapshots(ctx context.Context, q SnapshotQuery, limit int, before *Cursor) ([]UnitSnapshot, error) {
	args := []any{}
	where := "TRUE"
	add := func(cond string, v any) {
		args = append(args, v)
		where += fmt.Sprintf(" AND "+cond, len(args))
	}
	if q.UnitID != nil {
		add("sp.unit_id = $%d", *q.UnitID)
	}
	if q.IndicatorID != nil {
		add("sp.indicator_id = $%d", *q.IndicatorID)
	}
	if q.From != nil {
		add("sp.on_date >= $%d", *q.From)
	}
	if q.To != nil {
		add("sp.on_date <= $%d", *q.To)
	}
	if search := trim(q.Search); search != "" {
		add("(sp.value ILIKE '%%' || $%[1]d || '%%' OR sp.notes ILIKE '%%' || $%[1]d || '%%')", search)
	}
	if before != nil {
		args = append(args, before.At, before.ID)
		where += fmt.Sprintf(" AND (sp.created_at, sp.id) < ($%d, $%d)", len(args)-1, len(args))
	}
	args = append(args, limit)
	rows, err := s.db.Query(ctx, `SELECT `+snapshotCols+` FROM units.unit_snapshots sp`+
		snapshotJoins+` WHERE `+where+` ORDER BY sp.created_at DESC, sp.id DESC LIMIT $`+fmt.Sprint(len(args)), args...)
	if err != nil {
		return nil, fmt.Errorf("units: list snapshots: %w", err)
	}
	defer rows.Close()
	out := []UnitSnapshot{}
	for rows.Next() {
		sp, err := scanSnapshot(rows)
		if err != nil {
			return nil, fmt.Errorf("units: scan snapshot: %w", err)
		}
		out = append(out, sp)
	}
	return out, rows.Err()
}

func (s *PGStore) GetSnapshot(ctx context.Context, id uuid.UUID) (*UnitSnapshot, error) {
	sp, err := scanSnapshot(s.db.QueryRow(ctx, `SELECT `+snapshotCols+` FROM units.unit_snapshots sp`+
		snapshotJoins+` WHERE sp.id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("units: get snapshot: %w", err)
	}
	return &sp, nil
}

// ── finance summaries ───────────────────────────────────────────────────────

const financeSummaryCols = `
	f.id, f.unit_id, u.name, f.period_start, f.period_end, f.income_minor,
	f.expenses_minor, f.receivables_minor, f.created_at, f.updated_at`

func scanFinanceSummary(row rowScanner) (UnitFinanceSummary, error) {
	var f UnitFinanceSummary
	err := row.Scan(&f.ID, &f.UnitID, &f.UnitName, &f.PeriodStart, &f.PeriodEnd,
		&f.IncomeMinor, &f.ExpensesMinor, &f.ReceivablesMinor, &f.CreatedAt, &f.UpdatedAt)
	return f, err
}

const financeSummaryJoins = `
	JOIN units.business_units u ON u.id = f.unit_id`

func (s *PGStore) ListFinanceSummaries(ctx context.Context, q FinanceSummaryQuery, limit int, before *Cursor) ([]UnitFinanceSummary, error) {
	args := []any{}
	where := "TRUE"
	add := func(cond string, v any) {
		args = append(args, v)
		where += fmt.Sprintf(" AND "+cond, len(args))
	}
	if q.UnitID != nil {
		add("f.unit_id = $%d", *q.UnitID)
	}
	if q.From != nil {
		add("f.period_start >= $%d", *q.From)
	}
	if q.To != nil {
		add("f.period_end <= $%d", *q.To)
	}
	if search := trim(q.Search); search != "" {
		add("(u.name ILIKE '%%' || $%[1]d || '%%' OR f.unit_id::text ILIKE '%%' || $%[1]d || '%%')", search)
	}
	if before != nil {
		args = append(args, before.At, before.ID)
		where += fmt.Sprintf(" AND (f.created_at, f.id) < ($%d, $%d)", len(args)-1, len(args))
	}
	args = append(args, limit)
	rows, err := s.db.Query(ctx, `SELECT `+financeSummaryCols+` FROM units.unit_finance_summaries f`+
		financeSummaryJoins+` WHERE `+where+` ORDER BY f.created_at DESC, f.id DESC LIMIT $`+fmt.Sprint(len(args)), args...)
	if err != nil {
		return nil, fmt.Errorf("units: list finance summaries: %w", err)
	}
	defer rows.Close()
	out := []UnitFinanceSummary{}
	for rows.Next() {
		f, err := scanFinanceSummary(rows)
		if err != nil {
			return nil, fmt.Errorf("units: scan finance summary: %w", err)
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

func (s *PGStore) GetFinanceSummary(ctx context.Context, id uuid.UUID) (*UnitFinanceSummary, error) {
	f, err := scanFinanceSummary(s.db.QueryRow(ctx, `SELECT `+financeSummaryCols+` FROM units.unit_finance_summaries f`+
		financeSummaryJoins+` WHERE f.id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("units: get finance summary: %w", err)
	}
	return &f, nil
}

// sortSnapshots sorts snapshots by unit, indicator, then date so a unit's
// time series reads in order.
func sortSnapshots(snapshots []UnitSnapshot) {
	sort.SliceStable(snapshots, func(i, j int) bool {
		if snapshots[i].UnitID != snapshots[j].UnitID {
			return snapshots[i].UnitID.String() < snapshots[j].UnitID.String()
		}
		if snapshots[i].IndicatorID != snapshots[j].IndicatorID {
			return snapshots[i].IndicatorID.String() < snapshots[j].IndicatorID.String()
		}
		return snapshots[i].OnDate.Before(snapshots[j].OnDate)
	})
}
