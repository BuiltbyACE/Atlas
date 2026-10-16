package people

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/safaristack/atlas/apps/api/internal/outbox"
)

// SubmitDailyReport records or refreshes the caller's report for a day. There is
// at most one report per person per day (the table's unique key); re-submitting
// updates that day's report while it has not been reviewed. Emits
// people.daily_report.submitted.v1 and writes an audit row.
//
// The report time-of-day / deadline policy (§8.5, decision D5) is intentionally
// not enforced here: it is configuration that is still open, so any date is
// accepted and the policy is applied by a later decision.
func (s *Service) SubmitDailyReport(ctx context.Context, actor uuid.UUID, in DailyReportInput) (DailyReport, error) {
	r, err := validateDailyReport(DailyReport{
		ReporterID:  actor,
		ReportDate:  in.ReportDate,
		Status:      DailyReportSubmitted,
		WorkDone:    in.WorkDone,
		Blockers:    in.Blockers,
		Risks:       in.Risks,
		NextDay:     in.NextDay,
		HoursWorked: in.HoursWorked,
		Version:     1,
	}, s.now())
	if err != nil {
		return DailyReport{}, err
	}
	id, err := uuid.NewV7()
	if err != nil {
		return DailyReport{}, fmt.Errorf("people: new id: %w", err)
	}
	now := s.now()
	r.ID = id
	r.CreatedAt, r.UpdatedAt = now, now
	r.SubmittedAt = &now

	ev, err := outbox.NewDailyReportSubmitted(&actor, r.ID, actor, dateString(r.ReportDate))
	if err != nil {
		return DailyReport{}, err
	}
	saved, err := s.store.SaveDailyReport(ctx, r, &ev)
	if err != nil {
		return DailyReport{}, err
	}
	s.writeAudit(ctx, actor, "people.daily_report.submitted", EntityDailyReport, saved.ID, nil, dailyReportSnapshot(saved))
	return saved, nil
}

// GetDailyReport returns a report or ErrNotFound.
func (s *Service) GetDailyReport(ctx context.Context, id uuid.UUID) (DailyReport, error) {
	r, err := s.store.GetDailyReport(ctx, id)
	if err != nil {
		return DailyReport{}, err
	}
	if r == nil {
		return DailyReport{}, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	return *r, nil
}

// TodayReport returns the caller's report for today, or ErrNotFound when they
// have not filed one yet. It backs GET /api/v1/daily-reports/today.
func (s *Service) TodayReport(ctx context.Context, reporter uuid.UUID) (DailyReport, error) {
	r, err := s.store.GetDailyReportForDate(ctx, reporter, dateOnly(s.now()))
	if err != nil {
		return DailyReport{}, err
	}
	if r == nil {
		return DailyReport{}, fmt.Errorf("%w: no report for today", ErrNotFound)
	}
	return *r, nil
}

// ListDailyReports returns a page of reports, newest first.
func (s *Service) ListDailyReports(ctx context.Context, q ListDailyReportsQuery, limit int, before *Cursor) ([]DailyReport, error) {
	if limit <= 0 {
		limit = 50
	}
	return s.store.ListDailyReports(ctx, q, limit, before)
}

func dailyReportSnapshot(r DailyReport) map[string]any {
	return map[string]any{
		"reporter_person_id": r.ReporterID.String(),
		"report_date":        dateString(r.ReportDate),
		"status":             string(r.Status),
		"hours_worked":       r.HoursWorked,
	}
}
