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
	TypeTaskOverdueV1          = "work.task.overdue.v1"
	TypeEvidenceAddedV1        = "work.evidence.added.v1"
	TypeTimeEntryLoggedV1      = "work.time_entry.logged.v1"
	TypeTimeEntryUpdatedV1     = "work.time_entry.updated.v1"
	TypeTimeEntryRemovedV1     = "work.time_entry.removed.v1"
)

// Aggregate types for work events.
const (
	AggregateProject   = "project"
	AggregateTask      = "task"
	AggregateEvidence  = "evidence"
	AggregateTimeEntry = "time_entry"
)

// ProjectCreatedPayload is emitted when a project is created.
type ProjectCreatedPayload struct {
	ProjectID uuid.UUID `json:"project_id"`
	Name      string    `json:"name"`
	Status    string    `json:"status"`
	OwnerID   uuid.UUID `json:"owner_person_id,omitempty"`
	// OwnerUserID is the owner's account (core.user_accounts). The realtime bus
	// is keyed by account (§10.1); nil when the owner has no account yet.
	OwnerUserID *uuid.UUID `json:"owner_user_id,omitempty"`
	CreatedBy   uuid.UUID  `json:"created_by,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
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
	// AssigneeUserID is the assignee's account id, resolved by the producer
	// (§7.5); nil when the task is unassigned or the assignee has no account.
	AssigneeUserID *uuid.UUID `json:"assignee_user_id,omitempty"`
	CreatedBy      uuid.UUID  `json:"created_by,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
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
	// AssigneeUserID is the new assignee's account id (§7.5), nil when unknown.
	AssigneeUserID *uuid.UUID `json:"assignee_user_id,omitempty"`
	AssignedBy     uuid.UUID  `json:"assigned_by,omitempty"`
	AssignedAt     time.Time  `json:"assigned_at"`
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

// TaskOverduePayload is emitted the first time a task passes its deadline.
type TaskOverduePayload struct {
	TaskID     uuid.UUID  `json:"task_id"`
	ProjectID  uuid.UUID  `json:"project_id"`
	Title      string     `json:"title"`
	DueAt      time.Time  `json:"due_at"`
	AssigneeID *uuid.UUID `json:"assignee_person_id,omitempty"`
	// AssigneeUserID is the overdue assignee's account id (§7.5), nil when the
	// task is unassigned or the assignee has no account.
	AssigneeUserID *uuid.UUID `json:"assignee_user_id,omitempty"`
	EscalationID   uuid.UUID  `json:"escalation_id"`
	Level          int        `json:"level"`
	OverdueAt      time.Time  `json:"overdue_at"`
}

// NewProjectCreated builds a work.project.created.v1 event.
func NewProjectCreated(actorID *uuid.UUID, p ProjectRef) (Event, error) {
	return buildEvent(TypeProjectCreatedV1, AggregateProject, p.ID, actorID, nil, ProjectCreatedPayload{
		ProjectID:   p.ID,
		Name:        p.Name,
		Status:      p.Status,
		OwnerID:     p.OwnerID,
		OwnerUserID: p.OwnerUserID,
		CreatedBy:   p.CreatedBy,
		CreatedAt:   time.Now().UTC(),
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
		TaskID:         t.ID,
		ProjectID:      t.ProjectID,
		Title:          t.Title,
		Status:         t.Status,
		AssigneeID:     t.AssigneeID,
		AssigneeUserID: t.AssigneeUserID,
		CreatedBy:      t.CreatedBy,
		CreatedAt:      time.Now().UTC(),
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

// NewTaskAssigned builds a work.task.assigned.v1 event. assigneeUserID is the
// new assignee's account id (nil when the assignee has no account), already
// resolved by the caller so the event carries the realtime address (§7.5).
func NewTaskAssigned(actorID *uuid.UUID, id, projectID, assignee uuid.UUID, assignedBy uuid.UUID, assigneeUserID *uuid.UUID) (Event, error) {
	return buildEvent(TypeTaskAssignedV1, AggregateTask, id, actorID, nil, TaskAssignedPayload{
		TaskID:         id,
		ProjectID:      projectID,
		AssigneeID:     assignee,
		AssigneeUserID: assigneeUserID,
		AssignedBy:     assignedBy,
		AssignedAt:     time.Now().UTC(),
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

// TaskOverdueRef is the minimal view the overdue event builder needs.
type TaskOverdueRef struct {
	ID         uuid.UUID
	ProjectID  uuid.UUID
	Title      string
	DueAt      time.Time
	AssigneeID *uuid.UUID
	// AssigneeUserID is the assignee's account id, resolved by the producer.
	AssigneeUserID *uuid.UUID
	EscalationID   uuid.UUID
	Level          int
}

// NewTaskOverdue builds a work.task.overdue.v1 event.
func NewTaskOverdue(actorID *uuid.UUID, t TaskOverdueRef) (Event, error) {
	return buildEvent(TypeTaskOverdueV1, AggregateTask, t.ID, actorID, nil, TaskOverduePayload{
		TaskID:         t.ID,
		ProjectID:      t.ProjectID,
		Title:          t.Title,
		DueAt:          t.DueAt,
		AssigneeID:     t.AssigneeID,
		AssigneeUserID: t.AssigneeUserID,
		EscalationID:   t.EscalationID,
		Level:          t.Level,
		OverdueAt:      time.Now().UTC(),
	})
}

// EvidenceRef is the minimal view the evidence event builder needs (the
// polymorphic §6.5 row: a stored file or an external reference).
type EvidenceRef struct {
	ID           uuid.UUID
	TaskID       uuid.UUID
	ProjectID    uuid.UUID
	FileID       *uuid.UUID
	UploaderID   *uuid.UUID
	EvidenceType string
	Reference    string
	Kind         string
}

// NewEvidenceAdded builds a work.evidence.added.v1 event. actorID is the
// uploader or nil when an integration (§6.5 pull_request/payment_reference)
// recorded it.
func NewEvidenceAdded(actorID *uuid.UUID, e EvidenceRef) (Event, error) {
	return buildEvent(TypeEvidenceAddedV1, AggregateEvidence, e.ID, actorID, nil, EvidenceAddedPayload{
		EvidenceID:   e.ID,
		EvidenceType: e.EvidenceType,
		FileID:       e.FileID,
		Reference:    e.Reference,
		TaskID:       e.TaskID,
		ProjectID:    e.ProjectID,
		Kind:         e.Kind,
		UploaderID:   e.UploaderID,
		AddedAt:      time.Now().UTC(),
	})
}

// EvidenceAddedPayload is emitted when evidence is attached to a task or
// project. FileID and Reference are mutually exclusive per §6.5; the client
// fetches the row rather than reconstructing it from the event (ADR-009).
type EvidenceAddedPayload struct {
	EvidenceID   uuid.UUID  `json:"evidence_id"`
	EvidenceType string     `json:"evidence_type"`
	FileID       *uuid.UUID `json:"file_id,omitempty"`
	Reference    string     `json:"reference,omitempty"`
	TaskID       uuid.UUID  `json:"task_id,omitempty"`
	ProjectID    uuid.UUID  `json:"project_id,omitempty"`
	Kind         string     `json:"kind,omitempty"`
	UploaderID   *uuid.UUID `json:"uploader_person_id,omitempty"`
	AddedAt      time.Time  `json:"added_at"`
}

// TimeEntryChangedPayload is emitted when a time entry is logged, edited or
// removed (decision D7, docs/adr/ADR-019). Elixir only invalidates views from
// it; the hours themselves are data, fetched from Go (§7.4, ADR-009).
type TimeEntryChangedPayload struct {
	TimeEntryID uuid.UUID `json:"time_entry_id"`
	ProjectID   uuid.UUID `json:"project_id"`
	TaskID      uuid.UUID `json:"task_id"`
	PersonID    uuid.UUID `json:"person_id"`
	EntryDate   string    `json:"entry_date"`
	Minutes     int       `json:"minutes"`
	ChangedBy   uuid.UUID `json:"changed_by,omitempty"`
	ChangedAt   time.Time `json:"changed_at"`
}

// TimeEntryRef is the minimal time-entry view the event builders need.
type TimeEntryRef struct {
	ID        uuid.UUID
	ProjectID uuid.UUID
	TaskID    uuid.UUID
	PersonID  uuid.UUID
	EntryDate string
	Minutes   int
}

// NewTimeEntryLogged builds a work.time_entry.logged.v1 event.
func NewTimeEntryLogged(actorID *uuid.UUID, e TimeEntryRef, changedBy uuid.UUID) (Event, error) {
	return buildEvent(TypeTimeEntryLoggedV1, AggregateTimeEntry, e.ID, actorID, nil, TimeEntryChangedPayload{
		TimeEntryID: e.ID,
		ProjectID:   e.ProjectID,
		TaskID:      e.TaskID,
		PersonID:    e.PersonID,
		EntryDate:   e.EntryDate,
		Minutes:     e.Minutes,
		ChangedBy:   changedBy,
		ChangedAt:   time.Now().UTC(),
	})
}

// NewTimeEntryUpdated builds a work.time_entry.updated.v1 event.
func NewTimeEntryUpdated(actorID *uuid.UUID, e TimeEntryRef, changedBy uuid.UUID) (Event, error) {
	return buildEvent(TypeTimeEntryUpdatedV1, AggregateTimeEntry, e.ID, actorID, nil, TimeEntryChangedPayload{
		TimeEntryID: e.ID,
		ProjectID:   e.ProjectID,
		TaskID:      e.TaskID,
		PersonID:    e.PersonID,
		EntryDate:   e.EntryDate,
		Minutes:     e.Minutes,
		ChangedBy:   changedBy,
		ChangedAt:   time.Now().UTC(),
	})
}

// NewTimeEntryRemoved builds a work.time_entry.removed.v1 event.
func NewTimeEntryRemoved(actorID *uuid.UUID, e TimeEntryRef, changedBy uuid.UUID) (Event, error) {
	return buildEvent(TypeTimeEntryRemovedV1, AggregateTimeEntry, e.ID, actorID, nil, TimeEntryChangedPayload{
		TimeEntryID: e.ID,
		ProjectID:   e.ProjectID,
		TaskID:      e.TaskID,
		PersonID:    e.PersonID,
		EntryDate:   e.EntryDate,
		Minutes:     e.Minutes,
		ChangedBy:   changedBy,
		ChangedAt:   time.Now().UTC(),
	})
}

// ProjectRef is the minimal project view the event builders need.
type ProjectRef struct {
	ID      uuid.UUID
	Name    string
	Status  string
	OwnerID uuid.UUID
	// OwnerUserID is the owner's account id, resolved by the producer (§7.5).
	OwnerUserID *uuid.UUID
	CreatedBy   uuid.UUID
}

// TaskRef is the minimal task view the event builders need.
type TaskRef struct {
	ID         uuid.UUID
	ProjectID  uuid.UUID
	Title      string
	Status     string
	AssigneeID uuid.UUID
	// AssigneeUserID is the assignee's account id, resolved by the producer.
	AssigneeUserID *uuid.UUID
	CreatedBy      uuid.UUID
}
