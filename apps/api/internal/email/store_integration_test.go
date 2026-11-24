package email

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("ATLAS_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("ATLAS_TEST_DATABASE_URL is not set; skipping email integration tests")
	}
	pool, err := pgxpool.New(context.Background(), url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// newUser inserts a person and an active account with a unique login email.
func newUser(t *testing.T, pool *pgxpool.Pool, label string) Actor {
	t.Helper()
	ctx := context.Background()
	person, user := uuid.New(), uuid.New()
	email := label + "-" + user.String()[:8] + "@safaristacksolutions.co.ke"
	if _, err := pool.Exec(ctx,
		`INSERT INTO people.persons (id, full_name, employment_type, status) VALUES ($1,$2,'full_time','active')`,
		person, "Email Test "+label); err != nil {
		t.Fatalf("insert person: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO core.user_accounts (id, person_id, email, status) VALUES ($1,$2,$3,'active')`,
		user, person, email); err != nil {
		t.Fatalf("insert account: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM core.email_addresses WHERE user_id = $1`, user)
		_, _ = pool.Exec(ctx, `DELETE FROM core.user_accounts WHERE id = $1`, user)
		_, _ = pool.Exec(ctx, `DELETE FROM people.persons WHERE id = $1`, person)
	})
	return Actor{UserID: user, Email: email}
}

// addThread stores a thread with one message between the given addresses.
func addThread(t *testing.T, pool *pgxpool.Pool, st *PGStore, from string, to, cc, bcc []string) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	thread, err := st.CreateThread(ctx, "Subject "+uuid.NewString()[:6])
	if err != nil {
		t.Fatalf("CreateThread: %v", err)
	}
	id := uuid.New()
	msg := &Message{
		ID: id, ThreadID: thread, Direction: DirectionOutbound, FromAddress: from,
		ToAddresses: to, CCAddresses: cc, BCCAddresses: bcc, Subject: "s", BodyText: "b",
		MessageID: "<" + id.String() + "@test>", Status: StatusSent,
	}
	if err := st.CreateMessage(ctx, pool, msg); err != nil {
		t.Fatalf("CreateMessage: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM core.email_threads WHERE id = $1`, thread) })
	return thread
}

func TestInboxQueriesRunAndAreScopedToTheCaller(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	st := NewPGStore(pool)
	svc := NewService(st, nil)

	ada := newUser(t, pool, "ada")
	bob := newUser(t, pool, "bob")
	eve := newUser(t, pool, "eve")

	toBob := addThread(t, pool, st, ada.Email, []string{bob.Email}, nil, nil)
	ccBob := addThread(t, pool, st, ada.Email, []string{"client@example.com"}, []string{bob.Email}, nil)
	bccBob := addThread(t, pool, st, ada.Email, []string{"client@example.com"}, nil, []string{bob.Email})
	unrelated := addThread(t, pool, st, "x@example.com", []string{"y@example.com"}, nil, nil)

	ids := func(items []ThreadListItem) map[uuid.UUID]bool {
		m := map[uuid.UUID]bool{}
		for _, i := range items {
			m[i.ID] = true
		}
		return m
	}

	// This call failed in production with SQLSTATE 42601 before the fix.
	adaInbox, err := svc.ListThreads(ctx, ada, 50, nil)
	if err != nil {
		t.Fatalf("ListThreads(ada): %v", err)
	}
	if got := ids(adaInbox); !got[toBob] || !got[ccBob] || !got[bccBob] || got[unrelated] {
		t.Errorf("ada (sender) should see her three threads and not the unrelated one: %v", got)
	}

	bobInbox, err := svc.ListThreads(ctx, bob, 50, nil)
	if err != nil {
		t.Fatalf("ListThreads(bob): %v", err)
	}
	got := ids(bobInbox)
	if !got[toBob] || !got[ccBob] {
		t.Errorf("bob should see threads where he is To or Cc: %v", got)
	}
	if got[bccBob] {
		t.Error("a blind copy must not make the thread visible to the Bcc recipient's inbox")
	}
	if got[unrelated] {
		t.Error("bob must not see a thread he is not part of")
	}

	eveInbox, err := svc.ListThreads(ctx, eve, 50, nil)
	if err != nil {
		t.Fatalf("ListThreads(eve): %v", err)
	}
	if len(eveInbox) != 0 {
		t.Errorf("eve is on no thread and must see none: %v", ids(eveInbox))
	}
	if eveInbox == nil {
		t.Error("an empty inbox must be [] not null")
	}

	// The pagination cursor must also be valid SQL.
	if len(adaInbox) > 0 {
		if _, err := svc.ListThreads(ctx, ada, 1, &adaInbox[0].ID); err != nil {
			t.Fatalf("ListThreads with cursor: %v", err)
		}
	}

	// Reading a thread: allowed for participants, NotFound (not Forbidden) otherwise.
	if _, msgs, err := svc.GetThreadWithMessages(ctx, bob, toBob, 100, nil); err != nil || len(msgs) != 1 {
		t.Fatalf("bob reading his thread: msgs=%d err=%v", len(msgs), err)
	}
	if _, _, err := svc.GetThreadWithMessages(ctx, eve, toBob, 100, nil); !errors.Is(err, ErrNotFound) {
		t.Errorf("eve reading bob's thread: err = %v, want ErrNotFound", err)
	}
}

func TestMessageWithTwoAttachmentsIsReturnedOnce(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	st := NewPGStore(pool)
	svc := NewService(st, nil)
	ada := newUser(t, pool, "ada")

	thread := addThread(t, pool, st, ada.Email, []string{"c@example.com"}, nil, nil)
	var msgID uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT id FROM core.email_messages WHERE thread_id = $1`, thread).Scan(&msgID); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := st.CreateAttachment(ctx, pool, &Attachment{
			ID: uuid.New(), MessageID: msgID, Filename: "f.pdf", ContentType: "application/pdf", StorageKey: "k",
		}); err != nil {
			t.Fatal(err)
		}
	}
	_, msgs, err := svc.GetThreadWithMessages(ctx, ada, thread, 100, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 1 || len(msgs[0].Attachments) != 2 {
		t.Fatalf("got %d messages; want 1 message with 2 attachments (%+v)", len(msgs), msgs)
	}
}

func TestAddressRegistrationOwnershipAndVerification(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	st := NewPGStore(pool)
	svc := NewService(st, nil)
	ada := newUser(t, pool, "ada")
	bob := newUser(t, pool, "bob")

	// Listing registers the login email as verified and primary.
	list, err := svc.ListAddresses(ctx, ada)
	if err != nil || len(list) != 1 || !list[0].Verified || !list[0].IsPrimary || list[0].Address != ada.Email {
		t.Fatalf("login address: %+v err=%v", list, err)
	}
	// Calling again is idempotent.
	if list, _ = svc.ListAddresses(ctx, ada); len(list) != 1 {
		t.Fatalf("listing twice created duplicates: %+v", list)
	}

	// A different address registers unverified, owned by the caller (this was the
	// uuid.Nil foreign-key failure in production).
	shared := "info-" + uuid.NewString()[:8] + "@safaristacksolutions.co.ke"
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM core.email_addresses WHERE address = $1`, shared) })
	a, err := svc.RegisterAddress(ctx, ada, shared, false)
	if err != nil || a.Verified || a.UserID != ada.UserID {
		t.Fatalf("register: %+v err=%v", a, err)
	}
	// Someone else cannot take it.
	if _, err := svc.RegisterAddress(ctx, bob, shared, false); !errors.Is(err, errDuplicateAddress) {
		t.Errorf("registering a claimed address: err = %v, want errDuplicateAddress", err)
	}
	// An unverified address grants neither sending nor reading.
	thread := addThread(t, pool, st, shared, []string{"c@example.com"}, nil, nil)
	if _, _, err := svc.GetThreadWithMessages(ctx, ada, thread, 100, nil); !errors.Is(err, ErrNotFound) {
		t.Errorf("an unverified address must not reveal a shared mailbox: err = %v", err)
	}
}

// TestInboundMessageIsStoredThreadedAndDeduplicated exercises Receive against the
// real schema: a message with no Cc/Bcc (which wrote NULL into NOT NULL columns),
// a reply that joins the parent's thread, and a repeated delivery.
func TestInboundMessageIsStoredThreadedAndDeduplicated(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	st := NewPGStore(pool)
	svc := NewService(st, nil)
	ada := newUser(t, pool, "ada")

	first, err := svc.Receive(ctx, InboundMessage{
		MessageID:   "<in-" + uuid.NewString() + "@client>",
		FromAddress: "client@example.com", ToAddresses: []string{ada.Email}, Subject: "Question",
	})
	if err != nil || first == nil {
		t.Fatalf("Receive: msg=%v err=%v", first, err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM core.email_threads WHERE id = $1`, first.ThreadID) })

	reply, err := svc.Receive(ctx, InboundMessage{
		MessageID: "<in-" + uuid.NewString() + "@client>", InReplyTo: first.MessageID,
		FromAddress: "client@example.com", ToAddresses: []string{ada.Email}, Subject: "Re: Question",
	})
	if err != nil || reply == nil || reply.ThreadID != first.ThreadID {
		t.Fatalf("a reply must join the parent's thread: reply=%+v err=%v", reply, err)
	}

	dup, err := svc.Receive(ctx, InboundMessage{MessageID: first.MessageID, FromAddress: "client@example.com"})
	if err != nil || dup != nil {
		t.Fatalf("a repeated delivery must be acknowledged without storing: dup=%v err=%v", dup, err)
	}

	// The addressee sees it in their inbox.
	inbox, err := svc.ListThreads(ctx, ada, 50, nil)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, it := range inbox {
		found = found || it.ID == first.ThreadID
	}
	if !found {
		t.Error("the recipient should see the inbound thread in their inbox")
	}
}

func TestSetPrimaryAndDeleteAddress(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	st := NewPGStore(pool)
	svc := NewService(st, nil)
	ada := newUser(t, pool, "ada")
	bob := newUser(t, pool, "bob")

	list, err := svc.ListAddresses(ctx, ada)
	if err != nil || len(list) != 1 {
		t.Fatalf("list: %+v err=%v", list, err)
	}
	login := list[0]

	// The sign-in address cannot be removed; someone else's address is not found.
	if err := svc.DeleteAddress(ctx, ada, login.ID); !errors.Is(err, ErrInvalid) {
		t.Errorf("deleting the login address: err = %v, want ErrInvalid", err)
	}
	if err := svc.DeleteAddress(ctx, bob, login.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("deleting another user's address: err = %v, want ErrNotFound", err)
	}

	// An unverified address cannot become the default; removing it works.
	extra := "extra-" + uuid.NewString()[:8] + "@safaristacksolutions.co.ke"
	a, err := svc.RegisterAddress(ctx, ada, extra, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SetPrimaryAddress(ctx, ada, a.ID); !errors.Is(err, ErrInvalid) {
		t.Errorf("unverified default: err = %v, want ErrInvalid", err)
	}
	if got, err := svc.SetPrimaryAddress(ctx, ada, login.ID); err != nil || !got.IsPrimary {
		t.Errorf("setting the verified login address primary: %+v err=%v", got, err)
	}
	if err := svc.DeleteAddress(ctx, ada, a.ID); err != nil {
		t.Errorf("deleting an own extra address: %v", err)
	}
}
