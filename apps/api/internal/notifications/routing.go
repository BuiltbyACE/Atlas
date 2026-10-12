package notifications

import (
	"time"

	"github.com/google/uuid"
)

// defaultTimezone is used when a person has no preference row (§6.6).
const defaultTimezone = "Africa/Nairobi"

// DefaultPreference returns the defaults for a person who has never expressed
// a preference: in-app and email on, WhatsApp and SMS off, no quiet hours.
func DefaultPreference(personID uuid.UUID, timezone string) Preference {
	if timezone == "" {
		timezone = defaultTimezone
	}
	return Preference{PersonID: personID, InApp: true, Email: true, Timezone: timezone}
}

// ChannelsFor returns the channels a notification is created on: in-app always,
// plus email when the person has it enabled.
//
// Quiet hours are deliberately NOT decided here: they change *when* a channel is
// used, not *whether* it exists. Service.Request turns a quiet-hours email into a
// deferred one (DeliverAfter), so a person waking up still finds the record
// ("I never got the reminder" stays checkable, §6.4), and a critical item
// bypasses the window. WhatsApp and SMS are not routed yet — Phase 1 ships
// in-app and email (Appendix A) — so they are simply absent here.
func ChannelsFor(p Preference) []Channel {
	out := []Channel{ChannelInApp}
	if p.Email {
		out = append(out, ChannelEmail)
	}
	return out
}

// InQuietHours reports whether at falls inside the person's quiet hours, in
// their timezone. Quiet hours may wrap midnight (20:00 → 07:00). Equal start
// and end disables them.
func InQuietHours(p Preference, at time.Time) bool {
	if p.QuietStart == nil || p.QuietEnd == nil {
		return false
	}
	start, end := *p.QuietStart, *p.QuietEnd
	if start == end {
		return false
	}
	m := minuteOfDay(at, p.Timezone)
	if start < end {
		return m >= start && m < end
	}
	// Wraps midnight: quiet from start until the day rolls over, then until end.
	return m >= start || m < end
}

// QuietEndsAt returns the next moment quiet hours are over, in UTC, so a
// deferred notification is scheduled rather than dropped (§6.4).
func QuietEndsAt(p Preference, at time.Time) time.Time {
	if p.QuietEnd == nil {
		return at.UTC()
	}
	loc := location(p.Timezone)
	local := at.In(loc)
	end := *p.QuietEnd
	target := time.Date(local.Year(), local.Month(), local.Day(), end/60, end%60, 0, 0, loc)
	if !target.After(local) {
		target = target.AddDate(0, 0, 1)
	}
	return target.UTC()
}

func minuteOfDay(at time.Time, timezone string) int {
	local := at.In(location(timezone))
	return local.Hour()*60 + local.Minute()
}

func location(timezone string) *time.Location {
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		return time.UTC
	}
	return loc
}
