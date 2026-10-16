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

	"github.com/safaristack/atlas/apps/api/internal/outbox"
)

// Store is the people persistence interface.
type Store interface {
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
}

// PGStore implements Store over Postgres.
type PGStore struct{ db *pgxpool.Pool }

// NewPGStore builds a store over the pool.
func NewPGStore(db *pgxpool.Pool) *PGStore { return &PGStore{db: db} }

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
