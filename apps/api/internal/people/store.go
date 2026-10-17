package people

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/safaristack/atlas/apps/api/internal/identity"
	"github.com/safaristack/atlas/apps/api/internal/outbox"
)

// Store is the people persistence interface.
type Store interface {
	// AccountForPerson resolves a person to the user account a realtime nudge
	// is addressed to, or nil when the person has no account (§7.5).
	AccountForPerson(ctx context.Context, personID uuid.UUID) (*uuid.UUID, error)

	// Daily reports.
	SaveDailyReport(ctx context.Context, r DailyReport, ev *outbox.Event) (DailyReport, error)
	GetDailyReport(ctx context.Context, id uuid.UUID) (*DailyReport, error)
	GetDailyReportForDate(ctx context.Context, reporter uuid.UUID, date time.Time) (*DailyReport, error)
	ListDailyReports(ctx context.Context, q ListDailyReportsQuery, limit int, before *Cursor) ([]DailyReport, error)

	// Leave requests.
	CreateLeaveRequest(ctx context.Context, l LeaveRequest, ev *outbox.Event) (LeaveRequest, error)
	GetLeaveRequest(ctx context.Context, id uuid.UUID) (*LeaveRequest, error)
	ListLeaveRequests(ctx context.Context, q ListLeaveRequestsQuery, limit int, before *Cursor) ([]LeaveRequest, error)
	OverlappingLeave(ctx context.Context, personID uuid.UUID, from, to time.Time) (bool, error)
	UpdateLeaveRequest(ctx context.Context, l LeaveRequest, expectedVersion int, ev *outbox.Event) (LeaveRequest, error)

	// Availability and off-boarding (§8.5).
	ListPersons(ctx context.Context) ([]Person, error)
	ListApprovedLeaveOn(ctx context.Context, date time.Time) ([]LeaveRequest, error)
	OffboardPerson(ctx context.Context, personID uuid.UUID, endDate time.Time, ev *outbox.Event) (OffboardResult, error)

	// Session activity (§8.5): who is signed in, and for how long.
	ListActiveSessions(ctx context.Context, since time.Time) ([]Session, error)
	ListSessions(ctx context.Context, q ListSessionsQuery, limit int, before *Cursor) ([]Session, error)
}

// PGStore implements Store over Postgres.
type PGStore struct{ db *pgxpool.Pool }

// NewPGStore builds a store over the pool.
func NewPGStore(db *pgxpool.Pool) *PGStore { return &PGStore{db: db} }

// AccountForPerson resolves the person's account via the shared identity
// bridge, so events carry the realtime address rather than the domain id.
func (s *PGStore) AccountForPerson(ctx context.Context, personID uuid.UUID) (*uuid.UUID, error) {
	return identity.AccountForPerson(ctx, s.db, personID)
}

// rowScanner is satisfied by both pgx.Row and pgx.Rows.
type rowScanner interface {
	Scan(dest ...any) error
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func (s *PGStore) versionConflict(ctx context.Context, tx pgx.Tx, table string, id uuid.UUID, expected int) error {
	var cur int
	err := tx.QueryRow(ctx, `SELECT version FROM `+table+` WHERE id = $1`, id).Scan(&cur)
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	if err != nil {
		return fmt.Errorf("people: read version: %w", err)
	}
	return fmt.Errorf("%w: version %d is stale (current %d)", ErrConflict, expected, cur)
}

// ── daily reports ───────────────────────────────────────────────────────────

const dailyReportCols = `id, reporter_person_id, report_date, status,
	COALESCE(work_done,''), COALESCE(blockers,''), COALESCE(risks,''), COALESCE(next_day,''),
	hours_worked, submitted_at, reviewed_by_person_id, reviewed_at, COALESCE(notes,''),
	version, created_at, updated_at`

func scanDailyReport(row rowScanner) (DailyReport, error) {
	var r DailyReport
	var status string
	if err := row.Scan(&r.ID, &r.ReporterID, &r.ReportDate, &status,
		&r.WorkDone, &r.Blockers, &r.Risks, &r.NextDay,
		&r.HoursWorked, &r.SubmittedAt, &r.ReviewedBy, &r.ReviewedAt, &r.Notes,
		&r.Version, &r.CreatedAt, &r.UpdatedAt); err != nil {
		return r, err
	}
	r.Status = DailyReportStatus(status)
	return r, nil
}

// SaveDailyReport upserts one report per (reporter, date), keeping the unique
// key that enforces "one report per person per day". An already-reviewed report
// is immutable and returns ErrConflict.
func (s *PGStore) SaveDailyReport(ctx context.Context, r DailyReport, ev *outbox.Event) (DailyReport, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return DailyReport{}, fmt.Errorf("people: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	saved, err := scanDailyReport(tx.QueryRow(ctx, `
		INSERT INTO people.daily_reports (
			id, reporter_person_id, report_date, status, work_done, blockers, risks,
			next_day, hours_worked, submitted_at, version, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
		ON CONFLICT (reporter_person_id, report_date) DO UPDATE
		   SET status = EXCLUDED.status, work_done = EXCLUDED.work_done,
		       blockers = EXCLUDED.blockers, risks = EXCLUDED.risks,
		       next_day = EXCLUDED.next_day, hours_worked = EXCLUDED.hours_worked,
		       submitted_at = EXCLUDED.submitted_at,
		       version = people.daily_reports.version + 1
		 WHERE people.daily_reports.status <> 'reviewed'
		RETURNING `+dailyReportCols,
		r.ID, r.ReporterID, r.ReportDate, string(r.Status), nullIfEmpty(r.WorkDone),
		nullIfEmpty(r.Blockers), nullIfEmpty(r.Risks), nullIfEmpty(r.NextDay),
		r.HoursWorked, r.SubmittedAt, r.Version, r.CreatedAt, r.UpdatedAt))
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return DailyReport{}, fmt.Errorf("%w: the report for this date has been reviewed and cannot change", ErrConflict)
	case err != nil:
		return DailyReport{}, fmt.Errorf("people: save daily report: %w", err)
	}
	if ev != nil {
		if _, err := outbox.Write(ctx, tx, *ev); err != nil {
			return DailyReport{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return DailyReport{}, fmt.Errorf("people: commit: %w", err)
	}
	return saved, nil
}

func (s *PGStore) GetDailyReport(ctx context.Context, id uuid.UUID) (*DailyReport, error) {
	r, err := scanDailyReport(s.db.QueryRow(ctx, `SELECT `+dailyReportCols+` FROM people.daily_reports WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("people: get daily report: %w", err)
	}
	return &r, nil
}

func (s *PGStore) GetDailyReportForDate(ctx context.Context, reporter uuid.UUID, date time.Time) (*DailyReport, error) {
	r, err := scanDailyReport(s.db.QueryRow(ctx,
		`SELECT `+dailyReportCols+` FROM people.daily_reports WHERE reporter_person_id = $1 AND report_date = $2`,
		reporter, dateOnly(date)))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("people: get daily report for date: %w", err)
	}
	return &r, nil
}

func (s *PGStore) ListDailyReports(ctx context.Context, q ListDailyReportsQuery, limit int, before *Cursor) ([]DailyReport, error) {
	where := make([]string, 0, 5)
	args := make([]any, 0, 6)
	add := func(clause string, v any) {
		args = append(args, v)
		where = append(where, fmt.Sprintf(clause, len(args)))
	}
	if q.ReporterID != nil {
		add("reporter_person_id = $%d", *q.ReporterID)
	}
	if q.Status != "" {
		add("status = $%d", string(q.Status))
	}
	if q.FromDate != nil {
		add("report_date >= $%d", *q.FromDate)
	}
	if q.ToDate != nil {
		add("report_date <= $%d", *q.ToDate)
	}
	if before != nil {
		args = append(args, before.At.UTC(), before.ID)
		where = append(where, fmt.Sprintf("(created_at, id) < ($%d, $%d)", len(args)-1, len(args)))
	}
	args = append(args, limit)

	sql := `SELECT ` + dailyReportCols + ` FROM people.daily_reports`
	if len(where) > 0 {
		sql += " WHERE " + strings.Join(where, " AND ")
	}
	sql += fmt.Sprintf(" ORDER BY created_at DESC, id DESC LIMIT $%d", len(args))

	rows, err := s.db.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("people: list daily reports: %w", err)
	}
	defer rows.Close()
	out := make([]DailyReport, 0, 8)
	for rows.Next() {
		r, err := scanDailyReport(rows)
		if err != nil {
			return nil, fmt.Errorf("people: scan daily report: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ── leave requests ──────────────────────────────────────────────────────────

const leaveCols = `id, person_id, leave_type, from_date, to_date,
	COALESCE(reason,''), status, approver_person_id, approved_at, declined_at,
	COALESCE(cancel_reason,''), version, created_at, updated_at`

func scanLeave(row rowScanner) (LeaveRequest, error) {
	var l LeaveRequest
	var ltype, status string
	if err := row.Scan(&l.ID, &l.PersonID, &ltype, &l.FromDate, &l.ToDate,
		&l.Reason, &status, &l.ApproverID, &l.ApprovedAt, &l.DeclinedAt,
		&l.CancelReason, &l.Version, &l.CreatedAt, &l.UpdatedAt); err != nil {
		return l, err
	}
	l.Type = LeaveType(ltype)
	l.Status = LeaveStatus(status)
	return l, nil
}

func (s *PGStore) CreateLeaveRequest(ctx context.Context, l LeaveRequest, ev *outbox.Event) (LeaveRequest, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return LeaveRequest{}, fmt.Errorf("people: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	created, err := scanLeave(tx.QueryRow(ctx, `
		INSERT INTO people.leave_requests (
			id, person_id, leave_type, from_date, to_date, reason, status,
			approver_person_id, approved_at, declined_at, cancel_reason, version,
			created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
		RETURNING `+leaveCols,
		l.ID, l.PersonID, string(l.Type), l.FromDate, l.ToDate, nullIfEmpty(l.Reason),
		string(l.Status), l.ApproverID, l.ApprovedAt, l.DeclinedAt, nullIfEmpty(l.CancelReason),
		l.Version, l.CreatedAt, l.UpdatedAt))
	if err != nil {
		return LeaveRequest{}, fmt.Errorf("people: create leave request: %w", err)
	}
	if ev != nil {
		if _, err := outbox.Write(ctx, tx, *ev); err != nil {
			return LeaveRequest{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return LeaveRequest{}, fmt.Errorf("people: commit: %w", err)
	}
	return created, nil
}

func (s *PGStore) GetLeaveRequest(ctx context.Context, id uuid.UUID) (*LeaveRequest, error) {
	l, err := scanLeave(s.db.QueryRow(ctx, `SELECT `+leaveCols+` FROM people.leave_requests WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("people: get leave request: %w", err)
	}
	return &l, nil
}

func (s *PGStore) ListLeaveRequests(ctx context.Context, q ListLeaveRequestsQuery, limit int, before *Cursor) ([]LeaveRequest, error) {
	where := make([]string, 0, 5)
	args := make([]any, 0, 6)
	add := func(clause string, v any) {
		args = append(args, v)
		where = append(where, fmt.Sprintf(clause, len(args)))
	}
	if q.PersonID != nil {
		add("person_id = $%d", *q.PersonID)
	}
	if q.Status != "" {
		add("status = $%d", string(q.Status))
	}
	if q.FromDate != nil {
		add("to_date >= $%d", *q.FromDate)
	}
	if q.ToDate != nil {
		add("from_date <= $%d", *q.ToDate)
	}
	if before != nil {
		args = append(args, before.At.UTC(), before.ID)
		where = append(where, fmt.Sprintf("(created_at, id) < ($%d, $%d)", len(args)-1, len(args)))
	}
	args = append(args, limit)

	sql := `SELECT ` + leaveCols + ` FROM people.leave_requests`
	if len(where) > 0 {
		sql += " WHERE " + strings.Join(where, " AND ")
	}
	sql += fmt.Sprintf(" ORDER BY created_at DESC, id DESC LIMIT $%d", len(args))

	rows, err := s.db.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("people: list leave requests: %w", err)
	}
	defer rows.Close()
	out := make([]LeaveRequest, 0, 8)
	for rows.Next() {
		l, err := scanLeave(rows)
		if err != nil {
			return nil, fmt.Errorf("people: scan leave request: %w", err)
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// OverlappingLeave reports whether the person already has a requested or
// approved leave covering any part of [from, to].
func (s *PGStore) OverlappingLeave(ctx context.Context, personID uuid.UUID, from, to time.Time) (bool, error) {
	var exists bool
	if err := s.db.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM people.leave_requests
			 WHERE person_id = $1
			   AND status IN ('requested','approved')
			   AND from_date <= $3 AND to_date >= $2)`,
		personID, from, to).Scan(&exists); err != nil {
		return false, fmt.Errorf("people: overlap leave: %w", err)
	}
	return exists, nil
}

func (s *PGStore) UpdateLeaveRequest(ctx context.Context, l LeaveRequest, expectedVersion int, ev *outbox.Event) (LeaveRequest, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return LeaveRequest{}, fmt.Errorf("people: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	updated, err := scanLeave(tx.QueryRow(ctx, `
		UPDATE people.leave_requests
		   SET status = $2, approver_person_id = $3, approved_at = $4, declined_at = $5,
		       cancel_reason = $6, version = version + 1
		 WHERE id = $1 AND version = $7
		RETURNING `+leaveCols,
		l.ID, string(l.Status), l.ApproverID, l.ApprovedAt, l.DeclinedAt,
		nullIfEmpty(l.CancelReason), expectedVersion))
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return LeaveRequest{}, s.versionConflict(ctx, tx, "people.leave_requests", l.ID, expectedVersion)
	case err != nil:
		return LeaveRequest{}, fmt.Errorf("people: update leave request: %w", err)
	}
	if ev != nil {
		if _, err := outbox.Write(ctx, tx, *ev); err != nil {
			return LeaveRequest{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return LeaveRequest{}, fmt.Errorf("people: commit: %w", err)
	}
	return updated, nil
}

// ── availability and off-boarding ───────────────────────────────────────────

const personCols = `id, full_name, employment_type, department_id, status, end_date, version`

func scanPerson(row rowScanner) (Person, error) {
	var p Person
	var status string
	if err := row.Scan(&p.ID, &p.FullName, &p.EmploymentType, &p.DepartmentID,
		&status, &p.EndDate, &p.Version); err != nil {
		return p, err
	}
	p.Status = PersonStatus(status)
	return p, nil
}

// ListPersons returns every non-exited person, name-ordered, for the day's
// availability roster.
func (s *PGStore) ListPersons(ctx context.Context) ([]Person, error) {
	rows, err := s.db.Query(ctx,
		`SELECT `+personCols+` FROM people.persons WHERE status <> 'exited' ORDER BY full_name, id`)
	if err != nil {
		return nil, fmt.Errorf("people: list persons: %w", err)
	}
	defer rows.Close()
	out := make([]Person, 0, 16)
	for rows.Next() {
		p, err := scanPerson(rows)
		if err != nil {
			return nil, fmt.Errorf("people: scan person: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// ListApprovedLeaveOn returns the approved leave covering a day. Overlapping
// active leave is rejected when requested, so a person appears at most once.
func (s *PGStore) ListApprovedLeaveOn(ctx context.Context, date time.Time) ([]LeaveRequest, error) {
	rows, err := s.db.Query(ctx, `
		SELECT `+leaveCols+` FROM people.leave_requests
		 WHERE status = 'approved' AND from_date <= $1 AND to_date >= $1
		 ORDER BY from_date, id`, dateOnly(date))
	if err != nil {
		return nil, fmt.Errorf("people: list approved leave on date: %w", err)
	}
	defer rows.Close()
	out := make([]LeaveRequest, 0, 8)
	for rows.Next() {
		l, err := scanLeave(rows)
		if err != nil {
			return nil, fmt.Errorf("people: scan leave request: %w", err)
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// OffboardPerson marks a person exited in one transaction: it deactivates the
// linked account and revokes the account's sessions and role grants immediately
// (§8.5). An already-exited person is a conflict; an unknown person is not
// found.
func (s *PGStore) OffboardPerson(ctx context.Context, personID uuid.UUID, endDate time.Time, ev *outbox.Event) (OffboardResult, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return OffboardResult{}, fmt.Errorf("people: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	person, err := scanPerson(tx.QueryRow(ctx, `
		UPDATE people.persons
		   SET status = 'exited', end_date = $2, version = version + 1
		 WHERE id = $1 AND status <> 'exited'
		RETURNING `+personCols, personID, endDate))
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		var status string
		switch err := tx.QueryRow(ctx, `SELECT status FROM people.persons WHERE id = $1`, personID).Scan(&status); {
		case errors.Is(err, pgx.ErrNoRows):
			return OffboardResult{}, fmt.Errorf("%w: person %s", ErrNotFound, personID)
		case err != nil:
			return OffboardResult{}, fmt.Errorf("people: read person status: %w", err)
		}
		return OffboardResult{}, fmt.Errorf("%w: person is already %s", ErrConflict, status)
	case err != nil:
		return OffboardResult{}, fmt.Errorf("people: offboard person: %w", err)
	}

	out := OffboardResult{Person: person}
	var accountID uuid.UUID
	switch err := tx.QueryRow(ctx, `SELECT id FROM core.user_accounts WHERE person_id = $1`, personID).Scan(&accountID); {
	case errors.Is(err, pgx.ErrNoRows):
		// No login to revoke; the person record is the whole offboarding.
	case err != nil:
		return OffboardResult{}, fmt.Errorf("people: read account: %w", err)
	default:
		if _, err := tx.Exec(ctx,
			`UPDATE core.user_accounts SET status = 'deactivated' WHERE id = $1 AND status <> 'deactivated'`,
			accountID); err != nil {
			return OffboardResult{}, fmt.Errorf("people: deactivate account: %w", err)
		}
		sessions, err := tx.Exec(ctx,
			`UPDATE core.sessions SET revoked_at = now() WHERE user_id = $1 AND revoked_at IS NULL`, accountID)
		if err != nil {
			return OffboardResult{}, fmt.Errorf("people: revoke sessions: %w", err)
		}
		roles, err := tx.Exec(ctx,
			`UPDATE core.role_grants SET revoked_at = now() WHERE user_id = $1 AND revoked_at IS NULL`, accountID)
		if err != nil {
			return OffboardResult{}, fmt.Errorf("people: revoke role grants: %w", err)
		}
		out.AccountID = &accountID
		out.AccountStatus = "deactivated"
		out.SessionsRevoked = int(sessions.RowsAffected())
		out.RolesRevoked = int(roles.RowsAffected())
	}

	if ev != nil {
		if _, err := outbox.Write(ctx, tx, *ev); err != nil {
			return OffboardResult{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return OffboardResult{}, fmt.Errorf("people: commit: %w", err)
	}
	return out, nil
}

// ── session activity (§8.5) ─────────────────────────────────────────────────

// sessionCols/sessionJoin read a sign-in through its account to its person, so
// the activity view can name the person rather than echo a bare account id.
const sessionCols = `s.id, s.user_id, a.person_id, p.full_name,
	COALESCE(host(s.ip), ''), COALESCE(s.user_agent, ''),
	s.created_at, s.last_seen_at, s.revoked_at, s.expires_at`

const sessionJoin = `FROM core.sessions s
	JOIN core.user_accounts a ON a.id = s.user_id
	JOIN people.persons p ON p.id = a.person_id`

func scanSession(row rowScanner) (Session, error) {
	var s Session
	if err := row.Scan(&s.ID, &s.UserID, &s.PersonID, &s.FullName,
		&s.IP, &s.UserAgent, &s.StartedAt, &s.LastSeenAt, &s.RevokedAt, &s.ExpiresAt); err != nil {
		return s, err
	}
	return s, nil
}

// ListActiveSessions returns live, unexpired sessions last seen at or after
// since, most recently active first.
func (s *PGStore) ListActiveSessions(ctx context.Context, since time.Time) ([]Session, error) {
	rows, err := s.db.Query(ctx, `
		SELECT `+sessionCols+`
		`+sessionJoin+`
		 WHERE s.revoked_at IS NULL AND s.expires_at > now() AND s.last_seen_at >= $1
		 ORDER BY s.last_seen_at DESC, s.id DESC`, since)
	if err != nil {
		return nil, fmt.Errorf("people: list active sessions: %w", err)
	}
	defer rows.Close()

	out := make([]Session, 0, 8)
	for rows.Next() {
		sess, err := scanSession(rows)
		if err != nil {
			return nil, fmt.Errorf("people: scan session: %w", err)
		}
		out = append(out, sess)
	}
	return out, rows.Err()
}

// ListSessions returns a keyset page of sessions, newest activity first. From
// bounds last_seen_at (inclusive) and To bounds created_at (exclusive), so a
// session that straddles the window is still returned.
func (s *PGStore) ListSessions(ctx context.Context, q ListSessionsQuery, limit int, before *Cursor) ([]Session, error) {
	where := make([]string, 0, 4)
	args := make([]any, 0, 6)
	add := func(clause string, v any) {
		args = append(args, v)
		where = append(where, fmt.Sprintf(clause, len(args)))
	}
	if q.PersonID != nil {
		add("a.person_id = $%d", *q.PersonID)
	}
	if q.From != nil {
		add("s.last_seen_at >= $%d", *q.From)
	}
	if q.To != nil {
		add("s.created_at < $%d", *q.To)
	}
	if before != nil {
		args = append(args, before.At.UTC(), before.ID)
		where = append(where, fmt.Sprintf("(s.last_seen_at, s.id) < ($%d, $%d)", len(args)-1, len(args)))
	}
	args = append(args, limit)

	sql := `SELECT ` + sessionCols + "\n" + sessionJoin
	if len(where) > 0 {
		sql += " WHERE " + strings.Join(where, " AND ")
	}
	sql += fmt.Sprintf(" ORDER BY s.last_seen_at DESC, s.id DESC LIMIT $%d", len(args))

	rows, err := s.db.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("people: list sessions: %w", err)
	}
	defer rows.Close()

	out := make([]Session, 0, 8)
	for rows.Next() {
		sess, err := scanSession(rows)
		if err != nil {
			return nil, fmt.Errorf("people: scan session: %w", err)
		}
		out = append(out, sess)
	}
	return out, rows.Err()
}
