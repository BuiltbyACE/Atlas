// Package people owns the people & accountability module (docs/ATLAS.md §8.5):
// who is available and what they reported. Phase 2 implements daily reports and
// leave requests here; person and department rows are still written by the auth
// and approvals packages. Go is the sole writer (ADR-002); every state change is
// audited and, where the orchestrator or the browser must react, an outbox event
// is written in the same transaction (ADR-005, ADR-009).
//
// Optimistic concurrency (§6.6): every write takes the version the client read
// and the store rejects a stale one with ErrConflict. Listings are keyset-paged.
package people

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Sentinel errors. The HTTP layer maps these onto problem responses.
var (
	// ErrNotFound marks an unknown daily report or leave request.
	ErrNotFound = errors.New("people: not found")
	// ErrInvalid marks a request that fails validation.
	ErrInvalid = errors.New("people: invalid")
	// ErrConflict marks a stale version or an illegal transition.
	ErrConflict = errors.New("people: conflict")
	// ErrForbidden marks an action the caller may not take.
	ErrForbidden = errors.New("people: forbidden")
)

// Cursor is a keyset position in a daily-report or leave listing (created_at, id).
type Cursor struct {
	At time.Time
	ID uuid.UUID
}

// DailyReportStatus mirrors the people.daily_reports.status check constraint.
//
// The blueprint's lifecycle (§8.5) is due -> submitted/late/missed, or excused:
// migration 00011 shipped only the three hand-written statuses and migration
// 00017 widened the set for decision D5. `draft` and `reviewed` remain the
// human-side states the hand-written report flow uses.
type DailyReportStatus string

const (
	DailyReportDue       DailyReportStatus = "due"
	DailyReportDraft     DailyReportStatus = "draft"
	DailyReportSubmitted DailyReportStatus = "submitted"
	DailyReportLate      DailyReportStatus = "late"
	DailyReportMissed    DailyReportStatus = "missed"
	DailyReportExcused   DailyReportStatus = "excused"
	DailyReportReviewed  DailyReportStatus = "reviewed"
)

// Valid reports whether s is a known daily-report status.
func (s DailyReportStatus) Valid() bool {
	switch s {
	case DailyReportDue, DailyReportDraft, DailyReportSubmitted, DailyReportLate,
		DailyReportMissed, DailyReportExcused, DailyReportReviewed:
		return true
	}
	return false
}

// DailyReport is a row in people.daily_reports (§8.5).
type DailyReport struct {
	ID          uuid.UUID
	ReporterID  uuid.UUID
	ReportDate  time.Time
	Status      DailyReportStatus
	WorkDone    string
	Blockers    string
	Risks       string
	NextDay     string
	HoursWorked *float64
	SubmittedAt *time.Time
	ReviewedBy  *uuid.UUID
	ReviewedAt  *time.Time
	Notes       string
	Version     int
	CreatedAt   time.Time
	UpdatedAt   time.Time
	// DueAt is when the report was opened by the sweep's prompt (D5), and
	// MissedAt when its deadline passed with nothing filed. Both are nil for
	// hand-written reports, which never enter the timed lifecycle.
	DueAt    *time.Time
	MissedAt *time.Time
}

// DailyReportInput is the payload for submitting a daily report. ReportDate is a
// calendar date; a zero value defaults to today (UTC).
type DailyReportInput struct {
	ReportDate  time.Time
	WorkDone    string
	Blockers    string
	Risks       string
	NextDay     string
	HoursWorked *float64
}

// ListDailyReportsQuery filters a daily-report listing.
type ListDailyReportsQuery struct {
	ReporterID *uuid.UUID
	Status     DailyReportStatus
	FromDate   *time.Time
	ToDate     *time.Time
}

// LeaveType mirrors the people.leave_requests.leave_type check constraint. The
// set is [CONFIG] in the blueprint (§8.5).
type LeaveType string

const (
	LeaveAnnual        LeaveType = "annual"
	LeaveSick          LeaveType = "sick"
	LeaveCompassionate LeaveType = "compassionate"
	LeaveMaternity     LeaveType = "maternity"
	LeavePaternity     LeaveType = "paternity"
	LeaveUnpaid        LeaveType = "unpaid"
	LeaveOther         LeaveType = "other"
)

// Valid reports whether t is a known leave type.
func (t LeaveType) Valid() bool {
	switch t {
	case LeaveAnnual, LeaveSick, LeaveCompassionate, LeaveMaternity, LeavePaternity, LeaveUnpaid, LeaveOther:
		return true
	}
	return false
}

// LeaveStatus mirrors the people.leave_requests.status check constraint.
type LeaveStatus string

const (
	LeaveRequested LeaveStatus = "requested"
	LeaveApproved  LeaveStatus = "approved"
	LeaveDeclined  LeaveStatus = "declined"
	LeaveCancelled LeaveStatus = "cancelled"
)

// Valid reports whether s is a known leave status.
func (s LeaveStatus) Valid() bool {
	switch s {
	case LeaveRequested, LeaveApproved, LeaveDeclined, LeaveCancelled:
		return true
	}
	return false
}

// LeaveRequest is a row in people.leave_requests (§8.5).
type LeaveRequest struct {
	ID           uuid.UUID
	PersonID     uuid.UUID
	Type         LeaveType
	FromDate     time.Time
	ToDate       time.Time
	Reason       string
	Status       LeaveStatus
	ApproverID   *uuid.UUID
	ApprovedAt   *time.Time
	DeclinedAt   *time.Time
	CancelReason string
	Version      int
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// LeaveRequestInput is the payload for requesting leave.
type LeaveRequestInput struct {
	Type     LeaveType
	FromDate time.Time
	ToDate   time.Time
	Reason   string
}

// ListLeaveRequestsQuery filters a leave listing.
type ListLeaveRequestsQuery struct {
	PersonID *uuid.UUID
	Status   LeaveStatus
	FromDate *time.Time
	ToDate   *time.Time
}

// PersonStatus mirrors people.persons.status (§8.5). The people module does not
// own person rows yet, but it reads them for the availability view.
type PersonStatus string

const (
	PersonOnboarding PersonStatus = "onboarding"
	PersonActive     PersonStatus = "active"
	PersonOnLeave    PersonStatus = "on_leave"
	PersonExited     PersonStatus = "exited"
)

// Person is the slice of people.persons the availability view reads (§8.5).
type Person struct {
	ID             uuid.UUID
	FullName       string
	EmploymentType string
	DepartmentID   *uuid.UUID
	Status         PersonStatus
	EndDate        *time.Time
	Version        int
}

// Department is a business department (§8.5): a named grouping of people.
type Department struct {
	ID          uuid.UUID
	Name        string
	LeadID      *uuid.UUID
	CreatedAt   time.Time
	ArchivedAt  *time.Time
}

// AvailabilityEntry is one person on a given day, with the approved leave (if
// any) that covers that day.
type AvailabilityEntry struct {
	Person Person
	Leave  *LeaveRequest
}

// OffboardInput is the body of POST /api/v1/people/{id}/offboard. Both fields
// are optional: the last working day defaults to today and the reason is free
// text kept in the audit trail.
type OffboardInput struct {
	Reason         string
	LastWorkingDay *time.Time
}

// OffboardResult is the state after off-boarding: the exited person, the
// deactivated account (if the person had one) and how much access was revoked.
type OffboardResult struct {
	Person          Person
	AccountID       *uuid.UUID
	AccountStatus   string
	SessionsRevoked int
	RolesRevoked    int
}

func trim(s string) string { return strings.TrimSpace(s) }

// validateDailyReport normalises a daily-report payload. The report date is a
// plain calendar date; hours, when given, must be a plausible working day.
func validateDailyReport(r DailyReport, today time.Time) (DailyReport, error) {
	if r.ReportDate.IsZero() {
		r.ReportDate = dateOnly(today)
	} else {
		r.ReportDate = dateOnly(r.ReportDate)
	}
	r.WorkDone = trim(r.WorkDone)
	r.Blockers = trim(r.Blockers)
	r.Risks = trim(r.Risks)
	r.NextDay = trim(r.NextDay)
	if r.Status == "" {
		r.Status = DailyReportSubmitted
	}
	if !r.Status.Valid() {
		return r, fmt.Errorf("%w: status is invalid", ErrInvalid)
	}
	if r.HoursWorked != nil {
		if *r.HoursWorked < 0 || *r.HoursWorked > 24 {
			return r, fmt.Errorf("%w: hours_worked must be between 0 and 24", ErrInvalid)
		}
	}
	return r, nil
}

// validateLeaveRequest normalises a leave request.
func validateLeaveRequest(l LeaveRequest) (LeaveRequest, error) {
	if l.Type == "" {
		return l, fmt.Errorf("%w: leave_type is required", ErrInvalid)
	}
	if !l.Type.Valid() {
		return l, fmt.Errorf("%w: leave_type must be annual|sick|compassionate|maternity|paternity|unpaid|other", ErrInvalid)
	}
	if l.FromDate.IsZero() || l.ToDate.IsZero() {
		return l, fmt.Errorf("%w: from_date and to_date are required", ErrInvalid)
	}
	l.FromDate, l.ToDate = dateOnly(l.FromDate), dateOnly(l.ToDate)
	if l.ToDate.Before(l.FromDate) {
		return l, fmt.Errorf("%w: to_date is before from_date", ErrInvalid)
	}
	l.Reason = trim(l.Reason)
	if l.Status == "" {
		l.Status = LeaveRequested
	}
	return l, nil
}

// dateOnly truncates a timestamp to a UTC calendar date.
func dateOnly(t time.Time) time.Time {
	u := t.UTC()
	return time.Date(u.Year(), u.Month(), u.Day(), 0, 0, 0, 0, time.UTC)
}

func dateString(t time.Time) string { return t.Format("2006-01-02") }

// ── performance reviews (§8.5, decision D8) ───────────────────────────────

// ReviewStatus mirrors the people.reviews.status check constraint.
type ReviewStatus string

const (
	ReviewDraft     ReviewStatus = "draft"
	ReviewSubmitted ReviewStatus = "submitted"
	ReviewApproved  ReviewStatus = "approved"
	ReviewRejected  ReviewStatus = "rejected"
)

// Valid reports whether s is a known review status.
func (s ReviewStatus) Valid() bool {
	switch s {
	case ReviewDraft, ReviewSubmitted, ReviewApproved, ReviewRejected:
		return true
	}
	return false
}

// Review is a performance review record (§8.5).
type Review struct {
	ID          uuid.UUID
	PersonID    uuid.UUID    // reviewee
	ReviewerID  uuid.UUID    // reviewer
	PeriodStart time.Time    // review period start
	PeriodEnd   time.Time    // review period end
	Rating      int          // 1-5
	Comments    string       // free-form comments
	Status      ReviewStatus // draft|submitted|approved|rejected
	Version     int
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// ReviewInput is the payload for creating/updating a review.
type ReviewInput struct {
	PersonID    *uuid.UUID `json:"person_id"`
	ReviewerID  *uuid.UUID `json:"reviewer_id"`
	PeriodStart *time.Time `json:"period_start"`
	PeriodEnd   *time.Time `json:"period_end"`
	Rating      *int       `json:"rating"`
	Comments    *string    `json:"comments"`
	Status      *string    `json:"status"`
}

// ReviewView is the serialised review returned to the client.
type ReviewView struct {
	ID          string     `json:"id"`
	PersonID    string     `json:"person_id"`
	ReviewerID  string     `json:"reviewer_id"`
	PeriodStart string     `json:"period_start"`
	PeriodEnd   string     `json:"period_end"`
	Rating      int        `json:"rating"`
	Comments    string     `json:"comments"`
	Status      string     `json:"status"`
	Version     int        `json:"version"`
	CreatedAt   string     `json:"created_at"`
	UpdatedAt   string     `json:"updated_at"`
}

// viewReview serialises a Review for the client.
func viewReview(r Review) ReviewView {
	return ReviewView{
		ID:          r.ID.String(),
		PersonID:    r.PersonID.String(),
		ReviewerID:  r.ReviewerID.String(),
		PeriodStart: dateString(r.PeriodStart),
		PeriodEnd:   dateString(r.PeriodEnd),
		Rating:      r.Rating,
		Comments:    r.Comments,
		Status:      string(r.Status),
		Version:     r.Version,
		CreatedAt:   r.CreatedAt.Format(time.RFC3339),
		UpdatedAt:   r.UpdatedAt.Format(time.RFC3339),
	}
}

// validateReview normalises a review input.
func validateReview(in ReviewInput) (ReviewInput, error) {
	if in.PersonID == nil || *in.PersonID == uuid.Nil {
		return in, fmt.Errorf("%w: person_id is required", ErrInvalid)
	}
	if in.ReviewerID == nil || *in.ReviewerID == uuid.Nil {
		return in, fmt.Errorf("%w: reviewer_id is required", ErrInvalid)
	}
	if in.PeriodStart == nil || in.PeriodStart.IsZero() {
		return in, fmt.Errorf("%w: period_start is required", ErrInvalid)
	}
	if in.PeriodEnd == nil || in.PeriodEnd.IsZero() {
		return in, fmt.Errorf("%w: period_end is required", ErrInvalid)
	}
	in.PeriodStart = ptr(dateOnly(*in.PeriodStart))
	in.PeriodEnd = ptr(dateOnly(*in.PeriodEnd))
	if in.PeriodEnd.Before(*in.PeriodStart) {
		return in, fmt.Errorf("%w: period_end is before period_start", ErrInvalid)
	}
	if in.Rating == nil || *in.Rating < 1 || *in.Rating > 5 {
		return in, fmt.Errorf("%w: rating must be between 1 and 5", ErrInvalid)
	}
	if in.Comments != nil {
		in.Comments = ptr(trim(*in.Comments))
	}
	if in.Status != nil {
		st := ReviewStatus(*in.Status)
		if !st.Valid() {
			return in, fmt.Errorf("%w: status must be draft|submitted|approved|rejected", ErrInvalid)
		}
	} else {
		st := ReviewDraft
		in.Status = ptr(string(st))
	}
	return in, nil
}

func ptr[T any](v T) *T { return &v }
