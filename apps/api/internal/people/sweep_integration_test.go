package people

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/safaristack/atlas/apps/api/internal/outbox"
	"github.com/safaristack/atlas/apps/api/internal/work"
)

// The worker passes *work.Service as the sweep's escalation ladder; this
// assertion fails to compile if the two ever drift apart (ADR-017).
var _ Escalator = (*work.Service)(nil)

// sweepDays are the pinned calendar dates the store test drives. They are far
// enough in the past that no scheduled sweep shares them, and every report the
// test creates (for its own people and for anyone else the shared database
// already holds) is removed by cleanup.
var sweepDays = []time.Time{
	time.Date(2026, time.January, 5, 0, 0, 0, 0, time.UTC),
	time.Date(2026, time.January, 6, 0, 0, 0, 0, time.UTC),
	time.Date(2026, time.January, 7, 0, 0, 0, 0, time.UTC),
}

func cleanupSweepDays(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	t.Cleanup(func() {
		for _, d := range sweepDays {
			_, _ = pool.Exec(context.Background(),
				`DELETE FROM people.daily_reports WHERE report_date = $1`, d)
		}
	})
}

func mustDailyReport(t *testing.T, pool *pgxpool.Pool, reporter uuid.UUID, day time.Time, status DailyReportStatus, submittedAt *time.Time) uuid.UUID {
	t.Helper()
	id, err := uuid.NewV7()
	if err != nil {
		t.Fatalf("new report id: %v", err)
	}
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO people.daily_reports (id, reporter_person_id, report_date, status, submitted_at, version)
		VALUES ($1, $2, $3, $4, $5, 1)`,
		id, reporter, day, string(status), submittedAt); err != nil {
		t.Fatalf("insert daily report: %v", err)
	}
	return id
}

func statusOf(t *testing.T, pool *pgxpool.Pool, id uuid.UUID) (DailyReportStatus, *time.Time, *time.Time) {
	t.Helper()
	var status string
	var dueAt, missedAt *time.Time
	if err := pool.QueryRow(context.Background(),
		`SELECT status, due_at, missed_at FROM people.daily_reports WHERE id = $1`, id).
		Scan(&status, &dueAt, &missedAt); err != nil {
		t.Fatalf("read report: %v", err)
	}
	return DailyReportStatus(status), dueAt, missedAt
}

// TestDailyReportSweepStore exercises the sweep's SQL against Postgres: the
// prompt opens a due row, leave excuses it, the deadline marks it missed and
// returns the line manager, and a late filing is relabelled rather than missed.
func TestDailyReportSweepStore(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	cleanupSweepDays(t, pool)

	reporter := mustPerson(t, pool)
	manager := mustPerson(t, pool)
	other := mustPerson(t, pool)
	if _, err := pool.Exec(ctx, `UPDATE people.persons SET line_manager_id = $2 WHERE id = $1`, reporter, manager); err != nil {
		t.Fatalf("set line manager: %v", err)
	}

	store := NewPGStore(pool)
	dueAt := time.Date(2026, time.January, 5, 13, 30, 0, 0, time.UTC)

	// ── prompt: a working day's report is opened as due ────────────────────
	noop := func(r DailyReport) (outbox.Event, error) {
		return outbox.NewDailyReportDue(nil, r.ID, r.ReporterID, dateString(r.ReportDate))
	}
	opened, err := store.OpenDueReports(ctx, sweepDays[0], dueAt, 500, noop)
	if err != nil {
		t.Fatalf("OpenDueReports: %v", err)
	}
	var mine *DailyReport
	for i := range opened {
		if opened[i].ReporterID == reporter {
			mine = &opened[i]
		}
	}
	if mine == nil {
		t.Fatalf("the reporter was not among the opened reports (%d opened)", len(opened))
	}
	status, gotDue, _ := statusOf(t, pool, mine.ID)
	if status != DailyReportDue || gotDue == nil {
		t.Fatalf("status/due_at = %s/%v, want due with a due_at", status, gotDue)
	}

	// Open is idempotent: a second pass opens nothing for the reporter.
	again, err := store.OpenDueReports(ctx, sweepDays[0], dueAt, 500, noop)
	if err != nil {
		t.Fatalf("second OpenDueReports: %v", err)
	}
	for _, r := range again {
		if r.ReporterID == reporter {
			t.Fatalf("OpenDueReports re-opened the reporter's report")
		}
	}

	// ── leave excuses a due row ────────────────────────────────────────────
	leaveID, err := uuid.NewV7()
	if err != nil {
		t.Fatalf("new leave id: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO people.leave_requests (id, person_id, leave_type, from_date, to_date, status, version)
		VALUES ($1, $2, 'annual', $3, $3, 'approved', 1)`,
		leaveID, reporter, sweepDays[0]); err != nil {
		t.Fatalf("insert leave: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM people.leave_requests WHERE id = $1`, leaveID)
	})
	excused, err := store.ExcuseDueReports(ctx, sweepDays[0])
	if err != nil {
		t.Fatalf("ExcuseDueReports: %v", err)
	}
	if excused < 1 {
		t.Fatalf("ExcuseDueReports excused %d, want at least the reporter's", excused)
	}
	if status, _, _ := statusOf(t, pool, mine.ID); status != DailyReportExcused {
		t.Fatalf("status = %s, want excused", status)
	}

	// ── deadline marks a due row missed and yields the line manager ────────
	dueID := mustDailyReport(t, pool, reporter, sweepDays[1], DailyReportDue, nil)
	// A second person filed on the same day: the miss pass must leave it alone.
	// (Different person keeps the report-per-person-per-day unique constraint
	// satisfied.)
	filedID := mustDailyReport(t, pool, other, sweepDays[1], DailyReportSubmitted, ptrTime(time.Date(2026, time.January, 6, 12, 0, 0, 0, time.UTC)))
	missedAt := time.Date(2026, time.January, 7, 6, 0, 0, 0, time.UTC)
	capture := func(mr MissedReport) (outbox.Event, error) {
		return outbox.NewDailyReportMissed(nil, mr.ID, mr.ReporterID, dateString(mr.ReportDate), mr.LineManagerID)
	}
	missed, err := store.MarkMissedReports(ctx, sweepDays[1], missedAt, capture)
	if err != nil {
		t.Fatalf("MarkMissedReports: %v", err)
	}
	var mineMissed *MissedReport
	for i := range missed {
		if missed[i].ID == dueID {
			mineMissed = &missed[i]
		}
		if missed[i].ID == filedID {
			t.Fatalf("a submitted report was marked missed")
		}
	}
	if mineMissed == nil {
		t.Fatalf("the reporter's due report was not marked missed")
	}
	if mineMissed.LineManagerID == nil || *mineMissed.LineManagerID != manager {
		t.Fatalf("LineManagerID = %v, want the reporter's manager %s", mineMissed.LineManagerID, manager)
	}
	if mineMissed.ReporterName == "" {
		t.Error("ReporterName is empty, want the notice to carry a name")
	}
	status, _, gotMissed := statusOf(t, pool, dueID)
	if status != DailyReportMissed || gotMissed == nil || !gotMissed.Equal(missedAt) {
		t.Fatalf("status/missed_at = %s/%v, want missed at %s", status, gotMissed, missedAt)
	}
	if status, _, _ := statusOf(t, pool, filedID); status != DailyReportSubmitted {
		t.Fatalf("submitted report status = %s, want submitted", status)
	}

	// ── a filing after the deadline is late, not missed ────────────────────
	deadline := time.Date(2026, time.January, 7, 6, 0, 0, 0, time.UTC)
	lateSubmission := deadline.Add(30 * time.Minute)
	lateID := mustDailyReport(t, pool, reporter, sweepDays[2], DailyReportSubmitted, &lateSubmission)
	onTimeID := mustDailyReport(t, pool, other, sweepDays[2], DailyReportSubmitted, ptrTime(deadline.Add(-time.Hour)))
	late, err := store.MarkLateReports(ctx, sweepDays[2], deadline)
	if err != nil {
		t.Fatalf("MarkLateReports: %v", err)
	}
	if late < 1 {
		t.Fatalf("MarkLateReports relabelled %d, want at least the late one", late)
	}
	if status, _, _ := statusOf(t, pool, lateID); status != DailyReportLate {
		t.Errorf("late report status = %s, want late", status)
	}
	if status, _, _ := statusOf(t, pool, onTimeID); status != DailyReportSubmitted {
		t.Errorf("on-time report status = %s, want submitted", status)
	}

	// ── the miss count the escalation rule reads ───────────────────────────
	counts, err := store.MissedCounts(ctx, sweepDays[0], 1)
	if err != nil {
		t.Fatalf("MissedCounts: %v", err)
	}
	var seen bool
	for _, c := range counts {
		if c.PersonID == reporter {
			seen = true
			if c.Misses < 1 {
				t.Errorf("misses = %d, want at least 1", c.Misses)
			}
		}
	}
	if !seen {
		t.Error("the reporter is missing from MissedCounts")
	}
}

// TestTryInSavepointSkipsGonePerson covers the tolerance the sweep needs on a
// shared database: an insert whose reporter was deleted rolls back to the
// savepoint and reports "skipped" without aborting the surrounding transaction,
// so the rest of the batch still commits.
func TestTryInSavepointSkipsGonePerson(t *testing.T) {
	ctx := context.Background()
	tx, err := testPool(t).Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// A random reporter id that no person holds: the insert trips the
	// reporter_person_id foreign key.
	missing := uuid.New()
	ok, err := tryInSavepoint(ctx, tx, func() error {
		id, err := uuid.NewV7()
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO people.daily_reports (id, reporter_person_id, report_date, status, version)
			VALUES ($1, $2, current_date, 'due', 1)`, id, missing)
		return err
	})
	if err != nil {
		t.Fatalf("tryInSavepoint: %v", err)
	}
	if ok {
		t.Fatal("an insert for a missing person was not skipped")
	}

	// The transaction must still be usable, or the whole batch would be lost.
	var n int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM people.daily_reports`).Scan(&n); err != nil {
		t.Fatalf("transaction unusable after the skip: %v", err)
	}
}

// TestPersonEscalationLadder drives the real work ladder through the generic
// seam the people sweep uses: raising twice opens one escalation, and resolving
// closes it — proving the people module reuses the work ladder and table
// (ADR-017) rather than a second mechanism.
func TestPersonEscalationLadder(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	svc := work.NewService(work.NewPGStore(pool), nil, nil, discardLogger(), nil)

	entity := uuid.New()
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM work.escalations WHERE entity_id = $1`, entity)
	})

	raised, err := svc.RaiseEscalation(ctx, EscalationEntity, entity, EscalationLevel, "3 daily reports missed in 10 days", nil)
	if err != nil {
		t.Fatalf("RaiseEscalation: %v", err)
	}
	if !raised {
		t.Fatal("first RaiseEscalation returned false, want true")
	}

	// The partial unique index makes a second raise for the same entity and
	// level a no-op.
	raised, err = svc.RaiseEscalation(ctx, EscalationEntity, entity, EscalationLevel, "again", nil)
	if err != nil {
		t.Fatalf("second RaiseEscalation: %v", err)
	}
	if raised {
		t.Fatal("second RaiseEscalation returned true, want false")
	}

	open, err := svc.OpenEscalatedEntities(ctx, EscalationEntity)
	if err != nil {
		t.Fatalf("OpenEscalatedEntities: %v", err)
	}
	var found bool
	for _, id := range open {
		if id == entity {
			found = true
		}
	}
	if !found {
		t.Fatal("the entity is not in the open escalation list")
	}

	n, err := svc.ResolveEscalations(ctx, EscalationEntity, entity)
	if err != nil {
		t.Fatalf("ResolveEscalations: %v", err)
	}
	if n != 1 {
		t.Fatalf("ResolveEscalations closed %d, want 1", n)
	}
	open, err = svc.OpenEscalatedEntities(ctx, EscalationEntity)
	if err != nil {
		t.Fatalf("OpenEscalatedEntities after resolve: %v", err)
	}
	for _, id := range open {
		if id == entity {
			t.Fatal("the entity is still escalated after resolve")
		}
	}
}
