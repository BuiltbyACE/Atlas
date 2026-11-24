package email

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// fakeStore implements only what Send touches; any other call panics, which
// proves a refused send never reaches storage.
type fakeStore struct {
	Store
	addrs       []Address
	visible     bool
	messages    []*Message
	threadsMade int
}

func (f *fakeStore) EnsureLoginAddress(context.Context, uuid.UUID, string) error { return nil }
func (f *fakeStore) ListAddresses(context.Context, uuid.UUID) ([]Address, error) {
	return f.addrs, nil
}
func (f *fakeStore) ThreadVisibleTo(context.Context, uuid.UUID, uuid.UUID) (bool, error) {
	return f.visible, nil
}
func (f *fakeStore) CreateThread(context.Context, string) (uuid.UUID, error) {
	f.threadsMade++
	return uuid.New(), nil
}
func (f *fakeStore) CreateMessage(_ context.Context, _ Execer, m *Message) error {
	f.messages = append(f.messages, m)
	return nil
}
func (f *fakeStore) DB() *pgxpool.Pool { return nil }

type fakeMailer struct {
	err   error
	calls int
	from  string
	rcpts []string
	data  string
}

func (m *fakeMailer) SendRaw(_ context.Context, from string, rcpts []string, data []byte) error {
	m.calls++
	m.from, m.rcpts, m.data = from, rcpts, string(data)
	return m.err
}

func newSendFixture(mailer *fakeMailer) (*Service, *fakeStore, Actor) {
	user := uuid.New()
	st := &fakeStore{addrs: []Address{
		{UserID: user, Address: "ada@safaristacksolutions.co.ke", Verified: true},
		{UserID: user, Address: "info@safaristacksolutions.co.ke", Verified: false},
	}}
	svc := NewService(st, slog.New(slog.NewTextHandler(discard{}, nil)))
	if mailer != nil {
		svc.WithMailer(mailer)
	}
	return svc, st, Actor{UserID: user, Email: "ada@safaristacksolutions.co.ke"}
}

type discard struct{}

func (discard) Write(p []byte) (int, error) { return len(p), nil }

func validOpts() SendMessageOptions {
	return SendMessageOptions{
		FromAddress: "Ada@SafariStackSolutions.co.ke", ToAddresses: []string{"client@example.com"},
		BCCAddresses: []string{"boss@example.com"}, Subject: "Invoice", BodyText: "Hello",
	}
}

func TestSendDeliversRecordsSentAndKeepsBccOutOfHeaders(t *testing.T) {
	m := &fakeMailer{}
	svc, st, actor := newSendFixture(m)

	msg, err := svc.Send(context.Background(), actor, validOpts())
	if err != nil {
		t.Fatal(err)
	}
	if m.calls != 1 || m.from != "ada@safaristacksolutions.co.ke" {
		t.Fatalf("mailer calls=%d from=%q", m.calls, m.from)
	}
	if len(m.rcpts) != 2 {
		t.Fatalf("envelope recipients = %v; want To and Bcc", m.rcpts)
	}
	if strings.Contains(strings.ToLower(m.data), "bcc:") || strings.Contains(m.data, "boss@example.com") {
		t.Error("the blind copy leaked into the message headers or body")
	}
	if msg.Status != StatusSent || msg.SentAt == nil || len(st.messages) != 1 {
		t.Fatalf("status=%s stored=%d; want a stored, sent message", msg.Status, len(st.messages))
	}
}

func TestSendRefusesAnAddressThatIsNotVerifiedForTheCaller(t *testing.T) {
	for _, from := range []string{"info@safaristacksolutions.co.ke", "ceo@safaristacksolutions.co.ke"} {
		m := &fakeMailer{}
		svc, st, actor := newSendFixture(m)
		o := validOpts()
		o.FromAddress = from
		if _, err := svc.Send(context.Background(), actor, o); !errors.Is(err, ErrInvalid) {
			t.Errorf("from %s: err = %v, want ErrInvalid", from, err)
		}
		if m.calls != 0 || len(st.messages) != 0 {
			t.Errorf("from %s: a refused send must not deliver or store (calls=%d stored=%d)", from, m.calls, len(st.messages))
		}
	}
}

func TestSendRecordsAFailureAndReportsIt(t *testing.T) {
	m := &fakeMailer{err: errors.New("email: rcpt to client@example.com: 550 mailbox unavailable")}
	svc, st, actor := newSendFixture(m)

	msg, err := svc.Send(context.Background(), actor, validOpts())
	if !errors.Is(err, ErrDelivery) {
		t.Fatalf("err = %v, want ErrDelivery: the user must be told it did not go out", err)
	}
	if msg == nil || msg.Status != StatusFailed || msg.SentAt != nil || msg.ErrorMessage == "" {
		t.Fatalf("message = %+v; want a stored failed message with a reason", msg)
	}
	if len(st.messages) != 1 {
		t.Fatalf("stored = %d, want the failed attempt kept", len(st.messages))
	}
}

func TestSendWithoutAMailerNeverPretends(t *testing.T) {
	svc, st, actor := newSendFixture(nil)
	if _, err := svc.Send(context.Background(), actor, validOpts()); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("err = %v, want ErrNotConfigured", err)
	}
	if len(st.messages) != 0 {
		t.Fatal("nothing may be recorded as sent when delivery is not wired")
	}
}

func TestSendIntoAThreadTheCallerCannotSeeIsNotFound(t *testing.T) {
	m := &fakeMailer{}
	svc, _, actor := newSendFixture(m)
	o := validOpts()
	other := uuid.New()
	o.ThreadID = &other
	if _, err := svc.Send(context.Background(), actor, o); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
	if m.calls != 0 {
		t.Fatal("must not deliver into a thread the caller cannot see")
	}
}

func TestSendValidatesInput(t *testing.T) {
	cases := map[string]func(*SendMessageOptions){
		"no recipient":     func(o *SendMessageOptions) { o.ToAddresses = nil },
		"bad recipient":    func(o *SendMessageOptions) { o.ToAddresses = []string{"nope"} },
		"no subject":       func(o *SendMessageOptions) { o.Subject = "  " },
		"no body":          func(o *SendMessageOptions) { o.BodyText = "" },
		"header injection": func(o *SendMessageOptions) { o.ToAddresses = []string{"a@b.com\r\nBcc: x@y.com"} },
	}
	for name, mutate := range cases {
		m := &fakeMailer{}
		svc, _, actor := newSendFixture(m)
		o := validOpts()
		mutate(&o)
		if _, err := svc.Send(context.Background(), actor, o); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: err = %v, want ErrInvalid", name, err)
		}
		if m.calls != 0 {
			t.Errorf("%s: an invalid message must not be delivered", name)
		}
	}
}
