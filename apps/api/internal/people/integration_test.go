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

// mustSession inserts a sign-in row with explicit activity timestamps so the
// presence and activity derivation can be exercised against real SQL. The token
// hash is derived from the id, which keeps it unique.
func mustSession(t *testing.T, pool *pgxpool.Pool, account uuid.UUID, startedAt, lastSeenAt, expiresAt time.Time, revokedAt *time.Time) uuid.UUID {
	t.Helper()
	id, err := uuid.NewV7()
	if err != nil {
		t.Fatalf("new id: %v", err)
	}
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO core.sessions (id, user_id, token_hash, created_at, last_seen_at, expires_at, revoked_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		id, account, []byte(id.String()), startedAt, lastSeenAt, expiresAt, revokedAt); err != nil {
		t.Fatalf("insert session: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM core.sessions WHERE id = $1`, id)
	})
	return id
}

// findActivity returns the derivation for a session, or fails.
func findActivity(t *testing.T, entries []SessionActivity, id uuid.UUID) SessionActivity {
	t.Helper()
	for _, e := range entries {
		if e.Session.ID == id {
			return e
		}
	}
	t.Fatalf("no activity for session %s", id)
	return SessionActivity{}
}

func ptrTime(t time.Time) *time.Time { return &t }

// TestDailyReportLifecycle covers one-report-per-person-per-day, re-submission
// of an unreviewed day, listing and the hours bounds.
func TestDailyReportLifecycle(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	svc := NewService(NewPGStore(pool), nil, nil, 0)
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
	svc := NewService(NewPGStore(pool), nil, nil, 0)
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
	svc := NewService(NewPGStore(pool), nil, nil, 0)

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
	svc := NewService(NewPGStore(pool), nil, nil, 0)

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

// TestSessionActivity covers the sign-in activity view (§8.5) at the database
// layer: Presence returns only live, unexpired sessions seen inside the window,
// the history list derives online/ended/active for every sign-in, filters by
// person and by the activity window, and pages with a keyset cursor.
func TestSessionActivity(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	svc := NewService(NewPGStore(pool), nil, nil, DefaultPresenceWindow)

	person := mustPerson(t, pool)
	account := mustAccount(t, pool, person)
	other := mustPerson(t, pool)
	otherAccount := mustAccount(t, pool, other)

	now := time.Now().UTC().Truncate(time.Second)
	started := now.Add(-2 * time.Hour)
	// Every session lasts a day unless a test deliberately expires it.
	live := func() time.Time { return now.Add(24 * time.Hour) }

	online := mustSession(t, pool, account, started, now.Add(-30*time.Second), live(), nil)
	expired := mustSession(t, pool, account, started, now.Add(-45*time.Second), now.Add(-10*time.Minute), nil)
	revoked := now.Add(-30 * time.Minute)
	signedOut := mustSession(t, pool, account, started, revoked, live(), &revoked)
	abandoned := mustSession(t, pool, account, started, now.Add(-time.Hour), live(), nil)
	// A second person, seen right now, proves the presence roster is global while
	// the history filter is per person.
	otherOnline := mustSession(t, pool, otherAccount, started, now.Add(-10*time.Second), live(), nil)

	// Presence: the two sessions seen inside the window, and nothing else — the
	// abandoned, signed-out and expired rows are all excluded.
	presence, err := svc.Presence(ctx)
	if err != nil {
		t.Fatalf("Presence: %v", err)
	}
	gotOnline := map[uuid.UUID]bool{}
	for _, a := range presence {
		gotOnline[a.Session.ID] = true
		if !a.Online {
			t.Errorf("presence returned an offline session %s", a.Session.ID)
		}
	}
	for _, id := range []uuid.UUID{online, otherOnline} {
		if !gotOnline[id] {
			t.Errorf("presence is missing online session %s", id)
		}
	}
	for _, id := range []uuid.UUID{expired, signedOut, abandoned} {
		if gotOnline[id] {
			t.Errorf("presence wrongly includes session %s", id)
		}
	}

	// A presence entry names the person and reports the active span so far.
	if got := findActivity(t, presence, online); got.Session.PersonID != person ||
		got.Session.FullName != "Test Person" ||
		got.ActiveSeconds != int64((now.Add(-30*time.Second)).Sub(started).Seconds()) {
		t.Fatalf("presence entry = %+v, want person %s with a 7170s span", got, person)
	}

	// History for one person: newest activity first, with the derived fields.
	page, err := svc.ListSessions(ctx, ListSessionsQuery{PersonID: &person}, 50, nil)
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(page) != 4 {
		t.Fatalf("ListSessions = %d rows, want 4", len(page))
	}
	wantOrder := []uuid.UUID{online, expired, signedOut, abandoned}
	for i, id := range wantOrder {
		if page[i].Session.ID != id {
			t.Fatalf("row %d = %s, want %s", i, page[i].Session.ID, id)
		}
	}
	if page[0].Online != true || page[0].EndedAt != nil {
		t.Errorf("online row = %+v, want online and not ended", page[0])
	}
	if page[1].Online != false || page[1].EndedAt != nil {
		t.Errorf("expired row = %+v, want offline and not ended", page[1])
	}
	if page[2].Online != false || page[2].EndedAt == nil || !page[2].EndedAt.Equal(revoked) {
		t.Errorf("signed-out row = %+v, want ended at %s", page[2], revoked)
	}
	if page[3].Online != false || page[3].EndedAt != nil {
		t.Errorf("abandoned row = %+v, want offline and not ended", page[3])
	}
	// A sign-out ends the span at revoked_at, not at last_seen_at.
	if got, want := page[2].ActiveSeconds, int64(revoked.Sub(started).Seconds()); got != want {
		t.Errorf("signed-out active = %d, want %d", got, want)
	}

	// From bounds activity: only sessions seen in the last two minutes.
	recent, err := svc.ListSessions(ctx, ListSessionsQuery{PersonID: &person, From: ptrTime(now.Add(-2 * time.Minute))}, 50, nil)
	if err != nil {
		t.Fatalf("ListSessions(from): %v", err)
	}
	if len(recent) != 2 {
		t.Fatalf("ListSessions(from) = %d rows, want 2 (online, expired)", len(recent))
	}

	// To bounds creation: a window that closed before these rows existed is empty.
	past, err := svc.ListSessions(ctx, ListSessionsQuery{PersonID: &person, To: ptrTime(now.Add(-3 * time.Hour))}, 50, nil)
	if err != nil {
		t.Fatalf("ListSessions(to): %v", err)
	}
	if len(past) != 0 {
		t.Fatalf("ListSessions(to) = %d rows, want 0", len(past))
	}

	// Keyset pagination: one at a time, no gaps and no repeats.
	var seen []uuid.UUID
	var cursor *Cursor
	for {
		row, err := svc.ListSessions(ctx, ListSessionsQuery{PersonID: &person}, 1, cursor)
		if err != nil {
			t.Fatalf("ListSessions(page): %v", err)
		}
		if len(row) == 0 {
			break
		}
		seen = append(seen, row[0].Session.ID)
		cursor = &Cursor{At: row[0].Session.LastSeenAt, ID: row[0].Session.ID}
	}
	if len(seen) != 4 {
		t.Fatalf("paged %d sessions, want 4", len(seen))
	}
	for i, id := range wantOrder {
		if seen[i] != id {
			t.Fatalf("paged order %v, want %v", seen, wantOrder)
		}
	}
}
