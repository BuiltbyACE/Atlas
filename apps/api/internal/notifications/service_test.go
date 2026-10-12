package notifications

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/safaristack/atlas/apps/api/internal/outbox"
)

// fakeStore is an in-memory Store for the engine's unit tests. It records what
// the write path asked it to persist.
type fakeStore struct {
	recipient *Recipient
	hasPref   bool
	pref      Preference

	created   []Notification
	createdEv *outbox.Event
	createErr error

	items    []Notification
	unread   int
	markRead bool
	markAll  int

	saved *Preference
}

func (f *fakeStore) Recipient(context.Context, uuid.UUID) (*Recipient, error) {
	return f.recipient, nil
}

func (f *fakeStore) Preferences(_ context.Context, personID uuid.UUID) (Preference, error) {
	if !f.hasPref {
		return DefaultPreference(personID, defaultTimezone), nil
	}
	return f.pref, nil
}

func (f *fakeStore) SavePreferences(_ context.Context, personID uuid.UUID, p Preference) error {
	p.PersonID = personID
	f.saved = &p
	f.pref = p
	f.hasPref = true
	return nil
}

func (f *fakeStore) CreateWithEvent(_ context.Context, ns []Notification, ev outbox.Event) error {
	if f.createErr != nil {
		return f.createErr
	}
	f.created = append(f.created, ns...)
	e := ev
	f.createdEv = &e
	return nil
}

func (f *fakeStore) List(_ context.Context, _ uuid.UUID, limit int, _ *Cursor) ([]Notification, error) {
	if limit < len(f.items) {
		return f.items[:limit], nil
	}
	return f.items, nil
}

func (f *fakeStore) UnreadCount(context.Context, uuid.UUID) (int, error) { return f.unread, nil }
func (f *fakeStore) MarkRead(context.Context, uuid.UUID, uuid.UUID) (bool, error) {
	return f.markRead, nil
}
func (f *fakeStore) MarkAllRead(context.Context, uuid.UUID) (int, error) { return f.markAll, nil }
func (f *fakeStore) ClaimDue(context.Context, string, time.Duration, int) ([]Notification, error) {
	return nil, nil
}
func (f *fakeStore) Complete(context.Context, Notification, Outcome, *outbox.Event) error {
	return nil
}
func (f *fakeStore) Defer(context.Context, uuid.UUID, time.Time) error { return nil }

func testService(store Store) *Service {
	svc := NewService(store, discardLogger())
	svc.now = func() time.Time { return time.Date(2026, time.October, 13, 10, 0, 0, 0, time.UTC) }
	return svc
}

func TestRequestCreatesInAppAndEmail(t *testing.T) {
	pid := uuid.Must(uuid.NewV7())
	store := &fakeStore{
		recipient: &Recipient{PersonID: pid, FullName: "Elijah", Email: "elijah@example.com", Status: "active"},
		hasPref:   true,
		pref:      Preference{PersonID: pid, InApp: true, Email: true, Timezone: "Africa/Nairobi"},
	}

	created, err := testService(store).Request(context.Background(), Request{
		RecipientPersonID: pid,
		Template:          "work.task.assigned",
		Subject:           "A task was assigned to you",
		Body:              "Open Atlas to see it.",
	})
	if err != nil {
		t.Fatalf("Request: %v", err)
	}
	if len(created) != 2 {
		t.Fatalf("created %d notifications, want 2 (in-app + email)", len(created))
	}

	byChannel := map[Channel]Notification{}
	for _, n := range created {
		byChannel[n.Channel] = n
	}

	inApp, ok := byChannel[ChannelInApp]
	if !ok {
		t.Fatal("no in-app notification created")
	}
	if inApp.Status != StatusDelivered {
		t.Errorf("in-app status = %q, want %q", inApp.Status, StatusDelivered)
	}
	if inApp.SentAt == nil {
		t.Error("in-app notification has no sent_at; it needs no provider")
	}
	if inApp.DeliverAfter != nil {
		t.Error("in-app notification should never be deferred")
	}

	email, ok := byChannel[ChannelEmail]
	if !ok {
		t.Fatal("no email notification created")
	}
	if email.Status != StatusPending {
		t.Errorf("email status = %q, want %q", email.Status, StatusPending)
	}
	if email.MaxAttempts != DefaultMaxAttempts {
		t.Errorf("email max_attempts = %d, want %d", email.MaxAttempts, DefaultMaxAttempts)
	}

	if store.createdEv == nil {
		t.Fatal("no outbox event written with the notifications")
	}
	if store.createdEv.Type != outbox.TypeNotificationRequestedV1 {
		t.Errorf("event type = %q, want %q", store.createdEv.Type, outbox.TypeNotificationRequestedV1)
	}
}

func TestRequestRespectsDisabledEmail(t *testing.T) {
	pid := uuid.Must(uuid.NewV7())
	store := &fakeStore{
		recipient: &Recipient{PersonID: pid, Status: "active"},
		hasPref:   true,
		pref:      Preference{PersonID: pid, InApp: true, Email: false, Timezone: "Africa/Nairobi"},
	}

	created, err := testService(store).Request(context.Background(), Request{RecipientPersonID: pid, Body: "hello"})
	if err != nil {
		t.Fatalf("Request: %v", err)
	}
	if len(created) != 1 || created[0].Channel != ChannelInApp {
		t.Fatalf("created %v, want a single in-app notification", created)
	}
}

func TestRequestDefersEmailDuringQuietHours(t *testing.T) {
	pid := uuid.Must(uuid.NewV7())
	store := &fakeStore{
		recipient: &Recipient{PersonID: pid, Status: "active"},
		hasPref:   true,
		pref: Preference{
			PersonID: pid, InApp: true, Email: true, Timezone: "Africa/Nairobi",
			QuietStart: minutes(12 * 60), QuietEnd: minutes(14 * 60),
		},
	}

	// Service "now" is 10:00 UTC = 13:00 Nairobi, inside quiet hours.
	created, err := testService(store).Request(context.Background(), Request{RecipientPersonID: pid, Body: "reminder"})
	if err != nil {
		t.Fatalf("Request: %v", err)
	}
	var email *Notification
	for i := range created {
		if created[i].Channel == ChannelEmail {
			email = &created[i]
		}
	}
	if email == nil {
		t.Fatal("email notification was dropped rather than deferred")
	}
	if email.DeliverAfter == nil {
		t.Fatal("email during quiet hours should carry a deliver_after")
	}
	want := time.Date(2026, time.October, 13, 11, 0, 0, 0, time.UTC) // 14:00 Nairobi
	if !email.DeliverAfter.Equal(want) {
		t.Errorf("deliver_after = %s, want %s", email.DeliverAfter, want)
	}
}

func TestRequestCriticalBypassesQuietHours(t *testing.T) {
	pid := uuid.Must(uuid.NewV7())
	store := &fakeStore{
		recipient: &Recipient{PersonID: pid, Status: "active"},
		hasPref:   true,
		pref: Preference{
			PersonID: pid, InApp: true, Email: true, Timezone: "Africa/Nairobi",
			QuietStart: minutes(12 * 60), QuietEnd: minutes(14 * 60),
		},
	}

	created, err := testService(store).Request(context.Background(), Request{RecipientPersonID: pid, Body: "escalation", Critical: true})
	if err != nil {
		t.Fatalf("Request: %v", err)
	}
	for _, n := range created {
		if n.Channel == ChannelEmail && n.DeliverAfter != nil {
			t.Fatal("critical email must not be deferred by quiet hours")
		}
	}
}

func TestRequestUnknownRecipient(t *testing.T) {
	store := &fakeStore{recipient: nil}
	_, err := testService(store).Request(context.Background(), Request{RecipientPersonID: uuid.Must(uuid.NewV7())})
	if err == nil {
		t.Fatal("expected an error for an unknown recipient")
	}
}

func TestSavePreferencesValidates(t *testing.T) {
	pid := uuid.Must(uuid.NewV7())
	svc := testService(&fakeStore{recipient: &Recipient{PersonID: pid}})

	tests := []struct {
		name string
		p    Preference
	}{
		{"unknown timezone", Preference{Email: true, Timezone: "Mars/Olympus"}},
		{"quiet start out of range", Preference{Email: true, Timezone: "Africa/Nairobi", QuietStart: minutes(1500), QuietEnd: minutes(60)}},
		{"quiet start without end", Preference{Email: true, Timezone: "Africa/Nairobi", QuietStart: minutes(600)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := svc.SavePreferences(context.Background(), pid, tt.p); err == nil {
				t.Fatal("expected a validation error")
			}
		})
	}

	got, err := svc.SavePreferences(context.Background(), pid, Preference{
		InApp: true, Email: true, Timezone: "Africa/Nairobi",
		QuietStart: minutes(22 * 60), QuietEnd: minutes(7 * 60),
	})
	if err != nil {
		t.Fatalf("SavePreferences: %v", err)
	}
	if got.QuietStart == nil || *got.QuietStart != 22*60 {
		t.Errorf("quiet start = %v, want %d", got.QuietStart, 22*60)
	}
}

// discardLogger keeps test output quiet.
func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(discardWriter{}, nil))
}

type discardWriter struct{}

func (discardWriter) Write(p []byte) (int, error) { return len(p), nil }
