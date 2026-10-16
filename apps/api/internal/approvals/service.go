package approvals

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/safaristack/atlas/apps/api/internal/audit"
	"github.com/safaristack/atlas/apps/api/internal/authz"
	"github.com/safaristack/atlas/apps/api/internal/outbox"
)

// Service coordinates approval lifecycle with outbox events and audit.
type Service struct {
	store    Store
	audit    *audit.Writer
	notifier Notifier
	now      func() time.Time
	logger   *slog.Logger
}

// NewService builds the approvals service. notifier may be nil, in which case
// fan-out is skipped (tests, or a deployment without notifications).
func NewService(store Store, aw *audit.Writer, notifier Notifier, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{
		store:    store,
		audit:    aw,
		notifier: notifier,
		now:      func() time.Time { return time.Now().UTC() },
		logger:   logger,
	}
}

// Request creates an approval and emits core.approval.requested.v1.
//
// When the caller does not name an approver, the approval policy that governs
// the decision (core.approval_policies, §6.7) is consulted: it may supply the
// approver role and say whether one or two people must approve. A decision with
// no policy and no explicit approver is a caller error, not a silent no-op.
func (s *Service) Request(ctx context.Context, in Request, actor *uuid.UUID) (*Approval, error) {
	if err := in.valid(); err != nil {
		return nil, err
	}
	now := s.now()

	policy, err := s.store.LookupPolicy(ctx, PolicyQuery{
		DecisionType: in.EntityType,
		AmountMinor:  in.AmountMinor,
		Currency:     in.Currency,
		At:           now,
	})
	if err != nil {
		return nil, err
	}

	approverRole := strings.ToLower(strings.TrimSpace(in.ApproverRole))
	approverID := in.ApproverID
	policyRef := in.PolicyRef
	required := in.ApprovalsRequired
	if policy != nil {
		if policyRef == "" {
			policyRef = policy.ID.String()
		}
		if required == 0 {
			required = policy.ApprovalsRequired()
		}
		if approverRole == "" && approverID == nil {
			approverRole = policy.Role()
		}
	}
	if required == 0 {
		required = 1
	}
	if approverRole == "" && approverID == nil {
		if policy != nil {
			return nil, fmt.Errorf("%w: policy %s requires approver_rule %q, which does not name a single role; specify approver_role",
				ErrInvalid, policy.ID, policy.ApproverRule)
		}
		return nil, fmt.Errorf("%w: approver_role or approver_id required", ErrInvalid)
	}

	id, err := uuid.NewV7()
	if err != nil {
		return nil, fmt.Errorf("approvals: id: %w", err)
	}
	ref := generateRef(now.Year(), id)

	a := Approval{
		ID:                id,
		Ref:               ref,
		EntityType:        in.EntityType,
		EntityID:          in.EntityID,
		RequestedBy:       in.RequestedBy,
		AmountMinor:       in.AmountMinor,
		Currency:          in.Currency,
		ApproverRole:      approverRole,
		ApproverID:        approverID,
		Status:            StatusRequested,
		PolicyRef:         policyRef,
		ApprovalsRequired: required,
		DueAt:             in.DueAt,
		Version:           1,
		CreatedAt:         now,
		UpdatedAt:         now,
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
		"ref":                a.Ref,
		"entity_type":        a.EntityType,
		"entity_id":          a.EntityID,
		"requested_by":       a.RequestedBy,
		"amount_minor":       a.AmountMinor,
		"currency":           a.Currency,
		"approvals_required": a.ApprovalsRequired,
	})

	// Notify the approvers (best-effort, after commit).
	s.notifyApprovers(ctx, &a, actor)

	return &a, nil
}

// Decide approves or declines an approval. Nobody may act on their own request
// (§3.3, §6.4), and when two approvals are required a second, different person
// must supply the final one.
func (s *Service) Decide(ctx context.Context, in Decide) error {
	if in.ApprovalID == uuid.Nil || in.ActorID == uuid.Nil {
		return fmt.Errorf("%w: actor and approval required", ErrInvalid)
	}
	a, err := s.store.Get(ctx, in.ApprovalID)
	if err != nil {
		return err
	}
	if a == nil {
		return ErrNotFound
	}

	// Separation of duties: the actor and the requester must be different.
	if err := authz.FourEyes(in.ActorID, a.RequestedBy); err != nil {
		return fmt.Errorf("%w: %s", ErrForbidden, err)
	}
	if a.Status != StatusRequested && a.Status != StatusPending {
		return ErrNotFound
	}

	required := a.ApprovalsRequired
	if required < 1 {
		required = 1
	}

	_, prior, first, err := s.store.ApprovalProgress(ctx, a.ID)
	if err != nil {
		return err
	}

	var ev *outbox.Event
	if in.Approve {
		// The second eye must be a different person (§6.4 TwoPersonRule).
		if required > 1 && prior >= 1 {
			if err := authz.TwoPersonRule(first, in.ActorID); err != nil {
				return fmt.Errorf("%w: %s", ErrForbidden, err)
			}
		}
		if prior+1 >= required {
			e, err := outbox.NewApprovalApproved(&in.ActorID, a.ID, a.Ref, in.Reason, in.ActorID)
			if err != nil {
				return err
			}
			ev = &e
		}
	} else {
		e, err := outbox.NewApprovalDeclined(&in.ActorID, a.ID, a.Ref, in.Reason, in.ActorID)
		if err != nil {
			return err
		}
		ev = &e
	}

	status, err := s.store.DecideWithEvent(ctx, Decision{
		ApprovalID: a.ID,
		ActorID:    in.ActorID,
		Approve:    in.Approve,
		Reason:     in.Reason,
		Required:   required,
	}, ev)
	if err != nil {
		return err
	}

	action := "core.approval.declined"
	subject := "Approval declined"
	switch {
	case status == StatusApproved:
		action, subject = "core.approval.approved", "Approval approved"
	case status == StatusPending:
		action, subject = "core.approval.awaiting_second", "Approval awaiting a second approver"
	}
	_ = s.writeAudit(ctx, &in.ActorID, action, "approval", a.ID, nil, map[string]any{
		"ref":    a.Ref,
		"status": string(status),
		"reason": in.Reason,
	})

	a.Status = status
	s.notifyRequester(ctx, a, &in.ActorID, subject, in.Reason)
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

	if a, err := s.store.Get(ctx, in.ApprovalID); err == nil && a != nil {
		s.notifyEscalation(ctx, a, &in.ActorID, in.Reason)
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

// notify records one notification intent, swallowing (but logging) failures:
// fan-out must never fail the approval it describes.
func (s *Service) notify(ctx context.Context, n Notification) {
	if s.notifier == nil {
		return
	}
	if err := s.notifier.Notify(ctx, n); err != nil {
		s.logger.Warn("approvals: notification failed",
			slog.String("recipient", n.RecipientPersonID.String()),
			slog.String("template", n.Template),
			slog.String("error", err.Error()))
	}
}

// notifyApprovers tells the people who must act on a new approval. It never
// notifies the requester as an approver: that would invite self-approval.
func (s *Service) notifyApprovers(ctx context.Context, a *Approval, actor *uuid.UUID) {
	entityID := a.ID
	payload := map[string]any{
		"approval_id":  a.ID.String(),
		"ref":          a.Ref,
		"entity_type":  a.EntityType,
		"entity_id":    a.EntityID.String(),
		"amount_minor": a.AmountMinor,
		"currency":     a.Currency,
	}
	for _, pid := range s.approverRecipients(ctx, a) {
		if pid == a.RequestedBy {
			continue
		}
		s.notify(ctx, Notification{
			RecipientPersonID: pid,
			Template:          TemplateRequested,
			Subject:           "Approval needed: " + a.Ref,
			Body:              fmt.Sprintf("A %s needs your approval.", a.EntityType),
			Payload:           payload,
			EntityType:        "approval",
			EntityID:          &entityID,
			IdempotencyKey:    fmt.Sprintf("approval:%s:requested:%s", a.ID, pid),
			ActorID:           actor,
		})
	}
}

// notifyRequester tells the requester the outcome of their approval.
func (s *Service) notifyRequester(ctx context.Context, a *Approval, actor *uuid.UUID, subject, reason string) {
	if a.RequestedBy == uuid.Nil {
		return
	}
	if actor != nil && *actor == a.RequestedBy {
		return // don't notify someone about their own action
	}
	tmpl := TemplateApproved
	switch a.Status {
	case StatusDeclined:
		tmpl = TemplateDeclined
	case StatusPending:
		tmpl = TemplateAwaitSecond
	}
	entityID := a.ID
	s.notify(ctx, Notification{
		RecipientPersonID: a.RequestedBy,
		Template:          tmpl,
		Subject:           subject + ": " + a.Ref,
		Body:              fmt.Sprintf("Your %s request %s.", a.EntityType, strings.ToLower(subject)),
		Payload: map[string]any{
			"approval_id": a.ID.String(),
			"ref":         a.Ref,
			"status":      string(a.Status),
			"reason":      reason,
		},
		EntityType:     "approval",
		EntityID:       &entityID,
		IdempotencyKey: fmt.Sprintf("approval:%s:decided:%s", a.ID, a.Status),
		ActorID:        actor,
	})
}

// notifyEscalation tells the escalation target. Escalations are critical
// (§6.4): they bypass quiet hours.
func (s *Service) notifyEscalation(ctx context.Context, a *Approval, actor *uuid.UUID, reason string) {
	entityID := a.ID
	for _, pid := range s.approverRecipients(ctx, a) {
		if pid == a.RequestedBy {
			continue
		}
		s.notify(ctx, Notification{
			RecipientPersonID: pid,
			Template:          TemplateEscalated,
			Subject:           "Approval escalated: " + a.Ref,
			Body:              fmt.Sprintf("A %s approval was escalated to you.", a.EntityType),
			Payload: map[string]any{
				"approval_id": a.ID.String(),
				"ref":         a.Ref,
				"to_role":     a.ApproverRole,
				"reason":      reason,
			},
			EntityType:     "approval",
			EntityID:       &entityID,
			Critical:       true,
			IdempotencyKey: fmt.Sprintf("approval:%s:escalated:%s", a.ID, a.ApproverRole),
			ActorID:        actor,
		})
	}
}

// approverRecipients resolves who should be told about an approval: the named
// person if there is one, otherwise every active holder of the approver role.
func (s *Service) approverRecipients(ctx context.Context, a *Approval) []uuid.UUID {
	if a.ApproverID != nil {
		return []uuid.UUID{*a.ApproverID}
	}
	if a.ApproverRole == "" {
		return nil
	}
	people, err := s.store.PeopleWithRole(ctx, a.ApproverRole)
	if err != nil {
		s.logger.Warn("approvals: resolve role holders failed",
			slog.String("role", a.ApproverRole), slog.String("error", err.Error()))
		return nil
	}
	return people
}
