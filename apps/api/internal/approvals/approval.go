// Package approvals owns Atlas approvals (§6.3). The engine is fast and
// provider-free on the write path: state changes write to the database and emit
// outbox events in a single transaction. Sweep/expiry run in atlas-worker.
package approvals

import (
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Status mirrors core.approvals.status.
type Status string

const (
	StatusRequested Status = "requested"
	StatusPending   Status = "pending"
	StatusApproved  Status = "approved"
	StatusDeclined  Status = "declined"
	StatusEscalated Status = "escalated"
	StatusCancelled Status = "cancelled"
	StatusExpired   Status = "expired"
)

// ErrInvalid marks a validation error (maps to 400).
var ErrInvalid = errors.New("approvals: invalid request")

// ErrNotFound marks a missing approval.
var ErrNotFound = errors.New("approvals: not found")

// ErrForbidden marks a permission/authority violation (maps to 403).
var ErrForbidden = errors.New("approvals: forbidden")

// Approval is the approval record.
type Approval struct {
	ID            uuid.UUID
	Ref           string
	EntityType    string
	EntityID      uuid.UUID
	RequestedBy   uuid.UUID
	AmountMinor   int64
	Currency      string
	ApproverRole  string
	ApproverID    *uuid.UUID
	Status        Status
	PolicyRef     string
	DueAt         *time.Time
	DecidedAt     *time.Time
	DecidedBy     *uuid.UUID
	Reason        string
	Version       int
	CreatedAt     time.Time
	UpdatedAt     time.Time
	ArchivedAt    *time.Time
}

// Request models creating an approval.
type Request struct {
	EntityType   string
	EntityID     uuid.UUID
	RequestedBy  uuid.UUID
	AmountMinor  int64
	Currency     string
	ApproverRole string
	ApproverID    *uuid.UUID
	PolicyRef    string
	DueAt        *time.Time
}

// Decide models an approve/decline action.
type Decide struct {
	ApprovalID uuid.UUID
	ActorID    uuid.UUID
	Approve    bool
	Reason     string
}

// Escalate models escalating an approval.
type Escalate struct {
	ApprovalID uuid.UUID
	ActorID    uuid.UUID
	ToRole     string
	ToPersonID *uuid.UUID
	Reason     string
}

// Cancel models cancelling an approval.
type Cancel struct {
	ApprovalID uuid.UUID
	ActorID    uuid.UUID
	Reason     string
}

func (r Request) valid() error {
	if r.EntityType == "" {
		return fmt.Errorf("%w: entity_type is required", ErrInvalid)
	}
	if r.EntityID == uuid.Nil {
		return fmt.Errorf("%w: entity_id is required", ErrInvalid)
	}
	if r.RequestedBy == uuid.Nil {
		return fmt.Errorf("%w: requested_by is required", ErrInvalid)
	}
	if r.AmountMinor < 0 {
		return fmt.Errorf("%w: amount_minor must be >= 0", ErrInvalid)
	}
	if r.Currency != "" && len(r.Currency) != 3 {
		return fmt.Errorf("%w: currency must be a 3-letter code", ErrInvalid)
	}
	if r.ApproverRole == "" && r.ApproverID == nil {
		return fmt.Errorf("%w: approver_role or approver_id required", ErrInvalid)
	}
	return nil
}
