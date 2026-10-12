package outbox

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Notification event types (docs/ATLAS.md §7.4). They are named
// module.aggregate.past_tense_verb.vN (§7.3) and each has a JSON Schema in
// contracts/events so Go (producer) and Elixir (consumer) cannot drift (§7.1).
const (
	// TypeNotificationRequestedV1 is written in the same transaction as the
	// notification rows. It is the signal Elixir consumes to push on the
	// recipient's channel and, later, to schedule channel fan-out (§5.2).
	TypeNotificationRequestedV1 = "core.notification.requested.v1"
	// TypeNotificationSentV1 records a successful delivery.
	TypeNotificationSentV1 = "core.notification.sent.v1"
	// TypeNotificationFailedV1 records a delivery that exhausted its retries.
	TypeNotificationFailedV1 = "core.notification.failed.v1"
)

// AggregateNotification is the aggregate for notification events.
const AggregateNotification = "notification"

// NotificationRequestedPayload is the body of core.notification.requested.v1.
// It is a signal (ADR-009): just enough for Elixir to push on the recipient's
// channel and, later, decide fan-out. No business data beyond ids.
type NotificationRequestedPayload struct {
	RecipientPersonID uuid.UUID   `json:"recipient_person_id"`
	NotificationIDs   []uuid.UUID `json:"notification_ids"`
	Template          string      `json:"template,omitempty"`
	Critical          bool        `json:"critical"`
	RequestedAt       time.Time   `json:"requested_at"`
}

// NotificationSentPayload is the body of core.notification.sent.v1.
type NotificationSentPayload struct {
	RecipientPersonID uuid.UUID `json:"recipient_person_id"`
	NotificationID    uuid.UUID `json:"notification_id"`
	Provider          string    `json:"provider"`
	SentAt            time.Time `json:"sent_at"`
}

// NotificationFailedPayload is the body of core.notification.failed.v1.
type NotificationFailedPayload struct {
	RecipientPersonID uuid.UUID `json:"recipient_person_id"`
	NotificationID    uuid.UUID `json:"notification_id"`
	Reason            string    `json:"reason"`
	FailedAt          time.Time `json:"failed_at"`
}

// NewNotificationRequested builds a core.notification.requested.v1 event. The
// aggregate is the notification stream: the aggregate id is the first
// notification id, with every id carried in the payload.
func NewNotificationRequested(actorID, correlationID *uuid.UUID,
	recipient uuid.UUID, ids []uuid.UUID, template string, critical bool) (Event, error) {

	anchor := recipient
	if len(ids) > 0 {
		anchor = ids[0]
	}
	return buildEvent(TypeNotificationRequestedV1, AggregateNotification, anchor, actorID, correlationID,
		NotificationRequestedPayload{
			RecipientPersonID: recipient,
			NotificationIDs:   ids,
			Template:          template,
			Critical:          critical,
			RequestedAt:       time.Now().UTC(),
		})
}

// NewNotificationSent builds a core.notification.sent.v1 event.
func NewNotificationSent(actorID, correlationID *uuid.UUID, recipient, notificationID uuid.UUID, provider string) (Event, error) {
	return buildEvent(TypeNotificationSentV1, AggregateNotification, notificationID, actorID, correlationID,
		NotificationSentPayload{
			RecipientPersonID: recipient,
			NotificationID:    notificationID,
			Provider:          provider,
			SentAt:            time.Now().UTC(),
		})
}

// NewNotificationFailed builds a core.notification.failed.v1 event.
func NewNotificationFailed(actorID, correlationID *uuid.UUID, recipient, notificationID uuid.UUID, reason string) (Event, error) {
	return buildEvent(TypeNotificationFailedV1, AggregateNotification, notificationID, actorID, correlationID,
		NotificationFailedPayload{
			RecipientPersonID: recipient,
			NotificationID:    notificationID,
			Reason:            reason,
			FailedAt:          time.Now().UTC(),
		})
}

// buildEvent is the shared envelope construction (§7.2): a UUIDv7 id, the type,
// the aggregate, the actor/correlation chain and the JSON payload.
func buildEvent(eventType, aggregateType string, aggregateID uuid.UUID, actorID, correlationID *uuid.UUID, payload any) (Event, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return Event{}, fmt.Errorf("generate event id: %w", err)
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return Event{}, fmt.Errorf("marshal %s payload: %w", eventType, err)
	}
	return Event{
		ID:            id,
		Type:          eventType,
		AggregateType: aggregateType,
		AggregateID:   aggregateID,
		OccurredAt:    time.Now().UTC(),
		ActorID:       actorID,
		CorrelationID: correlationID,
		Payload:       body,
	}, nil
}
