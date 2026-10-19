package people

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/safaristack/atlas/apps/api/internal/outbox"
)

// ── in-memory store and doubles ─────────────────────────────────────────────

// memStore models only the daily-report lifecycle the sweep drives. It embeds
// the Store interface so the methods the sweep never calls need no
// implementation; a call to one of them panics, which is what we want.
type memStore struct {
	Store
	persons     []Person
	lineManager map[uuid.UUID]uuid.UUID
	onLeave     map[uuid.UUID]bool
	reports     map[uuid.UUID]*DailyReport
}

func newMemStore(persons ...uuid.UUID) *memStore {
	m := &memStore{
		lineManager: map[uuid.UUID]uuid.UUID{},
		onLeave:     map[uuid.UUID]bool{},
		reports:     map[uuid.UUID]*DailyReport{},
	}
	for _, p := range persons {
		m.persons = append(m.persons, Person{ID: p, FullName: "Test Person", Status: PersonActive})
	}
	return m
}

func (m *memStore) find(reporter uuid.UUID, day time.Time) *DailyReport {
	for _, r := range m.reports {
		if r.ReporterID == reporter && dateOnly(r.ReportDate).Equal(dateOnly(day)) {
			return r
		}
	}
	return nil
}

// seed inserts a report in a given state, for tests that start mid-lifecycle.
func (m *memStore) seed(reporter uuid.UUID, day time.Time, status DailyReportStatus, submittedAt *time.Time) *DailyReport {
	r := &DailyReport{
		ID:          uuid.New(),
		ReporterID:  reporter,
		ReportDate:  dateOnly(day),
		Status:      status,
		SubmittedAt: submittedAt,
		Version:     1,
	}
	m.reports[r.ID] = r
	return r
}

func (m *memStore) notice(r MissedReport) MissedReport {
	if lm, ok := m.lineManager[r.ReporterID]; ok {
		id := lm
		r.LineManagerID = &id
	}
	r.ReporterName = "Test Person"
	return r
}

func (m *memStore) OpenDueReports(_ context.Context, day, dueAt time.Time, limit int, build func(DailyReport) (outbox.Event, error)) ([]DailyReport, error) {
	at := dueAt
	var out []DailyReport
	for _, p := range m.persons {
		if p.Status != PersonActive || m.onLeave[p.ID] || m.find(p.ID, day) != nil {
			continue
		}
		r := DailyReport{
			ID:         uuid.New(),
			ReporterID: p.ID,
			ReportDate: dateOnly(day),
			Status:     DailyReportDue,
			DueAt:      &at,
			Version:    1,
		}
		if _, err := build(r); err != nil {
			return nil, err
		}
		m.reports[r.ID] = &r
		out = append(out, r)
	}
	return out, nil
}

func (m *memStore) ExcuseDueReports(_ context.Context, day time.Time) (int, error) {
	n := 0
	for _, r := range m.reports {
		if !dateOnly(r.ReportDate).Equal(dateOnly(day)) || !m.onLeave[r.ReporterID] {
			continue
		}
		if r.Status != DailyReportDue && r.Status != DailyReportMissed {
			continue
		}
		r.Status = DailyReportExcused
		r.Version++
		n++
	}
	return n, nil
}

func (m *memStore) MarkMissedReports(_ context.Context, day, missedAt time.Time, build func(MissedReport) (outbox.Event, error)) ([]MissedReport, error) {
	at := missedAt
	var out []MissedReport
	for _, r := range m.reports {
		if !dateOnly(r.ReportDate).Equal(dateOnly(day)) || r.Status != DailyReportDue || m.onLeave[r.ReporterID] {
			continue
		}
		r.Status = DailyReportMissed
		r.MissedAt = &at
		r.Version++
		mr := m.notice(MissedReport{DailyReport: *r})
		if _, err := build(mr); err != nil {
			return nil, err
		}
		out = append(out, mr)
	}
	for _, p := range m.persons {
		if p.Status != PersonActive || m.onLeave[p.ID] || m.find(p.ID, day) != nil {
			continue
		}
		r := DailyReport{
			ID:         uuid.New(),
			ReporterID: p.ID,
			ReportDate: dateOnly(day),
			Status:     DailyReportMissed,
			MissedAt:   &at,
			Version:    1,
		}
		m.reports[r.ID] = &r
		mr := m.notice(MissedReport{DailyReport: r})
		if _, err := build(mr); err != nil {
			return nil, err
		}
		out = append(out, mr)
	}
	return out, nil
}

func (m *memStore) MarkLateReports(_ context.Context, day, deadline time.Time) (int, error) {
	n := 0
	for _, r := range m.reports {
		if !dateOnly(r.ReportDate).Equal(dateOnly(day)) || r.Status != DailyReportSubmitted {
			continue
		}
		if r.SubmittedAt == nil || !r.SubmittedAt.After(deadline) {
			continue
		}
		r.Status = DailyReportLate
		r.Version++
		n++
	}
	return n, nil
}

func (m *memStore) MissedCounts(_ context.Context, since time.Time, min int) ([]MissCounts, error) {
	counts := map[uuid.UUID]int{}
	last := map[uuid.UUID]time.Time{}
	for _, r := range m.reports {
		if r.Status == DailyReportMissed && !dateOnly(r.ReportDate).Before(dateOnly(since)) {
			counts[r.ReporterID]++
			at := r.ReportDate
			if r.MissedAt != nil {
				at = *r.MissedAt
			}
			if at.After(last[r.ReporterID]) {
				last[r.ReporterID] = at
			}
		}
	}
	var out []MissCounts
	for _, p := range m.persons {
		if c := counts[p.ID]; c >= min {
			out = append(out, MissCounts{PersonID: p.ID, Misses: c, LastMissedAt: last[p.ID]})
		}
	}
	return out, nil
}

// fakeEscalator is an in-memory ladder: one open escalation per entity, exactly
// like the work.escalations partial unique index.
type fakeEscalator struct {
	l2       *uuid.UUID
	open     map[uuid.UUID]bool
	raised   []uuid.UUID
	resolved []uuid.UUID
	// resolvedAt records a resolution's time, as work.escalations.resolved_at.
	resolvedAt map[uuid.UUID]time.Time
}

func newFakeEscalator(l2 *uuid.UUID) *fakeEscalator {
	return &fakeEscalator{l2: l2, open: map[uuid.UUID]bool{}}
}

func (f *fakeEscalator) PersonForRole(context.Context, string) (*uuid.UUID, error) { return f.l2, nil }

func (f *fakeEscalator) RaiseEscalation(_ context.Context, _ string, entityID uuid.UUID, _ int, _ string, _ *uuid.UUID) (bool, error) {
	if f.open[entityID] {
		return false, nil
	}
	f.open[entityID] = true
	f.raised = append(f.raised, entityID)
	return true, nil
}

func (f *fakeEscalator) ResolveEscalations(_ context.Context, _ string, entityID uuid.UUID) (int, error) {
	if !f.open[entityID] {
		return 0, nil
	}
	delete(f.open, entityID)
	f.resolved = append(f.resolved, entityID)
	return 1, nil
}

func (f *fakeEscalator) LastResolvedAt(_ context.Context, _ string, entityID uuid.UUID) (*time.Time, error) {
	if at, ok := f.resolvedAt[entityID]; ok {
		return &at, nil
	}
	return nil, nil
}

func (f *fakeEscalator) OpenEscalatedEntities(context.Context, string) ([]uuid.UUID, error) {
	out := make([]uuid.UUID, 0, len(f.open))
	for id := range f.open {
		out = append(out, id)
	}
	return out, nil
}

// fakeNotifier captures the notices the sweep emits.
type fakeNotifier struct{ notices []MissedReportNotice }

func (f *fakeNotifier) NotifyMissedReport(_ context.Context, n MissedReportNotice) error {
	f.notices = append(f.notices, n)
	return nil
}

// ── helpers ────────────────────────────────────────────────────────────────

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// day is the canonical calendar-date representation people.daily_reports uses: a
// UTC midnight on the policy-zone date, which is what policy.Today produces, so
// tests seed rows the way the sweep looks them up.
func day(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// d5Policy is the default decision D5 rhythm unless a test narrows the window.
func d5Policy() DailyReportPolicy {
	return DailyReportPolicy{
		PromptMinute:     16*60 + 30,
		DeadlineMinute:   9 * 60,
		WorkingDays:      weekdays(),
		MissedThreshold:  3,
		MissedWindowDays: 10,
		Timezone:         eat,
		L2Role:           "ceo_internal",
	}
}

func newSweepService(store Store, now time.Time, policy DailyReportPolicy) (*Service, *fakeEscalator, *fakeNotifier) {
	svc := NewService(store, nil, discardLogger(), 0)
	svc.now = func() time.Time { return now.UTC() }
	svc.SetPolicy(StaticPolicy{Value: policy})
	l2 := uuid.New()
	esc := newFakeEscalator(&l2)
	ntf := &fakeNotifier{}
	svc.SetEscalator(esc)
	svc.SetNotifier(ntf)
	return svc, esc, ntf
}

// ── tests ──────────────────────────────────────────────────────────────────

func TestSweep_OpensDueReportsAfterPrompt(t *testing.T) {
	reporter := uuid.New()
	store := newMemStore(reporter)
	// 17:00 EAT on Friday: past the 16:30 prompt.
	now := at(2026, time.October, 16, 17, 0, eat)
	svc, _, _ := newSweepService(store, now, d5Policy())

	res, err := svc.Sweep(context.Background(), 100)
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if res.Opened != 1 {
		t.Fatalf("Opened = %d, want 1", res.Opened)
	}
	r := store.find(reporter, now)
	if r == nil || r.Status != DailyReportDue {
		t.Fatalf("report = %+v, want a due report for today", r)
	}
	if r.DueAt == nil || !r.DueAt.Equal(at(2026, time.October, 16, 13, 30, time.UTC)) {
		t.Errorf("DueAt = %v, want 16:30 EAT as an instant", r.DueAt)
	}
}

func TestSweep_DoesNotOpenBeforePrompt(t *testing.T) {
	reporter := uuid.New()
	store := newMemStore(reporter)
	// 15:00 EAT on Friday: before the 16:30 prompt.
	now := at(2026, time.October, 16, 15, 0, eat)
	svc, _, _ := newSweepService(store, now, d5Policy())

	res, err := svc.Sweep(context.Background(), 100)
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if res.Opened != 0 {
		t.Fatalf("Opened = %d, want 0 before the prompt", res.Opened)
	}
	if r := store.find(reporter, now); r != nil {
		t.Fatalf("report = %+v, want none before the prompt", r)
	}
}

func TestSweep_DoesNotOpenOnWeekend(t *testing.T) {
	reporter := uuid.New()
	store := newMemStore(reporter)
	// Saturday 17:00 EAT: past the prompt minute but not a working day.
	now := at(2026, time.October, 17, 17, 0, eat)
	svc, _, _ := newSweepService(store, now, d5Policy())

	res, err := svc.Sweep(context.Background(), 100)
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if res.Opened != 0 {
		t.Fatalf("Opened = %d, want 0 on a weekend", res.Opened)
	}
}

func TestSweep_MarksMissedAndNotifiesLineManager(t *testing.T) {
	reporter, manager := uuid.New(), uuid.New()
	store := newMemStore(reporter)
	store.lineManager[reporter] = manager
	store.seed(reporter, day(2026, time.October, 16), DailyReportDue, nil)

	// Monday 10:00 EAT: Friday's 09:00 deadline has passed. A one-day window
	// keeps the catch-up to exactly Friday.
	now := at(2026, time.October, 19, 10, 0, eat)
	policy := d5Policy()
	policy.MissedWindowDays = 1
	svc, esc, ntf := newSweepService(store, now, policy)

	res, err := svc.Sweep(context.Background(), 100)
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if res.Missed != 1 {
		t.Fatalf("Missed = %d, want 1", res.Missed)
	}

	r := store.find(reporter, day(2026, time.October, 16))
	if r == nil || r.Status != DailyReportMissed {
		t.Fatalf("Friday report = %+v, want missed", r)
	}
	if r.MissedAt == nil || !r.MissedAt.Equal(at(2026, time.October, 19, 6, 0, time.UTC)) {
		t.Errorf("MissedAt = %v, want the deadline instant", r.MissedAt)
	}

	// Exactly one notice, for the report that was just missed.
	if len(ntf.notices) != 1 {
		t.Fatalf("notices = %d, want 1", len(ntf.notices))
	}
	n := ntf.notices[0]
	if n.LineManagerID != manager || n.ReportID != r.ID || n.ReportDate != "2026-10-16" {
		t.Errorf("notice = %+v, want the Friday report to the line manager", n)
	}
	if n.IdempotencyKey != "people.daily_report.missed:"+r.ID.String() {
		t.Errorf("IdempotencyKey = %q, want one keyed on the report", n.IdempotencyKey)
	}
	// One miss is below the threshold: no escalation.
	if len(esc.raised) != 0 {
		t.Errorf("raised = %v, want none", esc.raised)
	}
}

// TestSweep_NoticesReportTheRunningCount pins the count the notice carries: a
// person who keeps missing is told how many misses are already in the window.
func TestSweep_NoticesReportTheRunningCount(t *testing.T) {
	reporter, manager := uuid.New(), uuid.New()
	store := newMemStore(reporter)
	store.lineManager[reporter] = manager
	store.seed(reporter, day(2026, time.October, 16), DailyReportDue, nil)

	now := at(2026, time.October, 19, 10, 0, eat)
	// A high threshold isolates the notice from escalation.
	policy := d5Policy()
	policy.MissedThreshold = 20
	svc, esc, ntf := newSweepService(store, now, policy)

	res, err := svc.Sweep(context.Background(), 100)
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if len(ntf.notices) != res.Missed || res.Missed < 1 {
		t.Fatalf("notices = %d, missed = %d, want one notice per miss", len(ntf.notices), res.Missed)
	}
	var found bool
	for _, n := range ntf.notices {
		if n.ReportDate == "2026-10-16" {
			found = true
			if n.Misses < 1 {
				t.Errorf("notice misses = %d, want at least the miss it reports", n.Misses)
			}
			if n.Threshold != 20 || n.WindowDays != 10 {
				t.Errorf("notice threshold/window = %d/%d, want 20/10", n.Threshold, n.WindowDays)
			}
		}
	}
	if !found {
		t.Fatal("no notice for the Friday report")
	}
	if len(esc.raised) != 0 {
		t.Errorf("raised = %v, want none below the threshold", esc.raised)
	}
}

func TestSweep_SkipsNoticeWithoutLineManager(t *testing.T) {
	reporter := uuid.New()
	store := newMemStore(reporter)
	store.seed(reporter, day(2026, time.October, 16), DailyReportDue, nil)

	now := at(2026, time.October, 19, 10, 0, eat)
	policy := d5Policy()
	policy.MissedWindowDays = 1
	svc, _, ntf := newSweepService(store, now, policy)

	if _, err := svc.Sweep(context.Background(), 100); err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if len(ntf.notices) != 0 {
		t.Fatalf("notices = %d, want 0 without a line manager", len(ntf.notices))
	}
}

func TestSweep_LateSubmissionIsRelabelled(t *testing.T) {
	reporter := uuid.New()
	store := newMemStore(reporter)
	// Filed Monday 10:00 EAT, an hour past the 09:00 deadline.
	submitted := at(2026, time.October, 19, 10, 0, eat)
	store.seed(reporter, day(2026, time.October, 16), DailyReportSubmitted, &submitted)

	now := at(2026, time.October, 19, 12, 0, eat)
	policy := d5Policy()
	policy.MissedWindowDays = 1
	svc, _, ntf := newSweepService(store, now, policy)

	res, err := svc.Sweep(context.Background(), 100)
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if res.Late != 1 {
		t.Fatalf("Late = %d, want 1", res.Late)
	}
	if res.Missed != 0 {
		t.Fatalf("Missed = %d, want 0: a late report still counts as filed", res.Missed)
	}
	r := store.find(reporter, day(2026, time.October, 16))
	if r == nil || r.Status != DailyReportLate {
		t.Fatalf("report = %+v, want late", r)
	}
	if len(ntf.notices) != 0 {
		t.Errorf("notices = %d, want 0 for a late (not missed) report", len(ntf.notices))
	}
}

func TestSweep_ExcusesApprovedLeave(t *testing.T) {
	reporter := uuid.New()
	store := newMemStore(reporter)
	store.onLeave[reporter] = true
	store.seed(reporter, day(2026, time.October, 16), DailyReportDue, nil)

	now := at(2026, time.October, 19, 10, 0, eat)
	policy := d5Policy()
	policy.MissedWindowDays = 1
	svc, _, ntf := newSweepService(store, now, policy)

	res, err := svc.Sweep(context.Background(), 100)
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if res.Excused != 1 || res.Missed != 0 {
		t.Fatalf("Excused/Missed = %d/%d, want 1/0", res.Excused, res.Missed)
	}
	r := store.find(reporter, day(2026, time.October, 16))
	if r == nil || r.Status != DailyReportExcused {
		t.Fatalf("report = %+v, want excused", r)
	}
	if len(ntf.notices) != 0 {
		t.Errorf("notices = %d, want 0 for excused leave", len(ntf.notices))
	}
}

func TestSweep_EscalatesAndResolvesOnRecovery(t *testing.T) {
	reporter, manager := uuid.New(), uuid.New()
	store := newMemStore(reporter)
	store.lineManager[reporter] = manager

	// Friday 08:00 EAT: before the prompt, and no deadline in the catch-up
	// window has passed yet, so the pass only looks at the rows we seed.
	now := at(2026, time.October, 16, 8, 0, eat)
	policy := d5Policy()
	svc, esc, _ := newSweepService(store, now, policy)

	// Every day the sweep will visit already has a row — the steady state when
	// the worker runs daily — so nothing is backfilled. The three newest are
	// missed, the rest filed.
	swept := policy.deadlineDays(now.UTC(), policy.Today(now.UTC()))
	if len(swept) < 4 {
		t.Fatalf("swept days = %d, want at least 4 for this test", len(swept))
	}
	missed := make([]*DailyReport, 0, 3)
	for i, d := range swept {
		if i < 3 {
			missed = append(missed, store.seed(reporter, d, DailyReportMissed, nil))
			continue
		}
		store.seed(reporter, d, DailyReportSubmitted, nil)
	}

	res, err := svc.Sweep(context.Background(), 100)
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if res.Escalated != 1 || len(esc.raised) != 1 || esc.raised[0] != reporter {
		t.Fatalf("escalated = %d, raised = %v, want one L2 for the reporter", res.Escalated, esc.raised)
	}

	// A second pass does not re-raise the same open escalation.
	res, err = svc.Sweep(context.Background(), 100)
	if err != nil {
		t.Fatalf("second Sweep: %v", err)
	}
	if res.Escalated != 0 || len(esc.raised) != 1 {
		t.Fatalf("second pass escalated = %d, raised = %v, want 0 and no duplicate", res.Escalated, esc.raised)
	}

	// Filing the owed reports drops the count below the threshold and resolves
	// the escalation.
	for _, r := range missed {
		r.Status = DailyReportSubmitted
	}
	res, err = svc.Sweep(context.Background(), 100)
	if err != nil {
		t.Fatalf("recovery Sweep: %v", err)
	}
	if res.Resolved != 1 || len(esc.resolved) != 1 || esc.resolved[0] != reporter {
		t.Fatalf("resolved = %d, %v, want the reporter's escalation resolved", res.Resolved, esc.resolved)
	}
	if len(esc.open) != 0 {
		t.Errorf("open escalations = %d, want 0 after recovery", len(esc.open))
	}
}

func TestSweep_AgedOutMissesResolve(t *testing.T) {
	reporter := uuid.New()
	store := newMemStore(reporter)
	// An escalation is open, but the misses have aged out of the window, so the
	// next pass must resolve it.
	l2 := uuid.New()
	esc := newFakeEscalator(&l2)
	esc.open[reporter] = true

	svc := NewService(store, nil, discardLogger(), 0)
	svc.now = func() time.Time { return at(2026, time.October, 16, 15, 0, eat).UTC() }
	policy := d5Policy()
	policy.MissedWindowDays = 1
	svc.SetPolicy(StaticPolicy{Value: policy})
	svc.SetEscalator(esc)

	res, err := svc.Sweep(context.Background(), 100)
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if res.Resolved != 1 {
		t.Fatalf("Resolved = %d, want 1", res.Resolved)
	}
}

func TestSweep_NoEscalatorSkipsEscalation(t *testing.T) {
	reporter := uuid.New()
	store := newMemStore(reporter)
	store.seed(reporter, day(2026, time.October, 14), DailyReportMissed, nil)
	store.seed(reporter, day(2026, time.October, 15), DailyReportMissed, nil)
	store.seed(reporter, day(2026, time.October, 16), DailyReportMissed, nil)

	svc := NewService(store, nil, discardLogger(), 0)
	svc.now = func() time.Time { return at(2026, time.October, 16, 8, 0, eat).UTC() }
	svc.SetPolicy(StaticPolicy{Value: d5Policy()})

	res, err := svc.Sweep(context.Background(), 100)
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if res.Escalated != 0 || res.Resolved != 0 {
		t.Fatalf("escalated/resolved = %d/%d, want 0/0 without an escalator", res.Escalated, res.Resolved)
	}
}

func TestSweep_PolicyErrorSkipsThePass(t *testing.T) {
	reporter := uuid.New()
	store := newMemStore(reporter)
	svc := NewService(store, nil, discardLogger(), 0)
	svc.now = func() time.Time { return at(2026, time.October, 16, 17, 0, eat).UTC() }
	svc.SetPolicy(errPolicy{})

	if _, err := svc.Sweep(context.Background(), 100); err == nil {
		t.Fatal("Sweep returned nil error, want the policy failure")
	}
	if r := store.find(reporter, day(2026, time.October, 16)); r != nil {
		t.Errorf("report = %+v, want no work when the policy cannot be read", r)
	}
}

// errPolicy is a PolicyReader that always fails, standing in for a settings
// outage.
type errPolicy struct{}

func (errPolicy) DailyReportPolicy(context.Context) (DailyReportPolicy, error) {
	return DailyReportPolicy{}, context.DeadlineExceeded
}

// A person resolved by hand while still over the threshold is not escalated
// again on the next pass (that produced "same reason raised 3x" duplicates);
// only a miss after the resolution opens a new episode.
func TestSweep_ManualResolveIsNotReRaisedUntilANewMiss(t *testing.T) {
	reporter := uuid.New()
	store := newMemStore(reporter)
	now := at(2026, time.October, 16, 8, 0, eat)
	policy := d5Policy()
	svc, esc, _ := newSweepService(store, now, policy)

	swept := policy.deadlineDays(now.UTC(), policy.Today(now.UTC()))
	if len(swept) < 4 {
		t.Fatalf("swept days = %d, want at least 4 for this test", len(swept))
	}
	for i, d := range swept {
		status := DailyReportSubmitted
		if i < 3 {
			status = DailyReportMissed
		}
		r := store.seed(reporter, d, status, nil)
		if status == DailyReportMissed {
			missedAt := d
			r.MissedAt = &missedAt
		}
	}

	if _, err := svc.Sweep(context.Background(), 100); err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if len(esc.raised) != 1 {
		t.Fatalf("raised = %v, want one L2", esc.raised)
	}

	// Someone resolves it by hand; the misses are still in the window.
	delete(esc.open, reporter)
	esc.resolvedAt = map[uuid.UUID]time.Time{reporter: now}

	res, err := svc.Sweep(context.Background(), 100)
	if err != nil {
		t.Fatalf("second Sweep: %v", err)
	}
	if res.Escalated != 0 || len(esc.raised) != 1 {
		t.Fatalf("after manual resolve escalated = %d, raised = %v, want no re-raise", res.Escalated, esc.raised)
	}

	// A miss recorded after the resolution starts a new episode.
	later := now.Add(time.Hour)
	for _, r := range store.reports {
		if r.Status == DailyReportMissed {
			r.MissedAt = &later
			break
		}
	}
	if res, err = svc.Sweep(context.Background(), 100); err != nil {
		t.Fatalf("third Sweep: %v", err)
	}
	if res.Escalated != 1 || len(esc.raised) != 2 {
		t.Fatalf("after a new miss escalated = %d, raised = %v, want a new L2", res.Escalated, esc.raised)
	}
}
