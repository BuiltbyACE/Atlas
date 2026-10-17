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
	TypeDailyReportDueV1       = "people.daily_report.due.v1"
	TypeDailyReportSubmittedV1 = "people.daily_report.submitted.v1"
	TypeDailyReportMissedV1    = "people.daily_report.missed.v1"
	TypeLeaveRequestedV1       = "people.leave.requested.v1"
	TypeLeaveApprovedV1        = "people.leave.approved.v1"
	TypeLeaveDeclinedV1        = "people.leave.declined.v1"
	TypeLeaveCancelledV1       = "people.leave.cancelled.v1"
	TypePersonOffboardedV1     = "people.person.offboarded.v1"
)

// Aggregate types for people events.
const (
	AggregateDailyReport  = "daily_report"
	AggregateLeaveRequest = "leave_request"
	AggregatePerson       = "person"
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

// DailyReportDuePayload is emitted when a working day's report is opened for a
// person at the prompt minute (§7.4, D5). Go records only that the report is now
// due; the Phase 7 notification fan-out (or a client) turns it into the prompt.
// The orchestrator invalidates its views either way.
type DailyReportDuePayload struct {
	ReportID   uuid.UUID `json:"report_id"`
	ReporterID uuid.UUID `json:"reporter_person_id"`
	ReportDate string    `json:"report_date"`
	DueAt      time.Time `json:"due_at"`
}

// NewDailyReportDue builds a people.daily_report.due.v1 event.
func NewDailyReportDue(actorID *uuid.UUID, reportID, reporterID uuid.UUID, reportDate string) (Event, error) {
	return buildEvent(TypeDailyReportDueV1, AggregateDailyReport, reportID, actorID, nil, DailyReportDuePayload{
		ReportID:   reportID,
		ReporterID: reporterID,
		ReportDate: reportDate,
		DueAt:      time.Now().UTC(),
	})
}

// DailyReportMissedPayload is emitted when a report's deadline passes with
// nothing filed (§7.4, D5). LineManagerID is carried so the fan-out can reach
// the line manager §8.5 asks be notified; nil when the person has none.
type DailyReportMissedPayload struct {
	ReportID      uuid.UUID  `json:"report_id"`
	ReporterID    uuid.UUID  `json:"reporter_person_id"`
	ReportDate    string     `json:"report_date"`
	LineManagerID *uuid.UUID `json:"line_manager_id,omitempty"`
	MissedAt      time.Time  `json:"missed_at"`
}

// NewDailyReportMissed builds a people.daily_report.missed.v1 event.
func NewDailyReportMissed(actorID *uuid.UUID, reportID, reporterID uuid.UUID, reportDate string, lineManagerID *uuid.UUID) (Event, error) {
	return buildEvent(TypeDailyReportMissedV1, AggregateDailyReport, reportID, actorID, nil, DailyReportMissedPayload{
		ReportID:      reportID,
		ReporterID:    reporterID,
		ReportDate:    reportDate,
		LineManagerID: lineManagerID,
		MissedAt:      time.Now().UTC(),
	})
}

// LeaveRequestPayload is the common shape for leave lifecycle events.
type LeaveRequestPayload struct {
	LeaveRequestID uuid.UUID `json:"leave_request_id"`
	PersonID       uuid.UUID `json:"person_id"`
	// PersonUserID is the requester's account id, resolved by the producer when
	// the event nudges them (§7.5); nil when the person has no account.
	PersonUserID *uuid.UUID `json:"person_user_id,omitempty"`
	Type         string     `json:"leave_type,omitempty"`
	FromDate     string     `json:"from_date,omitempty"`
	ToDate       string     `json:"to_date,omitempty"`
	By           uuid.UUID  `json:"by,omitempty"`
	Reason       string     `json:"reason,omitempty"`
	Occurred     time.Time  `json:"occurred_at"`
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
func NewLeaveApproved(actorID *uuid.UUID, leaveID, personID uuid.UUID, approvedBy uuid.UUID, personUserID *uuid.UUID) (Event, error) {
	return buildEvent(TypeLeaveApprovedV1, AggregateLeaveRequest, leaveID, actorID, nil, LeaveRequestPayload{
		LeaveRequestID: leaveID,
		PersonID:       personID,
		PersonUserID:   personUserID,
		By:             approvedBy,
		Occurred:       time.Now().UTC(),
	})
}

// NewLeaveDeclined builds a people.leave.declined.v1 event.
func NewLeaveDeclined(actorID *uuid.UUID, leaveID, personID uuid.UUID, declinedBy uuid.UUID, reason string, personUserID *uuid.UUID) (Event, error) {
	return buildEvent(TypeLeaveDeclinedV1, AggregateLeaveRequest, leaveID, actorID, nil, LeaveRequestPayload{
		LeaveRequestID: leaveID,
		PersonID:       personID,
		PersonUserID:   personUserID,
		By:             declinedBy,
		Reason:         reason,
		Occurred:       time.Now().UTC(),
	})
}

// NewLeaveCancelled builds a people.leave.cancelled.v1 event.
func NewLeaveCancelled(actorID *uuid.UUID, leaveID, personID uuid.UUID, cancelledBy uuid.UUID, reason string, personUserID *uuid.UUID) (Event, error) {
	return buildEvent(TypeLeaveCancelledV1, AggregateLeaveRequest, leaveID, actorID, nil, LeaveRequestPayload{
		LeaveRequestID: leaveID,
		PersonID:       personID,
		PersonUserID:   personUserID,
		By:             cancelledBy,
		Reason:         reason,
		Occurred:       time.Now().UTC(),
	})
}

// PersonOffboardedPayload is emitted when a person is off-boarded (§7.4, §8.5):
// their account is deactivated and sessions and role grants revoked.
type PersonOffboardedPayload struct {
	PersonID uuid.UUID `json:"person_id"`
	// PersonUserID is the off-boarded person's account id, resolved by the
	// producer so a consumer can close the account's realtime channels (§7.5);
	// nil when the person never had an account.
	PersonUserID *uuid.UUID `json:"person_user_id,omitempty"`
	EndDate      string     `json:"end_date,omitempty"`
	Reason       string     `json:"reason,omitempty"`
	OccurredAt   time.Time  `json:"occurred_at"`
}

// NewPersonOffboarded builds a people.person.offboarded.v1 event.
func NewPersonOffboarded(actorID *uuid.UUID, personID uuid.UUID, personUserID *uuid.UUID, endDate, reason string) (Event, error) {
	return buildEvent(TypePersonOffboardedV1, AggregatePerson, personID, actorID, nil, PersonOffboardedPayload{
		PersonID:     personID,
		PersonUserID: personUserID,
		EndDate:      endDate,
		Reason:       reason,
		OccurredAt:   time.Now().UTC(),
	})
}
