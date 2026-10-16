package people

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/safaristack/atlas/apps/api/internal/audit"
)

// Entity types used in the audit trail and event aggregate names.
const (
	EntityDailyReport  = "people.daily_report"
	EntityLeaveRequest = "people.leave_request"
	EntityPerson       = "people.person"
)

// Service coordinates the people module: validation, state transitions,
// optimistic concurrency, auditing and events.
type Service struct {
	store  Store
	audit  *audit.Writer
	now    func() time.Time
	logger *slog.Logger
}

// NewService builds the people service. aw may be nil in tests, in which case
// changes are still applied and only the audit row is skipped.
func NewService(store Store, aw *audit.Writer, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{store: store, audit: aw, now: func() time.Time { return time.Now().UTC() }, logger: logger}
}

// accountForPerson resolves a person to the account realtime nudges are
// addressed to, or nil when the person has no account (§7.5).
func (s *Service) accountForPerson(ctx context.Context, personID uuid.UUID) (*uuid.UUID, error) {
	return s.store.AccountForPerson(ctx, personID)
}

// writeAudit records a state change. A nil writer makes it a no-op.
func (s *Service) writeAudit(ctx context.Context, actor uuid.UUID, action, entityType string, entityID uuid.UUID, before, after any) {
	if s.audit == nil {
		return
	}
	id := entityID
	if err := s.audit.Write(ctx, nil, audit.Event{
		ActorType:  audit.ActorUser,
		ActorID:    actor.String(),
		Action:     action,
		EntityType: entityType,
		EntityID:   &id,
		Before:     before,
		After:      after,
	}); err != nil {
		s.logger.ErrorContext(ctx, "people: audit write failed",
			slog.String("action", action), slog.String("error", err.Error()))
	}
}
