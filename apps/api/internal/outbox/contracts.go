package outbox

import (
	"time"

	"github.com/google/uuid"
)

// Contract event types (§7.4). Signing is the trigger for the handover and the
// financial schedule (§9.1); approving a change request is the trigger for a new
// contract version (§8.3).
const (
	TypeContractCreatedV1       = "contracts.contract.created.v1"
	TypeContractStatusChangedV1 = "contracts.contract.status_changed.v1"
	TypeContractSignedV1        = "contracts.contract.signed.v1"
	TypeChangeRequestCreatedV1  = "contracts.change_request.created.v1"
	TypeChangeRequestApprovedV1 = "contracts.change_request.approved.v1"
	TypeChangeRequestRejectedV1 = "contracts.change_request.rejected.v1"
)

// Aggregate types for contract events.
const (
	AggregateContract      = "contract"
	AggregateChangeRequest = "change_request"
)

// ContractPayload is emitted when a contract is created or changes status.
type ContractPayload struct {
	ContractID     uuid.UUID  `json:"contract_id"`
	Ref            string     `json:"ref"`
	Status         string     `json:"status"`
	OrganizationID uuid.UUID  `json:"organization_id"`
	ContractType   string     `json:"contract_type,omitempty"`
	ValueMinor     *int64     `json:"value_minor,omitempty"`
	Currency       string     `json:"currency,omitempty"`
	ActorID        *uuid.UUID `json:"actor_id,omitempty"`
	OccurredAt     time.Time  `json:"occurred_at"`
}

// ContractStatusChangedPayload is emitted on a lifecycle move.
type ContractStatusChangedPayload struct {
	ContractID uuid.UUID `json:"contract_id"`
	Ref        string    `json:"ref"`
	From       string    `json:"from"`
	To         string    `json:"to"`
	ChangedBy  uuid.UUID `json:"changed_by,omitempty"`
	OccurredAt time.Time `json:"occurred_at"`
}

// ContractSignedPayload is emitted when a signature is recorded.
type ContractSignedPayload struct {
	ContractID uuid.UUID `json:"contract_id"`
	Ref        string    `json:"ref"`
	SignedBy   string    `json:"signed_by"`
	SignedAt   time.Time `json:"signed_at"`
	ActorID    uuid.UUID `json:"actor_id,omitempty"`
	OccurredAt time.Time `json:"occurred_at"`
}

// ChangeRequestPayload is emitted when a change request is raised or decided.
type ChangeRequestPayload struct {
	ChangeRequestID uuid.UUID `json:"change_request_id"`
	Ref             string    `json:"ref"`
	ContractID      uuid.UUID `json:"contract_id"`
	Status          string    `json:"status"`
	ActorID         uuid.UUID `json:"actor_id,omitempty"`
	OccurredAt      time.Time `json:"occurred_at"`
}

// ContractRef is the minimal contract view the builders need.
type ContractRef struct {
	ID             uuid.UUID
	Ref            string
	Status         string
	OrganizationID uuid.UUID
	ContractType   string
	ValueMinor     *int64
	Currency       string
}

// NewContractCreated builds a contracts.contract.created.v1 event.
func NewContractCreated(actorID *uuid.UUID, c ContractRef) (Event, error) {
	return buildEvent(TypeContractCreatedV1, AggregateContract, c.ID, actorID, nil, ContractPayload{
		ContractID: c.ID, Ref: c.Ref, Status: c.Status, OrganizationID: c.OrganizationID,
		ContractType: c.ContractType, ValueMinor: c.ValueMinor, Currency: c.Currency,
		ActorID: actorID, OccurredAt: time.Now().UTC(),
	})
}

// NewContractStatusChanged builds a contracts.contract.status_changed.v1 event.
func NewContractStatusChanged(actorID *uuid.UUID, id uuid.UUID, ref, from, to string, changedBy uuid.UUID) (Event, error) {
	return buildEvent(TypeContractStatusChangedV1, AggregateContract, id, actorID, nil, ContractStatusChangedPayload{
		ContractID: id, Ref: ref, From: from, To: to, ChangedBy: changedBy, OccurredAt: time.Now().UTC(),
	})
}

// NewContractSigned builds a contracts.contract.signed.v1 event.
func NewContractSigned(actorID *uuid.UUID, id uuid.UUID, ref, signedBy string, signedAt time.Time) (Event, error) {
	p := ContractSignedPayload{
		ContractID: id, Ref: ref, SignedBy: signedBy, SignedAt: signedAt,
		OccurredAt: time.Now().UTC(),
	}
	if actorID != nil {
		p.ActorID = *actorID
	}
	return buildEvent(TypeContractSignedV1, AggregateContract, id, actorID, nil, p)
}

// ChangeRequestRef is the minimal change-request view the builders need.
type ChangeRequestRef struct {
	ID         uuid.UUID
	Ref        string
	ContractID uuid.UUID
	Status     string
}

// NewChangeRequestCreated builds a contracts.change_request.created.v1 event.
func NewChangeRequestCreated(actorID uuid.UUID, cr ChangeRequestRef) (Event, error) {
	return buildEvent(TypeChangeRequestCreatedV1, AggregateChangeRequest, cr.ID, &actorID, nil, ChangeRequestPayload{
		ChangeRequestID: cr.ID, Ref: cr.Ref, ContractID: cr.ContractID, Status: cr.Status,
		ActorID: actorID, OccurredAt: time.Now().UTC(),
	})
}

// NewChangeRequestApproved builds a contracts.change_request.approved.v1 event.
func NewChangeRequestApproved(actorID uuid.UUID, cr ChangeRequestRef) (Event, error) {
	return buildEvent(TypeChangeRequestApprovedV1, AggregateChangeRequest, cr.ID, &actorID, nil, ChangeRequestPayload{
		ChangeRequestID: cr.ID, Ref: cr.Ref, ContractID: cr.ContractID, Status: cr.Status,
		ActorID: actorID, OccurredAt: time.Now().UTC(),
	})
}

// NewChangeRequestRejected builds a contracts.change_request.rejected.v1 event.
func NewChangeRequestRejected(actorID uuid.UUID, cr ChangeRequestRef) (Event, error) {
	return buildEvent(TypeChangeRequestRejectedV1, AggregateChangeRequest, cr.ID, &actorID, nil, ChangeRequestPayload{
		ChangeRequestID: cr.ID, Ref: cr.Ref, ContractID: cr.ContractID, Status: cr.Status,
		ActorID: actorID, OccurredAt: time.Now().UTC(),
	})
}
