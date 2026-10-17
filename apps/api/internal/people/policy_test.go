package people

import (
	"context"
	"testing"
	"time"
)

// eat is the policy zone D5 was decided in (Africa/Nairobi, UTC+3, no DST).
var eat = time.FixedZone("EAT", 3*60*60)

func at(y int, m time.Month, d, h, min int, loc *time.Location) time.Time {
	return time.Date(y, m, d, h, min, 0, 0, loc)
}

func TestDailyReportPolicy_Instants(t *testing.T) {
	// 2026-10-16 is a Friday; 2026-10-19 is the Monday after it.
	friday := time.Date(2026, time.October, 16, 0, 0, 0, 0, time.UTC)
	monday := time.Date(2026, time.October, 19, 0, 0, 0, 0, time.UTC)

	p := DefaultDailyReportPolicy().normalize()

	// 16:30 EAT is 13:30 UTC.
	if got, want := p.PromptInstant(friday), at(2026, time.October, 16, 13, 30, time.UTC); !got.Equal(want) {
		t.Errorf("PromptInstant(Fri) = %s, want %s", got, want)
	}

	// A Friday report falls due 09:00 the next working day: Monday.
	if got, want := p.DeadlineInstant(friday), at(2026, time.October, 19, 6, 0, time.UTC); !got.Equal(want) {
		t.Errorf("DeadlineInstant(Fri) = %s, want %s", got, want)
	}

	// A Monday report falls due Tuesday.
	if got, want := p.DeadlineInstant(monday), at(2026, time.October, 20, 6, 0, time.UTC); !got.Equal(want) {
		t.Errorf("DeadlineInstant(Mon) = %s, want %s", got, want)
	}
}

func TestDailyReportPolicy_NextWorkingDay(t *testing.T) {
	p := DefaultDailyReportPolicy().normalize()
	cases := []struct {
		name string
		day  time.Time
		want time.Time
	}{
		{"friday -> monday", time.Date(2026, 10, 16, 0, 0, 0, 0, time.UTC), time.Date(2026, 10, 19, 0, 0, 0, 0, time.UTC)},
		{"saturday -> monday", time.Date(2026, 10, 17, 0, 0, 0, 0, time.UTC), time.Date(2026, 10, 19, 0, 0, 0, 0, time.UTC)},
		{"monday -> tuesday", time.Date(2026, 10, 19, 0, 0, 0, 0, time.UTC), time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := p.NextWorkingDay(tc.day); !got.Equal(tc.want) {
				t.Errorf("NextWorkingDay(%s) = %s, want %s", tc.day, got, tc.want)
			}
		})
	}
}

func TestDailyReportPolicy_IsWorkingDayAndToday(t *testing.T) {
	p := DefaultDailyReportPolicy().normalize()

	if !p.IsWorkingDay(time.Date(2026, 10, 16, 0, 0, 0, 0, time.UTC)) {
		t.Error("Friday should be a working day")
	}
	if p.IsWorkingDay(time.Date(2026, 10, 17, 0, 0, 0, 0, time.UTC)) {
		t.Error("Saturday should not be a working day")
	}

	// 02:00 EAT on Saturday is only 23:00 UTC on Friday, but already Saturday in
	// the policy zone: Today must follow the policy zone, not the UTC date.
	earlySaturday := at(2026, time.October, 17, 2, 0, eat)
	if got, want := p.Today(earlySaturday), time.Date(2026, 10, 17, 0, 0, 0, 0, time.UTC); !got.Equal(want) {
		t.Errorf("Today(02:00 EAT Sat) = %s, want the policy-zone date %s", got, want)
	}
}

func TestDailyReportPolicy_Normalize(t *testing.T) {
	p := DailyReportPolicy{}.normalize()
	if p.Timezone != time.UTC {
		t.Errorf("nil zone = %s, want UTC", p.Timezone)
	}
	if len(p.WorkingDays) == 0 {
		t.Error("empty working week should normalize to Monday-Friday")
	}
	if p.MissedThreshold < 1 || p.MissedWindowDays < 1 {
		t.Errorf("miss rule = %d/%d, want both >= 1", p.MissedThreshold, p.MissedWindowDays)
	}
	if p.L2Role == "" {
		t.Error("empty L2 role should normalize to a role")
	}
	if got := p.PromptInstant(time.Date(2026, 10, 16, 0, 0, 0, 0, time.UTC)); got.IsZero() {
		t.Error("PromptInstant should not be the zero time")
	}
	// Minutes saturate rather than wrapping into the wrong day.
	over := DailyReportPolicy{PromptMinute: 5000, DeadlineMinute: -5}.normalize()
	if over.PromptMinute != 1439 || over.DeadlineMinute != 0 {
		t.Errorf("clamp = %d/%d, want 1439/0", over.PromptMinute, over.DeadlineMinute)
	}
}

func TestParseWorkingDays(t *testing.T) {
	cases := []struct {
		in   string
		want []time.Weekday
	}{
		{"1,2,3,4,5", []time.Weekday{time.Monday, time.Tuesday, time.Wednesday, time.Thursday, time.Friday}},
		{"6,7", []time.Weekday{time.Saturday, time.Sunday}},
		{" 1 , 3 ", []time.Weekday{time.Monday, time.Wednesday}},
		{"0,8,x", nil},
		{"", nil},
	}
	for _, tc := range cases {
		got := parseWorkingDays(tc.in)
		if len(got) != len(tc.want) {
			t.Errorf("parseWorkingDays(%q) has %d days, want %d", tc.in, len(got), len(tc.want))
			continue
		}
		for _, d := range tc.want {
			if !got[d] {
				t.Errorf("parseWorkingDays(%q) missing %s", tc.in, d)
			}
		}
	}
}

func TestDailyReportPolicy_WindowStart(t *testing.T) {
	p := DailyReportPolicy{MissedThreshold: 3, MissedWindowDays: 10, Timezone: time.UTC}
	today := time.Date(2026, 10, 16, 0, 0, 0, 0, time.UTC)
	// "10 days" counts today and the 9 days before it.
	want := time.Date(2026, 10, 7, 0, 0, 0, 0, time.UTC)
	if got := p.windowStart(today); !got.Equal(want) {
		t.Errorf("windowStart = %s, want %s", got, want)
	}
}

func TestDailyReportPolicy_DeadlineDays(t *testing.T) {
	p := DefaultDailyReportPolicy().normalize()
	// Monday 10:00 EAT: Friday's deadline (09:00 Monday) has passed, Monday's
	// own deadline (09:00 Tuesday) has not, and the weekend is skipped.
	now := at(2026, time.October, 19, 10, 0, eat)
	today := p.Today(now)

	days := p.deadlineDays(now, today)
	if len(days) == 0 {
		t.Fatal("deadlineDays returned nothing")
	}
	// Monday is the newest working day, but its deadline (Tuesday 09:00) has not
	// passed, so the newest qualifying day is Friday.
	if want := time.Date(2026, 10, 16, 0, 0, 0, 0, time.UTC); !days[0].Equal(want) {
		t.Errorf("first day = %s, want Friday", days[0])
	}
	for _, d := range days {
		if !p.IsWorkingDay(d) {
			t.Errorf("%s is not a working day", d)
		}
		if p.DeadlineInstant(d).After(now) {
			t.Errorf("deadline for %s has not passed yet", d)
		}
		if d.After(today) {
			t.Errorf("day %s is in the future", d)
		}
	}
	monday := time.Date(2026, 10, 19, 0, 0, 0, 0, time.UTC)
	for _, d := range days {
		if d.Equal(monday) {
			t.Error("Monday's deadline has not passed yet and must not be swept")
		}
	}
}

// TestServiceTodayFollowsPolicyZone guards the day boundary the API and the
// sweep must share: at 22:00 UTC it is already the next day in EAT, so an
// unfiled-today lookup and the sweep's opened day agree (ADR-017). Without a
// wired policy the boundary stays the UTC day, which is what tests rely on.
func TestServiceTodayFollowsPolicyZone(t *testing.T) {
	// 22:00 UTC on the 17th is 01:00 EAT on the 18th.
	svc := NewService(nil, nil, discardLogger(), 0)
	svc.now = func() time.Time { return time.Date(2026, 10, 17, 22, 0, 0, 0, time.UTC) }

	utcDay := time.Date(2026, 10, 17, 0, 0, 0, 0, time.UTC)
	if got := svc.today(context.Background()); !got.Equal(utcDay) {
		t.Errorf("unwired today = %s, want the UTC day %s", got, utcDay)
	}

	svc.SetPolicy(StaticPolicy{Value: d5Policy()}) // EAT
	eatDay := time.Date(2026, 10, 18, 0, 0, 0, 0, time.UTC)
	if got := svc.today(context.Background()); !got.Equal(eatDay) {
		t.Errorf("wired today = %s, want the EAT day %s", got, eatDay)
	}
}
