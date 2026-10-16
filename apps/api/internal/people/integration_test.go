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

// mustAccount gives a person a login so the offboarding test can assert that
// the account is deactivated and its sessions revoked.
func mustAccount(t *testing.T, pool *pgxpool.Pool, personID uuid.UUID) uuid.UUID {
	t.Helper()
	id, err := uuid.NewV7()
	if err != nil {
		t.Fatalf("new id: %v", err)
	}
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO core.user_accounts (id, person_id, email, status)
		 VALUES ($1, $2, $3, 'active')`,
		id, personID, id.String()+"@example.test"); err != nil {
		t.Fatalf("insert account: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM core.user_accounts WHERE id = $1`, id)
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

// findAvailability returns the entry for a person, or fails.
func findAvailability(t *testing.T, entries []AvailabilityEntry, personID uuid.UUID) AvailabilityEntry {
	t.Helper()
	for _, e := range entries {
		if e.Person.ID == personID {
			return e
		}
	}
	t.Fatalf("availability has no entry for %s", personID)
	return AvailabilityEntry{}
}

// TestAvailability covers the day roster: approved leave on the day rules the
// person out; a person with no leave is available; exited people are absent.
func TestAvailability(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	svc := NewService(NewPGStore(pool), nil, nil)

	onLeave := mustPerson(t, pool)
	available := mustPerson(t, pool)
	leaver := mustPerson(t, pool) // approver, keeps separation of duties

	day := dateOnly(time.Now().UTC())
	req, err := svc.RequestLeave(ctx, onLeave, LeaveRequestInput{Type: LeaveAnnual, FromDate: day, ToDate: day})
	if err != nil {
		t.Fatalf("RequestLeave: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM people.leave_requests WHERE id = $1`, req.ID)
	})
	if _, err := svc.DecideLeaveRequest(ctx, leaver, req.ID, true, ""); err != nil {
		t.Fatalf("approve leave: %v", err)
	}

	entries, err := svc.Availability(ctx, day)
	if err != nil {
		t.Fatalf("Availability: %v", err)
	}

	got := findAvailability(t, entries, onLeave)
	if got.Leave == nil {
		t.Fatalf("entry for %s = %+v, want leave", onLeave, got)
	}
	if got.Leave.ID != req.ID || got.Leave.Type != LeaveAnnual {
		t.Fatalf("leave = %+v, want %s (annual)", got.Leave, req.ID)
	}

	other := findAvailability(t, entries, available)
	if other.Leave != nil {
		t.Fatalf("entry for %s = %+v, want no leave", available, other.Leave)
	}

	// An exited person is not on the roster.
	if _, err := pool.Exec(ctx, `UPDATE people.persons SET status = 'exited' WHERE id = $1`, available); err != nil {
		t.Fatalf("exit person: %v", err)
	}
	after, err := svc.Availability(ctx, day)
	if err != nil {
		t.Fatalf("Availability after exit: %v", err)
	}
	for _, e := range after {
		if e.Person.ID == available {
			t.Fatalf("exited person %s is still on the roster", available)
		}
	}
}

// TestOffboardPerson covers the access lifecycle: the person exits, the account
// is deactivated, its sessions and role grants are revoked, and the offboarding
// event is written in the same transaction.
func TestOffboardPerson(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	svc := NewService(NewPGStore(pool), nil, nil)

	person := mustPerson(t, pool)
	account := mustAccount(t, pool, person)
	admin, err := uuid.NewV7()
	if err != nil {
		t.Fatalf("new id: %v", err)
	}

	// A live session and an active role grant, both of which offboarding must
	// revoke immediately (§8.5).
	session, err := uuid.NewV7()
	if err != nil {
		t.Fatalf("new id: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO core.sessions (id, user_id, token_hash, expires_at)
		 VALUES ($1, $2, $3, now() + interval '1 day')`,
		session, account, []byte(session.String())); err != nil {
		t.Fatalf("insert session: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM core.sessions WHERE id = $1`, session)
	})

	roleKey := "test_offboard_" + person.String()
	if _, err := pool.Exec(ctx, `INSERT INTO core.roles (key, name) VALUES ($1, $2)`, roleKey, "Offboard test role"); err != nil {
		t.Fatalf("insert role: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM core.roles WHERE key = $1`, roleKey)
	})
	grant, err := uuid.NewV7()
	if err != nil {
		t.Fatalf("new id: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO core.role_grants (id, user_id, role_key, scope_type, granted_by)
		 VALUES ($1, $2, $3, 'company', $4)`,
		grant, account, roleKey, admin); err != nil {
		t.Fatalf("insert role grant: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM core.role_grants WHERE id = $1`, grant)
		_, _ = pool.Exec(context.Background(), `DELETE FROM outbox.events WHERE aggregate_id = $1`, person)
	})

	end := dateOnly(time.Now().UTC())
	res, err := svc.OffboardPerson(ctx, admin, person, OffboardInput{Reason: "resigned", LastWorkingDay: &end})
	if err != nil {
		t.Fatalf("OffboardPerson: %v", err)
	}
	if res.Person.Status != PersonExited {
		t.Fatalf("status = %q, want exited", res.Person.Status)
	}
	if res.Person.EndDate == nil || !res.Person.EndDate.Equal(end) {
		t.Fatalf("end date = %v, want %s", res.Person.EndDate, dateString(end))
	}
	if res.AccountID == nil || *res.AccountID != account {
		t.Fatalf("account id = %v, want %s", res.AccountID, account)
	}
	if res.AccountStatus != "deactivated" {
		t.Fatalf("account status = %q, want deactivated", res.AccountStatus)
	}
	if res.SessionsRevoked != 1 || res.RolesRevoked != 1 {
		t.Fatalf("revoked sessions=%d roles=%d, want 1 and 1", res.SessionsRevoked, res.RolesRevoked)
	}

	// The account and the session really are closed.
	var accountStatus string
	if err := pool.QueryRow(ctx, `SELECT status FROM core.user_accounts WHERE id = $1`, account).Scan(&accountStatus); err != nil {
		t.Fatalf("read account: %v", err)
	}
	if accountStatus != "deactivated" {
		t.Fatalf("account status in db = %q, want deactivated", accountStatus)
	}
	var revoked *time.Time
	if err := pool.QueryRow(ctx, `SELECT revoked_at FROM core.sessions WHERE id = $1`, session).Scan(&revoked); err != nil {
		t.Fatalf("read session: %v", err)
	}
	if revoked == nil {
		t.Fatalf("session %s was not revoked", session)
	}

	// The event was written with the person as the actor-target aggregate.
	var events int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM outbox.events WHERE aggregate_id = $1 AND type = 'people.person.offboarded.v1'`,
		person).Scan(&events); err != nil {
		t.Fatalf("count events: %v", err)
	}
	if events != 1 {
		t.Fatalf("offboarding events = %d, want 1", events)
	}

	// A second offboarding is an illegal transition, not a silent success.
	if _, err := svc.OffboardPerson(ctx, admin, person, OffboardInput{}); !errors.Is(err, ErrConflict) {
		t.Fatalf("second offboard: err = %v, want ErrConflict", err)
	}
	// An unknown person is not found.
	missing := uuid.Must(uuid.NewV7())
	if _, err := svc.OffboardPerson(ctx, admin, missing, OffboardInput{}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("offboard missing: err = %v, want ErrNotFound", err)
	}
}
