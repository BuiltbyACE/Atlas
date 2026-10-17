package people

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/safaristack/atlas/apps/api/internal/notifications"
)

// TemplateMissedReport is the notification template for a missed daily report
// (§8.5). Templates are names, not code: the orchestrator and the channel
// adapters decide how one is rendered (§6.4).
const TemplateMissedReport = "people.daily_report.missed"

// MissedReportNotice is one intent to tell a person's line manager that a report
// was missed (§8.5). It is people's own type so this package does not depend on
// the notifications engine's request shape; NotificationsAdapter translates it.
type MissedReportNotice struct {
	// LineManagerID is the person the notice goes to.
	LineManagerID uuid.UUID
	// ReporterID and ReporterName identify whose report is missing.
	ReporterID   uuid.UUID
	ReporterName string
	ReportID     uuid.UUID
	// ReportDate is the report's calendar date, YYYY-MM-DD.
	ReportDate string
	// Misses is the reporter's miss count inside the policy window, and
	// Threshold the count at which L2 escalation starts — so the notice can say
	// how close the person is to escalation.
	Misses     int
	Threshold  int
	WindowDays int
	// IdempotencyKey makes a repeated sweep pass a no-op (§8.10).
	IdempotencyKey string
}

// Notifier records the line-manager notice for a missed report. Fan-out is
// best-effort and happens after the state change committed: the durable record
// is the missed row and its people.daily_report.missed.v1 event, so a
// notification failure must never fail a sweep pass (§6.4). The interface keeps
// the sweep testable without a database or a mail server.
type Notifier interface {
	NotifyMissedReport(ctx context.Context, n MissedReportNotice) error
}

// NotificationsAdapter bridges people.Notifier to the notifications engine.
type NotificationsAdapter struct {
	service *notifications.Service
}

// NewNotificationsAdapter wraps a notifications service for the people module. A
// nil service yields a no-op notifier, which is what tests and deployments
// without notifications want.
func NewNotificationsAdapter(service *notifications.Service) *NotificationsAdapter {
	return &NotificationsAdapter{service: service}
}

// NotifyMissedReport records the notification intent through the notifications
// engine.
func (a *NotificationsAdapter) NotifyMissedReport(ctx context.Context, n MissedReportNotice) error {
	if a == nil || a.service == nil {
		return nil
	}
	name := n.ReporterName
	if name == "" {
		name = "A teammate"
	}
	_, err := a.service.Request(ctx, notifications.Request{
		RecipientPersonID: n.LineManagerID,
		Template:          TemplateMissedReport,
		Subject:           fmt.Sprintf("Daily report missed: %s", n.ReportDate),
		Body: fmt.Sprintf("%s did not file a daily report for %s. That is %d missed in the last %d days (%d escalates to the CEO).",
			name, n.ReportDate, n.Misses, n.WindowDays, n.Threshold),
		Payload: map[string]any{
			"report_id":          n.ReportID.String(),
			"reporter_person_id": n.ReporterID.String(),
			"report_date":        n.ReportDate,
			"misses":             n.Misses,
			"threshold":          n.Threshold,
			"window_days":        n.WindowDays,
		},
		EntityType:     EntityDailyReport,
		EntityID:       &n.ReportID,
		IdempotencyKey: n.IdempotencyKey,
	})
	return err
}
