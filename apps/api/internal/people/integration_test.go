package people

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func testServerURL(t *testing.T) string {
	t.Helper()
	url := os.Getenv("ATLAS_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("ATLAS_TEST_DATABASE_URL is not set; skipping people integration tests")
	}
	return url
}

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), testServerURL(t))
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	if err := pool.Ping(context.Background()); err != nil {
		t.Fatalf("ping: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// mustPerson inserts a real person row (people tables have FK constraints on
// people.persons) and removes it at the end of the test.
func mustPerson(t *testing.T, pool *pgxpool.Pool) uuid.UUID {
	t.Helper()
	id, err := uuid.NewV7()
	if err != nil {
		t.Fatalf("new id: %v", err)
	}
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO people.persons (id, full_name, employment_type) VALUES ($1, $2, 'full_time')`,
		id, "Test Person"); err != nil {
		t.Fatalf("insert person: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM people.persons WHERE id = $1`, id)
	})
	return id
}

// TestDailyReportLifecycle covers one-report-per-person-per-day, re-submission
// of an unreviewed day, listing and the hours bounds.
func TestDailyReportLifecycle(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	svc := NewService(NewPGStore(pool), nil, nil)
	actor := mustPerson(t, pool)

	first, err := svc.SubmitDailyReport(ctx, actor, DailyReportInput{WorkDone: "wrote tests"})
	if err != nil {
		t.Fatalf("SubmitDailyReport: %v", err)
	}
	if first.Status != DailyReportSubmitted || first.SubmittedAt == nil {
		t.Fatalf("report = %+v, want submitted with submitted_at", first)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM people.daily_reports WHERE id = $1`, first.ID)
	})

	// Submitting again for the same day replaces that day's report in place.
	second, err := svc.SubmitDailyReport(ctx, actor, DailyReportInput{WorkDone: "wrote more tests"})
	if err != nil {
		t.Fatalf("re-submit: %v", err)
	}
	if second.ID != first.ID {
		t.Fatalf("re-submit created a new row: %s vs %s", second.ID, first.ID)
	}
	if second.Version != first.Version+1 || second.WorkDone != "wrote more tests" {
		t.Fatalf("re-submit = %+v, want version %d and updated text", second, first.Version+1)
	}

	got, err := svc.GetDailyReport(ctx, first.ID)
	if err != nil || got.Status != DailyReportSubmitted {
		t.Fatalf("GetDailyReport = %+v, err = %v", got, err)
	}
	today, err := svc.TodayReport(ctx, actor)
	if err != nil || today.ID != first.ID {
		t.Fatalf("TodayReport = %+v, err = %v, want %s", today, err, first.ID)
	}

	listed, err := svc.ListDailyReports(ctx, ListDailyReportsQuery{ReporterID: &actor}, 50, nil)
	if err != nil || len(listed) != 1 {
		t.Fatalf("ListDailyReports = %d, err = %v, want 1", len(listed), err)
	}

	bad := 25.0
	if _, err := svc.SubmitDailyReport(ctx, actor, DailyReportInput{HoursWorked: &bad}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("hours 25: err = %v, want ErrInvalid", err)
	}
}

// TestLeaveRequestLifecycle covers request, overlap rejection, separation of
// duties on the decision, and cancellation by the requester.
func TestLeaveRequestLifecycle(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	svc := NewService(NewPGStore(pool), nil, nil)
	requester := mustPerson(t, pool)
	manager := mustPerson(t, pool)

	from := dateOnly(time.Now().UTC()).AddDate(0, 0, 7)
	to := from.AddDate(0, 0, 2)
	l, err := svc.RequestLeave(ctx, requester, LeaveRequestInput{Type: LeaveAnnual, FromDate: from, ToDate: to, Reason: "family"})
	if err != nil {
		t.Fatalf("RequestLeave: %v", err)
	}
	if l.Status != LeaveRequested {
		t.Fatalf("leave = %+v, want requested", l)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM people.leave_requests WHERE id = $1`, l.ID)
	})

	// Overlapping dates are rejected while the request is active.
	if _, err := svc.RequestLeave(ctx, requester, LeaveRequestInput{Type: LeaveSick, FromDate: from, ToDate: to}); !errors.Is(err, ErrConflict) {
		t.Fatalf("overlap: err = %v, want ErrConflict", err)
	}
	// A requester cannot decide their own leave.
	if _, err := svc.DecideLeaveRequest(ctx, requester, l.ID, true, ""); !errors.Is(err, ErrForbidden) {
		t.Fatalf("self-decide: err = %v, want ErrForbidden", err)
	}
	approved, err := svc.DecideLeaveRequest(ctx, manager, l.ID, true, "")
	if err != nil {
		t.Fatalf("approve: %v", err)
	}
	if approved.Status != LeaveApproved || approved.ApproverID == nil || *approved.ApproverID != manager {
		t.Fatalf("approved = %+v, want approver %s", approved, manager)
	}
	if _, err := svc.DecideLeaveRequest(ctx, manager, l.ID, false, ""); !errors.Is(err, ErrConflict) {
		t.Fatalf("decide twice: err = %v, want ErrConflict", err)
	}

	// A second, non-overlapping request can be cancelled by its requester.
	from2 := from.AddDate(0, 1, 0)
	b, err := svc.RequestLeave(ctx, requester, LeaveRequestInput{Type: LeaveAnnual, FromDate: from2, ToDate: from2})
	if err != nil {
		t.Fatalf("RequestLeave 2: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM people.leave_requests WHERE id = $1`, b.ID)
	})
	if _, err := svc.CancelLeaveRequest(ctx, manager, b.ID, ""); !errors.Is(err, ErrForbidden) {
		t.Fatalf("other cancel: err = %v, want ErrForbidden", err)
	}
	cancelled, err := svc.CancelLeaveRequest(ctx, requester, b.ID, "plans changed")
	if err != nil || cancelled.Status != LeaveCancelled {
		t.Fatalf("cancel = %+v, err = %v", cancelled, err)
	}

	listed, err := svc.ListLeaveRequests(ctx, ListLeaveRequestsQuery{PersonID: &requester}, 50, nil)
	if err != nil || len(listed) != 2 {
		t.Fatalf("ListLeaveRequests = %d, err = %v, want 2", len(listed), err)
	}
}
