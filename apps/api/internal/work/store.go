package work

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/safaristack/atlas/apps/api/internal/identity"
	"github.com/safaristack/atlas/apps/api/internal/outbox"
)

// Store is the work persistence interface.
type Store interface {
	// AccountForPerson resolves a person to the user account a realtime nudge
	// is addressed to, or nil when the person has no account (§7.5).
	AccountForPerson(ctx context.Context, personID uuid.UUID) (*uuid.UUID, error)

	// Projects.
	CreateProject(ctx context.Context, p Project, ev *outbox.Event) (Project, error)
	GetProject(ctx context.Context, id uuid.UUID) (*Project, error)
	ListProjects(ctx context.Context, q ListProjectsQuery, limit int, before *Cursor) ([]Project, error)
	UpdateProject(ctx context.Context, p Project, expectedVersion int, ev *outbox.Event) (Project, error)
	AddMember(ctx context.Context, m ProjectMember) error
	RemoveMember(ctx context.Context, projectID, personID uuid.UUID) error
	ListMembers(ctx context.Context, projectID uuid.UUID) ([]ProjectMember, error)

	// Milestones.
	CreateMilestone(ctx context.Context, m Milestone) (Milestone, error)
	GetMilestone(ctx context.Context, id uuid.UUID) (*Milestone, error)
	ListMilestones(ctx context.Context, projectID uuid.UUID, limit int, before *Cursor) ([]Milestone, error)
	UpdateMilestone(ctx context.Context, m Milestone, expectedVersion int) (Milestone, error)

	// Tasks.
	CreateTask(ctx context.Context, t Task, ev *outbox.Event) (Task, error)
	GetTask(ctx context.Context, id uuid.UUID) (*Task, error)
	// TaskIDByRef resolves a human ref (PRJ-2026-007/T-0142) to its task.
	TaskIDByRef(ctx context.Context, ref string) (uuid.UUID, bool, error)
	ListTasks(ctx context.Context, q ListTasksQuery, limit int, before *Cursor) ([]Task, error)
	UpdateTask(ctx context.Context, t Task, expectedVersion int, ev *outbox.Event) (Task, error)
	AddDependency(ctx context.Context, d Dependency) error
	RemoveDependency(ctx context.Context, predecessor, successor uuid.UUID) error
	DependencyExists(ctx context.Context, predecessor, successor uuid.UUID) (bool, error)
	// Reaches reports whether there is a dependency path from `from` to `to`.
	// It is how cycle prevention works: a new predecessor→successor edge is a
	// cycle iff the successor can already reach the predecessor.
	Reaches(ctx context.Context, from, to uuid.UUID) (bool, error)

	// Blockers.
	AddBlocker(ctx context.Context, b Blocker, ev *outbox.Event) (Blocker, error)
	GetBlocker(ctx context.Context, blockerID uuid.UUID) (*Blocker, error)
	ResolveBlocker(ctx context.Context, blockerID uuid.UUID, resolvedBy uuid.UUID, ev *outbox.Event) (Blocker, error)
	ListBlockers(ctx context.Context, taskID uuid.UUID, includeResolved bool) ([]Blocker, error)
	CountOpenBlockers(ctx context.Context, taskID uuid.UUID) (int, error)

	// Evidence.
	CreateEvidence(ctx context.Context, e Evidence, ev *outbox.Event) (Evidence, error)
	GetEvidence(ctx context.Context, id uuid.UUID) (*Evidence, error)
	// EvidenceByReference finds a reference (D6: a merged PR URL) already
	// attached to a task, so a redelivered webhook reuses the row rather than
	// inserting a second.
	EvidenceByReference(ctx context.Context, taskID uuid.UUID, evidenceType, reference string) (*Evidence, error)
	ListEvidence(ctx context.Context, q ListEvidenceQuery) ([]Evidence, error)
	VerifyEvidence(ctx context.Context, id uuid.UUID, verifier uuid.UUID) (Evidence, error)
	DeleteEvidence(ctx context.Context, id uuid.UUID) error

	// Time entries (D7).
	CreateTimeEntry(ctx context.Context, e TimeEntry, ev *outbox.Event) (TimeEntry, error)
	GetTimeEntry(ctx context.Context, id uuid.UUID) (*TimeEntry, error)
	ListTimeEntries(ctx context.Context, q ListTimeEntriesQuery, limit int, before *Cursor) ([]TimeEntry, error)
	UpdateTimeEntry(ctx context.Context, e TimeEntry, expectedVersion int, ev *outbox.Event) (TimeEntry, error)
	DeleteTimeEntry(ctx context.Context, id uuid.UUID, ev *outbox.Event) error

	// Escalations.
	GetEscalation(ctx context.Context, id uuid.UUID) (*Escalation, error)
	ListEscalations(ctx context.Context, q ListEscalationsQuery, limit int, before *Cursor) ([]Escalation, error)
	ResolveEscalation(ctx context.Context, id, resolvedBy uuid.UUID, ev *outbox.Event) (Escalation, error)
	// SweepOverdueTasks claims a batch of overdue tasks that have not already
	// been escalated for their current deadline and raises one escalation per
	// task at the rung the D4 ladder says the task's overdue time has reached
	// (L1/L2/L3), writing the events, all in one transaction (FOR UPDATE SKIP
	// LOCKED) so a second worker skips them.
	SweepOverdueTasks(ctx context.Context, limit int, lad Ladder, l2Person *uuid.UUID, build func(t OverdueTask, esc Escalation) ([]outbox.Event, error)) ([]Escalation, error)
	// PromoteEscalations raises open escalations that have passed a higher rung
	// of the D4 ladder (L1 → L2 at E2, L2 → L3 at E2+E3), updating the row in
	// place so one overdue item is one queue entry, and writes the events in-tx.
	PromoteEscalations(ctx context.Context, limit int, lad Ladder, l2Person *uuid.UUID, build func(before, after Escalation) (outbox.Event, error)) ([]Escalation, error)
	// ResolveStaleEscalations resolves open escalations whose task is no longer
	// overdue (done, cancelled, or due again), writing each event in-tx.
	ResolveStaleEscalations(ctx context.Context, limit int, build func(e Escalation) (outbox.Event, error)) ([]Escalation, error)
	// PersonForRole resolves the person (via their account) granted a
	// company-scoped role — the D4 ladder uses it to find the domain CEO at L2.
	PersonForRole(ctx context.Context, roleKey string) (*uuid.UUID, error)
	// RaiseEscalationForEntity opens one escalation for an entity another module
	// owns (the people module's repeated-miss rule is the first caller). The
	// bool reports whether this call opened a new one; the event is written
	// in-tx only then.
	RaiseEscalationForEntity(ctx context.Context, entityType string, entityID uuid.UUID, level int, reason string, assignee *uuid.UUID, build func(e Escalation) (outbox.Event, error)) (*Escalation, bool, error)
	// ResolveEscalationsForEntity closes every open escalation for an entity
	// another module owns, writing each event in-tx.
	ResolveEscalationsForEntity(ctx context.Context, entityType string, entityID uuid.UUID, build func(e Escalation) (outbox.Event, error)) ([]Escalation, error)
	// OpenEscalationEntities returns the distinct entity ids with an open
	// escalation of the given type, so another module can resolve the ones whose
	// condition has cleared.
	OpenEscalationEntities(ctx context.Context, entityType string) ([]uuid.UUID, error)
	// LastEscalationResolvedAt returns when an entity's most recent escalation
	// was resolved, or nil when none ever was.
	LastEscalationResolvedAt(ctx context.Context, entityType string, entityID uuid.UUID) (*time.Time, error)
}

// PGStore implements Store over Postgres.
type PGStore struct{ db *pgxpool.Pool }

// NewPGStore builds a store over the pool.
func NewPGStore(db *pgxpool.Pool) *PGStore { return &PGStore{db: db} }

// AccountForPerson resolves the person's account via the shared identity
// bridge, so events carry the realtime address rather than the domain id.
func (s *PGStore) AccountForPerson(ctx context.Context, personID uuid.UUID) (*uuid.UUID, error) {
	return identity.AccountForPerson(ctx, s.db, personID)
}

// rowScanner is satisfied by both pgx.Row and pgx.Rows.
type rowScanner interface {
	Scan(dest ...any) error
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

const projectCols = `id, ref, name, COALESCE(description,''), COALESCE(project_type,''), status,
	owner_person_id, department_id, started_at, due_at, completed_at, version, created_at, updated_at`

func scanProject(row rowScanner) (Project, error) {
	var p Project
	var status string
	if err := row.Scan(&p.ID, &p.Ref, &p.Name, &p.Description, &p.ProjectType, &status,
		&p.OwnerPersonID, &p.DepartmentID, &p.StartedAt, &p.DueAt, &p.CompletedAt,
		&p.Version, &p.CreatedAt, &p.UpdatedAt); err != nil {
		return p, err
	}
	p.Status = ProjectStatus(status)
	return p, nil
}

func (s *PGStore) CreateProject(ctx context.Context, p Project, ev *outbox.Event) (Project, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Project{}, fmt.Errorf("work: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	ref, err := allocateProjectRef(ctx, tx, p.CreatedAt)
	if err != nil {
		return Project{}, err
	}

	created, err := scanProject(tx.QueryRow(ctx, `
		INSERT INTO work.projects (
			id, ref, name, description, project_type, status, owner_person_id, department_id,
			started_at, due_at, completed_at, version, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
		RETURNING `+projectCols,
		p.ID, ref, p.Name, nullIfEmpty(p.Description), nullIfEmpty(p.ProjectType), string(p.Status),
		p.OwnerPersonID, p.DepartmentID, p.StartedAt, p.DueAt, p.CompletedAt,
		p.Version, p.CreatedAt, p.UpdatedAt))
	if err != nil {
		return Project{}, fmt.Errorf("work: create project: %w", err)
	}
	if ev != nil {
		if _, err := outbox.Write(ctx, tx, *ev); err != nil {
			return Project{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Project{}, fmt.Errorf("work: commit: %w", err)
	}
	return created, nil
}

func (s *PGStore) GetProject(ctx context.Context, id uuid.UUID) (*Project, error) {
	p, err := scanProject(s.db.QueryRow(ctx, `SELECT `+projectCols+` FROM work.projects WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("work: get project: %w", err)
	}
	return &p, nil
}

func (s *PGStore) ListProjects(ctx context.Context, q ListProjectsQuery, limit int, before *Cursor) ([]Project, error) {
	where := make([]string, 0, 4)
	args := make([]any, 0, 5)
	add := func(clause string, v any) {
		args = append(args, v)
		where = append(where, fmt.Sprintf(clause, len(args)))
	}
	if q.Status != "" {
		add("status = $%d", string(q.Status))
	}
	if q.OwnerID != nil {
		add("owner_person_id = $%d", *q.OwnerID)
	}
	if q.Department != nil {
		add("department_id = $%d", *q.Department)
	}
	if before != nil {
		args = append(args, before.At.UTC(), before.ID)
		where = append(where, fmt.Sprintf("(created_at, id) < ($%d, $%d)", len(args)-1, len(args)))
	}
	args = append(args, limit)

	sql := `SELECT ` + projectCols + ` FROM work.projects`
	if len(where) > 0 {
		sql += " WHERE " + strings.Join(where, " AND ")
	}
	sql += fmt.Sprintf(" ORDER BY created_at DESC, id DESC LIMIT $%d", len(args))

	rows, err := s.db.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("work: list projects: %w", err)
	}
	defer rows.Close()
	out := make([]Project, 0, limit)
	for rows.Next() {
		p, err := scanProject(rows)
		if err != nil {
			return nil, fmt.Errorf("work: scan project: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *PGStore) UpdateProject(ctx context.Context, p Project, expectedVersion int, ev *outbox.Event) (Project, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Project{}, fmt.Errorf("work: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	updated, err := scanProject(tx.QueryRow(ctx, `
		UPDATE work.projects
		   SET name = $2, description = $3, project_type = $4, status = $5,
		       owner_person_id = $6, department_id = $7, started_at = $8, due_at = $9,
		       completed_at = $10, version = version + 1
		 WHERE id = $1 AND version = $11
		RETURNING `+projectCols,
		p.ID, p.Name, nullIfEmpty(p.Description), nullIfEmpty(p.ProjectType), string(p.Status),
		p.OwnerPersonID, p.DepartmentID, p.StartedAt, p.DueAt, p.CompletedAt, expectedVersion))
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return Project{}, s.versionConflict(ctx, tx, "work.projects", p.ID, expectedVersion)
	case err != nil:
		return Project{}, fmt.Errorf("work: update project: %w", err)
	}

	if ev != nil {
		if _, err := outbox.Write(ctx, tx, *ev); err != nil {
			return Project{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Project{}, fmt.Errorf("work: commit: %w", err)
	}
	return updated, nil
}

// versionConflict turns an empty UPDATE into the precise error: not found, or a
// stale version (§6.6).
func (s *PGStore) versionConflict(ctx context.Context, tx pgx.Tx, table string, id uuid.UUID, expected int) error {
	var cur int
	err := tx.QueryRow(ctx, `SELECT version FROM `+table+` WHERE id = $1`, id).Scan(&cur)
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	if err != nil {
		return fmt.Errorf("work: read version: %w", err)
	}
	return fmt.Errorf("%w: version %d is stale (current %d)", ErrConflict, expected, cur)
}

func (s *PGStore) AddMember(ctx context.Context, m ProjectMember) error {
	_, err := s.db.Exec(ctx, `
		INSERT INTO work.project_members (project_id, person_id, role, joined_at)
		VALUES ($1,$2,$3,$4)
		ON CONFLICT (project_id, person_id) DO UPDATE SET role = EXCLUDED.role`,
		m.ProjectID, m.PersonID, m.Role, m.JoinedAt)
	if err != nil {
		return fmt.Errorf("work: add member: %w", err)
	}
	return nil
}

func (s *PGStore) RemoveMember(ctx context.Context, projectID, personID uuid.UUID) error {
	tag, err := s.db.Exec(ctx, `DELETE FROM work.project_members WHERE project_id = $1 AND person_id = $2`, projectID, personID)
	if err != nil {
		return fmt.Errorf("work: remove member: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%w: member not found", ErrNotFound)
	}
	return nil
}

func (s *PGStore) ListMembers(ctx context.Context, projectID uuid.UUID) ([]ProjectMember, error) {
	rows, err := s.db.Query(ctx, `
		SELECT project_id, person_id, role, joined_at
		  FROM work.project_members WHERE project_id = $1
		 ORDER BY joined_at ASC, person_id ASC`, projectID)
	if err != nil {
		return nil, fmt.Errorf("work: list members: %w", err)
	}
	defer rows.Close()
	out := make([]ProjectMember, 0, 8)
	for rows.Next() {
		var m ProjectMember
		if err := rows.Scan(&m.ProjectID, &m.PersonID, &m.Role, &m.JoinedAt); err != nil {
			return nil, fmt.Errorf("work: scan member: %w", err)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

const milestoneCols = `id, project_id, name, COALESCE(description,''), status, due_at,
	completed_at, weight, version, created_at, updated_at`

func scanMilestone(row rowScanner) (Milestone, error) {
	var m Milestone
	var status string
	if err := row.Scan(&m.ID, &m.ProjectID, &m.Name, &m.Description, &status, &m.DueAt,
		&m.CompletedAt, &m.Weight, &m.Version, &m.CreatedAt, &m.UpdatedAt); err != nil {
		return m, err
	}
	m.Status = MilestoneStatus(status)
	return m, nil
}

func (s *PGStore) CreateMilestone(ctx context.Context, m Milestone) (Milestone, error) {
	created, err := scanMilestone(s.db.QueryRow(ctx, `
		INSERT INTO work.milestones (
			id, project_id, name, description, status, due_at, completed_at,
			weight, version, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		RETURNING `+milestoneCols,
		m.ID, m.ProjectID, m.Name, nullIfEmpty(m.Description), string(m.Status), m.DueAt,
		m.CompletedAt, m.Weight, m.Version, m.CreatedAt, m.UpdatedAt))
	if err != nil {
		return Milestone{}, fmt.Errorf("work: create milestone: %w", err)
	}
	return created, nil
}

func (s *PGStore) GetMilestone(ctx context.Context, id uuid.UUID) (*Milestone, error) {
	m, err := scanMilestone(s.db.QueryRow(ctx, `SELECT `+milestoneCols+` FROM work.milestones WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("work: get milestone: %w", err)
	}
	return &m, nil
}

func (s *PGStore) ListMilestones(ctx context.Context, projectID uuid.UUID, limit int, before *Cursor) ([]Milestone, error) {
	where := []string{"project_id = $1"}
	args := []any{projectID}
	if before != nil {
		args = append(args, before.At.UTC(), before.ID)
		where = append(where, fmt.Sprintf("(created_at, id) < ($%d, $%d)", len(args)-1, len(args)))
	}
	args = append(args, limit)
	sql := `SELECT ` + milestoneCols + ` FROM work.milestones WHERE ` + strings.Join(where, " AND ") +
		fmt.Sprintf(" ORDER BY created_at ASC, id ASC LIMIT $%d", len(args))
	rows, err := s.db.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("work: list milestones: %w", err)
	}
	defer rows.Close()
	out := make([]Milestone, 0, limit)
	for rows.Next() {
		m, err := scanMilestone(rows)
		if err != nil {
			return nil, fmt.Errorf("work: scan milestone: %w", err)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *PGStore) UpdateMilestone(ctx context.Context, m Milestone, expectedVersion int) (Milestone, error) {
	updated, err := scanMilestone(s.db.QueryRow(ctx, `
		UPDATE work.milestones
		   SET name = $2, description = $3, status = $4, due_at = $5,
		       completed_at = $6, weight = $7, version = version + 1
		 WHERE id = $1 AND version = $8
		RETURNING `+milestoneCols,
		m.ID, m.Name, nullIfEmpty(m.Description), string(m.Status), m.DueAt,
		m.CompletedAt, m.Weight, expectedVersion))
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		var cur int
		gerr := s.db.QueryRow(ctx, `SELECT version FROM work.milestones WHERE id = $1`, m.ID).Scan(&cur)
		if errors.Is(gerr, pgx.ErrNoRows) {
			return Milestone{}, fmt.Errorf("%w: %s", ErrNotFound, m.ID)
		}
		if gerr != nil {
			return Milestone{}, fmt.Errorf("work: read version: %w", gerr)
		}
		return Milestone{}, fmt.Errorf("%w: version %d is stale (current %d)", ErrConflict, expectedVersion, cur)
	case err != nil:
		return Milestone{}, fmt.Errorf("work: update milestone: %w", err)
	}
	return updated, nil
}

const taskCols = `id, ref, project_id, milestone_id, parent_id, title, COALESCE(description,''),
	assignee_person_id, reporter_person_id, status, priority, due_at, started_at, completed_at,
	blocked, COALESCE(blocked_reason,''), estimate_hours, actual_hours, version, created_at, updated_at`

func scanTask(row rowScanner) (Task, error) {
	var t Task
	var status, priority string
	if err := row.Scan(&t.ID, &t.Ref, &t.ProjectID, &t.MilestoneID, &t.ParentID, &t.Title, &t.Description,
		&t.AssigneePersonID, &t.ReporterPersonID, &status, &priority, &t.DueAt, &t.StartedAt,
		&t.CompletedAt, &t.Blocked, &t.BlockedReason, &t.EstimateHours, &t.ActualHours,
		&t.Version, &t.CreatedAt, &t.UpdatedAt); err != nil {
		return t, err
	}
	t.Status = TaskStatus(status)
	t.Priority = Priority(priority)
	return t, nil
}

func (s *PGStore) CreateTask(ctx context.Context, t Task, ev *outbox.Event) (Task, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Task{}, fmt.Errorf("work: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	ref, err := allocateTaskRef(ctx, tx, t.ProjectID)
	if err != nil {
		return Task{}, err
	}

	created, err := scanTask(tx.QueryRow(ctx, `
		INSERT INTO work.tasks (
			id, ref, project_id, milestone_id, parent_id, title, description,
			assignee_person_id, reporter_person_id, status, priority, due_at, started_at,
			completed_at, blocked, blocked_reason, estimate_hours, actual_hours,
			version, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21)
		RETURNING `+taskCols,
		t.ID, ref, t.ProjectID, t.MilestoneID, t.ParentID, t.Title, nullIfEmpty(t.Description),
		t.AssigneePersonID, t.ReporterPersonID, string(t.Status), string(t.Priority), t.DueAt,
		t.StartedAt, t.CompletedAt, t.Blocked, nullIfEmpty(t.BlockedReason), t.EstimateHours,
		t.ActualHours, t.Version, t.CreatedAt, t.UpdatedAt))
	if err != nil {
		return Task{}, fmt.Errorf("work: create task: %w", err)
	}
	if ev != nil {
		if _, err := outbox.Write(ctx, tx, *ev); err != nil {
			return Task{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Task{}, fmt.Errorf("work: commit: %w", err)
	}
	return created, nil
}

func (s *PGStore) GetTask(ctx context.Context, id uuid.UUID) (*Task, error) {
	t, err := scanTask(s.db.QueryRow(ctx, `SELECT `+taskCols+` FROM work.tasks WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("work: get task: %w", err)
	}
	return &t, nil
}

// TaskIDByRef resolves the human ref (PRJ-2026-007/T-0142) to its task id.
// found is false when no task carries it.
func (s *PGStore) TaskIDByRef(ctx context.Context, ref string) (uuid.UUID, bool, error) {
	var id uuid.UUID
	err := s.db.QueryRow(ctx, `SELECT id FROM work.tasks WHERE ref = $1`, ref).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, false, nil
	}
	if err != nil {
		return uuid.Nil, false, fmt.Errorf("work: resolve task ref: %w", err)
	}
	return id, true, nil
}

// allocateProjectRef bumps the per-year PRJ counter and formats it. Allocation
// runs in the caller's transaction, so a rolled-back create burns no ref.
func allocateProjectRef(ctx context.Context, tx pgx.Tx, at time.Time) (string, error) {
	if at.IsZero() {
		at = time.Now().UTC()
	}
	year := at.UTC().Format("2006")
	n, err := nextRef(ctx, tx, "PRJ-"+year)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("PRJ-%s-%03d", year, n), nil
}

// allocateTaskRef formats the next task as <project ref>/T-0001, mirroring the
// blueprint's PRJ-2026-007/T-0142. The task counter is global, so two projects
// never share an ordinal by accident.
func allocateTaskRef(ctx context.Context, tx pgx.Tx, projectID uuid.UUID) (string, error) {
	var projectRef string
	err := tx.QueryRow(ctx, `SELECT ref FROM work.projects WHERE id = $1`, projectID).Scan(&projectRef)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("%w: project %s", ErrNotFound, projectID)
	}
	if err != nil {
		return "", fmt.Errorf("work: read project ref: %w", err)
	}
	n, err := nextRef(ctx, tx, "T")
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s/T-%04d", projectRef, n), nil
}

// nextRef bumps a named counter in-tx and returns the new value. The scope
// ("PRJ-2026", "T") is the allocation domain; a missing scope starts at 1.
func nextRef(ctx context.Context, tx pgx.Tx, scope string) (int64, error) {
	var n int64
	err := tx.QueryRow(ctx, `
		INSERT INTO work.ref_counters (scope, value) VALUES ($1, 1)
		ON CONFLICT (scope) DO UPDATE SET value = work.ref_counters.value + 1
		RETURNING value`, scope).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("work: allocate ref %q: %w", scope, err)
	}
	return n, nil
}

func (s *PGStore) ListTasks(ctx context.Context, q ListTasksQuery, limit int, before *Cursor) ([]Task, error) {
	where := make([]string, 0, 6)
	args := make([]any, 0, 8)
	add := func(clause string, v any) {
		args = append(args, v)
		where = append(where, fmt.Sprintf(clause, len(args)))
	}
	if q.ProjectID != nil {
		add("project_id = $%d", *q.ProjectID)
	}
	if q.MilestoneID != nil {
		add("milestone_id = $%d", *q.MilestoneID)
	}
	if q.AssigneeID != nil {
		add("assignee_person_id = $%d", *q.AssigneeID)
	}
	if q.Status != "" {
		add("status = $%d", string(q.Status))
	}
	if q.Blocked != nil {
		add("blocked = $%d", *q.Blocked)
	}
	if q.Overdue {
		if q.Now.IsZero() {
			q.Now = time.Now().UTC()
		}
		args = append(args, q.Now)
		where = append(where, fmt.Sprintf("due_at IS NOT NULL AND due_at < $%d AND status NOT IN ('done','cancelled')", len(args)))
	}
	if before != nil {
		args = append(args, before.At.UTC(), before.ID)
		where = append(where, fmt.Sprintf("(created_at, id) < ($%d, $%d)", len(args)-1, len(args)))
	}
	args = append(args, limit)

	sql := `SELECT ` + taskCols + ` FROM work.tasks`
	if len(where) > 0 {
		sql += " WHERE " + strings.Join(where, " AND ")
	}
	sql += fmt.Sprintf(" ORDER BY created_at DESC, id DESC LIMIT $%d", len(args))

	rows, err := s.db.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("work: list tasks: %w", err)
	}
	defer rows.Close()
	out := make([]Task, 0, limit)
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return nil, fmt.Errorf("work: scan task: %w", err)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *PGStore) UpdateTask(ctx context.Context, t Task, expectedVersion int, ev *outbox.Event) (Task, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Task{}, fmt.Errorf("work: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	updated, err := scanTask(tx.QueryRow(ctx, `
		UPDATE work.tasks
		   SET milestone_id = $2, parent_id = $3, title = $4, description = $5,
		       assignee_person_id = $6, status = $7, priority = $8, due_at = $9,
		       started_at = $10, completed_at = $11, blocked = $12, blocked_reason = $13,
		       estimate_hours = $14, actual_hours = $15, version = version + 1
		 WHERE id = $1 AND version = $16
		RETURNING `+taskCols,
		t.ID, t.MilestoneID, t.ParentID, t.Title, nullIfEmpty(t.Description),
		t.AssigneePersonID, string(t.Status), string(t.Priority), t.DueAt, t.StartedAt,
		t.CompletedAt, t.Blocked, nullIfEmpty(t.BlockedReason), t.EstimateHours,
		t.ActualHours, expectedVersion))
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return Task{}, s.versionConflict(ctx, tx, "work.tasks", t.ID, expectedVersion)
	case err != nil:
		return Task{}, fmt.Errorf("work: update task: %w", err)
	}

	if ev != nil {
		if _, err := outbox.Write(ctx, tx, *ev); err != nil {
			return Task{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Task{}, fmt.Errorf("work: commit: %w", err)
	}
	return updated, nil
}

func (s *PGStore) AddDependency(ctx context.Context, d Dependency) error {
	_, err := s.db.Exec(ctx, `
		INSERT INTO work.task_dependencies (predecessor_id, successor_id, dependency_type, created_at)
		VALUES ($1,$2,$3,$4)`,
		d.PredecessorID, d.SuccessorID, string(d.Type), d.CreatedAt)
	if err != nil {
		return fmt.Errorf("work: add dependency: %w", err)
	}
	return nil
}

func (s *PGStore) RemoveDependency(ctx context.Context, predecessor, successor uuid.UUID) error {
	tag, err := s.db.Exec(ctx, `
		DELETE FROM work.task_dependencies WHERE predecessor_id = $1 AND successor_id = $2`,
		predecessor, successor)
	if err != nil {
		return fmt.Errorf("work: remove dependency: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%w: dependency not found", ErrNotFound)
	}
	return nil
}

func (s *PGStore) DependencyExists(ctx context.Context, predecessor, successor uuid.UUID) (bool, error) {
	var exists bool
	err := s.db.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM work.task_dependencies
			 WHERE predecessor_id = $1 AND successor_id = $2)`, predecessor, successor).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("work: dependency exists: %w", err)
	}
	return exists, nil
}

func (s *PGStore) Reaches(ctx context.Context, from, to uuid.UUID) (bool, error) {
	if from == to {
		return true, nil
	}
	var reaches bool
	err := s.db.QueryRow(ctx, `
		WITH RECURSIVE reach(node) AS (
			SELECT $1::uuid
			UNION
			SELECT d.successor_id
			  FROM work.task_dependencies d
			  JOIN reach r ON d.predecessor_id = r.node
		)
		SELECT EXISTS (SELECT 1 FROM reach WHERE node = $2)`, from, to).Scan(&reaches)
	if err != nil {
		return false, fmt.Errorf("work: reachability: %w", err)
	}
	return reaches, nil
}

const blockerCols = `id, task_id, reason, created_by_person_id, resolved_by_person_id, resolved_at, created_at`

func scanBlocker(row rowScanner) (Blocker, error) {
	var b Blocker
	if err := row.Scan(&b.ID, &b.TaskID, &b.Reason, &b.CreatedBy, &b.ResolvedBy, &b.ResolvedAt, &b.CreatedAt); err != nil {
		return b, err
	}
	return b, nil
}

func (s *PGStore) AddBlocker(ctx context.Context, b Blocker, ev *outbox.Event) (Blocker, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Blocker{}, fmt.Errorf("work: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	created, err := scanBlocker(tx.QueryRow(ctx, `
		INSERT INTO work.blockers (id, task_id, reason, created_by_person_id, created_at)
		VALUES ($1,$2,$3,$4,$5)
		RETURNING `+blockerCols,
		b.ID, b.TaskID, b.Reason, b.CreatedBy, b.CreatedAt))
	if err != nil {
		return Blocker{}, fmt.Errorf("work: add blocker: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE work.tasks SET blocked = true, blocked_reason = $2, version = version + 1
		 WHERE id = $1`, b.TaskID, b.Reason); err != nil {
		return Blocker{}, fmt.Errorf("work: mark blocked: %w", err)
	}
	if ev != nil {
		if _, err := outbox.Write(ctx, tx, *ev); err != nil {
			return Blocker{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Blocker{}, fmt.Errorf("work: commit: %w", err)
	}
	return created, nil
}

func (s *PGStore) ResolveBlocker(ctx context.Context, blockerID uuid.UUID, resolvedBy uuid.UUID, ev *outbox.Event) (Blocker, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Blocker{}, fmt.Errorf("work: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	updated, err := scanBlocker(tx.QueryRow(ctx, `
		UPDATE work.blockers
		   SET resolved_by_person_id = $2, resolved_at = now()
		 WHERE id = $1 AND resolved_at IS NULL
		RETURNING `+blockerCols, blockerID, resolvedBy))
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		// Either unknown, or already resolved.
		if _, gerr := scanBlocker(tx.QueryRow(ctx, `SELECT `+blockerCols+` FROM work.blockers WHERE id = $1`, blockerID)); errors.Is(gerr, pgx.ErrNoRows) {
			return Blocker{}, fmt.Errorf("%w: blocker %s", ErrNotFound, blockerID)
		}
		return Blocker{}, fmt.Errorf("%w: blocker is already resolved", ErrConflict)
	case err != nil:
		return Blocker{}, fmt.Errorf("work: resolve blocker: %w", err)
	}

	// If no open blockers remain, clear the task's blocked flag.
	var open int
	if err := tx.QueryRow(ctx, `
		SELECT count(*) FROM work.blockers WHERE task_id = $1 AND resolved_at IS NULL`,
		updated.TaskID).Scan(&open); err != nil {
		return Blocker{}, fmt.Errorf("work: count blockers: %w", err)
	}
	if open == 0 {
		if _, err := tx.Exec(ctx, `
			UPDATE work.tasks SET blocked = false, blocked_reason = NULL, version = version + 1
			 WHERE id = $1`, updated.TaskID); err != nil {
			return Blocker{}, fmt.Errorf("work: clear blocked: %w", err)
		}
	}

	if ev != nil {
		if _, err := outbox.Write(ctx, tx, *ev); err != nil {
			return Blocker{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Blocker{}, fmt.Errorf("work: commit: %w", err)
	}
	return updated, nil
}

func (s *PGStore) GetBlocker(ctx context.Context, blockerID uuid.UUID) (*Blocker, error) {
	b, err := scanBlocker(s.db.QueryRow(ctx, `SELECT `+blockerCols+` FROM work.blockers WHERE id = $1`, blockerID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("work: get blocker: %w", err)
	}
	return &b, nil
}

func (s *PGStore) ListBlockers(ctx context.Context, taskID uuid.UUID, includeResolved bool) ([]Blocker, error) {
	where := "task_id = $1"
	if !includeResolved {
		where += " AND resolved_at IS NULL"
	}
	rows, err := s.db.Query(ctx, `SELECT `+blockerCols+` FROM work.blockers WHERE `+where+` ORDER BY created_at DESC`, taskID)
	if err != nil {
		return nil, fmt.Errorf("work: list blockers: %w", err)
	}
	defer rows.Close()
	out := make([]Blocker, 0, 4)
	for rows.Next() {
		b, err := scanBlocker(rows)
		if err != nil {
			return nil, fmt.Errorf("work: scan blocker: %w", err)
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func (s *PGStore) CountOpenBlockers(ctx context.Context, taskID uuid.UUID) (int, error) {
	var n int
	if err := s.db.QueryRow(ctx, `
		SELECT count(*) FROM work.blockers WHERE task_id = $1 AND resolved_at IS NULL`, taskID).Scan(&n); err != nil {
		return 0, fmt.Errorf("work: count blockers: %w", err)
	}
	return n, nil
}

const evidenceCols = `id, task_id, project_id, file_id, uploader_person_id, evidence_type,
	COALESCE(reference,''), COALESCE(kind,''), COALESCE(description,''),
	verified_by_person_id, verified_at, created_at`

func scanEvidence(row rowScanner) (Evidence, error) {
	var e Evidence
	if err := row.Scan(&e.ID, &e.TaskID, &e.ProjectID, &e.FileID, &e.UploaderID,
		&e.EvidenceType, &e.Reference, &e.Kind, &e.Description,
		&e.VerifiedBy, &e.VerifiedAt, &e.CreatedAt); err != nil {
		return e, err
	}
	return e, nil
}

func (s *PGStore) CreateEvidence(ctx context.Context, e Evidence, ev *outbox.Event) (Evidence, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Evidence{}, fmt.Errorf("work: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	created, err := scanEvidence(tx.QueryRow(ctx, `
		INSERT INTO work.evidence (
			id, task_id, project_id, file_id, uploader_person_id, evidence_type, reference,
			kind, description, verified_by_person_id, verified_at, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
		RETURNING `+evidenceCols,
		e.ID, e.TaskID, e.ProjectID, e.FileID, e.UploaderID, e.EvidenceType,
		nullIfEmpty(e.Reference), nullIfEmpty(e.Kind), nullIfEmpty(e.Description),
		e.VerifiedBy, e.VerifiedAt, e.CreatedAt))
	if err != nil {
		return Evidence{}, fmt.Errorf("work: create evidence: %w", err)
	}
	if ev != nil {
		if _, err := outbox.Write(ctx, tx, *ev); err != nil {
			return Evidence{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Evidence{}, fmt.Errorf("work: commit: %w", err)
	}
	return created, nil
}

func (s *PGStore) GetEvidence(ctx context.Context, id uuid.UUID) (*Evidence, error) {
	e, err := scanEvidence(s.db.QueryRow(ctx, `SELECT `+evidenceCols+` FROM work.evidence WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("work: get evidence: %w", err)
	}
	return &e, nil
}

// EvidenceByReference returns the row already carrying (task, type, reference),
// or nil. It is the idempotent half of the D6 link: a redelivered webhook finds
// the evidence it recorded, so the delivery is not applied twice.
func (s *PGStore) EvidenceByReference(ctx context.Context, taskID uuid.UUID, evidenceType, reference string) (*Evidence, error) {
	ref := strings.TrimSpace(reference)
	if ref == "" {
		return nil, nil
	}
	e, err := scanEvidence(s.db.QueryRow(ctx, `
		SELECT `+evidenceCols+`
		  FROM work.evidence
		 WHERE task_id = $1 AND evidence_type = $2 AND reference = $3
		 ORDER BY created_at DESC, id DESC
		 LIMIT 1`, taskID, evidenceType, ref))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("work: find evidence by reference: %w", err)
	}
	return &e, nil
}

func (s *PGStore) ListEvidence(ctx context.Context, q ListEvidenceQuery) ([]Evidence, error) {
	where := make([]string, 0, 2)
	args := make([]any, 0, 2)
	if q.TaskID != nil {
		args = append(args, *q.TaskID)
		where = append(where, fmt.Sprintf("task_id = $%d", len(args)))
	}
	if q.ProjectID != nil {
		args = append(args, *q.ProjectID)
		where = append(where, fmt.Sprintf("project_id = $%d", len(args)))
	}
	sql := `SELECT ` + evidenceCols + ` FROM work.evidence`
	if len(where) > 0 {
		sql += " WHERE " + strings.Join(where, " AND ")
	}
	sql += " ORDER BY created_at DESC, id DESC"

	rows, err := s.db.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("work: list evidence: %w", err)
	}
	defer rows.Close()
	out := make([]Evidence, 0, 8)
	for rows.Next() {
		e, err := scanEvidence(rows)
		if err != nil {
			return nil, fmt.Errorf("work: scan evidence: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *PGStore) VerifyEvidence(ctx context.Context, id uuid.UUID, verifier uuid.UUID) (Evidence, error) {
	updated, err := scanEvidence(s.db.QueryRow(ctx, `
		UPDATE work.evidence
		   SET verified_by_person_id = $2, verified_at = now()
		 WHERE id = $1 AND verified_at IS NULL
		RETURNING `+evidenceCols, id, verifier))
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		var exists bool
		if gerr := s.db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM work.evidence WHERE id = $1)`, id).Scan(&exists); gerr != nil {
			return Evidence{}, fmt.Errorf("work: read evidence: %w", gerr)
		}
		if !exists {
			return Evidence{}, fmt.Errorf("%w: evidence %s", ErrNotFound, id)
		}
		return Evidence{}, fmt.Errorf("%w: evidence is already verified", ErrConflict)
	case err != nil:
		return Evidence{}, fmt.Errorf("work: verify evidence: %w", err)
	}
	return updated, nil
}

func (s *PGStore) DeleteEvidence(ctx context.Context, id uuid.UUID) error {
	tag, err := s.db.Exec(ctx, `DELETE FROM work.evidence WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("work: delete evidence: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%w: evidence %s", ErrNotFound, id)
	}
	return nil
}

// ── time entries (D7) ───────────────────────────────────────────────────────

const timeEntryCols = `id, person_id, task_id, project_id, entry_date, minutes,
	COALESCE(note,''), version, created_at, updated_at`

func scanTimeEntry(row rowScanner) (TimeEntry, error) {
	var e TimeEntry
	if err := row.Scan(&e.ID, &e.PersonID, &e.TaskID, &e.ProjectID, &e.EntryDate,
		&e.Minutes, &e.Note, &e.Version, &e.CreatedAt, &e.UpdatedAt); err != nil {
		return e, err
	}
	return e, nil
}

func (s *PGStore) CreateTimeEntry(ctx context.Context, e TimeEntry, ev *outbox.Event) (TimeEntry, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return TimeEntry{}, fmt.Errorf("work: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	created, err := scanTimeEntry(tx.QueryRow(ctx, `
		INSERT INTO work.time_entries (
			id, person_id, task_id, project_id, entry_date, minutes, note,
			version, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		RETURNING `+timeEntryCols,
		e.ID, e.PersonID, e.TaskID, e.ProjectID, e.EntryDate, e.Minutes,
		nullIfEmpty(e.Note), e.Version, e.CreatedAt, e.UpdatedAt))
	if err != nil {
		return TimeEntry{}, fmt.Errorf("work: create time entry: %w", err)
	}
	if ev != nil {
		if _, err := outbox.Write(ctx, tx, *ev); err != nil {
			return TimeEntry{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return TimeEntry{}, fmt.Errorf("work: commit: %w", err)
	}
	return created, nil
}

func (s *PGStore) GetTimeEntry(ctx context.Context, id uuid.UUID) (*TimeEntry, error) {
	e, err := scanTimeEntry(s.db.QueryRow(ctx, `SELECT `+timeEntryCols+` FROM work.time_entries WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("work: get time entry: %w", err)
	}
	return &e, nil
}

func (s *PGStore) ListTimeEntries(ctx context.Context, q ListTimeEntriesQuery, limit int, before *Cursor) ([]TimeEntry, error) {
	where := make([]string, 0, 6)
	args := make([]any, 0, 8)
	add := func(clause string, v any) {
		args = append(args, v)
		where = append(where, fmt.Sprintf(clause, len(args)))
	}
	if q.PersonID != nil {
		add("person_id = $%d", *q.PersonID)
	}
	if q.TaskID != nil {
		add("task_id = $%d", *q.TaskID)
	}
	if q.ProjectID != nil {
		add("project_id = $%d", *q.ProjectID)
	}
	if q.From != nil {
		add("entry_date >= $%d", dateOnly(*q.From))
	}
	if q.To != nil {
		add("entry_date <= $%d", dateOnly(*q.To))
	}
	if before != nil {
		args = append(args, before.At.UTC(), before.ID)
		where = append(where, fmt.Sprintf("(created_at, id) < ($%d, $%d)", len(args)-1, len(args)))
	}
	args = append(args, limit)

	sql := `SELECT ` + timeEntryCols + ` FROM work.time_entries`
	if len(where) > 0 {
		sql += " WHERE " + strings.Join(where, " AND ")
	}
	sql += fmt.Sprintf(" ORDER BY created_at DESC, id DESC LIMIT $%d", len(args))

	rows, err := s.db.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("work: list time entries: %w", err)
	}
	defer rows.Close()
	out := make([]TimeEntry, 0, limit)
	for rows.Next() {
		e, err := scanTimeEntry(rows)
		if err != nil {
			return nil, fmt.Errorf("work: scan time entry: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *PGStore) UpdateTimeEntry(ctx context.Context, e TimeEntry, expectedVersion int, ev *outbox.Event) (TimeEntry, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return TimeEntry{}, fmt.Errorf("work: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	updated, err := scanTimeEntry(tx.QueryRow(ctx, `
		UPDATE work.time_entries
		   SET person_id = $2, task_id = $3, project_id = $4, entry_date = $5,
		       minutes = $6, note = $7, version = version + 1
		 WHERE id = $1 AND version = $8
		RETURNING `+timeEntryCols,
		e.ID, e.PersonID, e.TaskID, e.ProjectID, e.EntryDate, e.Minutes,
		nullIfEmpty(e.Note), expectedVersion))
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return TimeEntry{}, s.versionConflict(ctx, tx, "work.time_entries", e.ID, expectedVersion)
	case err != nil:
		return TimeEntry{}, fmt.Errorf("work: update time entry: %w", err)
	}
	if ev != nil {
		if _, err := outbox.Write(ctx, tx, *ev); err != nil {
			return TimeEntry{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return TimeEntry{}, fmt.Errorf("work: commit: %w", err)
	}
	return updated, nil
}

func (s *PGStore) DeleteTimeEntry(ctx context.Context, id uuid.UUID, ev *outbox.Event) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("work: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	tag, err := tx.Exec(ctx, `DELETE FROM work.time_entries WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("work: delete time entry: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%w: time entry %s", ErrNotFound, id)
	}
	if ev != nil {
		if _, err := outbox.Write(ctx, tx, *ev); err != nil {
			return err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("work: commit: %w", err)
	}
	return nil
}

// ── escalations ─────────────────────────────────────────────────────────────

const escalationCols = `id, entity_type, entity_id, project_id, level, status,
	COALESCE(reason,''), assignee_person_id, raised_by_person_id, resolved_by_person_id,
	resolved_at, version, created_at, updated_at`

func scanEscalation(row rowScanner) (Escalation, error) {
	var e Escalation
	var status string
	if err := row.Scan(&e.ID, &e.EntityType, &e.EntityID, &e.ProjectID, &e.Level, &status,
		&e.Reason, &e.AssigneeID, &e.RaisedBy, &e.ResolvedBy, &e.ResolvedAt,
		&e.Version, &e.CreatedAt, &e.UpdatedAt); err != nil {
		return e, err
	}
	e.Status = EscalationStatus(status)
	return e, nil
}

// scanEscalations drains a result set of escalations (rows must not be open on
// the connection while more statements run, per pgx).
func scanEscalations(rows pgx.Rows) ([]Escalation, error) {
	defer rows.Close()
	out := make([]Escalation, 0, 2)
	for rows.Next() {
		e, err := scanEscalation(rows)
		if err != nil {
			return nil, fmt.Errorf("work: scan escalation: %w", err)
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *PGStore) GetEscalation(ctx context.Context, id uuid.UUID) (*Escalation, error) {
	e, err := scanEscalation(s.db.QueryRow(ctx, `SELECT `+escalationCols+` FROM work.escalations WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("work: get escalation: %w", err)
	}
	return &e, nil
}

func (s *PGStore) ListEscalations(ctx context.Context, q ListEscalationsQuery, limit int, before *Cursor) ([]Escalation, error) {
	where := make([]string, 0, 5)
	args := make([]any, 0, 6)
	add := func(clause string, v any) {
		args = append(args, v)
		where = append(where, fmt.Sprintf(clause, len(args)))
	}
	if q.EntityType != "" {
		add("entity_type = $%d", q.EntityType)
	}
	if q.EntityID != nil {
		add("entity_id = $%d", *q.EntityID)
	}
	if q.ProjectID != nil {
		add("project_id = $%d", *q.ProjectID)
	}
	if q.Status != "" {
		add("status = $%d", string(q.Status))
	}
	if before != nil {
		args = append(args, before.At.UTC(), before.ID)
		where = append(where, fmt.Sprintf("(created_at, id) < ($%d, $%d)", len(args)-1, len(args)))
	}
	args = append(args, limit)

	sql := `SELECT ` + escalationCols + ` FROM work.escalations`
	if len(where) > 0 {
		sql += " WHERE " + strings.Join(where, " AND ")
	}
	sql += fmt.Sprintf(" ORDER BY created_at DESC, id DESC LIMIT $%d", len(args))

	rows, err := s.db.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("work: list escalations: %w", err)
	}
	defer rows.Close()
	out := make([]Escalation, 0, 8)
	for rows.Next() {
		e, err := scanEscalation(rows)
		if err != nil {
			return nil, fmt.Errorf("work: scan escalation: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// ResolveEscalation closes an open escalation on behalf of a person. Resolving
// an escalation that is already resolved is a conflict, not a not-found.
func (s *PGStore) ResolveEscalation(ctx context.Context, id, resolvedBy uuid.UUID, ev *outbox.Event) (Escalation, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Escalation{}, fmt.Errorf("work: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	updated, err := scanEscalation(tx.QueryRow(ctx, `
		UPDATE work.escalations
		   SET status = 'resolved', resolved_by_person_id = $2, resolved_at = now(),
		       version = version + 1
		 WHERE id = $1 AND status = 'open'
		RETURNING `+escalationCols,
		id, resolvedBy))
	if errors.Is(err, pgx.ErrNoRows) {
		// Distinguish "missing" from "already resolved".
		if _, gerr := s.GetEscalation(ctx, id); gerr == nil {
			return Escalation{}, fmt.Errorf("%w: escalation %s is already resolved", ErrConflict, id)
		}
		return Escalation{}, fmt.Errorf("%w: escalation %s", ErrNotFound, id)
	}
	if err != nil {
		return Escalation{}, fmt.Errorf("work: resolve escalation: %w", err)
	}
	if ev != nil {
		if _, err := outbox.Write(ctx, tx, *ev); err != nil {
			return Escalation{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Escalation{}, fmt.Errorf("work: commit: %w", err)
	}
	return updated, nil
}

// SweepOverdueTasks claims a batch of overdue tasks that have not already been
// escalated for their current deadline, raises one escalation per task at the
// rung the D4 ladder says the task's overdue time has reached, and writes the
// caller's events — all in one transaction. The claim holds FOR UPDATE SKIP
// LOCKED on the task rows until the transaction commits, so two workers take
// disjoint, deterministic slices (oldest due first) and cannot raise the same
// escalation twice; the partial unique index on open escalations is the
// backstop.
//
// The rung is computed from due_at alone: below E1 the task is overdue but not
// yet on the ladder, at E2+E3 it lands directly on L3 (a sweep that was down
// for days does not walk the rungs one pass at a time). L1 goes to the project
// owner — the "owner's lead" of §5.8 — L2 to l2Person (the domain CEO), and L3
// to no one: it belongs to both CEOs' shared queue (ADR-016).
//
// "Current deadline" is what scopes an overdue episode: once an escalation has
// been raised at or after a task's due_at, that episode is closed and the sweep
// will not nag again unless the deadline is pushed out and passes afresh.
func (s *PGStore) SweepOverdueTasks(ctx context.Context, limit int, lad Ladder, l2Person *uuid.UUID, build func(t OverdueTask, esc Escalation) ([]outbox.Event, error)) ([]Escalation, error) {
	if limit <= 0 {
		limit = 100
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("work: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	rows, err := tx.Query(ctx, `
		SELECT t.id, t.project_id, t.title, t.assignee_person_id, p.owner_person_id, t.due_at,
		       CASE
		         WHEN now() - t.due_at >= make_interval(secs => ($3::float8 + $4::float8)) THEN 3
		         WHEN now() - t.due_at >= make_interval(secs => $3::float8) THEN 2
		         ELSE 1
		       END AS level
		  FROM work.tasks t
		  JOIN work.projects p ON p.id = t.project_id
		 WHERE t.status NOT IN ('done','cancelled')
		   AND t.due_at IS NOT NULL
		   AND t.due_at + make_interval(secs => $2::float8) <= now()
		   AND NOT EXISTS (
		         SELECT 1 FROM work.escalations e
		          WHERE e.entity_type = 'task' AND e.entity_id = t.id
		            AND e.created_at >= t.due_at)
		 ORDER BY t.due_at
		 LIMIT $1
		   FOR UPDATE OF t SKIP LOCKED`,
		limit, lad.L1After.Seconds(), lad.L2After.Seconds(), lad.L3After.Seconds())
	if err != nil {
		return nil, fmt.Errorf("work: claim overdue tasks: %w", err)
	}

	type claimedTask struct {
		t     OverdueTask
		level int
	}
	claimed := make([]claimedTask, 0, limit)
	for rows.Next() {
		var ct claimedTask
		if err := rows.Scan(&ct.t.ID, &ct.t.ProjectID, &ct.t.Title, &ct.t.AssigneeID, &ct.t.OwnerID, &ct.t.DueAt, &ct.level); err != nil {
			rows.Close()
			return nil, fmt.Errorf("work: scan overdue task: %w", err)
		}
		claimed = append(claimed, ct)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()

	now := time.Now().UTC()
	out := make([]Escalation, 0, len(claimed))
	for _, ct := range claimed {
		t := ct.t
		id, err := uuid.NewV7()
		if err != nil {
			return nil, fmt.Errorf("work: new escalation id: %w", err)
		}
		projectID := t.ProjectID
		var assignee *uuid.UUID
		switch ct.level {
		case 1:
			assignee = t.OwnerID
		case 2:
			assignee = l2Person
		default:
			assignee = nil // L3: the shared both-CEO queue has no single owner.
		}
		esc, err := scanEscalation(tx.QueryRow(ctx, `
			INSERT INTO work.escalations (
				id, entity_type, entity_id, project_id, level, status, reason,
				assignee_person_id, version, created_at, updated_at)
			VALUES ($1,'task',$2,$3,$4,'open',$5,$6,1,$7,$7)
			RETURNING `+escalationCols,
			id, t.ID, &projectID, ct.level,
			fmt.Sprintf("task overdue since %s", t.DueAt.UTC().Format(time.RFC3339)),
			assignee, now))
		if err != nil {
			return nil, fmt.Errorf("work: raise escalation: %w", err)
		}
		evs, err := build(t, esc)
		if err != nil {
			return nil, err
		}
		for _, ev := range evs {
			if _, err := outbox.Write(ctx, tx, ev); err != nil {
				return nil, err
			}
		}
		out = append(out, esc)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("work: commit: %w", err)
	}
	return out, nil
}

// PromoteEscalations walks open escalations up the D4 ladder: L1 → L2 once the
// task is overdue by E2, L2 → L3 once overdue by E2+E3. The row is updated in
// place (level, assignee, version), so one overdue item stays one entry in the
// shared queues and the escalation's own created_at keeps scoping its episode.
// Both rungs claim with FOR UPDATE SKIP LOCKED, and the claim only sees
// escalations whose task is still open and overdue — the resolve pass owns the
// ones whose task came back on time.
func (s *PGStore) PromoteEscalations(ctx context.Context, limit int, lad Ladder, l2Person *uuid.UUID, build func(before, after Escalation) (outbox.Event, error)) ([]Escalation, error) {
	if limit <= 0 {
		limit = 100
	}
	rungs := []struct {
		from, to  int
		threshold float64 // seconds overdue that promote to `to`
		assignee  *uuid.UUID
	}{
		{from: 1, to: 2, threshold: lad.L2After.Seconds(), assignee: l2Person},
		{from: 2, to: 3, threshold: (lad.L2After + lad.L3After).Seconds(), assignee: nil},
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("work: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	out := make([]Escalation, 0, 2)
	for _, rung := range rungs {
		rows, err := tx.Query(ctx, `
			SELECT e.id, e.assignee_person_id
			  FROM work.escalations e
			  JOIN work.tasks t ON t.id = e.entity_id
			 WHERE e.entity_type = 'task' AND e.status = 'open' AND e.level = $1
			   AND t.status NOT IN ('done','cancelled')
			   AND t.due_at IS NOT NULL AND t.due_at < now()
			   AND t.due_at + make_interval(secs => $2::float8) <= now()
			 ORDER BY t.due_at
			 LIMIT $3
			   FOR UPDATE OF e SKIP LOCKED`,
			rung.from, rung.threshold, limit)
		if err != nil {
			return nil, fmt.Errorf("work: claim escalations to promote: %w", err)
		}
		var ids []uuid.UUID
		before := map[uuid.UUID]*uuid.UUID{}
		for rows.Next() {
			var id uuid.UUID
			var prev *uuid.UUID
			if err := rows.Scan(&id, &prev); err != nil {
				rows.Close()
				return nil, fmt.Errorf("work: scan promotable escalation: %w", err)
			}
			ids = append(ids, id)
			before[id] = prev
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, err
		}
		rows.Close()
		if len(ids) == 0 {
			continue
		}

		updated, err := tx.Query(ctx, `
			UPDATE work.escalations
			   SET level = $2, assignee_person_id = $3, version = version + 1
			 WHERE id = ANY($1)
			RETURNING `+escalationCols, ids, rung.to, rung.assignee)
		if err != nil {
			return nil, fmt.Errorf("work: promote escalations: %w", err)
		}
		promoted, err := scanEscalations(updated)
		if err != nil {
			return nil, fmt.Errorf("work: promote escalations: %w", err)
		}
		for _, after := range promoted {
			// The before-snapshot is the row's state at the previous rung, so
			// the audit records the transition, not just the new level.
			beforeEsc := after
			beforeEsc.Level = rung.from
			beforeEsc.AssigneeID = before[after.ID]
			ev, err := build(beforeEsc, after)
			if err != nil {
				return nil, err
			}
			if _, err := outbox.Write(ctx, tx, ev); err != nil {
				return nil, err
			}
			out = append(out, after)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("work: commit: %w", err)
	}
	return out, nil
}

// PersonForRole resolves the person behind a company-scoped role grant — the
// D4 ladder's "domain CEO" lookup (ADR-016). The oldest active account wins so
// the target is deterministic when several people hold the role; nil, nil means
// nobody holds it and the rung has no single assignee.
func (s *PGStore) PersonForRole(ctx context.Context, roleKey string) (*uuid.UUID, error) {
	var id *uuid.UUID
	err := s.db.QueryRow(ctx, `
		SELECT a.person_id
		  FROM core.role_grants g
		  JOIN core.user_accounts a ON a.id = g.user_id
		 WHERE g.role_key = $1 AND g.scope_type = 'company' AND g.revoked_at IS NULL
		   AND a.status = 'active'
		 ORDER BY a.created_at
		 LIMIT 1`,
		roleKey).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("work: resolve person for role %s: %w", roleKey, err)
	}
	return id, nil
}

// OpenEscalationEntities returns the distinct entity ids that have an open
// escalation of the given type. Another module uses it to learn which of its
// entities are currently escalated, so it can resolve the ones whose condition
// has cleared (§8.5, D5).
func (s *PGStore) OpenEscalationEntities(ctx context.Context, entityType string) ([]uuid.UUID, error) {
	rows, err := s.db.Query(ctx, `
		SELECT DISTINCT entity_id
		  FROM work.escalations
		 WHERE entity_type = $1 AND status = 'open'
		 ORDER BY entity_id`, entityType)
	if err != nil {
		return nil, fmt.Errorf("work: list open escalation entities: %w", err)
	}
	defer rows.Close()
	out := make([]uuid.UUID, 0, 4)
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("work: scan escalation entity: %w", err)
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// LastEscalationResolvedAt returns the latest resolved_at among an entity's
// resolved escalations, or nil when none was ever resolved. The people module
// uses it to scope a repeated-miss episode: a person resolved by hand is not
// re-escalated until they miss another report (ADR-017).
func (s *PGStore) LastEscalationResolvedAt(ctx context.Context, entityType string, entityID uuid.UUID) (*time.Time, error) {
	var at *time.Time
	if err := s.db.QueryRow(ctx, `
		SELECT max(resolved_at)
		  FROM work.escalations
		 WHERE entity_type = $1 AND entity_id = $2 AND status = 'resolved'`,
		entityType, entityID).Scan(&at); err != nil {
		return nil, fmt.Errorf("work: last escalation resolution: %w", err)
	}
	return at, nil
}

// RaiseEscalationForEntity opens one escalation for an entity another module
// owns — the people module's repeated-miss escalation is the first caller
// (ADR-017). The ladder's one-open-per-entity rule (idx_work_escalations_open)
// makes a conflicting insert a no-op, so the bool reports whether this call
// opened a new one; the caller's event is written only then, in the same
// transaction.
func (s *PGStore) RaiseEscalationForEntity(ctx context.Context, entityType string, entityID uuid.UUID, level int, reason string, assignee *uuid.UUID, build func(e Escalation) (outbox.Event, error)) (*Escalation, bool, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("work: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	id, err := uuid.NewV7()
	if err != nil {
		return nil, false, fmt.Errorf("work: new escalation id: %w", err)
	}
	now := time.Now().UTC()
	esc, err := scanEscalation(tx.QueryRow(ctx, `
		INSERT INTO work.escalations (
			id, entity_type, entity_id, project_id, level, status, reason,
			assignee_person_id, version, created_at, updated_at)
		VALUES ($1,$2,$3,NULL,$4,'open',$5,$6,1,$7,$7)
		ON CONFLICT (entity_type, entity_id, level) WHERE status = 'open' DO NOTHING
		RETURNING `+escalationCols,
		id, entityType, entityID, level, reason, assignee, now))
	if errors.Is(err, pgx.ErrNoRows) {
		// An escalation for this entity and level is already open: nothing to
		// do, and no event to write.
		if err := tx.Commit(ctx); err != nil {
			return nil, false, fmt.Errorf("work: commit: %w", err)
		}
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("work: raise escalation: %w", err)
	}
	ev, err := build(esc)
	if err != nil {
		return nil, false, err
	}
	if _, err := outbox.Write(ctx, tx, ev); err != nil {
		return nil, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, false, fmt.Errorf("work: commit: %w", err)
	}
	return &esc, true, nil
}

// ResolveEscalationsForEntity closes every open escalation for an entity another
// module owns, writing each event in the same transaction, and returns the rows
// it closed so the caller can audit them.
func (s *PGStore) ResolveEscalationsForEntity(ctx context.Context, entityType string, entityID uuid.UUID, build func(e Escalation) (outbox.Event, error)) ([]Escalation, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("work: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	rows, err := tx.Query(ctx, `
		UPDATE work.escalations
		   SET status = 'resolved', resolved_at = now(), version = version + 1
		 WHERE entity_type = $1 AND entity_id = $2 AND status = 'open'
		RETURNING `+escalationCols,
		entityType, entityID)
	if err != nil {
		return nil, fmt.Errorf("work: resolve escalations: %w", err)
	}
	defer rows.Close()

	out := make([]Escalation, 0, 2)
	for rows.Next() {
		e, err := scanEscalation(rows)
		if err != nil {
			return nil, fmt.Errorf("work: scan escalation: %w", err)
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()

	// Build and write the events only after the result set is drained: pgx does
	// not allow another statement on the connection while rows are open.
	for _, e := range out {
		ev, err := build(e)
		if err != nil {
			return nil, err
		}
		if _, err := outbox.Write(ctx, tx, ev); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("work: commit: %w", err)
	}
	return out, nil
}

// ResolveStaleEscalations closes open escalations whose task is no longer
// overdue (done, cancelled, or given a future deadline again), writing each
// event in the same transaction. Resolutions are reported to the caller so the
// service can audit them.
func (s *PGStore) ResolveStaleEscalations(ctx context.Context, limit int, build func(e Escalation) (outbox.Event, error)) ([]Escalation, error) {
	if limit <= 0 {
		limit = 100
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("work: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	rows, err := tx.Query(ctx, `
		UPDATE work.escalations e
		   SET status = 'resolved', resolved_at = now(), version = version + 1
		 WHERE e.id IN (
		       SELECT e2.id
		         FROM work.escalations e2
		         JOIN work.tasks t ON t.id = e2.entity_id
		        WHERE e2.entity_type = 'task' AND e2.status = 'open'
		          AND (t.status IN ('done','cancelled') OR t.due_at IS NULL OR t.due_at >= now())
		        ORDER BY e2.created_at
		        LIMIT $1
		          FOR UPDATE OF e2 SKIP LOCKED)
		RETURNING `+escalationCols,
		limit)
	if err != nil {
		return nil, fmt.Errorf("work: resolve stale escalations: %w", err)
	}
	defer rows.Close()

	out := make([]Escalation, 0, limit)
	for rows.Next() {
		e, err := scanEscalation(rows)
		if err != nil {
			return nil, fmt.Errorf("work: scan escalation: %w", err)
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()

	// Build and write the events only after the result set is drained: pgx does
	// not allow another statement on the connection while rows are open.
	for _, e := range out {
		ev, err := build(e)
		if err != nil {
			return nil, err
		}
		if _, err := outbox.Write(ctx, tx, ev); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("work: commit: %w", err)
	}
	return out, nil
}
