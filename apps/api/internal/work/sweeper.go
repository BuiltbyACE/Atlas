package work

import (
	"context"
	"log/slog"
	"time"
)

// Sweeper runs the overdue-escalation ladder (docs/ATLAS.md §3.2, §5.8). It
// belongs to atlas-worker: escalation is time-driven, not request-driven, so it
// must not run in the API's request path (§4.1). The sweeper owns the clock and
// the batch loop and nothing else — the claim, the state changes, the events and
// the audits are the Service's.
type Sweeper struct {
	service  *Service
	interval time.Duration
	limit    int
	logger   *slog.Logger
}

// NewSweeper builds a sweeper. interval and limit come from configuration
// (§2.10); invalid values are clamped to safe defaults so a bad environment
// cannot make the loop spin or sweep nothing.
func NewSweeper(service *Service, interval time.Duration, limit int, logger *slog.Logger) *Sweeper {
	if interval <= 0 {
		interval = time.Minute
	}
	if limit <= 0 {
		limit = 100
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Sweeper{service: service, interval: interval, limit: limit, logger: logger}
}

// Run sweeps on interval until ctx is cancelled. It returns ctx.Err() on
// shutdown; a failed sweep is logged and retried on the next tick rather than
// killing the worker, because one bad batch must not stop escalation.
func (s *Sweeper) Run(ctx context.Context) error {
	s.logger.Info("work: escalation sweeper started",
		slog.Duration("interval", s.interval), slog.Int("batch", s.limit))
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			s.logger.Info("work: escalation sweeper stopped")
			return ctx.Err()
		case <-ticker.C:
			s.sweep(ctx)
		}
	}
}

// sweep runs one pass, draining in batches so a backlog larger than the batch
// size is not left waiting a whole interval.
func (s *Sweeper) sweep(ctx context.Context) {
	raised, resolved := 0, 0
	for {
		r, d, err := s.service.Sweep(ctx, s.limit)
		if err != nil {
			s.logger.Warn("work: escalation sweep failed", slog.String("error", err.Error()))
			return
		}
		raised, resolved = raised+r, resolved+d
		// Another pass is only useful while a batch came back full.
		if r < s.limit && d < s.limit {
			break
		}
	}
	if raised > 0 || resolved > 0 {
		s.logger.Info("work: escalation sweep",
			slog.Int("raised", raised), slog.Int("resolved", resolved))
	}
}

// SweepOnce runs a single pass and returns how many escalations were raised and
// resolved. It exists so callers (and tests) can escalate on demand without the
// ticker.
func (s *Sweeper) SweepOnce(ctx context.Context) (raised, resolved int, err error) {
	return s.service.Sweep(ctx, s.limit)
}
