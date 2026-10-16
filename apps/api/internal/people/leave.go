package people

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/safaristack/atlas/apps/api/internal/outbox"
)

// RequestLeave records a leave request. A person cannot hold two active
// (requested or approved) requests covering the same day, which keeps the
// availability view honest. Emits people.leave.requested.v1.
func (s *Service) RequestLeave(ctx context.Context, actor uuid.UUID, in LeaveRequestInput) (LeaveRequest, error) {
	l, err := validateLeaveRequest(LeaveRequest{
		PersonID: actor,
		Type:     in.Type,
		FromDate: in.FromDate,
		ToDate:   in.ToDate,
		Reason:   in.Reason,
		Status:   LeaveRequested,
		Version:  1,
	})
	if err != nil {
		return LeaveRequest{}, err
	}
	overlap, err := s.store.OverlappingLeave(ctx, actor, l.FromDate, l.ToDate)
	if err != nil {
		return LeaveRequest{}, err
	}
	if overlap {
		return LeaveRequest{}, fmt.Errorf("%w: leave already covers part of that period", ErrConflict)
	}
	id, err := uuid.NewV7()
	if err != nil {
		return LeaveRequest{}, fmt.Errorf("people: new id: %w", err)
	}
	now := s.now()
	l.ID = id
	l.CreatedAt, l.UpdatedAt = now, now

	ev, err := outbox.NewLeaveRequested(&actor, l.ID, actor, string(l.Type), dateString(l.FromDate), dateString(l.ToDate))
	if err != nil {
		return LeaveRequest{}, err
	}
	created, err := s.store.CreateLeaveRequest(ctx, l, &ev)
	if err != nil {
		return LeaveRequest{}, err
	}
	s.writeAudit(ctx, actor, "people.leave.requested", EntityLeaveRequest, created.ID, nil, leaveSnapshot(created))
	return created, nil
}

// GetLeaveRequest returns a request or ErrNotFound.
func (s *Service) GetLeaveRequest(ctx context.Context, id uuid.UUID) (LeaveRequest, error) {
	l, err := s.store.GetLeaveRequest(ctx, id)
	if err != nil {
		return LeaveRequest{}, err
	}
	if l == nil {
		return LeaveRequest{}, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	return *l, nil
}

// ListLeaveRequests returns a page of requests, newest first.
func (s *Service) ListLeaveRequests(ctx context.Context, q ListLeaveRequestsQuery, limit int, before *Cursor) ([]LeaveRequest, error) {
	if limit <= 0 {
		limit = 50
	}
	return s.store.ListLeaveRequests(ctx, q, limit, before)
}

// DecideLeaveRequest approves or declines a pending request. A person cannot
// decide their own request (separation of duties, §3.3). Emits
// people.leave.approved.v1 / people.leave.declined.v1.
func (s *Service) DecideLeaveRequest(ctx context.Context, actor, requestID uuid.UUID, approve bool, reason string) (LeaveRequest, error) {
	current, err := s.GetLeaveRequest(ctx, requestID)
	if err != nil {
		return LeaveRequest{}, err
	}
	if current.PersonID == actor {
		return LeaveRequest{}, fmt.Errorf("%w: you cannot decide your own leave", ErrForbidden)
	}
	if current.Status != LeaveRequested {
		return LeaveRequest{}, fmt.Errorf("%w: only a requested leave can be decided", ErrConflict)
	}
	now := s.now()
	next := current
	next.ApproverID = &actor
	if approve {
		next.Status = LeaveApproved
		next.ApprovedAt = &now
	} else {
		next.Status = LeaveDeclined
		next.DeclinedAt = &now
		next.CancelReason = trim(reason)
	}

	requesterUserID, err := s.accountForPerson(ctx, current.PersonID)
	if err != nil {
		return LeaveRequest{}, err
	}

	var ev outbox.Event
	if approve {
		ev, err = outbox.NewLeaveApproved(&actor, current.ID, current.PersonID, actor, requesterUserID)
	} else {
		ev, err = outbox.NewLeaveDeclined(&actor, current.ID, current.PersonID, actor, trim(reason), requesterUserID)
	}
	if err != nil {
		return LeaveRequest{}, err
	}
	updated, err := s.store.UpdateLeaveRequest(ctx, next, current.Version, &ev)
	if err != nil {
		return LeaveRequest{}, err
	}
	action := "people.leave.declined"
	if approve {
		action = "people.leave.approved"
	}
	s.writeAudit(ctx, actor, action, EntityLeaveRequest, updated.ID, leaveSnapshot(current), leaveSnapshot(updated))
	return updated, nil
}

// CancelLeaveRequest withdraws the caller's own pending or approved request.
// Emits people.leave.cancelled.v1.
func (s *Service) CancelLeaveRequest(ctx context.Context, actor, requestID uuid.UUID, reason string) (LeaveRequest, error) {
	current, err := s.GetLeaveRequest(ctx, requestID)
	if err != nil {
		return LeaveRequest{}, err
	}
	if current.PersonID != actor {
		return LeaveRequest{}, fmt.Errorf("%w: you can only cancel your own leave", ErrForbidden)
	}
	if current.Status != LeaveRequested && current.Status != LeaveApproved {
		return LeaveRequest{}, fmt.Errorf("%w: this leave cannot be cancelled", ErrConflict)
	}
	next := current
	next.Status = LeaveCancelled
	next.CancelReason = trim(reason)

	requesterUserID, err := s.accountForPerson(ctx, current.PersonID)
	if err != nil {
		return LeaveRequest{}, err
	}

	ev, err := outbox.NewLeaveCancelled(&actor, current.ID, current.PersonID, actor, trim(reason), requesterUserID)
	if err != nil {
		return LeaveRequest{}, err
	}
	updated, err := s.store.UpdateLeaveRequest(ctx, next, current.Version, &ev)
	if err != nil {
		return LeaveRequest{}, err
	}
	s.writeAudit(ctx, actor, "people.leave.cancelled", EntityLeaveRequest, updated.ID, leaveSnapshot(current), leaveSnapshot(updated))
	return updated, nil
}

func leaveSnapshot(l LeaveRequest) map[string]any {
	return map[string]any{
		"person_id":  l.PersonID.String(),
		"leave_type": string(l.Type),
		"from_date":  dateString(l.FromDate),
		"to_date":    dateString(l.ToDate),
		"status":     string(l.Status),
	}
}
