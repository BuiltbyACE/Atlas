package approvals

import (
	"context"

	"github.com/google/uuid"

	"github.com/safaristack/atlas/apps/api/internal/notifications"
)

// Notification is one intent to tell a person something about an approval. It
// is approvals' own type so this package does not depend on the notifications
// engine's request shape; NotificationsAdapter translates it.
type Notification struct {
	RecipientPersonID uuid.UUID
	Template          string
	Subject           string
	Body              string
	Payload           map[string]any
	EntityType        string
	EntityID          *uuid.UUID
	Critical          bool
	IdempotencyKey    string
	ActorID           *uuid.UUID
}

// Notifier records approval notifications. Fan-out is best-effort and happens
// after the state change has committed: the durable record of the change is the
// approval row and its core.approval.*.v1 event, so a notification failure must
// never roll back an approval (§6.4). The interface keeps the service testable
// without an SMTP server or a database.
type Notifier interface {
	Notify(ctx context.Context, n Notification) error
}

// Notification templates for the approval lifecycle.
const (
	TemplateRequested   = "core.approval.requested"
	TemplateApproved    = "core.approval.approved"
	TemplateDeclined    = "core.approval.declined"
	TemplateEscalated   = "core.approval.escalated"
	TemplateCancelled   = "core.approval.cancelled"
	TemplateExpired     = "core.approval.expired"
	TemplateAwaitSecond = "core.approval.awaiting_second"
)

// NotificationsAdapter bridges approvals.Notifier to the notifications engine.
type NotificationsAdapter struct {
	service *notifications.Service
}

// NewNotificationsAdapter wraps a notifications service for the approvals
// engine. A nil service yields a no-op notifier, which is what tests and
// deployments without notifications want.
func NewNotificationsAdapter(service *notifications.Service) *NotificationsAdapter {
	return &NotificationsAdapter{service: service}
}

// Notify records the notification intent through the notifications engine.
func (a *NotificationsAdapter) Notify(ctx context.Context, n Notification) error {
	if a == nil || a.service == nil {
		return nil
	}
	_, err := a.service.Request(ctx, notifications.Request{
		RecipientPersonID: n.RecipientPersonID,
		Template:          n.Template,
		Subject:           n.Subject,
		Body:              n.Body,
		Payload:           n.Payload,
		EntityType:        n.EntityType,
		EntityID:          n.EntityID,
		Critical:          n.Critical,
		IdempotencyKey:    n.IdempotencyKey,
		ActorID:           n.ActorID,
	})
	return err
}
