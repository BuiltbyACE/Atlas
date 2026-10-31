package intel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// rowScanner is what pgx's Row and Rows have in common.
type rowScanner interface {
	Scan(dest ...any) error
}

// Store is the intel read model (§8.8). Writes go through Writer, added when
// the decision/risk/brief lifecycle lands.
type Store interface {
	// Decisions.
	ListDecisions(ctx context.Context, q DecisionQuery, limit int, before *Cursor) ([]Decision, error)
	GetDecision(ctx context.Context, id uuid.UUID) (*Decision, error)

	// Risks.
	ListRisks(ctx context.Context, q RiskQuery, limit int, before *Cursor) ([]Risk, error)
	GetRisk(ctx context.Context, id uuid.UUID) (*Risk, error)

	// Briefs.
	ListBriefs(ctx context.Context, q BriefQuery, limit int, before *Cursor) ([]Brief, error)
	GetBrief(ctx context.Context, id uuid.UUID) (*Brief, error)

	// Metric snapshots.
	ListMetricSnapshots(ctx context.Context, q MetricSnapshotQuery, limit int, before *Cursor) ([]MetricSnapshot, error)
	GetMetricSnapshot(ctx context.Context, id uuid.UUID) (*MetricSnapshot, error)
}

// PGStore is the Postgres-backed read model.
type PGStore struct{ db *pgxpool.Pool }

// NewPGStore builds the read model over a pool.
func NewPGStore(db *pgxpool.Pool) *PGStore { return &PGStore{db: db} }

// ── decisions ───────────────────────────────────────────────────────────────

const decisionCols = `
	d.id, d.ref, d.question, d.context_links, d.options, d.recommendation,
	d.requested_by, COALESCE(p.full_name, ''), d.deciders, d.due_on, d.outcome,
	d.rationale, d.version, d.created_at, d.updated_at`

func scanDecision(row rowScanner) (Decision, error) {
	var d Decision
	var requestedBy *uuid.UUID
	err := row.Scan(&d.ID, &d.Ref, &d.Question, &d.ContextLinks, &d.Options,
		&d.Recommendation, &requestedBy, &d.RequestedByName, &d.Deciders,
		&d.DueOn, &d.Outcome, &d.Rationale, &d.Version, &d.CreatedAt, &d.UpdatedAt)
	if err != nil {
		return Decision{}, err
	}
	if requestedBy != nil {
		d.RequestedBy = requestedBy
	}
	return d, nil
}

func (s *PGStore) ListDecisions(ctx context.Context, q DecisionQuery, limit int, before *Cursor) ([]Decision, error) {
	args := []any{}
	where := "TRUE"
	add := func(cond string, v any) {
		args = append(args, v)
		where += fmt.Sprintf(" AND "+cond, len(args))
	}
	if q.Outcome != nil {
		add("d.outcome = $%d", string(*q.Outcome))
	}
	if q.Decider != nil {
		add("$%d = ANY(d.deciders)", *q.Decider)
	}
	if q.From != nil {
		add("d.created_at >= $%d", *q.From)
	}
	if q.To != nil {
		add("d.created_at <= $%d", *q.To)
	}
	if search := trim(q.Search); search != "" {
		add("(d.question ILIKE '%%' || $%[1]d || '%%' OR d.ref ILIKE '%%' || $%[1]d || '%%')", search)
	}
	if before != nil {
		args = append(args, before.At, before.ID)
		where += fmt.Sprintf(" AND (d.created_at, d.id) < ($%d, $%d)", len(args)-1, len(args))
	}
	args = append(args, limit)
	rows, err := s.db.Query(ctx, `SELECT `+decisionCols+` FROM intel.decisions d
		LEFT JOIN people.persons p ON p.id = d.requested_by WHERE `+where+
		` ORDER BY d.created_at DESC, d.id DESC LIMIT $`+fmt.Sprint(len(args)), args...)
	if err != nil {
		return nil, fmt.Errorf("intel: list decisions: %w", err)
	}
	defer rows.Close()
	out := []Decision{}
	for rows.Next() {
		d, err := scanDecision(rows)
		if err != nil {
			return nil, fmt.Errorf("intel: scan decision: %w", err)
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *PGStore) GetDecision(ctx context.Context, id uuid.UUID) (*Decision, error) {
	d, err := scanDecision(s.db.QueryRow(ctx, `SELECT `+decisionCols+` FROM intel.decisions d
		LEFT JOIN people.persons p ON p.id = d.requested_by WHERE d.id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("intel: get decision: %w", err)
	}
	return &d, nil
}

// ── risks ───────────────────────────────────────────────────────────────────

const riskCols = `
	r.id, r.ref, r.title, r.kind, r.owner_person_id, COALESCE(p.full_name, ''),
	r.likelihood, r.impact, r.mitigation, r.status, r.version, r.created_at, r.updated_at`

func scanRisk(row rowScanner) (Risk, error) {
	var r Risk
	var owner *uuid.UUID
	err := row.Scan(&r.ID, &r.Ref, &r.Title, &r.Kind, &owner, &r.OwnerName,
		&r.Likelihood, &r.Impact, &r.Mitigation, &r.Status, &r.Version,
		&r.CreatedAt, &r.UpdatedAt)
	if err != nil {
		return Risk{}, err
	}
	if owner != nil {
		r.OwnerPersonID = owner
	}
	return r, nil
}

func (s *PGStore) ListRisks(ctx context.Context, q RiskQuery, limit int, before *Cursor) ([]Risk, error) {
	args := []any{}
	where := "TRUE"
	add := func(cond string, v any) {
		args = append(args, v)
		where += fmt.Sprintf(" AND "+cond, len(args))
	}
	if q.Kind != nil {
		add("r.kind = $%d", string(*q.Kind))
	}
	if q.Status != nil {
		add("r.status = $%d", string(*q.Status))
	}
	if q.OwnerID != nil {
		add("r.owner_person_id = $%d", *q.OwnerID)
	}
	if search := trim(q.Search); search != "" {
		add("(r.title ILIKE '%%' || $%[1]d || '%%' OR r.ref ILIKE '%%' || $%[1]d || '%%')", search)
	}
	if before != nil {
		args = append(args, before.At, before.ID)
		where += fmt.Sprintf(" AND (r.created_at, r.id) < ($%d, $%d)", len(args)-1, len(args))
	}
	args = append(args, limit)
	rows, err := s.db.Query(ctx, `SELECT `+riskCols+` FROM intel.risks r
		LEFT JOIN people.persons p ON p.id = r.owner_person_id WHERE `+where+
		` ORDER BY r.created_at DESC, r.id DESC LIMIT $`+fmt.Sprint(len(args)), args...)
	if err != nil {
		return nil, fmt.Errorf("intel: list risks: %w", err)
	}
	defer rows.Close()
	out := []Risk{}
	for rows.Next() {
		r, err := scanRisk(rows)
		if err != nil {
			return nil, fmt.Errorf("intel: scan risk: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *PGStore) GetRisk(ctx context.Context, id uuid.UUID) (*Risk, error) {
	r, err := scanRisk(s.db.QueryRow(ctx, `SELECT `+riskCols+` FROM intel.risks r
		LEFT JOIN people.persons p ON p.id = r.owner_person_id WHERE r.id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("intel: get risk: %w", err)
	}
	return &r, nil
}

// ── briefs ──────────────────────────────────────────────────────────────────

const briefCols = `
	b.id, b.ref, b.for_date, b.summary, b.sections, b.delivered, b.delivered_at,
	b.version, b.created_at, b.updated_at`

func scanBrief(row rowScanner) (Brief, error) {
	var b Brief
	var sections []byte
	var deliveredAt *time.Time
	err := row.Scan(&b.ID, &b.Ref, &b.ForDate, &b.Summary, &sections, &b.Delivered,
		&deliveredAt, &b.Version, &b.CreatedAt, &b.UpdatedAt)
	if err != nil {
		return Brief{}, err
	}
	if len(sections) > 0 {
		if err := json.Unmarshal(sections, &b.Sections); err != nil {
			return Brief{}, err
		}
	}
	b.DeliveredAt = deliveredAt
	return b, nil
}

func (s *PGStore) ListBriefs(ctx context.Context, q BriefQuery, limit int, before *Cursor) ([]Brief, error) {
	args := []any{}
	where := "TRUE"
	add := func(cond string, v any) {
		args = append(args, v)
		where += fmt.Sprintf(" AND "+cond, len(args))
	}
	if q.From != nil {
		add("b.for_date >= $%d", *q.From)
	}
	if q.To != nil {
		add("b.for_date <= $%d", *q.To)
	}
	if search := trim(q.Search); search != "" {
		add("(b.summary ILIKE '%%' || $%[1]d || '%%' OR b.ref ILIKE '%%' || $%[1]d || '%%')", search)
	}
	if before != nil {
		args = append(args, before.At, before.ID)
		where += fmt.Sprintf(" AND (b.created_at, b.id) < ($%d, $%d)", len(args)-1, len(args))
	}
	args = append(args, limit)
	rows, err := s.db.Query(ctx, `SELECT `+briefCols+` FROM intel.briefs b WHERE `+where+
		` ORDER BY b.created_at DESC, b.id DESC LIMIT $`+fmt.Sprint(len(args)), args...)
	if err != nil {
		return nil, fmt.Errorf("intel: list briefs: %w", err)
	}
	defer rows.Close()
	out := []Brief{}
	for rows.Next() {
		b, err := scanBrief(rows)
		if err != nil {
			return nil, fmt.Errorf("intel: scan brief: %w", err)
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func (s *PGStore) GetBrief(ctx context.Context, id uuid.UUID) (*Brief, error) {
	b, err := scanBrief(s.db.QueryRow(ctx, `SELECT `+briefCols+` FROM intel.briefs b WHERE b.id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("intel: get brief: %w", err)
	}
	return &b, nil
}

// ── metric snapshots ────────────────────────────────────────────────────────

const metricSnapshotCols = `
	m.id, m.ref, m.metric_name, m.metric_value, m.as_of, m.source, m.version,
	m.created_at`

func scanMetricSnapshot(row rowScanner) (MetricSnapshot, error) {
	var m MetricSnapshot
	err := row.Scan(&m.ID, &m.Ref, &m.MetricName, &m.MetricValue, &m.AsOf,
		&m.Source, &m.Version, &m.CreatedAt)
	return m, err
}

func (s *PGStore) ListMetricSnapshots(ctx context.Context, q MetricSnapshotQuery, limit int, before *Cursor) ([]MetricSnapshot, error) {
	args := []any{}
	where := "TRUE"
	add := func(cond string, v any) {
		args = append(args, v)
		where += fmt.Sprintf(" AND "+cond, len(args))
	}
	if q.MetricName != nil {
		add("m.metric_name = $%d", *q.MetricName)
	}
	if q.From != nil {
		add("m.as_of >= $%d", *q.From)
	}
	if q.To != nil {
		add("m.as_of <= $%d", *q.To)
	}
	if search := trim(q.Search); search != "" {
		add("(m.metric_name ILIKE '%%' || $%[1]d || '%%' OR m.ref ILIKE '%%' || $%[1]d || '%%')", search)
	}
	if before != nil {
		args = append(args, before.At, before.ID)
		where += fmt.Sprintf(" AND (m.created_at, m.id) < ($%d, $%d)", len(args)-1, len(args))
	}
	args = append(args, limit)
	rows, err := s.db.Query(ctx, `SELECT `+metricSnapshotCols+` FROM intel.metric_snapshots m WHERE `+where+
		` ORDER BY m.created_at DESC, m.id DESC LIMIT $`+fmt.Sprint(len(args)), args...)
	if err != nil {
		return nil, fmt.Errorf("intel: list metric snapshots: %w", err)
	}
	defer rows.Close()
	out := []MetricSnapshot{}
	for rows.Next() {
		m, err := scanMetricSnapshot(rows)
		if err != nil {
			return nil, fmt.Errorf("intel: scan metric snapshot: %w", err)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *PGStore) GetMetricSnapshot(ctx context.Context, id uuid.UUID) (*MetricSnapshot, error) {
	m, err := scanMetricSnapshot(s.db.QueryRow(ctx, `SELECT `+metricSnapshotCols+` FROM intel.metric_snapshots m WHERE m.id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("intel: get metric snapshot: %w", err)
	}
	return &m, nil
}

// sortDecisions sorts decisions by created_at descending.
func sortDecisions(decisions []Decision) {
	sort.SliceStable(decisions, func(i, j int) bool {
		return decisions[i].CreatedAt.After(decisions[j].CreatedAt)
	})
}

// sortRisks sorts risks by created_at descending.
func sortRisks(risks []Risk) {
	sort.SliceStable(risks, func(i, j int) bool {
		return risks[i].CreatedAt.After(risks[j].CreatedAt)
	})
}
