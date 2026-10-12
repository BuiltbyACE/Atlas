// Package notifications owns Atlas notifications (docs/ATLAS.md §6.4): the
// durable record, per-person preferences and quiet hours, and delivery through
// channel adapters.
//
// The engine is built for the §11 SLOs. The write path — Service.Request — never
// talks to a provider: it records the notification(s) and an outbox event in a
// single transaction and returns. That is what keeps a business action fast and
// the outbox lag bounded. Sends happen asynchronously in Dispatcher, which
// applies a per-attempt timeout, retries with exponential backoff and jitter, a
// per-provider circuit breaker, per-provider concurrency limits and idempotency
// keys, as §8.10 requires.
//
// In-app is different from email and deliberately so: the record is delivered
// the moment Elixir consumes the outbox event and pushes on the user's channel
// (ADR-009), so the browser updates without polling.
package notifications

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Channel is a delivery channel (§6.4).
type Channel string

const (
	ChannelInApp    Channel = "in_app"
	ChannelEmail    Channel = "email"
	ChannelWhatsApp Channel = "whatsapp"
	ChannelSMS      Channel = "sms"
)

// Deliverable reports whether the engine can actually send on the channel in
// this phase. Phase 1 ships in-app and email (Appendix A); WhatsApp template
// messages and SMS fallback arrive with their adapters.
func (c Channel) Deliverable() bool {
	return c == ChannelInApp || c == ChannelEmail
}

// Status mirrors the core.notifications.status check constraint.
type Status string

const (
	StatusPending   Status = "pending"
	StatusSent      Status = "sent"
	StatusDelivered Status = "delivered"
	StatusFailed    Status = "failed"
	StatusRead      Status = "read"
)

// DefaultMaxAttempts caps how many times the dispatcher tries a send before the
// notification is marked failed and a core.notification.failed.v1 event is
// emitted for someone to notice. It is stored per notification, so changing the
// policy never re-interprets rows already in flight.
const DefaultMaxAttempts = 5

// ErrInvalid marks a caller error (a validation failure), so the HTTP layer can
// answer 400 rather than 500. ErrNotFound marks a missing recipient or record.
var (
	ErrInvalid  = errors.New("notifications: invalid request")
	ErrNotFound = errors.New("notifications: not found")
)

// Notification mirrors a row in core.notifications.
type Notification struct {
	ID                uuid.UUID
	RecipientPersonID uuid.UUID
	Channel           Channel
	Template          string
	Subject           string
	Body              string
	Payload           map[string]any
	EntityType        string
	EntityID          *uuid.UUID
	Status            Status
	Attempts          int
	MaxAttempts       int
	LastError         string
	Critical          bool
	IdempotencyKey    string
	DeliverAfter      *time.Time
	CreatedAt         time.Time
	SentAt            *time.Time
	ReadAt            *time.Time

	// RecipientEmail is join-only: the recipient's work email, filled by the
	// dispatcher's claim query. It is never a column on core.notifications.
	RecipientEmail string
}

// Preference is a person's notification choices (§6.4). Absence means the
// defaults, so a Preference is always fully populated by the store.
// Preference mirrors core.notification_preferences with defaults applied.
type Preference struct {
	PersonID   uuid.UUID
	InApp      bool
	Email      bool
	WhatsApp   bool
	SMS        bool
	QuietStart *int // minutes since midnight, in Timezone
	QuietEnd   *int
	Timezone   string
	UpdatedAt  time.Time
}

// Recipient is the delivery identity for a person.
type Recipient struct {
	PersonID uuid.UUID
	FullName string
	Email    string
	Status   string
}

// Request is the input to Service.Request. It describes an intent to notify one
// person; the engine decides the channel set from preferences and quiet hours.
type Request struct {
	RecipientPersonID uuid.UUID
	Template          string
	Subject           string
	Body              string
	Payload           map[string]any
	EntityType        string
	EntityID          *uuid.UUID
	// Critical items bypass quiet hours (§6.4: escalations L2+ only).
	Critical bool
	// IdempotencyKey, when set, makes a repeated request a no-op (§8.10).
	IdempotencyKey string
	// ActorID is the person who caused the notification, for the outbox envelope.
	ActorID *uuid.UUID
}

// Message is what a channel adapter sends.
type Message struct {
	To             string // email address or E.164 phone
	Subject        string
	Body           string
	IdempotencyKey string
}

// Sender is a channel adapter. Adapters live in internal/adapters and are the
// only place a provider is called (§5.2, §8.10).
type Sender interface {
	Channel() Channel
	// Provider names the adapter for attempt records and metrics, e.g. "smtp".
	Provider() string
	Send(ctx context.Context, msg Message) (providerRef string, err error)
}

// Outcome is the result of one delivery attempt, written by the dispatcher.
type Outcome struct {
	Success     bool
	Provider    string
	ProviderRef string
	Err         string
	// NextAttempt, when set, reschedules the notification instead of failing it.
	NextAttempt *time.Time
}

// valid reports the minimum a request must carry.
func (e *Request) valid() error {
	if e.RecipientPersonID == uuid.Nil {
		return fmt.Errorf("%w: recipient_person_id is required", ErrInvalid)
	}
	return nil
}
