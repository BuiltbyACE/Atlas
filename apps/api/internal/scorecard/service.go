package scorecard

import (
	"context"
	"log/slog"
	"time"
)

// Service assembles the company scorecard from the read model (§8.9).
type Service struct {
	store Store
	logger *slog.Logger
	now    func() time.Time
}

// NewService builds the service.
func NewService(store Store, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{store: store, logger: logger, now: time.Now}
}

// Scorecard returns the company health roll-up as at now (§8.9).
func (s *Service) Scorecard(ctx context.Context) (Scorecard, error) {
	now := s.now().UTC()
	sc, err := s.store.Scorecard(ctx, now)
	if err != nil {
		return Scorecard{}, err
	}
	sc.GeneratedAt = now
	return sc, nil
}
