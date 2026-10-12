package notifications

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func minutes(v int) *int { return &v }

// nairobi renders a Nairobi wall-clock time as UTC for assertions.
func nairobi(y int, mo time.Month, d, h, mi int) time.Time {
	return time.Date(y, mo, d, h, mi, 0, 0, time.FixedZone("EAT", 3*60*60)).UTC()
}

func TestChannelsFor(t *testing.T) {
	tests := []struct {
		name string
		pref Preference
		want []Channel
	}{
		{
			name: "in-app always, email by default",
			pref: DefaultPreference(uuid.Nil, "Africa/Nairobi"),
			want: []Channel{ChannelInApp, ChannelEmail},
		},
		{
			name: "email disabled",
			pref: Preference{InApp: true, Email: false, Timezone: "Africa/Nairobi"},
			want: []Channel{ChannelInApp},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ChannelsFor(tt.pref)
			if len(got) != len(tt.want) {
				t.Fatalf("ChannelsFor = %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("ChannelsFor = %v, want %v", got, tt.want)
				}
			}
		})
	}
}

func TestInQuietHours(t *testing.T) {
	sameDay := Preference{QuietStart: minutes(12 * 60), QuietEnd: minutes(14 * 60), Timezone: "Africa/Nairobi"}
	wrap := Preference{QuietStart: minutes(22 * 60), QuietEnd: minutes(7 * 60), Timezone: "Africa/Nairobi"}
	disabled := Preference{Timezone: "Africa/Nairobi"}
	empty := Preference{QuietStart: minutes(600), QuietEnd: minutes(600), Timezone: "Africa/Nairobi"}

	tests := []struct {
		name string
		pref Preference
		at   time.Time
		want bool
	}{
		{"inside same-day", sameDay, nairobi(2026, time.October, 13, 13, 0), true},
		{"before same-day", sameDay, nairobi(2026, time.October, 13, 11, 59), false},
		{"at start is inside", sameDay, nairobi(2026, time.October, 13, 12, 0), true},
		{"at end is outside", sameDay, nairobi(2026, time.October, 13, 14, 0), false},
		{"after midnight in wrap", wrap, nairobi(2026, time.October, 13, 2, 0), true},
		{"before wrap start", wrap, nairobi(2026, time.October, 13, 21, 0), false},
		{"after wrap ends", wrap, nairobi(2026, time.October, 13, 9, 0), false},
		{"no preference", disabled, nairobi(2026, time.October, 13, 13, 0), false},
		{"equal start and end disables", empty, nairobi(2026, time.October, 13, 10, 0), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := InQuietHours(tt.pref, tt.at); got != tt.want {
				t.Fatalf("InQuietHours = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestQuietEndsAt(t *testing.T) {
	p := Preference{QuietStart: minutes(22 * 60), QuietEnd: minutes(7 * 60), Timezone: "Africa/Nairobi"}

	// 23:00 Nairobi (20:00 UTC) — the next 07:00 is the following morning.
	got := QuietEndsAt(p, time.Date(2026, time.October, 13, 20, 0, 0, 0, time.UTC))
	want := time.Date(2026, time.October, 14, 4, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("QuietEndsAt = %s, want %s", got, want)
	}

	// 02:00 Nairobi (23:00 UTC previous day) — the next 07:00 is the same day.
	got = QuietEndsAt(p, time.Date(2026, time.October, 13, 23, 0, 0, 0, time.UTC))
	want = time.Date(2026, time.October, 14, 4, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("QuietEndsAt = %s, want %s", got, want)
	}
}

func TestCursorRoundTrip(t *testing.T) {
	at := time.Date(2026, time.October, 13, 10, 11, 12, 123456789, time.UTC)
	id := uuid.Must(uuid.NewV7())

	decoded, err := decodeCursor(encodeCursor(at, id))
	if err != nil {
		t.Fatalf("decodeCursor: %v", err)
	}
	if !decoded.At.Equal(at) {
		t.Fatalf("At = %s, want %s", decoded.At, at)
	}
	if decoded.ID != id {
		t.Fatalf("ID = %s, want %s", decoded.ID, id)
	}
}

func TestDecodeCursorRejectsGarbage(t *testing.T) {
	if _, err := decodeCursor("not-a-cursor"); err == nil {
		t.Fatal("decodeCursor accepted garbage")
	}
}
