package people

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

// uuidOf builds a deterministic distinct id for a test row.
func uuidOf(n byte) uuid.UUID { return uuid.UUID{15: n} }

// TestClassifySessionActivity pins the rules the activity view depends on:
// online means live, unexpired and seen inside the presence window; the active
// span runs from sign-in to the explicit end (revoked) or the last heartbeat.
func TestClassifySessionActivity(t *testing.T) {
	window := 2 * time.Minute
	svc := NewService(nil, nil, nil, window)
	now := time.Date(2026, 2, 3, 12, 0, 0, 0, time.UTC)
	started := now.Add(-2 * time.Hour)

	revoked := now.Add(-30 * time.Minute)
	revokedAt := revoked

	sessions := []Session{
		{ // online: seen a minute ago
			ID: uuidOf(1), StartedAt: started, LastSeenAt: now.Add(-time.Minute),
			ExpiresAt: now.Add(time.Hour),
		},
		{ // abandoned: live but last seen past the window
			ID: uuidOf(2), StartedAt: started, LastSeenAt: now.Add(-10 * time.Minute),
			ExpiresAt: now.Add(time.Hour),
		},
		{ // signed out: explicit end wins over last_seen
			ID: uuidOf(3), StartedAt: started, LastSeenAt: now.Add(-5 * time.Minute),
			RevokedAt: &revokedAt, ExpiresAt: now.Add(time.Hour),
		},
		{ // expired: a live row past its absolute lifetime is not online
			ID: uuidOf(4), StartedAt: started, LastSeenAt: now.Add(-time.Minute),
			ExpiresAt: now.Add(-time.Second),
		},
	}

	got := svc.classify(sessions, now)
	if len(got) != len(sessions) {
		t.Fatalf("classify returned %d entries, want %d", len(got), len(sessions))
	}

	want := []struct {
		online bool
		ended  bool
		active int64
	}{
		{online: true, ended: false, active: int64((now.Add(-time.Minute)).Sub(started).Seconds())},
		{online: false, ended: false, active: int64((now.Add(-10 * time.Minute)).Sub(started).Seconds())},
		{online: false, ended: true, active: int64(revoked.Sub(started).Seconds())},
		{online: false, ended: false, active: int64((now.Add(-time.Minute)).Sub(started).Seconds())},
	}
	for i, w := range want {
		if got[i].Online != w.online {
			t.Errorf("session %d: online = %v, want %v", i, got[i].Online, w.online)
		}
		if (got[i].EndedAt != nil) != w.ended {
			t.Errorf("session %d: ended = %v, want %v", i, got[i].EndedAt != nil, w.ended)
		}
		if got[i].ActiveSeconds != w.active {
			t.Errorf("session %d: active = %d, want %d", i, got[i].ActiveSeconds, w.active)
		}
	}
}

// TestClassifyClampsNegativeSpan guards against a clock skew that puts the end
// before the start: the span never goes negative.
func TestClassifyClampsNegativeSpan(t *testing.T) {
	svc := NewService(nil, nil, nil, time.Minute)
	now := time.Date(2026, 2, 3, 12, 0, 0, 0, time.UTC)
	got := svc.classify([]Session{{
		ID: uuidOf(9), StartedAt: now.Add(time.Minute), LastSeenAt: now,
		ExpiresAt: now.Add(time.Hour),
	}}, now)
	if got[0].ActiveSeconds != 0 {
		t.Errorf("active = %d, want 0", got[0].ActiveSeconds)
	}
}
