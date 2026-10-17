package people

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/safaristack/atlas/apps/api/internal/settings"
)

// DailyReportPolicy is decision D5's daily-report rhythm (docs/ATLAS.md §8.5,
// §9.2; docs/adr/ADR-017): the time a working day's report is prompted, the time
// it falls due, which days are working days, and the repeated-miss rule. It is
// configuration, not schema — the sweep reads it from the typed settings store
// on every pass (§6.7), so a CEO can retune it without a deploy and every change
// is audited.
type DailyReportPolicy struct {
	// PromptMinute and DeadlineMinute are minutes since local midnight.
	PromptMinute   int
	DeadlineMinute int
	// WorkingDays is the set of ISO weekday numbers (1=Mon .. 7=Sun) that count
	// as working days.
	WorkingDays map[time.Weekday]bool
	// MissedThreshold misses within MissedWindowDays escalate a person to L2.
	MissedThreshold  int
	MissedWindowDays int
	// Timezone is the zone the times and working days are expressed in; values
	// are stored in UTC and only this rhythm is local.
	Timezone *time.Location
	// L2Role is the role key a repeated-miss escalation goes to at L2.
	L2Role string
}

// DefaultDailyReportPolicy is decision D5's decided starting configuration
// (ADR-017): prompt at 16:30, due 09:00 the next working day, Monday-Friday,
// three misses in ten days -> L2. The settings store serves the same numbers as
// catalogue defaults, so this only applies where no settings store is wired
// (tests).
func DefaultDailyReportPolicy() DailyReportPolicy {
	return DailyReportPolicy{
		PromptMinute:     16*60 + 30,
		DeadlineMinute:   9 * 60,
		WorkingDays:      weekdays(),
		MissedThreshold:  3,
		MissedWindowDays: 10,
		Timezone:         policyLocation("Africa/Nairobi"),
		L2Role:           "ceo_internal",
	}
}

// IsWorkingDay reports whether t falls on a working day in the policy's zone.
func (p DailyReportPolicy) IsWorkingDay(t time.Time) bool {
	return p.WorkingDays[t.In(p.Timezone).Weekday()]
}

// Today returns the policy-zone calendar date of now, truncated to a UTC date
// (the representation people.daily_reports.report_date uses). It must not go
// through dateOnly: that normalises to UTC first, which would make the day
// boundary fall at 00:00 UTC instead of 00:00 in the policy zone, so a rhythm
// near midnight would open the wrong day's report.
func (p DailyReportPolicy) Today(now time.Time) time.Time {
	t := now.In(p.Timezone)
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

// PromptInstant is the instant on day (a date) at which the report prompt fires.
func (p DailyReportPolicy) PromptInstant(day time.Time) time.Time {
	return p.atMinute(day, p.PromptMinute)
}

// DeadlineInstant is the instant a report for day falls due: the deadline minute
// on the next working day after day. A Friday report is due 09:00 Monday.
func (p DailyReportPolicy) DeadlineInstant(day time.Time) time.Time {
	return p.atMinute(p.NextWorkingDay(day), p.DeadlineMinute)
}

// NextWorkingDay returns the first working day strictly after day. The loop is
// bounded so a policy that (mis)configures no working days falls back to the
// next calendar day instead of spinning forever.
func (p DailyReportPolicy) NextWorkingDay(day time.Time) time.Time {
	d := dateOnly(day).AddDate(0, 0, 1)
	for i := 0; i < 14; i++ {
		if p.IsWorkingDay(d) {
			return d
		}
		d = d.AddDate(0, 0, 1)
	}
	return d
}

// atMinute places day's calendar date at minute-of-day in the policy zone. The
// date components come from day (already a policy-zone date via Today/dateOnly),
// so the constructed instant is local midnight plus the minute.
func (p DailyReportPolicy) atMinute(day time.Time, minute int) time.Time {
	u := day.UTC()
	return time.Date(u.Year(), u.Month(), u.Day(), minute/60, minute%60, 0, 0, p.Timezone)
}

// normalize clamps a misconfigured policy into one that can still walk: a nil
// zone becomes UTC, minutes saturate into a day, an empty working week becomes
// Monday-Friday, and the miss rule cannot be empty. Misconfiguration degrades
// the rhythm, never its correctness.
func (p DailyReportPolicy) normalize() DailyReportPolicy {
	if p.Timezone == nil {
		p.Timezone = time.UTC
	}
	p.PromptMinute = clampMinute(p.PromptMinute)
	p.DeadlineMinute = clampMinute(p.DeadlineMinute)
	if len(p.WorkingDays) == 0 {
		p.WorkingDays = weekdays()
	}
	if p.MissedThreshold < 1 {
		p.MissedThreshold = 1
	}
	if p.MissedWindowDays < 1 {
		p.MissedWindowDays = 1
	}
	if strings.TrimSpace(p.L2Role) == "" {
		p.L2Role = "ceo_internal"
	}
	return p
}

func clampMinute(m int) int {
	if m < 0 {
		return 0
	}
	if m > 1439 {
		return 1439
	}
	return m
}

// weekdays is the default Monday-Friday working week.
func weekdays() map[time.Weekday]bool {
	return map[time.Weekday]bool{
		time.Monday:    true,
		time.Tuesday:   true,
		time.Wednesday: true,
		time.Thursday:  true,
		time.Friday:    true,
	}
}

// PolicyReader supplies the daily-report policy at sweep time.
type PolicyReader interface {
	DailyReportPolicy(ctx context.Context) (DailyReportPolicy, error)
}

// StaticPolicy serves one fixed policy; tests and any process without a settings
// store use it.
type StaticPolicy struct{ Value DailyReportPolicy }

// DailyReportPolicy implements PolicyReader.
func (s StaticPolicy) DailyReportPolicy(context.Context) (DailyReportPolicy, error) {
	return s.Value.normalize(), nil
}

// SettingsPolicy reads the policy from the typed settings store (§6.7). A
// settings failure is returned, not papered over with defaults: the sweep then
// skips the pass and retries on the next tick, which is safer than marking
// reports missed on a rhythm the operator did not choose.
type SettingsPolicy struct{ settings *settings.Service }

// NewSettingsPolicy builds a reader over the settings service.
func NewSettingsPolicy(s *settings.Service) *SettingsPolicy { return &SettingsPolicy{settings: s} }

// DailyReportPolicy implements PolicyReader.
func (r *SettingsPolicy) DailyReportPolicy(ctx context.Context) (DailyReportPolicy, error) {
	out := DefaultDailyReportPolicy()

	prompt, err := policyInt(ctx, r.settings, "people.daily_report.prompt_minute")
	if err != nil {
		return DailyReportPolicy{}, err
	}
	out.PromptMinute = prompt

	deadline, err := policyInt(ctx, r.settings, "people.daily_report.deadline_minute")
	if err != nil {
		return DailyReportPolicy{}, err
	}
	out.DeadlineMinute = deadline

	threshold, err := policyInt(ctx, r.settings, "people.daily_report.missed_threshold")
	if err != nil {
		return DailyReportPolicy{}, err
	}
	out.MissedThreshold = threshold

	window, err := policyInt(ctx, r.settings, "people.daily_report.missed_window_days")
	if err != nil {
		return DailyReportPolicy{}, err
	}
	out.MissedWindowDays = window

	days, err := policyString(ctx, r.settings, "people.daily_report.working_days")
	if err != nil {
		return DailyReportPolicy{}, err
	}
	out.WorkingDays = parseWorkingDays(days)

	tz, err := policyString(ctx, r.settings, "company.timezone")
	if err != nil {
		return DailyReportPolicy{}, err
	}
	out.Timezone = policyLocation(tz)

	role, err := policyString(ctx, r.settings, "escalation.l2_role")
	if err != nil {
		return DailyReportPolicy{}, err
	}
	out.L2Role = role

	return out.normalize(), nil
}

// parseWorkingDays parses a comma-separated list of ISO weekday numbers
// (1=Mon .. 7=Sun). Unknown tokens are ignored; an empty or all-invalid list
// normalizes back to Monday-Friday.
func parseWorkingDays(s string) map[time.Weekday]bool {
	out := make(map[time.Weekday]bool)
	for _, tok := range strings.Split(s, ",") {
		n, err := strconv.Atoi(strings.TrimSpace(tok))
		if err != nil || n < 1 || n > 7 {
			continue
		}
		out[time.Weekday(n%7)] = true // ISO 7 (Sunday) -> Weekday 0
	}
	return out
}

// policyInt reads one known int setting. Values are validated on write, so a
// wrong type means the definition changed underneath old data; the definition
// default is served rather than poisoning the sweep.
func policyInt(ctx context.Context, svc *settings.Service, key string) (int, error) {
	st, err := svc.Get(ctx, key)
	if err != nil {
		if errors.Is(err, settings.ErrNotFound) {
			return 0, fmt.Errorf("people: daily-report setting %s is not in the catalogue", key)
		}
		return 0, fmt.Errorf("people: read setting %s: %w", key, err)
	}
	switch v := st.Value.(type) {
	case float64:
		return int(v), nil
	case int:
		return v, nil
	case int64:
		return int(v), nil
	default:
		def, ok := settings.DefinitionFor(key)
		if !ok {
			return 0, fmt.Errorf("people: daily-report setting %s is not in the catalogue", key)
		}
		if d, ok := def.Default.(float64); ok {
			return int(d), nil
		}
		return 0, fmt.Errorf("people: daily-report setting %s has no numeric default", key)
	}
}

// policyString reads one known string setting, with the same fallback rule.
func policyString(ctx context.Context, svc *settings.Service, key string) (string, error) {
	st, err := svc.Get(ctx, key)
	if err != nil {
		if errors.Is(err, settings.ErrNotFound) {
			return "", fmt.Errorf("people: daily-report setting %s is not in the catalogue", key)
		}
		return "", fmt.Errorf("people: read setting %s: %w", key, err)
	}
	if s, ok := st.Value.(string); ok {
		return s, nil
	}
	if def, ok := settings.DefinitionFor(key); ok {
		if s, ok := def.Default.(string); ok {
			return s, nil
		}
	}
	return "", fmt.Errorf("people: daily-report setting %s has no string default", key)
}

// policyLocation resolves an IANA zone, falling back to UTC when the name is
// unknown. The binaries import time/tzdata so this also resolves inside the
// distroless image, which ships no zoneinfo of its own.
func policyLocation(tz string) *time.Location {
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return time.UTC
	}
	return loc
}
