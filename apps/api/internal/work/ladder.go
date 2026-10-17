package work

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/safaristack/atlas/apps/api/internal/settings"
)

// Ladder is decision D4's escalation timing (docs/ATLAS.md §3.3, §5.8;
// docs/adr/ADR-016): how long a task may be overdue before each rung of the
// ladder fires. The windows are configuration, not schema — the sweep reads
// them on every pass from the typed settings store (§6.7), so a CEO can retune
// them without a deploy and every change is audited.
//
// E1, E2 and E3 are measured against the task's deadline (§5.8): L1 fires once
// a task is overdue by E1, L2 once overdue by E2, and L3 once overdue by E2+E3
// — the "L2 unresolved after E3" rung, expressed against the same deadline so
// one timestamp (due_at) scopes the whole episode, exactly as it did when only
// L1 existed.
type Ladder struct {
	L1After time.Duration // E1
	L2After time.Duration // E2
	L3After time.Duration // E3, applied on top of L2After
	L2Role  string        // role key escalated to at L2 (the "domain CEO")
}

// DefaultLadder is decision D4's decided starting configuration: L1 after 24h
// overdue, L2 after 48h, L3 48h after L2 (ADR-016). The settings store serves
// the same numbers as catalogue defaults, so this only applies where no
// settings store is wired (tests).
func DefaultLadder() Ladder {
	return Ladder{
		L1After: 24 * time.Hour,
		L2After: 48 * time.Hour,
		L3After: 48 * time.Hour,
		L2Role:  "ceo_internal",
	}
}

// Level returns the rung an escalation for a task overdue by `overdue` sits at:
// 0 means "not yet on the ladder" (below E1), then 1, 2 and 3. Thresholds are
// evaluated highest-first so a task swept for the first time long after its
// deadline lands directly on the rung its overdue time has reached.
func (l Ladder) Level(overdue time.Duration) int {
	switch {
	case overdue >= l.L2After+l.L3After:
		return 3
	case overdue >= l.L2After:
		return 2
	case overdue >= l.L1After:
		return 1
	default:
		return 0
	}
}

// normalize clamps a misconfigured ladder into one that can still walk: negative
// windows become zero and E1 can never exceed E2, so no rung can be skipped or
// fired out of order. Misconfiguration degrades the ladder's timing, never its
// correctness.
func (l Ladder) normalize() Ladder {
	if l.L1After < 0 {
		l.L1After = 0
	}
	if l.L2After < 0 {
		l.L2After = 0
	}
	if l.L3After < 0 {
		l.L3After = 0
	}
	if l.L2After < l.L1After {
		l.L2After = l.L1After
	}
	return l
}

// LadderReader supplies the ladder at sweep time.
type LadderReader interface {
	Ladder(ctx context.Context) (Ladder, error)
}

// StaticLadder serves one fixed ladder; tests and any process without a
// settings store use it.
type StaticLadder struct{ Value Ladder }

// Ladder implements LadderReader.
func (s StaticLadder) Ladder(context.Context) (Ladder, error) { return s.Value.normalize(), nil }

// SettingsLadder reads the ladder from the typed settings store (§6.7). A
// settings failure is returned, not papered over with defaults: the sweep then
// skips the pass and retries on the next tick, which is safer than escalating
// on numbers the operator did not choose.
type SettingsLadder struct{ settings *settings.Service }

// NewSettingsLadder builds a reader over the settings service.
func NewSettingsLadder(s *settings.Service) *SettingsLadder { return &SettingsLadder{settings: s} }

// Ladder implements LadderReader.
func (r *SettingsLadder) Ladder(ctx context.Context) (Ladder, error) {
	out := DefaultLadder()
	for _, s := range []struct {
		key string
		dst *time.Duration
	}{
		{"escalation.e1_hours", &out.L1After},
		{"escalation.e2_hours", &out.L2After},
		{"escalation.e3_hours", &out.L3After},
	} {
		hours, err := intSetting(ctx, r.settings, s.key)
		if err != nil {
			return Ladder{}, err
		}
		*s.dst = time.Duration(hours) * time.Hour
	}
	role, err := stringSetting(ctx, r.settings, "escalation.l2_role")
	if err != nil {
		return Ladder{}, err
	}
	out.L2Role = role
	return out.normalize(), nil
}

// intSetting reads one known int setting. Values are validated on write, so a
// wrong type means the definition changed underneath old data; the default is
// served rather than poisoning the sweep.
func intSetting(ctx context.Context, svc *settings.Service, key string) (int, error) {
	st, err := svc.Get(ctx, key)
	if err != nil {
		if errors.Is(err, settings.ErrNotFound) {
			return 0, fmt.Errorf("work: escalation setting %s is not in the catalogue", key)
		}
		return 0, fmt.Errorf("work: read setting %s: %w", key, err)
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
			return 0, fmt.Errorf("work: escalation setting %s is not in the catalogue", key)
		}
		if d, ok := def.Default.(float64); ok {
			return int(d), nil
		}
		return 0, fmt.Errorf("work: escalation setting %s has no numeric default", key)
	}
}

// stringSetting reads one known string setting, with the same fallback rule.
func stringSetting(ctx context.Context, svc *settings.Service, key string) (string, error) {
	st, err := svc.Get(ctx, key)
	if err != nil {
		if errors.Is(err, settings.ErrNotFound) {
			return "", fmt.Errorf("work: escalation setting %s is not in the catalogue", key)
		}
		return "", fmt.Errorf("work: read setting %s: %w", key, err)
	}
	if s, ok := st.Value.(string); ok {
		return s, nil
	}
	if def, ok := settings.DefinitionFor(key); ok {
		if s, ok := def.Default.(string); ok {
			return s, nil
		}
	}
	return "", fmt.Errorf("work: escalation setting %s has no string default", key)
}
