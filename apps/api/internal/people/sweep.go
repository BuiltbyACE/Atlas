package people

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/safaristack/atlas/apps/api/internal/audit"
	"github.com/safaristack/atlas/apps/api/internal/outbox"
)

// EscalationEntity is the work.escalations.entity_type the repeated-miss rule
// uses: one open escalation per person, keyed by the person id, so the work
// module's own task ladders (entity_type 'task') are untouched (§8.5, D5).
const EscalationEntity = "person"

// EscalationLevel is the rung a repeated-miss escalation sits at (§5.8, D5):
// L2, the domain CEO's rung, matching the ladder the work sweep uses.
const EscalationLevel = 2

// MissedReport is a report the sweep has just marked missed, carrying the two
// person fields the line-manager notice needs (§8.5).
type MissedReport struct {
	DailyReport
	// LineManagerID is the reporter's line manager, or nil when the person has
	// none — the notice is then skipped (§8.5).
	LineManagerID *uuid.UUID
	ReporterName  string
}

// MissCounts is one person's missed-report count inside the policy window.
type MissCounts struct {
	PersonID uuid.UUID
	Misses   int
	// LastMissedAt is when the newest of those reports was marked missed. It
	// scopes the escalation episode: see Service.escalate.
	LastMissedAt time.Time
}

// Escalator is the work module's escalation ladder seen from people: the
// repeated-miss rule raises an L2 escalation on a person and resolves it when
// the person's compliance recovers (§8.5, D5). *work.Service implements it, so
// the two modules share one ladder and one escalations table without people
// importing work — the same seam the module boundary asks for (ADR-002).
type Escalator interface {
	// PersonForRole resolves the person a company-scoped role is granted to.
	PersonForRole(ctx context.Context, roleKey string) (*uuid.UUID, error)
	// RaiseEscalation opens an escalation, returning true when it opened a new
	// one and false when one was already open for that entity and level.
	RaiseEscalation(ctx context.Context, entityType string, entityID uuid.UUID, level int, reason string, assignee *uuid.UUID) (bool, error)
	// ResolveEscalations closes every open escalation for an entity.
	ResolveEscalations(ctx context.Context, entityType string, entityID uuid.UUID) (int, error)
	// OpenEscalatedEntities lists the entities of a type with an open
	// escalation, so people can resolve the ones whose condition has cleared.
	OpenEscalatedEntities(ctx context.Context, entityType string) ([]uuid.UUID, error)
	// LastResolvedAt reports when the entity's latest escalation was resolved,
	// or nil if none ever was.
	LastResolvedAt(ctx context.Context, entityType string, entityID uuid.UUID) (*time.Time, error)
}

// SweepResult counts one pass's work, for the worker's log line.
type SweepResult struct {
	Opened    int
	Excused   int
	Missed    int
	Late      int
	Escalated int
	Resolved  int
}

func (r SweepResult) empty() bool {
	return r.Opened == 0 && r.Excused == 0 && r.Missed == 0 && r.Late == 0 &&
		r.Escalated == 0 && r.Resolved == 0
}

// Sweep runs one pass of the daily-report lifecycle (§8.5, decision D5,
// ADR-017). It makes every decision the policy implies: the submit path never
// judges lateness, so a report is always accepted and this pass decides whether
// it was on time. That keeps the write path fast (§4.1) and the judgement of
// lateness with the one clock the worker owns. (The submit path shares the
// policy's day boundary but applies none of its decisions.)
//
// The pass is idempotent and safe to run every minute: each step only acts on
// rows still in the previous state, so a repeat pass finds nothing to do and a
// pass after downtime catches up the days it slept through.
func (s *Service) Sweep(ctx context.Context, limit int) (SweepResult, error) {
	var res SweepResult
	policy, err := s.policyFor(ctx)
	if err != nil {
		return res, err
	}
	now := s.now()
	today := policy.Today(now)

	// 1. Prompt: open today's report for everyone who owes one, once the
	// prompt minute is reached. Idempotent — it only opens people with no row.
	if policy.IsWorkingDay(today) && !now.Before(policy.PromptInstant(today)) {
		opened, err := s.store.OpenDueReports(ctx, today, policy.PromptInstant(today), limit,
			func(r DailyReport) (outbox.Event, error) {
				return outbox.NewDailyReportDue(nil, r.ID, r.ReporterID, dateString(r.ReportDate))
			})
		if err != nil {
			return res, fmt.Errorf("people: open due reports: %w", err)
		}
		res.Opened = len(opened)
		s.auditOpened(ctx, opened)
	}

	// 2. Deadline: for every recent working day whose filing deadline has
	// passed, excuse leave, mark the rest missed, then relabel late filings.
	var missed []MissedReport
	for _, day := range policy.deadlineDays(now, today) {
		excused, err := s.store.ExcuseDueReports(ctx, day)
		if err != nil {
			return res, fmt.Errorf("people: excuse daily reports: %w", err)
		}
		res.Excused += excused

		deadline := policy.DeadlineInstant(day)
		dayMissed, err := s.store.MarkMissedReports(ctx, day, deadline,
			func(mr MissedReport) (outbox.Event, error) {
				return outbox.NewDailyReportMissed(nil, mr.ID, mr.ReporterID, dateString(mr.ReportDate), mr.LineManagerID)
			})
		if err != nil {
			return res, fmt.Errorf("people: mark reports missed: %w", err)
		}
		res.Missed += len(dayMissed)
		missed = append(missed, dayMissed...)
		s.auditMissed(ctx, dayMissed)

		late, err := s.store.MarkLateReports(ctx, day, deadline)
		if err != nil {
			return res, fmt.Errorf("people: mark reports late: %w", err)
		}
		res.Late += late
	}

	// 3. Miss counts over the window, used by both the notice and the ladder.
	counts, err := s.store.MissedCounts(ctx, policy.windowStart(today), 1)
	if err != nil {
		return res, fmt.Errorf("people: count missed reports: %w", err)
	}
	byPerson := make(map[uuid.UUID]int, len(counts))
	for _, c := range counts {
		byPerson[c.PersonID] = c.Misses
	}

	// 4. Notify the line manager for each report this pass marked missed. The
	// durable record is the row and its event, so this is best-effort (§6.4).
	s.notifyMissed(ctx, policy, missed, byPerson)

	// 5. Escalation: L2 for anyone at or over the threshold, and resolve the
	// escalations of anyone whose count has fallen back below it.
	if s.escalator != nil {
		breaching, raised, err := s.escalate(ctx, policy, counts)
		if err != nil {
			return res, err
		}
		res.Escalated = raised
		resolved, err := s.resolveRecovered(ctx, breaching)
		if err != nil {
			return res, err
		}
		res.Resolved = resolved
	}

	return res, nil
}

// escalate raises one L2 per person at or over the miss threshold and returns
// the set of person ids that are currently breaching, so the resolve pass knows
// which escalations to leave alone, along with how many it raised.
func (s *Service) escalate(ctx context.Context, policy DailyReportPolicy, counts []MissCounts) (map[uuid.UUID]bool, int, error) {
	breaching := make(map[uuid.UUID]bool)
	assignee, err := s.escalator.PersonForRole(ctx, policy.L2Role)
	if err != nil {
		return nil, 0, fmt.Errorf("people: resolve L2 assignee: %w", err)
	}
	if assignee == nil {
		s.logger.WarnContext(ctx, "people: no one holds the L2 role; escalating unassigned",
			slog.String("role", policy.L2Role))
	}
	raised := 0
	for _, c := range counts {
		if c.Misses < policy.MissedThreshold {
			continue
		}
		breaching[c.PersonID] = true
		// An escalation someone resolved closes that episode: the one-open
		// index alone would let the next pass raise a fresh copy at once while
		// the count is still over the threshold (the "same reason raised 3x"
		// duplicates). Only a miss after that resolution opens a new episode.
		resolvedAt, err := s.escalator.LastResolvedAt(ctx, EscalationEntity, c.PersonID)
		if err != nil {
			return nil, raised, fmt.Errorf("people: read miss escalation history: %w", err)
		}
		if resolvedAt != nil && !c.LastMissedAt.After(*resolvedAt) {
			continue
		}
		reason := fmt.Sprintf("%d daily reports missed in %d days", c.Misses, policy.MissedWindowDays)
		opened, err := s.escalator.RaiseEscalation(ctx, EscalationEntity, c.PersonID, EscalationLevel, reason, assignee)
		if err != nil {
			return nil, raised, fmt.Errorf("people: raise miss escalation: %w", err)
		}
		if opened {
			raised++
			s.logger.InfoContext(ctx, "people: raised L2 for missed daily reports",
				slog.String("person_id", c.PersonID.String()), slog.Int("misses", c.Misses))
		}
	}
	return breaching, raised, nil
}

// resolveRecovered closes the open person escalations for anyone no longer
// breaching — a person who filed the reports they owed, or whose misses have
// aged out of the window. It returns how many escalations it closed.
func (s *Service) resolveRecovered(ctx context.Context, breaching map[uuid.UUID]bool) (int, error) {
	open, err := s.escalator.OpenEscalatedEntities(ctx, EscalationEntity)
	if err != nil {
		return 0, fmt.Errorf("people: list open miss escalations: %w", err)
	}
	resolved := 0
	for _, personID := range open {
		if breaching[personID] {
			continue
		}
		n, err := s.escalator.ResolveEscalations(ctx, EscalationEntity, personID)
		if err != nil {
			return resolved, fmt.Errorf("people: resolve miss escalation: %w", err)
		}
		if n > 0 {
			s.logger.InfoContext(ctx, "people: resolved miss escalation",
				slog.String("person_id", personID.String()))
		}
		resolved += n
	}
	return resolved, nil
}

// notifyMissed tells each missed report's line manager (§8.5). Fan-out is
// best-effort and happens after the state change committed: a notification
// failure is logged, never returned, so it cannot fail a sweep whose durable
// work is already done (§6.4).
func (s *Service) notifyMissed(ctx context.Context, policy DailyReportPolicy, missed []MissedReport, byPerson map[uuid.UUID]int) {
	if s.notifier == nil || len(missed) == 0 {
		return
	}
	for _, mr := range missed {
		if mr.LineManagerID == nil {
			continue
		}
		n := MissedReportNotice{
			LineManagerID:  *mr.LineManagerID,
			ReporterID:     mr.ReporterID,
			ReporterName:   mr.ReporterName,
			ReportID:       mr.ID,
			ReportDate:     dateString(mr.ReportDate),
			Misses:         byPerson[mr.ReporterID],
			Threshold:      policy.MissedThreshold,
			WindowDays:     policy.MissedWindowDays,
			IdempotencyKey: "people.daily_report.missed:" + mr.ID.String(),
		}
		if err := s.notifier.NotifyMissedReport(ctx, n); err != nil {
			s.logger.ErrorContext(ctx, "people: missed-report notice failed",
				slog.String("report_id", mr.ID.String()),
				slog.String("error", err.Error()))
		}
	}
}

func (s *Service) auditOpened(ctx context.Context, opened []DailyReport) {
	for _, r := range opened {
		s.writeAuditAs(ctx, audit.ActorSystem, "system", "people.daily_report.due", EntityDailyReport, r.ID,
			nil, dailyReportSnapshot(r))
	}
}

func (s *Service) auditMissed(ctx context.Context, missed []MissedReport) {
	for _, mr := range missed {
		s.writeAuditAs(ctx, audit.ActorSystem, "system", "people.daily_report.missed", EntityDailyReport, mr.ID,
			nil, dailyReportSnapshot(mr.DailyReport))
	}
}

// policyFor reads the policy the sweep applies. With no reader wired (the API,
// and tests) the D5 defaults are used; a wired reader's failure is returned so
// the pass is skipped and retried rather than applied on a rhythm the operator
// did not choose (policy.go).
func (s *Service) policyFor(ctx context.Context) (DailyReportPolicy, error) {
	if s.policy == nil {
		return DefaultDailyReportPolicy().normalize(), nil
	}
	return s.policy.DailyReportPolicy(ctx)
}

// windowStart is the first report date the miss-count window covers: the
// threshold is "N misses in D days" counting today and the D-1 days before it.
func (p DailyReportPolicy) windowStart(today time.Time) time.Time {
	return today.AddDate(0, 0, -(p.MissedWindowDays - 1))
}

// deadlineDays returns the working days whose filing deadline has passed as of
// now, newest first, bounded by the escalation window. A worker that was down
// catches up on the misses it slept through; nothing older than the window can
// still change a person's escalation state, so the walk stops there.
func (p DailyReportPolicy) deadlineDays(now, today time.Time) []time.Time {
	back := p.MissedWindowDays + 2
	out := make([]time.Time, 0, back)
	for i := 0; i <= back; i++ {
		day := today.AddDate(0, 0, -i)
		if !p.IsWorkingDay(day) {
			continue
		}
		if p.DeadlineInstant(day).After(now) {
			continue
		}
		out = append(out, day)
	}
	return out
}
