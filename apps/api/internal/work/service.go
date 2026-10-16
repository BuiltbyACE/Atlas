package work

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/safaristack/atlas/apps/api/internal/audit"
	"github.com/safaristack/atlas/apps/api/internal/outbox"
)

// Entity types used in the audit trail and event aggregate names.
const (
	EntityProject   = "work.project"
	EntityMilestone = "work.milestone"
	EntityTask      = "work.task"
	EntityBlocker   = "work.blocker"
)

// Service coordinates work orchestration: validation, state transitions,
// optimistic concurrency, auditing and events.
type Service struct {
	store  Store
	audit  *audit.Writer
	now    func() time.Time
	logger *slog.Logger
}

// NewService builds the work service. aw may be nil in tests; changes are still
// applied, only the global audit write is skipped.
func NewService(store Store, aw *audit.Writer, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{store: store, audit: aw, now: func() time.Time { return time.Now().UTC() }, logger: logger}
}

// ProjectInput is the replacement payload for a project create/update (PUT
// semantics: omitted fields become their zero value).
type ProjectInput struct {
	Name          string
	Description   string
	ProjectType   string
	Status        ProjectStatus
	OwnerPersonID *uuid.UUID
	DepartmentID  *uuid.UUID
	StartedAt     *time.Time
	DueAt         *time.Time
	CompletedAt   *time.Time
}

// MilestoneInput is the replacement payload for a milestone create/update.
type MilestoneInput struct {
	Name        string
	Description string
	Status      MilestoneStatus
	DueAt       *time.Time
	CompletedAt *time.Time
	Weight      int
}

// TaskInput is the replacement payload for a task create/update.
type TaskInput struct {
	MilestoneID      *uuid.UUID
	ParentID         *uuid.UUID
	Title            string
	Description      string
	AssigneePersonID *uuid.UUID
	Status           TaskStatus
	Priority         Priority
	DueAt            *time.Time
	StartedAt        *time.Time
	CompletedAt      *time.Time
	EstimateHours    *float64
	ActualHours      *float64
}

// CreateProject validates and inserts a project, writing an audit row and a
// work.project.created.v1 event in the same transaction.
func (s *Service) CreateProject(ctx context.Context, actor uuid.UUID, in ProjectInput) (Project, error) {
	p, err := validateProject(Project{
		Name:          in.Name,
		Description:   in.Description,
		ProjectType:   in.ProjectType,
		Status:        in.Status,
		OwnerPersonID: in.OwnerPersonID,
		DepartmentID:  in.DepartmentID,
		StartedAt:     in.StartedAt,
		DueAt:         in.DueAt,
		CompletedAt:   in.CompletedAt,
		Version:       1,
	})
	if err != nil {
		return Project{}, err
	}
	id, err := uuid.NewV7()
	if err != nil {
		return Project{}, fmt.Errorf("work: new id: %w", err)
	}
	p.ID = id
	now := s.now()
	p.CreatedAt, p.UpdatedAt = now, now
	if p.StartedAt == nil && p.Status == ProjectActive {
		p.StartedAt = &now
	}
	if p.Status == ProjectCompleted {
		p.CompletedAt = &now
	}

	ev, err := outbox.NewProjectCreated(&actor, outbox.ProjectRef{
		ID: p.ID, Name: p.Name, Status: string(p.Status),
		OwnerID: uuidOrZero(p.OwnerPersonID), CreatedBy: actor,
	})
	if err != nil {
		return Project{}, err
	}
	created, err := s.store.CreateProject(ctx, p, &ev)
	if err != nil {
		return Project{}, err
	}
	s.writeAudit(ctx, actor, "work.project.created", EntityProject, created.ID, nil, projectSnapshot(created))
	return created, nil
}

// GetProject returns a project or ErrNotFound.
func (s *Service) GetProject(ctx context.Context, id uuid.UUID) (Project, error) {
	p, err := s.store.GetProject(ctx, id)
	if err != nil {
		return Project{}, err
	}
	if p == nil {
		return Project{}, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	return *p, nil
}

// ListProjects returns a page of projects, newest first.
func (s *Service) ListProjects(ctx context.Context, q ListProjectsQuery, limit int, before *Cursor) ([]Project, error) {
	if limit <= 0 {
		limit = 50
	}
	return s.store.ListProjects(ctx, q, limit, before)
}

// UpdateProject applies a replacement to an existing project. A status change
// must be an allowed transition and emits work.project.status_changed.v1.
func (s *Service) UpdateProject(ctx context.Context, actor uuid.UUID, id uuid.UUID, expectedVersion int, in ProjectInput) (Project, error) {
	cur, err := s.GetProject(ctx, id)
	if err != nil {
		return Project{}, err
	}

	next, err := validateProject(Project{
		ID:   id,
		Name: in.Name, Description: in.Description, ProjectType: in.ProjectType,
		Status: in.Status, OwnerPersonID: in.OwnerPersonID, DepartmentID: in.DepartmentID,
		StartedAt: in.StartedAt, DueAt: in.DueAt, CompletedAt: in.CompletedAt,
	})
	if err != nil {
		return Project{}, err
	}

	statusChanged := next.Status != cur.Status
	if statusChanged && !allowedProjectTransition(cur.Status, next.Status) {
		return Project{}, fmt.Errorf("%w: cannot move a project from %s to %s", ErrConflict, cur.Status, next.Status)
	}
	now := s.now()
	if statusChanged {
		applyProjectTimestamps(&next, cur, now)
	} else {
		// Preserve derived timestamps when the status did not change.
		next.StartedAt, next.CompletedAt = cur.StartedAt, cur.CompletedAt
	}

	var ev *outbox.Event
	if statusChanged {
		built, err := outbox.NewProjectStatusChanged(&actor, id, string(cur.Status), string(next.Status), actor)
		if err != nil {
			return Project{}, err
		}
		ev = &built
	}

	updated, err := s.store.UpdateProject(ctx, next, expectedVersion, ev)
	if err != nil {
		return Project{}, err
	}
	action := "work.project.updated"
	if statusChanged {
		action = "work.project.status_changed"
	}
	s.writeAudit(ctx, actor, action, EntityProject, updated.ID, projectSnapshot(cur), projectSnapshot(updated))
	return updated, nil
}

// AddMember grants a person a role on a project.
func (s *Service) AddMember(ctx context.Context, actor, projectID, personID uuid.UUID, role string) error {
	if _, err := s.GetProject(ctx, projectID); err != nil {
		return err
	}
	role = trim(role)
	if role == "" {
		role = "member"
	}
	if err := s.store.AddMember(ctx, ProjectMember{ProjectID: projectID, PersonID: personID, Role: role, JoinedAt: s.now()}); err != nil {
		return err
	}
	s.writeAudit(ctx, actor, "work.project.member_added", EntityProject, projectID, nil, map[string]any{
		"person_id": personID.String(), "role": role,
	})
	return nil
}

// RemoveMember revokes a person's membership on a project.
func (s *Service) RemoveMember(ctx context.Context, actor, projectID, personID uuid.UUID) error {
	if err := s.store.RemoveMember(ctx, projectID, personID); err != nil {
		return err
	}
	s.writeAudit(ctx, actor, "work.project.member_removed", EntityProject, projectID, map[string]any{
		"person_id": personID.String(),
	}, nil)
	return nil
}

// ListMembers returns a project's members.
func (s *Service) ListMembers(ctx context.Context, projectID uuid.UUID) ([]ProjectMember, error) {
	return s.store.ListMembers(ctx, projectID)
}

// CreateMilestone adds a milestone to a project.
func (s *Service) CreateMilestone(ctx context.Context, actor, projectID uuid.UUID, in MilestoneInput) (Milestone, error) {
	if _, err := s.GetProject(ctx, projectID); err != nil {
		return Milestone{}, err
	}
	m, err := validateMilestone(Milestone{
		ProjectID: projectID, Name: in.Name, Description: in.Description,
		Status: in.Status, DueAt: in.DueAt, CompletedAt: in.CompletedAt, Weight: in.Weight, Version: 1,
	})
	if err != nil {
		return Milestone{}, err
	}
	id, err := uuid.NewV7()
	if err != nil {
		return Milestone{}, fmt.Errorf("work: new id: %w", err)
	}
	m.ID = id
	now := s.now()
	m.CreatedAt, m.UpdatedAt = now, now
	if m.Status == MilestoneCompleted {
		m.CompletedAt = &now
	}
	created, err := s.store.CreateMilestone(ctx, m)
	if err != nil {
		return Milestone{}, err
	}
	s.writeAudit(ctx, actor, "work.milestone.created", EntityMilestone, created.ID, nil, milestoneSnapshot(created))
	return created, nil
}

// GetMilestone returns a milestone or ErrNotFound.
func (s *Service) GetMilestone(ctx context.Context, id uuid.UUID) (Milestone, error) {
	m, err := s.store.GetMilestone(ctx, id)
	if err != nil {
		return Milestone{}, err
	}
	if m == nil {
		return Milestone{}, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	return *m, nil
}

// ListMilestones returns a project's milestones, oldest first.
func (s *Service) ListMilestones(ctx context.Context, projectID uuid.UUID, limit int, before *Cursor) ([]Milestone, error) {
	if limit <= 0 {
		limit = 50
	}
	return s.store.ListMilestones(ctx, projectID, limit, before)
}

// UpdateMilestone applies a replacement to a milestone.
func (s *Service) UpdateMilestone(ctx context.Context, actor uuid.UUID, id uuid.UUID, expectedVersion int, in MilestoneInput) (Milestone, error) {
	cur, err := s.GetMilestone(ctx, id)
	if err != nil {
		return Milestone{}, err
	}
	next, err := validateMilestone(Milestone{
		ID: id, ProjectID: cur.ProjectID, Name: in.Name, Description: in.Description,
		Status: in.Status, DueAt: in.DueAt, CompletedAt: in.CompletedAt, Weight: in.Weight,
	})
	if err != nil {
		return Milestone{}, err
	}
	now := s.now()
	if next.Status == MilestoneCompleted && cur.Status != MilestoneCompleted && next.CompletedAt == nil {
		next.CompletedAt = &now
	}
	if next.Status != MilestoneCompleted {
		next.CompletedAt = cur.CompletedAt
	}
	updated, err := s.store.UpdateMilestone(ctx, next, expectedVersion)
	if err != nil {
		return Milestone{}, err
	}
	s.writeAudit(ctx, actor, "work.milestone.updated", EntityMilestone, updated.ID, milestoneSnapshot(cur), milestoneSnapshot(updated))
	return updated, nil
}

// CreateTask adds a task to a project, writing work.task.created.v1.
func (s *Service) CreateTask(ctx context.Context, actor, projectID uuid.UUID, in TaskInput) (Task, error) {
	if _, err := s.GetProject(ctx, projectID); err != nil {
		return Task{}, err
	}
	t, err := validateTask(Task{
		ProjectID: projectID, MilestoneID: in.MilestoneID, ParentID: in.ParentID,
		Title: in.Title, Description: in.Description, AssigneePersonID: in.AssigneePersonID,
		Status: in.Status, Priority: in.Priority, DueAt: in.DueAt, StartedAt: in.StartedAt,
		CompletedAt: in.CompletedAt, EstimateHours: in.EstimateHours, ActualHours: in.ActualHours, Version: 1,
	})
	if err != nil {
		return Task{}, err
	}
	if t.MilestoneID != nil {
		m, err := s.GetMilestone(ctx, *t.MilestoneID)
		if err != nil {
			return Task{}, err
		}
		if m.ProjectID != projectID {
			return Task{}, fmt.Errorf("%w: milestone is not on this project", ErrInvalid)
		}
	}
	id, err := uuid.NewV7()
	if err != nil {
		return Task{}, fmt.Errorf("work: new id: %w", err)
	}
	t.ID = id
	t.ReporterPersonID = &actor
	now := s.now()
	t.CreatedAt, t.UpdatedAt = now, now
	applyTaskTimestamps(&t, Task{}, now)

	ev, err := outbox.NewTaskCreated(&actor, outbox.TaskRef{
		ID: t.ID, ProjectID: t.ProjectID, Title: t.Title, Status: string(t.Status),
		AssigneeID: uuidOrZero(t.AssigneePersonID), CreatedBy: actor,
	})
	if err != nil {
		return Task{}, err
	}
	created, err := s.store.CreateTask(ctx, t, &ev)
	if err != nil {
		return Task{}, err
	}
	s.writeAudit(ctx, actor, "work.task.created", EntityTask, created.ID, nil, taskSnapshot(created))
	return created, nil
}

// GetTask returns a task or ErrNotFound.
func (s *Service) GetTask(ctx context.Context, id uuid.UUID) (Task, error) {
	t, err := s.store.GetTask(ctx, id)
	if err != nil {
		return Task{}, err
	}
	if t == nil {
		return Task{}, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	return *t, nil
}

// ListTasks returns a page of tasks, newest first.
func (s *Service) ListTasks(ctx context.Context, q ListTasksQuery, limit int, before *Cursor) ([]Task, error) {
	if limit <= 0 {
		limit = 50
	}
	return s.store.ListTasks(ctx, q, limit, before)
}

// UpdateTask applies a replacement to a task. A status change must be an allowed
// transition; an assignee change emits work.task.assigned.v1.
func (s *Service) UpdateTask(ctx context.Context, actor uuid.UUID, id uuid.UUID, expectedVersion int, in TaskInput) (Task, error) {
	cur, err := s.GetTask(ctx, id)
	if err != nil {
		return Task{}, err
	}
	next, err := validateTask(Task{
		ID: id, ProjectID: cur.ProjectID, MilestoneID: in.MilestoneID, ParentID: in.ParentID,
		Title: in.Title, Description: in.Description, AssigneePersonID: in.AssigneePersonID,
		Status: in.Status, Priority: in.Priority, DueAt: in.DueAt, StartedAt: in.StartedAt,
		CompletedAt: in.CompletedAt, EstimateHours: in.EstimateHours, ActualHours: in.ActualHours,
		Blocked: cur.Blocked, BlockedReason: cur.BlockedReason,
	})
	if err != nil {
		return Task{}, err
	}
	if next.MilestoneID != nil {
		m, err := s.GetMilestone(ctx, *next.MilestoneID)
		if err != nil {
			return Task{}, err
		}
		if m.ProjectID != cur.ProjectID {
			return Task{}, fmt.Errorf("%w: milestone is not on this project", ErrInvalid)
		}
	}
	if next.ParentID != nil && *next.ParentID == id {
		return Task{}, fmt.Errorf("%w: a task cannot be its own parent", ErrInvalid)
	}

	statusChanged := next.Status != cur.Status
	if statusChanged && !allowedTaskTransition(cur.Status, next.Status) {
		return Task{}, fmt.Errorf("%w: cannot move a task from %s to %s", ErrConflict, cur.Status, next.Status)
	}
	assigneeChanged := !sameUUID(next.AssigneePersonID, cur.AssigneePersonID)

	now := s.now()
	if statusChanged {
		applyTaskTimestamps(&next, cur, now)
	} else {
		next.StartedAt, next.CompletedAt = cur.StartedAt, cur.CompletedAt
	}

	var ev *outbox.Event
	switch {
	case statusChanged:
		built, err := outbox.NewTaskStatusChanged(&actor, id, cur.ProjectID, string(cur.Status), string(next.Status), actor)
		if err != nil {
			return Task{}, err
		}
		ev = &built
	case assigneeChanged:
		built, err := outbox.NewTaskAssigned(&actor, id, cur.ProjectID, uuidOrZero(next.AssigneePersonID), actor)
		if err != nil {
			return Task{}, err
		}
		ev = &built
	}

	updated, err := s.store.UpdateTask(ctx, next, expectedVersion, ev)
	if err != nil {
		return Task{}, err
	}
	switch {
	case statusChanged:
		s.writeAudit(ctx, actor, "work.task.status_changed", EntityTask, updated.ID, taskSnapshot(cur), taskSnapshot(updated))
	case assigneeChanged:
		s.writeAudit(ctx, actor, "work.task.assigned", EntityTask, updated.ID, taskSnapshot(cur), taskSnapshot(updated))
	default:
		s.writeAudit(ctx, actor, "work.task.updated", EntityTask, updated.ID, taskSnapshot(cur), taskSnapshot(updated))
	}
	return updated, nil
}

// AddDependency records that predecessor must precede successor. It rejects
// self-loops, duplicates and any edge that would create a cycle.
func (s *Service) AddDependency(ctx context.Context, actor, predecessor, successor uuid.UUID, dt DependencyType) error {
	if predecessor == successor {
		return fmt.Errorf("%w: a task cannot depend on itself", ErrInvalid)
	}
	if dt == "" {
		dt = FinishToStart
	}
	if !dt.Valid() {
		return fmt.Errorf("%w: dependency_type must be fs|ss|ff|sf", ErrInvalid)
	}
	if _, err := s.GetTask(ctx, predecessor); err != nil {
		return err
	}
	if _, err := s.GetTask(ctx, successor); err != nil {
		return err
	}
	exists, err := s.store.DependencyExists(ctx, predecessor, successor)
	if err != nil {
		return err
	}
	if exists {
		return fmt.Errorf("%w: dependency already exists", ErrConflict)
	}
	// A cycle appears iff the successor can already reach the predecessor.
	reaches, err := s.store.Reaches(ctx, successor, predecessor)
	if err != nil {
		return err
	}
	if reaches {
		return fmt.Errorf("%w: that dependency would create a cycle", ErrConflict)
	}
	if err := s.store.AddDependency(ctx, Dependency{
		PredecessorID: predecessor, SuccessorID: successor, Type: dt, CreatedAt: s.now(),
	}); err != nil {
		return err
	}
	s.writeAudit(ctx, actor, "work.task.dependency_added", EntityTask, successor, nil, map[string]any{
		"predecessor_id": predecessor.String(), "type": string(dt),
	})
	return nil
}

// RemoveDependency deletes a dependency edge.
func (s *Service) RemoveDependency(ctx context.Context, actor, predecessor, successor uuid.UUID) error {
	if err := s.store.RemoveDependency(ctx, predecessor, successor); err != nil {
		return err
	}
	s.writeAudit(ctx, actor, "work.task.dependency_removed", EntityTask, successor, map[string]any{
		"predecessor_id": predecessor.String(),
	}, nil)
	return nil
}

// RaiseBlocker records a blocker on a task and marks it blocked, emitting
// work.task.blocked.v1.
func (s *Service) RaiseBlocker(ctx context.Context, actor, taskID uuid.UUID, reason string) (Blocker, error) {
	t, err := s.GetTask(ctx, taskID)
	if err != nil {
		return Blocker{}, err
	}
	reason = trim(reason)
	if reason == "" {
		return Blocker{}, fmt.Errorf("%w: reason is required", ErrInvalid)
	}
	id, err := uuid.NewV7()
	if err != nil {
		return Blocker{}, fmt.Errorf("work: new id: %w", err)
	}
	ev, err := outbox.NewTaskBlocked(&actor, taskID, t.ProjectID, id, reason, actor)
	if err != nil {
		return Blocker{}, err
	}
	created, err := s.store.AddBlocker(ctx, Blocker{
		ID: id, TaskID: taskID, Reason: reason, CreatedBy: &actor, CreatedAt: s.now(),
	}, &ev)
	if err != nil {
		return Blocker{}, err
	}
	s.writeAudit(ctx, actor, "work.task.blocker_raised", EntityTask, taskID, nil, map[string]any{
		"blocker_id": created.ID.String(), "reason": reason,
	})
	return created, nil
}

// ResolveBlocker clears a blocker and, when it was the last open one, unblocks
// the task, emitting work.task.unblocked.v1 in the same transaction.
func (s *Service) ResolveBlocker(ctx context.Context, actor, blockerID uuid.UUID) (Blocker, error) {
	existing, err := s.store.GetBlocker(ctx, blockerID)
	if err != nil {
		return Blocker{}, err
	}
	if existing == nil {
		return Blocker{}, fmt.Errorf("%w: blocker %s", ErrNotFound, blockerID)
	}
	if existing.ResolvedAt != nil {
		return Blocker{}, fmt.Errorf("%w: blocker is already resolved", ErrConflict)
	}
	task, err := s.GetTask(ctx, existing.TaskID)
	if err != nil {
		return Blocker{}, err
	}
	ev, err := outbox.NewTaskUnblocked(&actor, existing.TaskID, task.ProjectID, existing.ID, actor)
	if err != nil {
		return Blocker{}, err
	}
	resolved, err := s.store.ResolveBlocker(ctx, blockerID, actor, &ev)
	if err != nil {
		return Blocker{}, err
	}
	s.writeAudit(ctx, actor, "work.task.blocker_resolved", EntityTask, resolved.TaskID,
		map[string]any{"blocker_id": resolved.ID.String()},
		map[string]any{"blocker_id": resolved.ID.String(), "resolved_by": actor.String()})
	return resolved, nil
}

// ListBlockers returns a task's blockers (open only unless includeResolved).
func (s *Service) ListBlockers(ctx context.Context, taskID uuid.UUID, includeResolved bool) ([]Blocker, error) {
	return s.store.ListBlockers(ctx, taskID, includeResolved)
}

// ── helpers ─────────────────────────────────────────────────────────────────

func sameUUID(a, b *uuid.UUID) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func applyProjectTimestamps(p *Project, cur Project, now time.Time) {
	// Keep an existing start; stamp a new one when work begins.
	p.StartedAt = cur.StartedAt
	if p.StartedAt == nil && (p.Status == ProjectActive || p.Status == ProjectCompleted) {
		p.StartedAt = &now
	}
	if p.Status == ProjectCompleted {
		if cur.CompletedAt != nil {
			p.CompletedAt = cur.CompletedAt
		} else {
			p.CompletedAt = &now
		}
	} else {
		p.CompletedAt = nil
	}
}

func applyTaskTimestamps(t *Task, cur Task, now time.Time) {
	t.StartedAt = cur.StartedAt
	if t.StartedAt == nil && (t.Status == TaskInProgress || t.Status == TaskReview || t.Status == TaskDone) {
		t.StartedAt = &now
	}
	if t.Status == TaskDone {
		if cur.CompletedAt != nil {
			t.CompletedAt = cur.CompletedAt
		} else {
			t.CompletedAt = &now
		}
	} else {
		t.CompletedAt = nil
	}
}

func projectSnapshot(p Project) map[string]any {
	return map[string]any{
		"name":            p.Name,
		"description":     p.Description,
		"project_type":    p.ProjectType,
		"status":          string(p.Status),
		"owner_person_id": uuidOrZero(p.OwnerPersonID).String(),
		"department_id":   uuidOrZero(p.DepartmentID).String(),
	}
}

func milestoneSnapshot(m Milestone) map[string]any {
	return map[string]any{
		"name":        m.Name,
		"description": m.Description,
		"status":      string(m.Status),
		"weight":      m.Weight,
	}
}

func taskSnapshot(t Task) map[string]any {
	return map[string]any{
		"title":              t.Title,
		"description":        t.Description,
		"status":             string(t.Status),
		"priority":           string(t.Priority),
		"assignee_person_id": uuidOrZero(t.AssigneePersonID).String(),
		"milestone_id":       uuidOrZero(t.MilestoneID).String(),
		"blocked":            t.Blocked,
	}
}

func (s *Service) writeAudit(ctx context.Context, actor uuid.UUID, action, entityType string, entityID uuid.UUID, before, after any) {
	if s.audit == nil {
		return
	}
	id := entityID
	if err := s.audit.Write(ctx, nil, audit.Event{
		ActorType:  audit.ActorUser,
		ActorID:    actor.String(),
		Action:     action,
		EntityType: entityType,
		EntityID:   &id,
		Before:     before,
		After:      after,
	}); err != nil {
		s.logger.ErrorContext(ctx, "work: audit write failed",
			slog.String("action", action), slog.String("error", err.Error()))
	}
}
