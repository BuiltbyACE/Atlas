package work

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"

	"github.com/safaristack/atlas/apps/api/internal/outbox"
)

// GetEscalation returns an escalation or ErrNotFound.
func (s *Service) GetEscalation(ctx context.Context, id uuid.UUID) (Escalation, error) {
	e, err := s.store.GetEscalation(ctx, id)
	if err != nil {
		return Escalation{}, err
	}
	if e == nil {
		return Escalation{}, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	return *e, nil
}

// ListEscalations returns a page of escalations, newest first.
func (s *Service) ListEscalations(ctx context.Context, q ListEscalationsQuery, limit int, before *Cursor) ([]Escalation, error) {
	if limit <= 0 {
		limit = 50
	}
	return s.store.ListEscalations(ctx, q, limit, before)
}

// ResolveEscalation lets a person close an escalation they have dealt with.
// An escalation raised by the sweep has no person to resolve it, so this is the
// human counterpart to the automatic resolution that follows once the underlying
// task is no longer overdue. Emits core.escalation.resolved.v1.
func (s *Service) ResolveEscalation(ctx context.Context, actor, id uuid.UUID) (Escalation, error) {
	current, err := s.GetEscalation(ctx, id)
	if err != nil {
		return Escalation{}, err
	}
	if current.Status != EscalationOpen {
		return Escalation{}, fmt.Errorf("%w: escalation %s is already resolved", ErrConflict, id)
	}
	resolved := current
	resolved.Status = EscalationResolved
	resolved.ResolvedBy = &actor
	ev, err := outbox.NewEscalationResolved(&actor, escalationRef(resolved))
	if err != nil {
		return Escalation{}, err
	}
	updated, err := s.store.ResolveEscalation(ctx, id, actor, &ev)
	if err != nil {
		return Escalation{}, err
	}
	s.writeAudit(ctx, actor, "work.escalation.resolved", EntityEscalation, updated.ID,
		escalationSnapshot(current), escalationSnapshot(updated))
	return updated, nil
}

// Sweep is the overdue-escalation ladder's engine and belongs to atlas-worker:
// escalation is time-driven, so it must not run in the API's request path
// (docs/ATLAS.md §4.1). It raises level-1 escalations for overdue tasks and
// resolves escalations whose task is no longer overdue, then audits both.
// It returns how many escalations it raised and how many it resolved.
//
// The ladder's later rungs (E1-E3 hours before L2/L3) are decision D4 and still
// open, so only L1 is raised here; the level column already exists so that later
// decision needs configuration, not schema.
func (s *Service) Sweep(ctx context.Context, limit int) (raised, resolved int, err error) {
	raisedEsc, err := s.store.SweepOverdueTasks(ctx, limit, func(t OverdueTask, esc Escalation) ([]outbox.Event, error) {
		assigneeUser, err := s.accountForPerson(ctx, t.AssigneeID)
		if err != nil {
			return nil, err
		}
		overdue, err := outbox.NewTaskOverdue(nil, outbox.TaskOverdueRef{
			ID:             t.ID,
			ProjectID:      t.ProjectID,
			Title:          t.Title,
			DueAt:          t.DueAt,
			AssigneeID:     t.AssigneeID,
			AssigneeUserID: assigneeUser,
			EscalationID:   esc.ID,
			Level:          esc.Level,
		})
		if err != nil {
			return nil, err
		}
		ref := escalationRef(esc)
		ref.AssigneeUserID = assigneeUser
		raised, err := outbox.NewEscalationRaised(nil, ref)
		if err != nil {
			return nil, err
		}
		return []outbox.Event{overdue, raised}, nil
	})
	if err != nil {
		return 0, 0, err
	}
	for _, e := range raisedEsc {
		s.writeAuditAs(ctx, "system", "system", "work.escalation.raised", EntityEscalation,
			e.ID, nil, escalationSnapshot(e))
		s.logger.DebugContext(ctx, "work: escalation raised",
			slog.String("escalation_id", e.ID.String()),
			slog.String("entity_id", e.EntityID.String()),
			slog.Int("level", e.Level))
	}

	resolvedEsc, err := s.store.ResolveStaleEscalations(ctx, limit, func(e Escalation) (outbox.Event, error) {
		return outbox.NewEscalationResolved(nil, escalationRef(e))
	})
	if err != nil {
		return len(raisedEsc), 0, err
	}
	for _, e := range resolvedEsc {
		s.writeAuditAs(ctx, "system", "system", "work.escalation.resolved", EntityEscalation,
			e.ID, escalationSnapshot(e), escalationSnapshot(e))
	}
	return len(raisedEsc), len(resolvedEsc), nil
}

func escalationRef(e Escalation) outbox.EscalationRef {
	return outbox.EscalationRef{
		ID:         e.ID,
		EntityType: e.EntityType,
		EntityID:   e.EntityID,
		ProjectID:  e.ProjectID,
		Level:      e.Level,
		Status:     string(e.Status),
		Reason:     e.Reason,
		AssigneeID: e.AssigneeID,
		RaisedBy:   e.RaisedBy,
		ResolvedBy: e.ResolvedBy,
	}
}

func escalationSnapshot(e Escalation) map[string]any {
	return map[string]any{
		"entity_type": e.EntityType,
		"entity_id":   e.EntityID.String(),
		"level":       e.Level,
		"status":      string(e.Status),
		"assignee":    uuidOrZero(e.AssigneeID).String(),
	}
}
