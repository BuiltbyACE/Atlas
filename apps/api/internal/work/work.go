// Package work owns Phase 2 work orchestration: projects, milestones, tasks,
// their dependencies and blockers (docs/ATLAS.md Phase 2). Go is the sole writer
// (ADR-002); every state change is audited and, where the orchestrator or the
// browser must react, an outbox event is written in the same transaction
// (ADR-005, ADR-009).
//
// Optimistic concurrency (§6.6): every write takes the version the client read
// and the store rejects a stale one with ErrConflict. Listings are keyset-paged.
package work

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Sentinel errors. The HTTP layer maps these onto problem responses.
var (
	// ErrNotFound marks an unknown project/milestone/task.
	ErrNotFound = errors.New("work: not found")
	// ErrInvalid marks a request that fails validation.
	ErrInvalid = errors.New("work: invalid")
	// ErrConflict marks a stale version or an illegal transition.
	ErrConflict = errors.New("work: conflict")
	// ErrForbidden marks an action the caller may not take.
	ErrForbidden = errors.New("work: forbidden")
)

// ProjectStatus mirrors the work.projects.status check constraint.
type ProjectStatus string

const (
	ProjectPlanning  ProjectStatus = "planning"
	ProjectActive    ProjectStatus = "active"
	ProjectPaused    ProjectStatus = "paused"
	ProjectCompleted ProjectStatus = "completed"
	ProjectCancelled ProjectStatus = "cancelled"
)

// Valid reports whether s is a known project status.
func (s ProjectStatus) Valid() bool {
	switch s {
	case ProjectPlanning, ProjectActive, ProjectPaused, ProjectCompleted, ProjectCancelled:
		return true
	}
	return false
}

// MilestoneStatus mirrors the work.milestones.status check constraint.
type MilestoneStatus string

const (
	MilestoneNotStarted MilestoneStatus = "not_started"
	MilestoneInProgress MilestoneStatus = "in_progress"
	MilestoneBlocked    MilestoneStatus = "blocked"
	MilestoneCompleted  MilestoneStatus = "completed"
	MilestoneCancelled  MilestoneStatus = "cancelled"
)

// Valid reports whether s is a known milestone status.
func (s MilestoneStatus) Valid() bool {
	switch s {
	case MilestoneNotStarted, MilestoneInProgress, MilestoneBlocked, MilestoneCompleted, MilestoneCancelled:
		return true
	}
	return false
}

// TaskStatus mirrors the work.tasks.status check constraint.
type TaskStatus string

const (
	TaskTodo       TaskStatus = "todo"
	TaskInProgress TaskStatus = "in_progress"
	TaskBlocked    TaskStatus = "blocked"
	TaskReview     TaskStatus = "review"
	TaskDone       TaskStatus = "done"
	TaskCancelled  TaskStatus = "cancelled"
)

// Valid reports whether s is a known task status.
func (s TaskStatus) Valid() bool {
	switch s {
	case TaskTodo, TaskInProgress, TaskBlocked, TaskReview, TaskDone, TaskCancelled:
		return true
	}
	return false
}

// Terminal reports whether no further work is expected on a task.
func (s TaskStatus) Terminal() bool { return s == TaskDone || s == TaskCancelled }

// Priority mirrors the work.tasks.priority check constraint.
type Priority string

const (
	PriorityLow    Priority = "low"
	PriorityMedium Priority = "medium"
	PriorityHigh   Priority = "high"
	PriorityUrgent Priority = "urgent"
)

// Valid reports whether p is a known priority.
func (p Priority) Valid() bool {
	switch p {
	case PriorityLow, PriorityMedium, PriorityHigh, PriorityUrgent:
		return true
	}
	return false
}

// DependencyType mirrors work.task_dependencies.dependency_type.
type DependencyType string

const (
	FinishToStart  DependencyType = "fs"
	StartToStart   DependencyType = "ss"
	FinishToFinish DependencyType = "ff"
	StartToFinish  DependencyType = "sf"
)

// Valid reports whether d is a known dependency type.
func (d DependencyType) Valid() bool {
	switch d {
	case FinishToStart, StartToStart, FinishToFinish, StartToFinish:
		return true
	}
	return false
}

// Project is a row in work.projects.
type Project struct {
	ID            uuid.UUID
	Name          string
	Description   string
	ProjectType   string
	Status        ProjectStatus
	OwnerPersonID *uuid.UUID
	DepartmentID  *uuid.UUID
	StartedAt     *time.Time
	DueAt         *time.Time
	CompletedAt   *time.Time
	Version       int
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// Milestone is a row in work.milestones.
type Milestone struct {
	ID          uuid.UUID
	ProjectID   uuid.UUID
	Name        string
	Description string
	Status      MilestoneStatus
	DueAt       *time.Time
	CompletedAt *time.Time
	Weight      int
	Version     int
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Task is a row in work.tasks.
type Task struct {
	ID               uuid.UUID
	ProjectID        uuid.UUID
	MilestoneID      *uuid.UUID
	ParentID         *uuid.UUID
	Title            string
	Description      string
	AssigneePersonID *uuid.UUID
	ReporterPersonID *uuid.UUID
	Status           TaskStatus
	Priority         Priority
	DueAt            *time.Time
	StartedAt        *time.Time
	CompletedAt      *time.Time
	Blocked          bool
	BlockedReason    string
	EstimateHours    *float64
	ActualHours      *float64
	Version          int
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// Blockers counts open blockers; used to decide whether a task is still blocked.
type Blocker struct {
	ID         uuid.UUID
	TaskID     uuid.UUID
	Reason     string
	CreatedBy  *uuid.UUID
	ResolvedBy *uuid.UUID
	ResolvedAt *time.Time
	CreatedAt  time.Time
}

// Evidence is a row in work.evidence: a core.files file attached to a task or a
// project, optionally verified by a reviewer (§Phase 2).
type Evidence struct {
	ID          uuid.UUID
	TaskID      *uuid.UUID
	ProjectID   *uuid.UUID
	FileID      uuid.UUID
	UploaderID  uuid.UUID
	Kind        string
	Description string
	VerifiedBy  *uuid.UUID
	VerifiedAt  *time.Time
	CreatedAt   time.Time
}

// EvidenceInput is the payload for attaching a file as evidence. Exactly one of
// TaskID / ProjectID must be set; when TaskID is set the project is derived.
type EvidenceInput struct {
	TaskID      *uuid.UUID
	ProjectID   *uuid.UUID
	FileID      uuid.UUID
	Kind        string
	Description string
}

// EvidenceFile is the minimal view of a stored file evidence needs. Keeping it
// local means the work package does not depend on the files package.
type EvidenceFile struct {
	ID       uuid.UUID
	Filename string
	Status   string
}

// FileResolver resolves a stored file for evidence. The files service is
// adapted to it in cmd wiring; a nil resolver disables availability checks.
type FileResolver interface {
	ResolveFile(ctx context.Context, id uuid.UUID) (*EvidenceFile, error)
}

// FileStatusAvailable is the core.files status a file must have to attach it as
// evidence (a pending upload or a quarantined object may not be linked).
const FileStatusAvailable = "available"

// Dependency is a row in work.task_dependencies.
type Dependency struct {
	PredecessorID uuid.UUID
	SuccessorID   uuid.UUID
	Type          DependencyType
	CreatedAt     time.Time
}

// ProjectMember is a row in work.project_members.
type ProjectMember struct {
	ProjectID uuid.UUID
	PersonID  uuid.UUID
	Role      string
	JoinedAt  time.Time
}

// Cursor is a keyset position in a project/task listing (created_at, id).
type Cursor struct {
	At time.Time
	ID uuid.UUID
}

// ListProjectsQuery filters a project listing.
type ListProjectsQuery struct {
	Status     ProjectStatus
	OwnerID    *uuid.UUID
	Department *uuid.UUID
}

// ListTasksQuery filters a task listing.
type ListTasksQuery struct {
	ProjectID   *uuid.UUID
	MilestoneID *uuid.UUID
	AssigneeID  *uuid.UUID
	Status      TaskStatus
	Blocked     *bool
	Overdue     bool
	Now         time.Time
}

// ListEvidenceQuery filters an evidence listing. Filtering by TaskID or
// ProjectID selects the evidence attached to that work item.
type ListEvidenceQuery struct {
	TaskID    *uuid.UUID
	ProjectID *uuid.UUID
}

func trim(s string) string { return strings.TrimSpace(s) }

// validateProject normalises a create/update payload.
func validateProject(p Project) (Project, error) {
	p.Name = trim(p.Name)
	if p.Name == "" {
		return p, fmt.Errorf("%w: name is required", ErrInvalid)
	}
	if len(p.Name) > 200 {
		return p, fmt.Errorf("%w: name is too long", ErrInvalid)
	}
	p.Description = trim(p.Description)
	p.ProjectType = trim(p.ProjectType)
	if p.Status == "" {
		p.Status = ProjectPlanning
	}
	if !p.Status.Valid() {
		return p, fmt.Errorf("%w: status must be planning|active|paused|completed|cancelled", ErrInvalid)
	}
	if p.DueAt != nil && p.StartedAt != nil && p.DueAt.Before(*p.StartedAt) {
		return p, fmt.Errorf("%w: due_at is before started_at", ErrInvalid)
	}
	return p, nil
}

// validateMilestone normalises a milestone payload.
func validateMilestone(m Milestone) (Milestone, error) {
	m.Name = trim(m.Name)
	if m.Name == "" {
		return m, fmt.Errorf("%w: name is required", ErrInvalid)
	}
	m.Description = trim(m.Description)
	if m.Status == "" {
		m.Status = MilestoneNotStarted
	}
	if !m.Status.Valid() {
		return m, fmt.Errorf("%w: status is invalid", ErrInvalid)
	}
	if m.Weight < 0 {
		return m, fmt.Errorf("%w: weight must not be negative", ErrInvalid)
	}
	return m, nil
}

// validateTask normalises a task payload.
func validateTask(t Task) (Task, error) {
	t.Title = trim(t.Title)
	if t.Title == "" {
		return t, fmt.Errorf("%w: title is required", ErrInvalid)
	}
	if len(t.Title) > 300 {
		return t, fmt.Errorf("%w: title is too long", ErrInvalid)
	}
	t.Description = trim(t.Description)
	t.BlockedReason = trim(t.BlockedReason)
	if t.Status == "" {
		t.Status = TaskTodo
	}
	if !t.Status.Valid() {
		return t, fmt.Errorf("%w: status is invalid", ErrInvalid)
	}
	if t.Priority == "" {
		t.Priority = PriorityMedium
	}
	if !t.Priority.Valid() {
		return t, fmt.Errorf("%w: priority must be low|medium|high|urgent", ErrInvalid)
	}
	if t.EstimateHours != nil && *t.EstimateHours < 0 {
		return t, fmt.Errorf("%w: estimate_hours must not be negative", ErrInvalid)
	}
	if t.ActualHours != nil && *t.ActualHours < 0 {
		return t, fmt.Errorf("%w: actual_hours must not be negative", ErrInvalid)
	}
	return t, nil
}

// uuidOrZero dereferences an optional id for event payloads.
func uuidOrZero(p *uuid.UUID) uuid.UUID {
	if p == nil {
		return uuid.UUID{}
	}
	return *p
}

// allowedProjectTransition reports whether cur -> next is permitted. Same-status
// moves never reach here (the caller only asks on a change).
func allowedProjectTransition(cur, next ProjectStatus) bool {
	switch cur {
	case ProjectPlanning:
		return next == ProjectActive || next == ProjectCancelled
	case ProjectActive:
		return next == ProjectPaused || next == ProjectCompleted || next == ProjectCancelled
	case ProjectPaused:
		return next == ProjectActive || next == ProjectCancelled
	case ProjectCompleted, ProjectCancelled:
		return false
	}
	return false
}

// allowedTaskTransition reports whether cur -> next is permitted.
func allowedTaskTransition(cur, next TaskStatus) bool {
	switch cur {
	case TaskTodo:
		return next == TaskInProgress || next == TaskBlocked || next == TaskCancelled
	case TaskInProgress:
		return next == TaskReview || next == TaskDone || next == TaskBlocked || next == TaskCancelled
	case TaskBlocked:
		return next == TaskInProgress || next == TaskTodo || next == TaskCancelled
	case TaskReview:
		return next == TaskInProgress || next == TaskDone || next == TaskCancelled
	case TaskDone, TaskCancelled:
		return false
	}
	return false
}
