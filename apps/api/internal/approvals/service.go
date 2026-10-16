package approvals

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/safaristack/atlas/apps/api/internal/audit"
	"github.com/safaristack/atlas/apps/api/internal/outbox"
)

// Service coordinates approval lifecycle with outbox events and audit.
type Service struct {
	store  Store
	audit  *audit.Writer
	now    func() time.Time
	logger *slog.Logger
}

// NewService builds the approvals service.
func NewService(store Store, aw *audit.Writer, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{
		store:  store,
		audit:  aw,
		now:    func() time.Time { return time.Now().UTC() },
		logger: logger,
	}
}

// Request creates an approval and emits core.approval.requested.v1.
func (s *Service) Request(ctx context.Context, in Request, actor *uuid.UUID) (*Approval, error) {
	if err := in.valid(); err != nil {
		return nil, err
	}
	now := s.now()
	id, err := uuid.NewV7()
	if err != nil {
		return nil, fmt.Errorf("approvals: id: %w", err)
	}
	ref := generateRef(now.Year(), id)

	a := Approval{
		ID:           id,
		Ref:          ref,
		EntityType:   in.EntityType,
		EntityID:     in.EntityID,
		RequestedBy:  in.RequestedBy,
		AmountMinor:  in.AmountMinor,
		Currency:     in.Currency,
		ApproverRole: in.ApproverRole,
		ApproverID:   in.ApproverID,
		Status:       StatusRequested,
		PolicyRef:    in.PolicyRef,
		DueAt:        in.DueAt,
		Version:      1,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	h := History{
		ID:         uuid.Must(uuid.NewV7()),
		ApprovalID: a.ID,
		ActorID:    actor,
		Action:     "request",
		FromStatus: "",
		ToStatus:   string(StatusRequested),
	}

	ev, err := outbox.NewApprovalRequested(actor, outbox.ApprovalRef{
		ID:           a.ID,
		Ref:          a.Ref,
		EntityType:   a.EntityType,
		EntityID:     a.EntityID,
		RequestedBy:  a.RequestedBy,
		AmountMinor:  a.AmountMinor,
		Currency:     a.Currency,
		ApproverRole: a.ApproverRole,
		ApproverID:   zeroIfNil(a.ApproverID),
		DueAt:        zeroTime(a.DueAt),
	})
	if err != nil {
		return nil, err
	}

	if err := s.store.CreateWithEvent(ctx, a, []History{h}, ev); err != nil {
		return nil, err
	}

	// Audit: write in best-effort (non-blocking on failure semantics; store tx already committed)
	_ = s.writeAudit(ctx, actor, "core.approval.requested", "approval", a.ID, nil, map[string]any{
		"ref":          a.Ref,
		"entity_type":  a.EntityType,
		"entity_id":    a.EntityID,
		"requested_by": a.RequestedBy,
		"amount_minor": a.AmountMinor,
		"currency":     a.Currency,
	})

	return &a, nil
}

// Decide approves or declines an approval.
func (s *Service) Decide(ctx context.Context, in Decide) error {
	if in.ApprovalID == uuid.Nil || in.ActorID == uuid.Nil {
		return fmt.Errorf("%w: actor and approval required", ErrInvalid)
	}
	_, err := s.store.Get(ctx, in.ApprovalID)
	if err != nil {
		return err
	}

	var ev outbox.Event
	var errEv error
	if in.Approve {
		ev, errEv = outbox.NewApprovalApproved(&in.ActorID, in.ApprovalID, "", in.Reason, in.ActorID)
	} else {
		ev, errEv = outbox.NewApprovalDeclined(&in.ActorID, in.ApprovalID, "", in.Reason, in.ActorID)
	}
	if errEv != nil {
		return errEv
	}
	if err := s.store.DecideWithEvent(ctx, in.ApprovalID, in.ActorID, in.Approve, in.Reason, ev); err != nil {
		return err
	}
	return nil
}

// Escalate escalates an approval.
func (s *Service) Escalate(ctx context.Context, in Escalate) error {
	if in.ApprovalID == uuid.Nil || in.ActorID == uuid.Nil || in.ToRole == "" {
		return fmt.Errorf("%w: approval, actor and to_role required", ErrInvalid)
	}
	var toPerson uuid.UUID
	if in.ToPersonID != nil {
		toPerson = *in.ToPersonID
	}
	ev, err := outbox.NewApprovalEscalated(&in.ActorID, in.ApprovalID, "", "", in.ToRole, toPerson, in.ActorID, in.Reason)
	if err != nil {
		return err
	}
	if err := s.store.EscalateWithEvent(ctx, in.ApprovalID, in.ActorID, in.ToRole, in.ToPersonID, in.Reason, ev); err != nil {
		return err
	}
	return nil
}

// Cancel cancels an approval.
func (s *Service) Cancel(ctx context.Context, in Cancel) error {
	if in.ApprovalID == uuid.Nil || in.ActorID == uuid.Nil {
		return fmt.Errorf("%w: approval and actor required", ErrInvalid)
	}
	ev, err := outbox.NewApprovalCancelled(&in.ActorID, in.ApprovalID, "", in.Reason, in.ActorID)
	if err != nil {
		return err
	}
	if err := s.store.CancelWithEvent(ctx, in.ApprovalID, in.ActorID, in.Reason, ev); err != nil {
		return err
	}
	return nil
}

// Get fetches an approval by id.
func (s *Service) Get(ctx context.Context, id uuid.UUID) (*Approval, error) {
	a, err := s.store.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if a == nil {
		return nil, ErrNotFound
	}
	return a, nil
}

// ListPendingForRole lists pending approvals for a role.
func (s *Service) ListPendingForRole(ctx context.Context, role string, limit int, before *Cursor) ([]Approval, error) {
	if limit <= 0 {
		limit = 50
	}
	return s.store.ListPendingForRole(ctx, role, limit, before)
}

// ListRequestedBy lists approvals requested by a person.
func (s *Service) ListRequestedBy(ctx context.Context, requestedBy uuid.UUID, limit int, before *Cursor) ([]Approval, error) {
	if limit <= 0 {
		limit = 50
	}
	return s.store.ListRequestedBy(ctx, requestedBy, limit, before)
}

func (s *Service) writeAudit(ctx context.Context, actor *uuid.UUID, action, entityType string, entityID uuid.UUID, before, after any) error {
	if s.audit == nil {
		return nil
	}
	actorType := audit.ActorSystem
	actorID := "system"
	if actor != nil {
		actorType = audit.ActorUser
		actorID = actor.String()
	}
	return s.audit.Write(ctx, nil, audit.Event{
		ActorType:  actorType,
		ActorID:    actorID,
		Action:     action,
		EntityType: entityType,
		EntityID:   &entityID,
		Before:     before,
		After:      after,
	})
}

// generateRef builds a human reference (docs/ATLAS.md 6.6). It is derived from
// the approval's UUIDv7 so two approvals created in the same nanosecond can
// never collide; core.approvals has a unique index on ref.
func generateRef(year int, id uuid.UUID) string {
	return fmt.Sprintf("APR-%d-%s", year, strings.ToUpper(id.String()[:8]))
}

func zeroIfNil(p *uuid.UUID) uuid.UUID {
	if p == nil {
		return uuid.Nil
	}
	return *p
}

func zeroTime(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}
	return *t
}
