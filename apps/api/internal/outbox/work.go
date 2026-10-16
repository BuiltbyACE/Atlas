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
	TypeDailyReportSubmittedV1 = "work.daily_report.submitted.v1"
	TypeAbsenceRequestedV1     = "work.absence.requested.v1"
	TypeAbsenceApprovedV1      = "work.absence.approved.v1"
	TypeAbsenceDeclinedV1      = "work.absence.declined.v1"
	TypeAbsenceCancelledV1     = "work.absence.cancelled.v1"
)

// Aggregate types for work events.
const (
	AggregateProject     = "project"
	AggregateTask        = "task"
	AggregateEvidence    = "evidence"
	AggregateDailyReport = "daily_report"
	AggregateAbsence     = "absence"
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

// TaskOverduePayload is emitted the first time a task passes its deadline.
type TaskOverduePayload struct {
	TaskID       uuid.UUID  `json:"task_id"`
	ProjectID    uuid.UUID  `json:"project_id"`
	Title        string     `json:"title"`
	DueAt        time.Time  `json:"due_at"`
	AssigneeID   *uuid.UUID `json:"assignee_person_id,omitempty"`
	EscalationID uuid.UUID  `json:"escalation_id"`
	Level        int        `json:"level"`
	OverdueAt    time.Time  `json:"overdue_at"`
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

// TaskOverdueRef is the minimal view the overdue event builder needs.
type TaskOverdueRef struct {
	ID           uuid.UUID
	ProjectID    uuid.UUID
	Title        string
	DueAt        time.Time
	AssigneeID   *uuid.UUID
	EscalationID uuid.UUID
	Level        int
}

// NewTaskOverdue builds a work.task.overdue.v1 event.
func NewTaskOverdue(actorID *uuid.UUID, t TaskOverdueRef) (Event, error) {
	return buildEvent(TypeTaskOverdueV1, AggregateTask, t.ID, actorID, nil, TaskOverduePayload{
		TaskID:       t.ID,
		ProjectID:    t.ProjectID,
		Title:        t.Title,
		DueAt:        t.DueAt,
		AssigneeID:   t.AssigneeID,
		EscalationID: t.EscalationID,
		Level:        t.Level,
		OverdueAt:    time.Now().UTC(),
	})
}

// NewEvidenceAdded builds a work.evidence.added.v1 event.
func NewEvidenceAdded(actorID *uuid.UUID, evidenceID, fileID uuid.UUID, taskID, projectID uuid.UUID, kind string, addedBy uuid.UUID) (Event, error) {
	return buildEvent(TypeEvidenceAddedV1, AggregateEvidence, evidenceID, actorID, nil, EvidenceAddedPayload{
		EvidenceID: evidenceID,
		FileID:     fileID,
		TaskID:     taskID,
		ProjectID:  projectID,
		Kind:       kind,
		AddedBy:    addedBy,
		AddedAt:    time.Now().UTC(),
	})
}

// EvidenceAddedPayload is emitted when a file is attached as evidence.
type EvidenceAddedPayload struct {
	EvidenceID uuid.UUID `json:"evidence_id"`
	FileID     uuid.UUID `json:"file_id"`
	TaskID     uuid.UUID `json:"task_id,omitempty"`
	ProjectID  uuid.UUID `json:"project_id,omitempty"`
	Kind       string    `json:"kind,omitempty"`
	AddedBy    uuid.UUID `json:"added_by,omitempty"`
	AddedAt    time.Time `json:"added_at"`
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

// DailyReportSubmittedPayload is emitted when a daily report is submitted (§11).
type DailyReportSubmittedPayload struct {
	ReportID    uuid.UUID `json:"report_id"`
	ReporterID  uuid.UUID `json:"reporter_person_id"`
	ReportDate  string    `json:"report_date"`
	SubmittedAt time.Time `json:"submitted_at"`
}

// NewDailyReportSubmitted builds a work.daily_report.submitted.v1 event.
func NewDailyReportSubmitted(actorID *uuid.UUID, reportID, reporterID uuid.UUID, reportDate string) (Event, error) {
	return buildEvent(TypeDailyReportSubmittedV1, AggregateDailyReport, reportID, actorID, nil, DailyReportSubmittedPayload{
		ReportID:    reportID,
		ReporterID:  reporterID,
		ReportDate:  reportDate,
		SubmittedAt: time.Now().UTC(),
	})
}

// AbsencePayload is the common shape for absence lifecycle events.
type AbsencePayload struct {
	AbsenceID uuid.UUID `json:"absence_id"`
	PersonID  uuid.UUID `json:"person_id"`
	Type      string    `json:"absence_type"`
	FromDate  string    `json:"from_date"`
	ToDate    string    `json:"to_date"`
	By        uuid.UUID `json:"by,omitempty"`
	Reason    string    `json:"reason,omitempty"`
	Occurred  time.Time `json:"occurred_at"`
}

// NewAbsenceRequested builds a work.absence.requested.v1 event.
func NewAbsenceRequested(actorID *uuid.UUID, absenceID, personID uuid.UUID, absenceType, from, to string) (Event, error) {
	return buildEvent(TypeAbsenceRequestedV1, AggregateAbsence, absenceID, actorID, nil, AbsencePayload{
		AbsenceID: absenceID,
		PersonID:  personID,
		Type:      absenceType,
		FromDate:  from,
		ToDate:    to,
		Occurred:  time.Now().UTC(),
	})
}

// NewAbsenceApproved builds a work.absence.approved.v1 event.
func NewAbsenceApproved(actorID *uuid.UUID, absenceID, personID uuid.UUID, approvedBy uuid.UUID) (Event, error) {
	return buildEvent(TypeAbsenceApprovedV1, AggregateAbsence, absenceID, actorID, nil, AbsencePayload{
		AbsenceID: absenceID,
		PersonID:  personID,
		By:        approvedBy,
		Occurred:  time.Now().UTC(),
	})
}

// NewAbsenceDeclined builds a work.absence.declined.v1 event.
func NewAbsenceDeclined(actorID *uuid.UUID, absenceID, personID uuid.UUID, declinedBy uuid.UUID, reason string) (Event, error) {
	return buildEvent(TypeAbsenceDeclinedV1, AggregateAbsence, absenceID, actorID, nil, AbsencePayload{
		AbsenceID: absenceID,
		PersonID:  personID,
		By:        declinedBy,
		Reason:    reason,
		Occurred:  time.Now().UTC(),
	})
}

// NewAbsenceCancelled builds a work.absence.cancelled.v1 event.
func NewAbsenceCancelled(actorID *uuid.UUID, absenceID, personID uuid.UUID, cancelledBy uuid.UUID, reason string) (Event, error) {
	return buildEvent(TypeAbsenceCancelledV1, AggregateAbsence, absenceID, actorID, nil, AbsencePayload{
		AbsenceID: absenceID,
		PersonID:  personID,
		By:        cancelledBy,
		Reason:    reason,
		Occurred:  time.Now().UTC(),
	})
}
