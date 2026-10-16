package dashboard

import (
	"context"
	"log/slog"
	"time"

	"github.com/safaristack/atlas/apps/api/internal/audit"
)

// defaultAuditHighlights is how many recent audit events the dashboard shows.
const defaultAuditHighlights = 10

// Service assembles the dashboard from the read model and the audit trail.
type Service struct {
	store      Store
	auditStore audit.Store
	logger     *slog.Logger
	now        func() time.Time
}

// NewService builds the service. auditStore supplies the recent-highlights
// tile through the same reader the audit views use, redaction included.
func NewService(store Store, auditStore audit.Store, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{store: store, auditStore: auditStore, logger: logger, now: time.Now}
}

// Summary gathers every tile as at now.
func (s *Service) Summary(ctx context.Context) (Summary, error) {
	now := s.now().UTC()

	people, err := s.store.PeopleByStatus(ctx)
	if err != nil {
		return Summary{}, err
	}
	approvals, err := s.store.Approvals(ctx, now)
	if err != nil {
		return Summary{}, err
	}
	recent, err := s.auditStore.List(ctx, audit.Query{}, defaultAuditHighlights, nil)
	if err != nil {
		return Summary{}, err
	}
	if approvals.PendingByRole == nil {
		approvals.PendingByRole = map[string]int{}
	}

	return Summary{
		GeneratedAt: now,
		People:      peopleSummary(people),
		Approvals:   approvals,
		RecentAudit: highlights(recent),
	}, nil
}

// peopleSummary fills the known statuses with zero so the tile has a stable
// shape, and totals every row (including any status outside the known set).
func peopleSummary(counts map[string]int) PeopleSummary {
	byStatus := make(map[string]int, len(PersonStatuses)+len(counts))
	for _, status := range PersonStatuses {
		byStatus[status] = 0
	}
	total := 0
	for status, n := range counts {
		byStatus[status] = n
		total += n
	}
	return PeopleSummary{Total: total, ByStatus: byStatus}
}

func highlights(entries []audit.Entry) []AuditHighlight {
	out := make([]AuditHighlight, 0, len(entries))
	for _, e := range entries {
		h := AuditHighlight{
			ID:         e.ID.String(),
			OccurredAt: e.OccurredAt,
			ActorType:  e.ActorType,
			ActorID:    e.ActorID,
			Action:     e.Action,
			EntityType: e.EntityType,
		}
		if e.EntityID != nil {
			h.EntityID = e.EntityID.String()
		}
		out = append(out, h)
	}
	return out
}
