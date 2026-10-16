package outbox

import (
	"time"

	"github.com/google/uuid"
)

// People event types (§7.4). Daily reports and leave are the people module's
// facts; the orchestrator reacts to these to nudge reviewers, notify approvers
// and keep availability views honest (ADR-009). Go records the fact; Elixir
// decides what to push.
const (
	TypeDailyReportSubmittedV1 = "people.daily_report.submitted.v1"
	TypeLeaveRequestedV1       = "people.leave.requested.v1"
	TypeLeaveApprovedV1        = "people.leave.approved.v1"
	TypeLeaveDeclinedV1        = "people.leave.declined.v1"
	TypeLeaveCancelledV1       = "people.leave.cancelled.v1"
)

// Aggregate types for people events.
const (
	AggregateDailyReport  = "daily_report"
	AggregateLeaveRequest = "leave_request"
)

// DailyReportSubmittedPayload is emitted when a daily report is submitted (§7.4).
type DailyReportSubmittedPayload struct {
	ReportID    uuid.UUID `json:"report_id"`
	ReporterID  uuid.UUID `json:"reporter_person_id"`
	ReportDate  string    `json:"report_date"`
	SubmittedAt time.Time `json:"submitted_at"`
}

// NewDailyReportSubmitted builds a people.daily_report.submitted.v1 event.
func NewDailyReportSubmitted(actorID *uuid.UUID, reportID, reporterID uuid.UUID, reportDate string) (Event, error) {
	return buildEvent(TypeDailyReportSubmittedV1, AggregateDailyReport, reportID, actorID, nil, DailyReportSubmittedPayload{
		ReportID:    reportID,
		ReporterID:  reporterID,
		ReportDate:  reportDate,
		SubmittedAt: time.Now().UTC(),
	})
}

// LeaveRequestPayload is the common shape for leave lifecycle events.
type LeaveRequestPayload struct {
	LeaveRequestID uuid.UUID `json:"leave_request_id"`
	PersonID       uuid.UUID `json:"person_id"`
	Type           string    `json:"leave_type,omitempty"`
	FromDate       string    `json:"from_date,omitempty"`
	ToDate         string    `json:"to_date,omitempty"`
	By             uuid.UUID `json:"by,omitempty"`
	Reason         string    `json:"reason,omitempty"`
	Occurred       time.Time `json:"occurred_at"`
}

// NewLeaveRequested builds a people.leave.requested.v1 event.
func NewLeaveRequested(actorID *uuid.UUID, leaveID, personID uuid.UUID, leaveType, from, to string) (Event, error) {
	return buildEvent(TypeLeaveRequestedV1, AggregateLeaveRequest, leaveID, actorID, nil, LeaveRequestPayload{
		LeaveRequestID: leaveID,
		PersonID:       personID,
		Type:           leaveType,
		FromDate:       from,
		ToDate:         to,
		Occurred:       time.Now().UTC(),
	})
}

// NewLeaveApproved builds a people.leave.approved.v1 event.
func NewLeaveApproved(actorID *uuid.UUID, leaveID, personID uuid.UUID, approvedBy uuid.UUID) (Event, error) {
	return buildEvent(TypeLeaveApprovedV1, AggregateLeaveRequest, leaveID, actorID, nil, LeaveRequestPayload{
		LeaveRequestID: leaveID,
		PersonID:       personID,
		By:             approvedBy,
		Occurred:       time.Now().UTC(),
	})
}

// NewLeaveDeclined builds a people.leave.declined.v1 event.
func NewLeaveDeclined(actorID *uuid.UUID, leaveID, personID uuid.UUID, declinedBy uuid.UUID, reason string) (Event, error) {
	return buildEvent(TypeLeaveDeclinedV1, AggregateLeaveRequest, leaveID, actorID, nil, LeaveRequestPayload{
		LeaveRequestID: leaveID,
		PersonID:       personID,
		By:             declinedBy,
		Reason:         reason,
		Occurred:       time.Now().UTC(),
	})
}

// NewLeaveCancelled builds a people.leave.cancelled.v1 event.
func NewLeaveCancelled(actorID *uuid.UUID, leaveID, personID uuid.UUID, cancelledBy uuid.UUID, reason string) (Event, error) {
	return buildEvent(TypeLeaveCancelledV1, AggregateLeaveRequest, leaveID, actorID, nil, LeaveRequestPayload{
		LeaveRequestID: leaveID,
		PersonID:       personID,
		By:             cancelledBy,
		Reason:         reason,
		Occurred:       time.Now().UTC(),
	})
}
