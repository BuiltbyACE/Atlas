package outbox

import (
	"time"

	"github.com/google/uuid"
)

// Escalation event types (§7). An escalation is raised when a work item passes
// its deadline and resolved when the item is no longer overdue. The orchestrator
// reacts to these to nudge the assignee and invalidate the executive dashboard
// (ADR-009): Go records the fact, Elixir decides what to push.
const (
	TypeEscalationRaisedV1   = "core.escalation.raised.v1"
	TypeEscalationResolvedV1 = "core.escalation.resolved.v1"
)

// AggregateEscalation groups escalation events.
const AggregateEscalation = "escalation"

// EscalationRef is the minimal view the escalation event builders need.
type EscalationRef struct {
	ID         uuid.UUID
	EntityType string
	EntityID   uuid.UUID
	ProjectID  *uuid.UUID
	Level      int
	Status     string
	Reason     string
	AssigneeID *uuid.UUID
	// AssigneeUserID is the assignee's account id, resolved by the producer.
	AssigneeUserID *uuid.UUID
	RaisedBy       *uuid.UUID
	ResolvedBy     *uuid.UUID
}

// EscalationRaisedPayload is emitted when an escalation is raised.
type EscalationRaisedPayload struct {
	EscalationID uuid.UUID  `json:"escalation_id"`
	EntityType   string     `json:"entity_type"`
	EntityID     uuid.UUID  `json:"entity_id"`
	ProjectID    *uuid.UUID `json:"project_id,omitempty"`
	Level        int        `json:"level"`
	Reason       string     `json:"reason,omitempty"`
	AssigneeID   *uuid.UUID `json:"assignee_person_id,omitempty"`
	// AssigneeUserID is the assignee's account id (§7.5), nil when the task is
	// unassigned or the assignee has no account.
	AssigneeUserID *uuid.UUID `json:"assignee_user_id,omitempty"`
	RaisedBy       *uuid.UUID `json:"raised_by_person_id,omitempty"`
	RaisedAt       time.Time  `json:"raised_at"`
}

// NewEscalationRaised builds a core.escalation.raised.v1 event.
func NewEscalationRaised(actorID *uuid.UUID, e EscalationRef) (Event, error) {
	return buildEvent(TypeEscalationRaisedV1, AggregateEscalation, e.ID, actorID, nil, EscalationRaisedPayload{
		EscalationID:   e.ID,
		EntityType:     e.EntityType,
		EntityID:       e.EntityID,
		ProjectID:      e.ProjectID,
		Level:          e.Level,
		Reason:         e.Reason,
		AssigneeID:     e.AssigneeID,
		AssigneeUserID: e.AssigneeUserID,
		RaisedBy:       e.RaisedBy,
		RaisedAt:       time.Now().UTC(),
	})
}

// EscalationResolvedPayload is emitted when an escalation is resolved.
type EscalationResolvedPayload struct {
	EscalationID uuid.UUID  `json:"escalation_id"`
	EntityType   string     `json:"entity_type"`
	EntityID     uuid.UUID  `json:"entity_id"`
	ProjectID    *uuid.UUID `json:"project_id,omitempty"`
	Level        int        `json:"level"`
	ResolvedBy   *uuid.UUID `json:"resolved_by_person_id,omitempty"`
	Reason       string     `json:"reason,omitempty"`
	ResolvedAt   time.Time  `json:"resolved_at"`
}

// NewEscalationResolved builds a core.escalation.resolved.v1 event.
func NewEscalationResolved(actorID *uuid.UUID, e EscalationRef) (Event, error) {
	return buildEvent(TypeEscalationResolvedV1, AggregateEscalation, e.ID, actorID, nil, EscalationResolvedPayload{
		EscalationID: e.ID,
		EntityType:   e.EntityType,
		EntityID:     e.EntityID,
		ProjectID:    e.ProjectID,
		Level:        e.Level,
		ResolvedBy:   e.ResolvedBy,
		Reason:       e.Reason,
		ResolvedAt:   time.Now().UTC(),
	})
}
