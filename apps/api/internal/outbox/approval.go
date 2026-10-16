package outbox

import (
	"time"

	"github.com/google/uuid"
)

// Approval event types (§7.4).
const (
	TypeApprovalRequestedV1 = "core.approval.requested.v1"
	TypeApprovalApprovedV1  = "core.approval.approved.v1"
	TypeApprovalDeclinedV1  = "core.approval.declined.v1"
	TypeApprovalEscalatedV1 = "core.approval.escalated.v1"
	TypeApprovalCancelledV1 = "core.approval.cancelled.v1"
	TypeApprovalExpiredV1   = "core.approval.expired.v1"
)

const AggregateApproval = "approval"

// ApprovalRequestedPayload is emitted when an approval is created.
type ApprovalRequestedPayload struct {
	ApprovalID   uuid.UUID `json:"approval_id"`
	Ref          string    `json:"ref"`
	EntityType   string    `json:"entity_type"`
	EntityID     uuid.UUID `json:"entity_id"`
	RequestedBy  uuid.UUID `json:"requested_by"`
	AmountMinor  int64     `json:"amount_minor,omitempty"`
	Currency     string    `json:"currency,omitempty"`
	ApproverRole string    `json:"approver_role,omitempty"`
	ApproverID   uuid.UUID `json:"approver_id,omitempty"`
	DueAt        time.Time `json:"due_at,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

// ApprovalDecidedPayload is emitted on approve/decline.
type ApprovalDecidedPayload struct {
	ApprovalID uuid.UUID `json:"approval_id"`
	Ref        string    `json:"ref"`
	DecidedBy  uuid.UUID `json:"decided_by,omitempty"`
	Reason     string    `json:"reason,omitempty"`
	DecidedAt  time.Time `json:"decided_at"`
}

// ApprovalEscalatedPayload is emitted when escalated.
type ApprovalEscalatedPayload struct {
	ApprovalID  uuid.UUID `json:"approval_id"`
	Ref         string    `json:"ref"`
	FromRole    string    `json:"from_role,omitempty"`
	ToRole      string    `json:"to_role,omitempty"`
	ToPersonID  uuid.UUID `json:"to_person_id,omitempty"`
	EscalatedBy uuid.UUID `json:"escalated_by,omitempty"`
	Reason      string    `json:"reason,omitempty"`
	EscalatedAt time.Time `json:"escalated_at"`
}

// ApprovalCancelledPayload is emitted on cancel.
type ApprovalCancelledPayload struct {
	ApprovalID  uuid.UUID `json:"approval_id"`
	Ref         string    `json:"ref"`
	CancelledBy uuid.UUID `json:"cancelled_by,omitempty"`
	Reason      string    `json:"reason,omitempty"`
	CancelledAt time.Time `json:"cancelled_at"`
}

// ApprovalExpiredPayload is emitted on expiry.
type ApprovalExpiredPayload struct {
	ApprovalID uuid.UUID `json:"approval_id"`
	Ref        string    `json:"ref"`
	ExpiredAt  time.Time `json:"expired_at"`
}

func NewApprovalRequested(actorID *uuid.UUID, a ApprovalRef) (Event, error) {
	return buildEvent(TypeApprovalRequestedV1, AggregateApproval, a.ID, actorID, nil, ApprovalRequestedPayload{
		ApprovalID:   a.ID,
		Ref:          a.Ref,
		EntityType:   a.EntityType,
		EntityID:     a.EntityID,
		RequestedBy:  a.RequestedBy,
		AmountMinor:  a.AmountMinor,
		Currency:     a.Currency,
		ApproverRole: a.ApproverRole,
		ApproverID:   a.ApproverID,
		DueAt:        a.DueAt,
		CreatedAt:    time.Now().UTC(),
	})
}

func NewApprovalApproved(actorID *uuid.UUID, id uuid.UUID, ref, reason string, decidedBy uuid.UUID) (Event, error) {
	return buildEvent(TypeApprovalApprovedV1, AggregateApproval, id, actorID, nil, ApprovalDecidedPayload{
		ApprovalID: id,
		Ref:        ref,
		DecidedBy:  decidedBy,
		Reason:     reason,
		DecidedAt:  time.Now().UTC(),
	})
}

func NewApprovalDeclined(actorID *uuid.UUID, id uuid.UUID, ref, reason string, decidedBy uuid.UUID) (Event, error) {
	return buildEvent(TypeApprovalDeclinedV1, AggregateApproval, id, actorID, nil, ApprovalDecidedPayload{
		ApprovalID: id,
		Ref:        ref,
		DecidedBy:  decidedBy,
		Reason:     reason,
		DecidedAt:  time.Now().UTC(),
	})
}

func NewApprovalEscalated(actorID *uuid.UUID, id uuid.UUID, ref, fromRole, toRole string, toPerson uuid.UUID, escalatedBy uuid.UUID, reason string) (Event, error) {
	return buildEvent(TypeApprovalEscalatedV1, AggregateApproval, id, actorID, nil, ApprovalEscalatedPayload{
		ApprovalID:  id,
		Ref:         ref,
		FromRole:    fromRole,
		ToRole:      toRole,
		ToPersonID:  toPerson,
		EscalatedBy: escalatedBy,
		Reason:      reason,
		EscalatedAt: time.Now().UTC(),
	})
}

func NewApprovalCancelled(actorID *uuid.UUID, id uuid.UUID, ref, reason string, cancelledBy uuid.UUID) (Event, error) {
	return buildEvent(TypeApprovalCancelledV1, AggregateApproval, id, actorID, nil, ApprovalCancelledPayload{
		ApprovalID:  id,
		Ref:         ref,
		CancelledBy: cancelledBy,
		Reason:      reason,
		CancelledAt: time.Now().UTC(),
	})
}

func NewApprovalExpired(actorID *uuid.UUID, id uuid.UUID, ref string) (Event, error) {
	return buildEvent(TypeApprovalExpiredV1, AggregateApproval, id, actorID, nil, ApprovalExpiredPayload{
		ApprovalID: id,
		Ref:        ref,
		ExpiredAt:  time.Now().UTC(),
	})
}

// ApprovalRef is a minimal view used to build requested event.
type ApprovalRef struct {
	ID           uuid.UUID
	Ref          string
	EntityType   string
	EntityID     uuid.UUID
	RequestedBy  uuid.UUID
	AmountMinor  int64
	Currency     string
	ApproverRole string
	ApproverID   uuid.UUID
	DueAt        time.Time
}
