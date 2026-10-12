package notifications

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/safaristack/atlas/apps/api/internal/outbox"
)

// Service is the notification engine's request side: it turns an intent into
// durable records and events, and serves a person's own notification centre.
type Service struct {
	store  Store
	now    func() time.Time
	logger *slog.Logger
}

// NewService builds the notification service.
func NewService(store Store, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{
		store:  store,
		now:    func() time.Time { return time.Now().UTC() },
		logger: logger,
	}
}

// Request records an intent to notify one person and returns the rows created.
//
// This is the write path and it is deliberately boring: resolve, decide the
// channel set, insert rows + one outbox event in a single transaction. It never
// touches a provider. A caller on a request-handling path can await it without
// risking the §11 latency budget.
func (s *Service) Request(ctx context.Context, in Request) ([]Notification, error) {
	if err := in.valid(); err != nil {
		return nil, err
	}

	recipient, err := s.store.Recipient(ctx, in.RecipientPersonID)
	if err != nil {
		return nil, err
	}
	if recipient == nil {
		return nil, ErrNotFound
	}

	pref, err := s.store.Preferences(ctx, in.RecipientPersonID)
	if err != nil {
		return nil, err
	}

	now := s.now()
	drafts := make([]Notification, 0, 2)
	for _, ch := range ChannelsFor(pref) {
		n := Notification{
			RecipientPersonID: in.RecipientPersonID,
			Channel:           ch,
			Template:          in.Template,
			Subject:           in.Subject,
			Body:              in.Body,
			Payload:           in.Payload,
			EntityType:        in.EntityType,
			EntityID:          in.EntityID,
			Critical:          in.Critical,
			IdempotencyKey:    in.IdempotencyKey,
			MaxAttempts:       DefaultMaxAttempts,
			CreatedAt:         now,
			Status:            StatusPending,
		}
		id, err := uuid.NewV7()
		if err != nil {
			return nil, fmt.Errorf("notifications: id: %w", err)
		}
		n.ID = id

		switch ch {
		case ChannelInApp:
			// In-app is delivered the moment it is recorded: it needs no
			// provider, just Elixir pushing the signal (ADR-009).
			n.Status = StatusDelivered
			sent := now
			n.SentAt = &sent
		case ChannelEmail:
			// Quiet hours defer, they never drop (§6.4).
			if !in.Critical && InQuietHours(pref, now) {
				until := QuietEndsAt(pref, now)
				n.DeliverAfter = &until
			}
		}
		drafts = append(drafts, n)
	}

	if len(drafts) == 0 {
		return nil, nil
	}

	ids := make([]uuid.UUID, len(drafts))
	for i := range drafts {
		ids[i] = drafts[i].ID
	}
	ev, err := outbox.NewNotificationRequested(in.ActorID, nil, in.RecipientPersonID, ids, in.Template, in.Critical)
	if err != nil {
		return nil, err
	}

	if err := s.store.CreateWithEvent(ctx, drafts, ev); err != nil {
		return nil, err
	}
	return drafts, nil
}

// List returns a person's in-app notifications, newest first.
func (s *Service) List(ctx context.Context, personID uuid.UUID, limit int, before *Cursor) ([]Notification, error) {
	if limit <= 0 {
		limit = 50
	}
	return s.store.List(ctx, personID, limit, before)
}

// UnreadCount returns how many in-app notifications a person has not read.
func (s *Service) UnreadCount(ctx context.Context, personID uuid.UUID) (int, error) {
	return s.store.UnreadCount(ctx, personID)
}

// MarkRead marks one of a person's in-app notifications read.
func (s *Service) MarkRead(ctx context.Context, personID, id uuid.UUID) error {
	found, err := s.store.MarkRead(ctx, personID, id)
	if err != nil {
		return err
	}
	if !found {
		return ErrNotFound
	}
	return nil
}

// MarkAllRead marks every unread in-app notification read, returning the count.
func (s *Service) MarkAllRead(ctx context.Context, personID uuid.UUID) (int, error) {
	return s.store.MarkAllRead(ctx, personID)
}

// Preferences returns a person's preferences, applying defaults when unset.
func (s *Service) Preferences(ctx context.Context, personID uuid.UUID) (Preference, error) {
	return s.store.Preferences(ctx, personID)
}

// SavePreferences validates and stores a person's choices, returning what was
// persisted.
func (s *Service) SavePreferences(ctx context.Context, personID uuid.UUID, p Preference) (Preference, error) {
	p.PersonID = personID
	if p.Timezone == "" {
		p.Timezone = defaultTimezone
	}
	if _, err := time.LoadLocation(p.Timezone); err != nil {
		return Preference{}, fmt.Errorf("%w: unknown timezone %q", ErrInvalid, p.Timezone)
	}
	if err := validateQuiet(p.QuietStart, "quiet_start"); err != nil {
		return Preference{}, err
	}
	if err := validateQuiet(p.QuietEnd, "quiet_end"); err != nil {
		return Preference{}, err
	}
	if p.QuietStart != nil && p.QuietEnd == nil || p.QuietStart == nil && p.QuietEnd != nil {
		return Preference{}, fmt.Errorf("%w: quiet hours need both a start and an end", ErrInvalid)
	}
	if err := s.store.SavePreferences(ctx, personID, p); err != nil {
		return Preference{}, err
	}
	return s.store.Preferences(ctx, personID)
}

func validateQuiet(minute *int, name string) error {
	if minute == nil {
		return nil
	}
	if *minute < 0 || *minute > 1439 {
		return fmt.Errorf("%w: %s must be minutes since midnight (0-1439)", ErrInvalid, name)
	}
	return nil
}
