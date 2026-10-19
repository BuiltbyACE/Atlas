package work

import (
	"time"

	"context"
	"fmt"
	"log/slog"
	"strings"

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
// (docs/ATLAS.md §4.1). It raises escalations for overdue tasks at the rung the
// D4 ladder has reached (§5.8, ADR-016), promotes open escalations that have
// passed a higher rung, and resolves escalations whose task is no longer
// overdue, then audits all three. It returns how many escalations it raised —
// new or promoted — and how many it resolved.
func (s *Service) Sweep(ctx context.Context, limit int) (raised, resolved int, err error) {
	lad, err := s.ladderFor(ctx)
	if err != nil {
		return 0, 0, err
	}
	// L2 goes to the domain CEO (the l2_role holder, ADR-016); L3 is the
	// shared both-CEO queue and deliberately has no assignee. Nobody holding
	// the role degrades L2 to the same shared visibility, never to nobody.
	var l2Person *uuid.UUID
	if lad.L2Role != "" {
		l2Person, err = s.store.PersonForRole(ctx, lad.L2Role)
		if err != nil {
			return 0, 0, err
		}
	}

	raisedEsc, err := s.store.SweepOverdueTasks(ctx, limit, lad, l2Person, func(t OverdueTask, esc Escalation) ([]outbox.Event, error) {
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

	// Promotions keep the escalation's identity and update its rung in place,
	// so the audit trail records each transition (§6.6) and the shared queues
	// hold one entry per overdue item.
	prev := make(map[uuid.UUID]Escalation, 2)
	promoted, err := s.store.PromoteEscalations(ctx, limit, lad, l2Person, func(before, after Escalation) (outbox.Event, error) {
		prev[after.ID] = before
		ref := escalationRef(after)
		assigneeUser, err := s.accountForPerson(ctx, after.AssigneeID)
		if err != nil {
			return outbox.Event{}, err
		}
		ref.AssigneeUserID = assigneeUser
		return outbox.NewEscalationRaised(nil, ref)
	})
	if err != nil {
		return len(raisedEsc), 0, err
	}
	for _, e := range promoted {
		s.writeAuditAs(ctx, "system", "system", "work.escalation.raised", EntityEscalation,
			e.ID, escalationSnapshot(prev[e.ID]), escalationSnapshot(e))
		s.logger.DebugContext(ctx, "work: escalation promoted",
			slog.String("escalation_id", e.ID.String()),
			slog.String("entity_id", e.EntityID.String()),
			slog.Int("level", e.Level))
	}

	resolvedEsc, err := s.store.ResolveStaleEscalations(ctx, limit, func(e Escalation) (outbox.Event, error) {
		return outbox.NewEscalationResolved(nil, escalationRef(e))
	})
	if err != nil {
		return len(raisedEsc) + len(promoted), 0, err
	}
	for _, e := range resolvedEsc {
		s.writeAuditAs(ctx, "system", "system", "work.escalation.resolved", EntityEscalation,
			e.ID, escalationSnapshot(e), escalationSnapshot(e))
	}
	return len(raisedEsc) + len(promoted), len(resolvedEsc), nil
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

// PersonForRole resolves the person behind a company-scoped role grant — the
// ladder's "domain CEO" lookup. Exported so another module can escalate through
// the same ladder; the people module's repeated-miss rule does (ADR-017).
func (s *Service) PersonForRole(ctx context.Context, roleKey string) (*uuid.UUID, error) {
	return s.store.PersonForRole(ctx, roleKey)
}

// RaiseEscalation opens one escalation for an entity another module owns,
// returning true when it opened a new one and false when one was already open
// for that entity and level. It is the generic counterpart to the task sweep:
// the people module escalates a person's repeated missed daily reports to L2
// through it (ADR-017). It emits core.escalation.raised.v1 and audits the raise
// as the system, exactly as the sweep does.
func (s *Service) RaiseEscalation(ctx context.Context, entityType string, entityID uuid.UUID, level int, reason string, assignee *uuid.UUID) (bool, error) {
	if strings.TrimSpace(entityType) == "" {
		return false, fmt.Errorf("%w: escalation needs an entity type", ErrInvalid)
	}
	if level < 1 {
		return false, fmt.Errorf("%w: escalation level must be >= 1", ErrInvalid)
	}
	assigneeUser, err := s.accountForPerson(ctx, assignee)
	if err != nil {
		return false, err
	}
	esc, raised, err := s.store.RaiseEscalationForEntity(ctx, entityType, entityID, level, reason, assignee,
		func(e Escalation) (outbox.Event, error) {
			ref := escalationRef(e)
			ref.AssigneeUserID = assigneeUser
			return outbox.NewEscalationRaised(nil, ref)
		})
	if err != nil {
		return false, err
	}
	if !raised {
		return false, nil
	}
	s.writeAuditAs(ctx, "system", "system", "work.escalation.raised", EntityEscalation, esc.ID,
		nil, escalationSnapshot(*esc))
	s.logger.DebugContext(ctx, "work: escalation raised",
		slog.String("entity_type", esc.EntityType),
		slog.String("entity_id", esc.EntityID.String()),
		slog.Int("level", esc.Level))
	return true, nil
}

// ResolveEscalations closes every open escalation for an entity another module
// owns and returns how many it closed, emitting core.escalation.resolved.v1 per
// row and auditing each. The people sweep calls it once a person's compliance
// recovers (ADR-017).
func (s *Service) ResolveEscalations(ctx context.Context, entityType string, entityID uuid.UUID) (int, error) {
	closed, err := s.store.ResolveEscalationsForEntity(ctx, entityType, entityID,
		func(e Escalation) (outbox.Event, error) {
			return outbox.NewEscalationResolved(nil, escalationRef(e))
		})
	if err != nil {
		return 0, err
	}
	for _, e := range closed {
		s.writeAuditAs(ctx, "system", "system", "work.escalation.resolved", EntityEscalation, e.ID,
			escalationSnapshot(e), escalationSnapshot(e))
	}
	return len(closed), nil
}

// LastResolvedAt reports when an entity's most recent escalation was resolved
// (nil if never), so another module can tell a new episode from the one a
// person already closed (the people module does, ADR-017).
func (s *Service) LastResolvedAt(ctx context.Context, entityType string, entityID uuid.UUID) (*time.Time, error) {
	return s.store.LastEscalationResolvedAt(ctx, entityType, entityID)
}

// OpenEscalatedEntities returns the entity ids of the given type that currently
// have an open escalation. Another module pairs it with ResolveEscalations to
// clear the escalations whose condition has passed (the people module does,
// ADR-017).
func (s *Service) OpenEscalatedEntities(ctx context.Context, entityType string) ([]uuid.UUID, error) {
	return s.store.OpenEscalationEntities(ctx, entityType)
}
