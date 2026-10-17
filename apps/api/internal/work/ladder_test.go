package work

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/safaristack/atlas/apps/api/internal/settings"
)

// TestLadderLevel pins the D4 rung arithmetic (ADR-016): a task joins the
// ladder at E1, moves to L2 at E2, and to L3 once overdue by E2+E3. The
// boundaries are inclusive so a task swept exactly on the threshold escalates.
func TestLadderLevel(t *testing.T) {
	lad := Ladder{L1After: 24 * time.Hour, L2After: 48 * time.Hour, L3After: 48 * time.Hour}

	tests := []struct {
		name    string
		overdue time.Duration
		want    int
	}{
		{"fresh task is below the ladder", time.Minute, 0},
		{"an hour overdue is still the owner's problem", time.Hour, 0},
		{"just below E1 stays off the ladder", 23*time.Hour + 59*time.Minute, 0},
		{"E1 raises L1", 24 * time.Hour, 1},
		{"between E1 and E2 stays L1", 47 * time.Hour, 1},
		{"E2 raises L2", 48 * time.Hour, 2},
		{"between E2 and E2+E3 stays L2", 95 * time.Hour, 2},
		{"E2+E3 raises L3", 96 * time.Hour, 3},
		{"long-overdue stays L3", 400 * time.Hour, 3},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := lad.Level(tc.overdue); got != tc.want {
				t.Fatalf("Level(%v) = %d, want %d", tc.overdue, got, tc.want)
			}
		})
	}
}

// TestLadderNormalize proves a misconfigured ladder can degrade its timing but
// never its order: negative windows clamp to zero and E1 can never jump past
// E2, so no rung is skipped.
func TestLadderNormalize(t *testing.T) {
	got := Ladder{L1After: -time.Hour, L2After: -2 * time.Hour, L3After: -3 * time.Hour}.normalize()
	if got.L1After != 0 || got.L2After != 0 || got.L3After != 0 {
		t.Fatalf("negative windows = %+v, want all zero", got)
	}

	got = Ladder{L1After: 48 * time.Hour, L2After: 24 * time.Hour, L3After: 0}.normalize()
	if got.L2After < got.L1After {
		t.Fatalf("E2 (%v) below E1 (%v) after normalize", got.L2After, got.L1After)
	}
	if got.Level(60*time.Hour) < 1 {
		t.Fatalf("Level(60h) = %d, want at least L1", got.Level(60*time.Hour))
	}

	// DefaultLadder is decided D4 (ADR-016) and must walk L1 → L2 → L3.
	d := DefaultLadder()
	if d.L1After != 24*time.Hour || d.L2After != 48*time.Hour || d.L3After != 48*time.Hour {
		t.Fatalf("DefaultLadder = %+v, want 24h/48h/+48h (ADR-016)", d)
	}
	if d.L2Role != "ceo_internal" {
		t.Fatalf("DefaultLadder L2 role = %q, want ceo_internal (ADR-016)", d.L2Role)
	}
	if d.Level(50*time.Hour) != 2 || d.Level(100*time.Hour) != 3 {
		t.Fatalf("DefaultLadder rungs wrong: 50h → %d, 100h → %d, want 2/3", d.Level(50*time.Hour), d.Level(100*time.Hour))
	}
}

// stubSettings serves the D4 keys from memory: a missing key returns no row, so
// the settings service falls back to the catalogue default.
type stubSettings struct{ values map[string]any }

func (s stubSettings) List(context.Context) ([]settings.Setting, error) { return nil, nil }

func (s stubSettings) Get(_ context.Context, key string) (*settings.Setting, error) {
	if v, ok := s.values[key]; ok {
		return &settings.Setting{Key: key, Value: v}, nil
	}
	return nil, nil
}

func (s stubSettings) Set(context.Context, settings.SetInput) (*settings.Setting, *settings.Change, error) {
	return nil, nil, errors.New("stubSettings: not implemented")
}

func (s stubSettings) History(context.Context, string, int, *settings.Cursor) ([]settings.Change, error) {
	return nil, nil
}

// TestSettingsLadder proves the sweep reads D4's windows from the typed
// settings store and falls back to the catalogue defaults when a key has no
// row — the production wiring in atlas-worker.
func TestSettingsLadder(t *testing.T) {
	// No rows: the decided defaults (ADR-016) apply.
	fresh, err := NewSettingsLadder(settings.NewService(stubSettings{}, nil, nil)).Ladder(context.Background())
	if err != nil {
		t.Fatalf("Ladder with defaults: %v", err)
	}
	if want := DefaultLadder(); fresh != want {
		t.Fatalf("Ladder with no rows = %+v, want defaults %+v", fresh, want)
	}

	// Stored values win, and only the keys that are set move.
	svc := settings.NewService(stubSettings{values: map[string]any{
		"escalation.e1_hours": float64(4),
		"escalation.l2_role":  "ceo_commercial",
	}}, nil, nil)
	got, err := NewSettingsLadder(svc).Ladder(context.Background())
	if err != nil {
		t.Fatalf("Ladder: %v", err)
	}
	want := Ladder{L1After: 4 * time.Hour, L2After: 48 * time.Hour, L3After: 48 * time.Hour, L2Role: "ceo_commercial"}
	if got != want {
		t.Fatalf("Ladder = %+v, want %+v", got, want)
	}
}
