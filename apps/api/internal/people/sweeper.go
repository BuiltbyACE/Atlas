package people

import (
	"context"
	"log/slog"
	"time"
)

// Sweeper runs the daily-report lifecycle (§8.5, decision D5). It belongs to
// atlas-worker: the prompt, the miss and the late judgement are all
// time-driven, so they must not run in the API's request path (§4.1). The
// sweeper owns only the clock and the batch loop — the state changes, the
// events and the audits are the Service's.
type Sweeper struct {
	service  *Service
	interval time.Duration
	limit    int
	logger   *slog.Logger
}

// NewSweeper builds a sweeper. interval and limit come from configuration
// (§2.10); invalid values are clamped to safe defaults so a bad environment
// cannot make the loop spin or the batch do nothing.
func NewSweeper(service *Service, interval time.Duration, limit int, logger *slog.Logger) *Sweeper {
	if interval <= 0 {
		interval = time.Minute
	}
	if limit <= 0 {
		limit = 500
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Sweeper{service: service, interval: interval, limit: limit, logger: logger}
}

// Run sweeps on interval until ctx is cancelled. It returns ctx.Err() on
// shutdown; a failed pass is logged and retried on the next tick rather than
// killing the worker, because one bad pass must not stop the lifecycle.
func (s *Sweeper) Run(ctx context.Context) error {
	s.logger.Info("people: daily-report sweeper started",
		slog.Duration("interval", s.interval), slog.Int("batch", s.limit))
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			s.logger.Info("people: daily-report sweeper stopped")
			return ctx.Err()
		case <-ticker.C:
			s.sweep(ctx)
		}
	}
}

// sweep runs one pass. Unlike the task ladder the pass is not batch-drained:
// every step is already bounded by the day window, and each is idempotent, so a
// second run inside the same tick would find nothing.
func (s *Sweeper) sweep(ctx context.Context) {
	res, err := s.service.Sweep(ctx, s.limit)
	if err != nil {
		s.logger.Warn("people: daily-report sweep failed", slog.String("error", err.Error()))
		return
	}
	if res.empty() {
		return
	}
	s.logger.Info("people: daily-report sweep",
		slog.Int("opened", res.Opened),
		slog.Int("excused", res.Excused),
		slog.Int("missed", res.Missed),
		slog.Int("late", res.Late),
		slog.Int("escalated", res.Escalated),
		slog.Int("resolved", res.Resolved))
}

// SweepOnce runs a single pass and returns its counts. It exists so callers
// (and tests) can advance the lifecycle on demand without the ticker.
func (s *Sweeper) SweepOnce(ctx context.Context) (SweepResult, error) {
	return s.service.Sweep(ctx, s.limit)
}
