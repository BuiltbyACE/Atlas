package outbox

import (
	"time"

	"github.com/google/uuid"
)

// Work event types (§7.4). Projects and tasks are the Phase 2 work spine; the
// orchestrator reacts to these to invalidate delivery views and nudge people
// (ADR-009). Go records the fact; Elixir decides what to push.
const (
	TypeProjectCreatedV1       = "work.project.created.v1"
	TypeProjectStatusChangedV1 = "work.project.status_changed.v1"
	TypeTaskCreatedV1          = "work.task.created.v1"
	TypeTaskStatusChangedV1    = "work.task.status_changed.v1"
	TypeTaskAssignedV1         = "work.task.assigned.v1"
	TypeTaskBlockedV1          = "work.task.blocked.v1"
	TypeTaskUnblockedV1        = "work.task.unblocked.v1"
)

// Aggregate types for work events.
const (
	AggregateProject = "project"
	AggregateTask    = "task"
)

// ProjectCreatedPayload is emitted when a project is created.
type ProjectCreatedPayload struct {
	ProjectID uuid.UUID `json:"project_id"`
	Name      string    `json:"name"`
	Status    string    `json:"status"`
	OwnerID   uuid.UUID `json:"owner_person_id,omitempty"`
	CreatedBy uuid.UUID `json:"created_by,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// ProjectStatusChangedPayload is emitted when a project's status changes.
type ProjectStatusChangedPayload struct {
	ProjectID uuid.UUID `json:"project_id"`
	From      string    `json:"from"`
	To        string    `json:"to"`
	ChangedBy uuid.UUID `json:"changed_by,omitempty"`
	ChangedAt time.Time `json:"changed_at"`
}

// TaskCreatedPayload is emitted when a task is created.
type TaskCreatedPayload struct {
	TaskID     uuid.UUID `json:"task_id"`
	ProjectID  uuid.UUID `json:"project_id"`
	Title      string    `json:"title"`
	Status     string    `json:"status"`
	AssigneeID uuid.UUID `json:"assignee_person_id,omitempty"`
	CreatedBy  uuid.UUID `json:"created_by,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

// TaskStatusChangedPayload is emitted on a task status transition.
type TaskStatusChangedPayload struct {
	TaskID    uuid.UUID `json:"task_id"`
	ProjectID uuid.UUID `json:"project_id"`
	From      string    `json:"from"`
	To        string    `json:"to"`
	ChangedBy uuid.UUID `json:"changed_by,omitempty"`
	ChangedAt time.Time `json:"changed_at"`
}

// TaskAssignedPayload is emitted when a task's assignee changes.
type TaskAssignedPayload struct {
	TaskID     uuid.UUID `json:"task_id"`
	ProjectID  uuid.UUID `json:"project_id"`
	AssigneeID uuid.UUID `json:"assignee_person_id,omitempty"`
	AssignedBy uuid.UUID `json:"assigned_by,omitempty"`
	AssignedAt time.Time `json:"assigned_at"`
}

// TaskBlockedPayload is emitted when a blocker is raised on a task.
type TaskBlockedPayload struct {
	TaskID    uuid.UUID `json:"task_id"`
	ProjectID uuid.UUID `json:"project_id"`
	BlockerID uuid.UUID `json:"blocker_id"`
	Reason    string    `json:"reason"`
	RaisedBy  uuid.UUID `json:"raised_by,omitempty"`
	RaisedAt  time.Time `json:"raised_at"`
}

// TaskUnblockedPayload is emitted when a task's last open blocker is resolved.
type TaskUnblockedPayload struct {
	TaskID     uuid.UUID `json:"task_id"`
	ProjectID  uuid.UUID `json:"project_id"`
	BlockerID  uuid.UUID `json:"blocker_id"`
	ResolvedBy uuid.UUID `json:"resolved_by,omitempty"`
	ResolvedAt time.Time `json:"resolved_at"`
}

// NewProjectCreated builds a work.project.created.v1 event.
func NewProjectCreated(actorID *uuid.UUID, p ProjectRef) (Event, error) {
	return buildEvent(TypeProjectCreatedV1, AggregateProject, p.ID, actorID, nil, ProjectCreatedPayload{
		ProjectID: p.ID,
		Name:      p.Name,
		Status:    p.Status,
		OwnerID:   p.OwnerID,
		CreatedBy: p.CreatedBy,
		CreatedAt: time.Now().UTC(),
	})
}

// NewProjectStatusChanged builds a work.project.status_changed.v1 event.
func NewProjectStatusChanged(actorID *uuid.UUID, id uuid.UUID, from, to string, changedBy uuid.UUID) (Event, error) {
	return buildEvent(TypeProjectStatusChangedV1, AggregateProject, id, actorID, nil, ProjectStatusChangedPayload{
		ProjectID: id,
		From:      from,
		To:        to,
		ChangedBy: changedBy,
		ChangedAt: time.Now().UTC(),
	})
}

// NewTaskCreated builds a work.task.created.v1 event.
func NewTaskCreated(actorID *uuid.UUID, t TaskRef) (Event, error) {
	return buildEvent(TypeTaskCreatedV1, AggregateTask, t.ID, actorID, nil, TaskCreatedPayload{
		TaskID:     t.ID,
		ProjectID:  t.ProjectID,
		Title:      t.Title,
		Status:     t.Status,
		AssigneeID: t.AssigneeID,
		CreatedBy:  t.CreatedBy,
		CreatedAt:  time.Now().UTC(),
	})
}

// NewTaskStatusChanged builds a work.task.status_changed.v1 event.
func NewTaskStatusChanged(actorID *uuid.UUID, id, projectID uuid.UUID, from, to string, changedBy uuid.UUID) (Event, error) {
	return buildEvent(TypeTaskStatusChangedV1, AggregateTask, id, actorID, nil, TaskStatusChangedPayload{
		TaskID:    id,
		ProjectID: projectID,
		From:      from,
		To:        to,
		ChangedBy: changedBy,
		ChangedAt: time.Now().UTC(),
	})
}

// NewTaskAssigned builds a work.task.assigned.v1 event.
func NewTaskAssigned(actorID *uuid.UUID, id, projectID, assignee uuid.UUID, assignedBy uuid.UUID) (Event, error) {
	return buildEvent(TypeTaskAssignedV1, AggregateTask, id, actorID, nil, TaskAssignedPayload{
		TaskID:     id,
		ProjectID:  projectID,
		AssigneeID: assignee,
		AssignedBy: assignedBy,
		AssignedAt: time.Now().UTC(),
	})
}

// NewTaskBlocked builds a work.task.blocked.v1 event.
func NewTaskBlocked(actorID *uuid.UUID, taskID, projectID, blockerID uuid.UUID, reason string, raisedBy uuid.UUID) (Event, error) {
	return buildEvent(TypeTaskBlockedV1, AggregateTask, taskID, actorID, nil, TaskBlockedPayload{
		TaskID:    taskID,
		ProjectID: projectID,
		BlockerID: blockerID,
		Reason:    reason,
		RaisedBy:  raisedBy,
		RaisedAt:  time.Now().UTC(),
	})
}

// NewTaskUnblocked builds a work.task.unblocked.v1 event.
func NewTaskUnblocked(actorID *uuid.UUID, taskID, projectID, blockerID uuid.UUID, resolvedBy uuid.UUID) (Event, error) {
	return buildEvent(TypeTaskUnblockedV1, AggregateTask, taskID, actorID, nil, TaskUnblockedPayload{
		TaskID:     taskID,
		ProjectID:  projectID,
		BlockerID:  blockerID,
		ResolvedBy: resolvedBy,
		ResolvedAt: time.Now().UTC(),
	})
}

// ProjectRef is the minimal project view the event builders need.
type ProjectRef struct {
	ID        uuid.UUID
	Name      string
	Status    string
	OwnerID   uuid.UUID
	CreatedBy uuid.UUID
}

// TaskRef is the minimal task view the event builders need.
type TaskRef struct {
	ID         uuid.UUID
	ProjectID  uuid.UUID
	Title      string
	Status     string
	AssigneeID uuid.UUID
	CreatedBy  uuid.UUID
}
