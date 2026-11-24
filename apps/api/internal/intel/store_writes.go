package intel

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/safaristack/atlas/apps/api/internal/outbox"
)

// Writer is the intel write model (§8.8). Go owns every rule: once a decision
// is decided it cannot be edited; a delivered brief cannot be redelivered.
type Writer interface {
	// AllocateRef allocates a human reference for a new record.
	AllocateRef(ctx context.Context, pType string, pYear int) (string, error)

	// Decisions.
	CreateDecision(ctx context.Context, d Decision, ev outbox.Event) (Decision, error)
	UpdateDecision(ctx context.Context, d Decision, expectedVersion int, ev outbox.Event) (Decision, error)
	DecideDecision(ctx context.Context, d Decision, expectedVersion int, ev outbox.Event) (Decision, error)

	// Risks.
	CreateRisk(ctx context.Context, r Risk, ev outbox.Event) (Risk, error)
	UpdateRisk(ctx context.Context, r Risk, expectedVersion int, ev outbox.Event) (Risk, error)

	// Briefs.
	CreateBrief(ctx context.Context, b Brief, ev outbox.Event) (Brief, error)
	UpdateBrief(ctx context.Context, b Brief, expectedVersion int, ev outbox.Event) (Brief, error)
	DeliverBrief(ctx context.Context, b Brief, expectedVersion int, ev outbox.Event) (Brief, error)

	// Metric snapshots.
	CreateMetricSnapshot(ctx context.Context, m MetricSnapshot, ev outbox.Event) (MetricSnapshot, error)
}

var _ Writer = (*PGStore)(nil)

// AllocateRef reserves the next human reference for a given type and year.
func (s *PGStore) AllocateRef(ctx context.Context, refType string, year int) (string, error) {
	var ref string
	if err := s.db.QueryRow(ctx, `SELECT core.next_reference($1, $2)`, refType, year).Scan(&ref); err != nil {
		return "", fmt.Errorf("intel: allocate ref %q: %w", refType, err)
	}
	return ref, nil
}

func writeErr(op string, err error) error {
	return fmt.Errorf("intel: %s: %w", op, err)
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

// ── decisions ───────────────────────────────────────────────────────────────

func (s *PGStore) CreateDecision(ctx context.Context, d Decision, ev outbox.Event) (Decision, error) {
	if !validDecisionOutcome(d.Outcome) {
		d.Outcome = DecisionOpen
	}
	ref, err := s.AllocateRef(ctx, "decision", time.Now().UTC().Year())
	if err != nil {
		return Decision{}, writeErr("allocate ref", err)
	}
	d.Ref = ref
	d.Version = 1
	err = s.db.QueryRow(ctx, `
		INSERT INTO intel.decisions (ref, question, context_links, options, recommendation, requested_by, deciders, due_on, outcome, rationale, version)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		RETURNING id, created_at, updated_at`,
		d.Ref, d.Question, d.ContextLinks, d.Options, d.Recommendation, d.RequestedBy,
		d.Deciders, d.DueOn, string(d.Outcome), d.Rationale, d.Version).
		Scan(&d.ID, &d.CreatedAt, &d.UpdatedAt)
	if err != nil {
		return Decision{}, writeErr("create decision", err)
	}
	if _, err := outbox.Write(ctx, s.db, ev); err != nil {
		return Decision{}, writeErr("write outbox event", err)
	}
	return d, nil
}

func (s *PGStore) UpdateDecision(ctx context.Context, d Decision, expectedVersion int, ev outbox.Event) (Decision, error) {
	current, err := s.GetDecision(ctx, d.ID)
	if err != nil {
		return Decision{}, err
	}
	if current == nil {
		return Decision{}, ErrNotFound
	}
	if current.Version != expectedVersion {
		return Decision{}, withCode(ErrConflict, CodeDecisionAlreadyDecided)
	}
	if !DecisionEditable(current.Outcome) {
		return Decision{}, withCode(ErrForbidden, CodeDecisionNotEditable)
	}
	d.Ref = current.Ref
	d.Version = current.Version + 1
	err = s.db.QueryRow(ctx, `
		UPDATE intel.decisions
		SET question = $1, context_links = $2, options = $3, recommendation = $4,
		    requested_by = $5, deciders = $6, due_on = $7, outcome = $8, rationale = $9, version = $10
		WHERE id = $11
		RETURNING updated_at`,
		d.Question, d.ContextLinks, d.Options, d.Recommendation, d.RequestedBy,
		d.Deciders, d.DueOn, string(d.Outcome), d.Rationale, d.Version, d.ID).
		Scan(&d.UpdatedAt)
	if err != nil {
		return Decision{}, writeErr("update decision", err)
	}
	if _, err := outbox.Write(ctx, s.db, ev); err != nil {
		return Decision{}, writeErr("write outbox event", err)
	}
	return d, nil
}

func (s *PGStore) DecideDecision(ctx context.Context, d Decision, expectedVersion int, ev outbox.Event) (Decision, error) {
	current, err := s.GetDecision(ctx, d.ID)
	if err != nil {
		return Decision{}, err
	}
	if current == nil {
		return Decision{}, ErrNotFound
	}
	if current.Version != expectedVersion {
		return Decision{}, withCode(ErrConflict, CodeDecisionAlreadyDecided)
	}
	if !DecisionEditable(current.Outcome) {
		return Decision{}, withCode(ErrForbidden, CodeDecisionNotEditable)
	}
	d.Ref = current.Ref
	d.Version = current.Version + 1
	err = s.db.QueryRow(ctx, `
		UPDATE intel.decisions
		SET outcome = $1, rationale = $2, version = $3
		WHERE id = $4
		RETURNING updated_at`,
		string(d.Outcome), d.Rationale, d.Version, d.ID).
		Scan(&d.UpdatedAt)
	if err != nil {
		return Decision{}, writeErr("decide decision", err)
	}
	if _, err := outbox.Write(ctx, s.db, ev); err != nil {
		return Decision{}, writeErr("write outbox event", err)
	}
	return d, nil
}

// ── risks ───────────────────────────────────────────────────────────────────

func (s *PGStore) CreateRisk(ctx context.Context, r Risk, ev outbox.Event) (Risk, error) {
	if !validRiskKind(r.Kind) {
		return Risk{}, withCode(errInvalidField("kind"), CodeInvalidRiskKind)
	}
	if !validRiskLikelihood(r.Likelihood) {
		return Risk{}, withCode(errInvalidField("likelihood"), CodeInvalidRiskLikelihood)
	}
	if !validRiskImpact(r.Impact) {
		return Risk{}, withCode(errInvalidField("impact"), CodeInvalidRiskImpact)
	}
	ref, err := s.AllocateRef(ctx, "risk", time.Now().UTC().Year())
	if err != nil {
		return Risk{}, writeErr("allocate ref", err)
	}
	r.Ref = ref
	r.Version = 1
	err = s.db.QueryRow(ctx, `
		INSERT INTO intel.risks (ref, title, kind, owner_person_id, likelihood, impact, mitigation, status, version)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		RETURNING id, created_at, updated_at`,
		r.Ref, r.Title, string(r.Kind), r.OwnerPersonID, string(r.Likelihood),
		string(r.Impact), r.Mitigation, string(r.Status), r.Version).
		Scan(&r.ID, &r.CreatedAt, &r.UpdatedAt)
	if err != nil {
		return Risk{}, writeErr("create risk", err)
	}
	if _, err := outbox.Write(ctx, s.db, ev); err != nil {
		return Risk{}, writeErr("write outbox event", err)
	}
	return r, nil
}

func (s *PGStore) UpdateRisk(ctx context.Context, r Risk, expectedVersion int, ev outbox.Event) (Risk, error) {
	current, err := s.GetRisk(ctx, r.ID)
	if err != nil {
		return Risk{}, err
	}
	if current == nil {
		return Risk{}, ErrNotFound
	}
	if current.Version != expectedVersion {
		return Risk{}, withCode(ErrConflict, CodeRiskNotEditable)
	}
	if !RiskEditable(current.Status) {
		return Risk{}, withCode(ErrForbidden, CodeRiskNotEditable)
	}
	r.Ref = current.Ref
	r.Version = current.Version + 1
	err = s.db.QueryRow(ctx, `
		UPDATE intel.risks
		SET title = $1, kind = $2, owner_person_id = $3, likelihood = $4, impact = $5,
		    mitigation = $6, status = $7, version = $8
		WHERE id = $9
		RETURNING updated_at`,
		r.Title, string(r.Kind), r.OwnerPersonID, string(r.Likelihood), string(r.Impact),
		r.Mitigation, string(r.Status), r.Version, r.ID).
		Scan(&r.UpdatedAt)
	if err != nil {
		return Risk{}, writeErr("update risk", err)
	}
	if _, err := outbox.Write(ctx, s.db, ev); err != nil {
		return Risk{}, writeErr("write outbox event", err)
	}
	return r, nil
}

// ── briefs ──────────────────────────────────────────────────────────────────

func (s *PGStore) CreateBrief(ctx context.Context, b Brief, ev outbox.Event) (Brief, error) {
	ref, err := s.AllocateRef(ctx, "brief", time.Now().UTC().Year())
	if err != nil {
		return Brief{}, writeErr("allocate ref", err)
	}
	b.Ref = ref
	b.Version = 1
	sections, err := json.Marshal(b.Sections)
	if err != nil {
		return Brief{}, writeErr("marshal sections", err)
	}
	err = s.db.QueryRow(ctx, `
		INSERT INTO intel.briefs (ref, for_date, summary, sections, delivered, version)
		VALUES ($1,$2,$3,$4,$5,$6)
		RETURNING id, created_at, updated_at`,
		b.Ref, b.ForDate, b.Summary, sections, b.Delivered, b.Version).
		Scan(&b.ID, &b.CreatedAt, &b.UpdatedAt)
	if err != nil {
		return Brief{}, writeErr("create brief", err)
	}
	if _, err := outbox.Write(ctx, s.db, ev); err != nil {
		return Brief{}, writeErr("write outbox event", err)
	}
	return b, nil
}

func (s *PGStore) UpdateBrief(ctx context.Context, b Brief, expectedVersion int, ev outbox.Event) (Brief, error) {
	current, err := s.GetBrief(ctx, b.ID)
	if err != nil {
		return Brief{}, err
	}
	if current == nil {
		return Brief{}, ErrNotFound
	}
	if current.Version != expectedVersion {
		return Brief{}, withCode(ErrConflict, CodeBriefNotEditable)
	}
	b.Ref = current.Ref
	b.Version = current.Version + 1
	sections, err := json.Marshal(b.Sections)
	if err != nil {
		return Brief{}, writeErr("marshal sections", err)
	}
	err = s.db.QueryRow(ctx, `
		UPDATE intel.briefs
		SET for_date = $1, summary = $2, sections = $3, delivered = $4, version = $5
		WHERE id = $6
		RETURNING updated_at`,
		b.ForDate, b.Summary, sections, b.Delivered, b.Version, b.ID).
		Scan(&b.UpdatedAt)
	if err != nil {
		return Brief{}, writeErr("update brief", err)
	}
	if _, err := outbox.Write(ctx, s.db, ev); err != nil {
		return Brief{}, writeErr("write outbox event", err)
	}
	return b, nil
}

func (s *PGStore) DeliverBrief(ctx context.Context, b Brief, expectedVersion int, ev outbox.Event) (Brief, error) {
	current, err := s.GetBrief(ctx, b.ID)
	if err != nil {
		return Brief{}, err
	}
	if current == nil {
		return Brief{}, ErrNotFound
	}
	if current.Version != expectedVersion {
		return Brief{}, withCode(ErrConflict, CodeBriefNotEditable)
	}
	if current.Delivered {
		return Brief{}, withCode(ErrConflict, CodeBriefAlreadyDelivered)
	}
	b.Ref = current.Ref
	b.Version = current.Version + 1
	now := time.Now().UTC()
	err = s.db.QueryRow(ctx, `
		UPDATE intel.briefs
		SET delivered = $1, delivered_at = $2, version = $3
		WHERE id = $4
		RETURNING updated_at`,
		b.Delivered, now, b.Version, b.ID).
		Scan(&b.UpdatedAt)
	if err != nil {
		return Brief{}, writeErr("deliver brief", err)
	}
	if _, err := outbox.Write(ctx, s.db, ev); err != nil {
		return Brief{}, writeErr("write outbox event", err)
	}
	b.DeliveredAt = &now
	return b, nil
}

// ── metric snapshots ────────────────────────────────────────────────────────

func (s *PGStore) CreateMetricSnapshot(ctx context.Context, m MetricSnapshot, ev outbox.Event) (MetricSnapshot, error) {
	ref, err := s.AllocateRef(ctx, "metric", time.Now().UTC().Year())
	if err != nil {
		return MetricSnapshot{}, writeErr("allocate ref", err)
	}
	m.Ref = ref
	m.Version = 1
	err = s.db.QueryRow(ctx, `
		INSERT INTO intel.metric_snapshots (ref, metric_name, metric_value, as_of, source, version)
		VALUES ($1,$2,$3,$4,$5,$6)
		RETURNING id, created_at`,
		m.Ref, m.MetricName, m.MetricValue, m.AsOf, m.Source, m.Version).
		Scan(&m.ID, &m.CreatedAt)
	if err != nil {
		return MetricSnapshot{}, writeErr("create metric snapshot", err)
	}
	if _, err := outbox.Write(ctx, s.db, ev); err != nil {
		return MetricSnapshot{}, writeErr("write outbox event", err)
	}
	return m, nil
}
