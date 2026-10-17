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
	// presenceWindow decides when a live session counts as "online" (§8.5).
	presenceWindow time.Duration
	// policy reads decision D5's daily-report rhythm at sweep time. Only the
	// worker's sweeper needs it; a nil policy falls back to the defaults.
	policy PolicyReader
	// escalator is the work module's escalation ladder, used by the sweep's
	// repeated-miss rule (§8.5, D5). A nil escalator skips escalation.
	escalator Escalator
	// notifier fans out the line-manager notice for a missed report (§8.5, D5).
	notifier Notifier
}

// DefaultPresenceWindow is the fallback "online" window: two missed one-minute
// heartbeats. Production sets ATLAS_SESSION_PRESENCE_WINDOW.
const DefaultPresenceWindow = 2 * time.Minute

// NewService builds the people service. aw may be nil in tests, in which case
// changes are still applied and only the audit row is skipped. A non-positive
// presenceWindow falls back to DefaultPresenceWindow.
func NewService(store Store, aw *audit.Writer, logger *slog.Logger, presenceWindow time.Duration) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	if presenceWindow <= 0 {
		presenceWindow = DefaultPresenceWindow
	}
	return &Service{
		store:          store,
		audit:          aw,
		now:            func() time.Time { return time.Now().UTC() },
		logger:         logger,
		presenceWindow: presenceWindow,
	}
}

// SetPolicy wires the daily-report policy (§8.5, D5). The worker's sweep reads
// the rhythm and the miss rule from it; the API reads only its zone for the
// "today" day boundary, so an unfiled-today lookup and the sweep agree. Left
// unwired, the sweep uses DefaultDailyReportPolicy and "today" is the UTC day.
func (s *Service) SetPolicy(p PolicyReader) { s.policy = p }

// SetEscalator wires the work module's escalation ladder for the repeated-miss
// rule (§8.5, D5). Left unwired the sweep still marks misses but escalates
// nothing — the right default for a process that is not the worker.
func (s *Service) SetEscalator(e Escalator) { s.escalator = e }

// SetNotifier wires the line-manager notice for a missed report (§8.5, D5).
// Fan-out is best-effort; left unwired it is skipped.
func (s *Service) SetNotifier(n Notifier) { s.notifier = n }

// accountForPerson resolves a person to the account realtime nudges are
// addressed to, or nil when the person has no account (§7.5).
func (s *Service) accountForPerson(ctx context.Context, personID uuid.UUID) (*uuid.UUID, error) {
	return s.store.AccountForPerson(ctx, personID)
}

// writeAudit records a state change. A nil writer makes it a no-op.
func (s *Service) writeAudit(ctx context.Context, actor uuid.UUID, action, entityType string, entityID uuid.UUID, before, after any) {
	s.writeAuditAs(ctx, audit.ActorUser, actor.String(), action, entityType, entityID, before, after)
}

// writeAuditAs is writeAudit with an explicit actor type, so the time-driven
// daily-report sweep (no person involved) is recorded as the system rather than
// a person.
func (s *Service) writeAuditAs(ctx context.Context, actorType, actorID, action, entityType string, entityID uuid.UUID, before, after any) {
	if s.audit == nil {
		return
	}
	id := entityID
	if err := s.audit.Write(ctx, nil, audit.Event{
		ActorType:  actorType,
		ActorID:    actorID,
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
