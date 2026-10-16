package approvals

import (
	"context"
	"log/slog"
	"time"
)

// Sweeper expires overdue approvals. It belongs to atlas-worker: expiry is
// time-driven, not request-driven, so it must not run in the API's request path
// (docs/ATLAS.md §4.1, §6.3). The sweeper owns the clock and the batch loop and
// nothing else — the claim, the state change, the event and the fan-out are the
// Service's.
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

// Run sweeps on a fixed interval until ctx is cancelled. It returns ctx.Err()
// on shutdown; a failed sweep is logged and retried on the next tick rather
// than killing the worker, because one bad batch must not stop expiry.
func (s *Sweeper) Run(ctx context.Context) error {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	s.logger.Info("approvals: expiry sweeper started", slog.Duration("interval", s.interval))
	for {
		select {
		case <-ctx.Done():
			s.logger.Info("approvals: expiry sweeper stopped")
			return ctx.Err()
		case <-ticker.C:
			s.sweep(ctx)
		}
	}
}

// sweep runs one pass, draining in batches so a backlog larger than the batch
// size is not left waiting a whole interval.
func (s *Sweeper) sweep(ctx context.Context) {
	total := 0
	for {
		n, err := s.service.ExpireDue(ctx, s.limit)
		if err != nil {
			s.logger.Warn("approvals: expiry sweep failed", slog.String("error", err.Error()))
			return
		}
		total += n
		if n < s.limit {
			break
		}
	}
	if total > 0 {
		s.logger.Info("approvals: expired approvals", slog.Int("count", total))
	}
}

// SweepOnce runs a single pass and returns how many approvals were expired. It
// exists so callers (and tests) can expire on demand without the ticker.
func (s *Sweeper) SweepOnce(ctx context.Context) (int, error) {
	return s.service.ExpireDue(ctx, s.limit)
}
