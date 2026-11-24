// Package email serves Atlas's two-way email pipeline (§8.4).
//
// Outbound mail goes through the SMTP adapter already used for invitation
// emails. Inbound mail arrives through a webhook from the inbound mail
// provider and is stored as thread/message records for the UI.
package email

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/safaristack/atlas/apps/api/internal/outbox"
)

// Service is the email pipeline: send, receive, and address management.
type Service struct {
	store  Store
	logger *slog.Logger
	now    func() time.Time
	mailer Mailer

	// outbox is optional; when present, every email write also emits an
	// outbox event so the Elixir consumer can push realtime updates.
	outbox func(ctx context.Context, e outbox.Event) error
}

// NewService builds the email service.
func NewService(store Store, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{
		store:  store,
		logger: logger,
		now:    time.Now,
	}
}

// WithOutbox wires the outbox event writer so email writes produce events.
func (s *Service) WithOutbox(write func(ctx context.Context, e outbox.Event) error) *Service {
	s.outbox = write
	return s
}

// SendAddressOptions is the input for registering a send-as address.
type SendAddressOptions struct {
	UserID    uuid.UUID
	Address   string
	IsPrimary bool
}

// SendMessageOptions is the input for sending an email.
type SendMessageOptions struct {
	ThreadID       *uuid.UUID
	Subject        string
	BodyText       string
	BodyHTML       string
	FromAddress    string
	ToAddresses    []string
	CCAddresses    []string
	BCCAddresses   []string
	InReplyTo      string
	MsgReferences  string
	Attachments    []AttachmentInput
	IdempotencyKey string
}

// AttachmentInput describes a file to attach to an outgoing email.
type AttachmentInput struct {
	Filename    string
	ContentType string
	SizeBytes   int64
	StorageKey  string
}

// Actor is the signed-in person an email operation runs as. Every read and
// write is scoped to them: Go is the authority on who may see which mail.
type Actor struct {
	UserID uuid.UUID
	Email  string
}

func (a Actor) valid() bool { return a.UserID != uuid.Nil }

// prepare makes sure the actor's login email exists as a verified address.
func (s *Service) prepare(ctx context.Context, a Actor) error {
	if !a.valid() {
		return ErrForbidden
	}
	return s.store.EnsureLoginAddress(ctx, a.UserID, a.Email)
}

// RegisterAddress records a send-as address for the actor. The actor's own login
// email is verified already; any other address is stored unverified and cannot
// be used to send or to read mail until an administrator verifies it, because
// otherwise anyone could claim a shared mailbox and read it.
func (s *Service) RegisterAddress(ctx context.Context, a Actor, address string, isPrimary bool) (*Address, error) {
	if err := s.prepare(ctx, a); err != nil {
		return nil, err
	}
	bare, err := normalizeAddress(address)
	if err != nil {
		return nil, err
	}
	addrs, err := s.store.ListAddresses(ctx, a.UserID)
	if err != nil {
		return nil, err
	}
	for i := range addrs {
		if addrs[i].Address == bare {
			return &addrs[i], nil // already registered to this user
		}
	}
	created, err := s.store.CreateAddress(ctx, s.store.DB(), a.UserID, bare, false)
	if err != nil {
		return nil, err
	}
	return created, nil
}

// DeleteAddress removes one of the actor's own addresses. The sign-in address is
// not removable: it is the identity the account itself proves.
func (s *Service) DeleteAddress(ctx context.Context, a Actor, id uuid.UUID) error {
	if err := s.prepare(ctx, a); err != nil {
		return err
	}
	addrs, err := s.store.ListAddresses(ctx, a.UserID)
	if err != nil {
		return err
	}
	for _, addr := range addrs {
		if addr.ID != id {
			continue
		}
		if addr.Address == strings.ToLower(strings.TrimSpace(a.Email)) {
			return fmt.Errorf("%w: your sign-in address cannot be removed", ErrInvalid)
		}
		if _, err := s.store.DeleteAddress(ctx, a.UserID, id); err != nil {
			return err
		}
		return nil
	}
	return ErrNotFound
}

// SetPrimaryAddress makes one of the actor's verified addresses the default From.
func (s *Service) SetPrimaryAddress(ctx context.Context, a Actor, id uuid.UUID) (*Address, error) {
	if err := s.prepare(ctx, a); err != nil {
		return nil, err
	}
	addrs, err := s.store.ListAddresses(ctx, a.UserID)
	if err != nil {
		return nil, err
	}
	for i := range addrs {
		if addrs[i].ID != id {
			continue
		}
		if !addrs[i].Verified {
			return nil, fmt.Errorf("%w: only a verified address can be the default", ErrInvalid)
		}
		tx, err := s.store.DB().Begin(ctx)
		if err != nil {
			return nil, fmt.Errorf("email: begin tx: %w", err)
		}
		defer func() { _ = tx.Rollback(ctx) }()
		if err := s.store.SetPrimaryAddress(ctx, tx, a.UserID, id); err != nil {
			return nil, err
		}
		if err := tx.Commit(ctx); err != nil {
			return nil, fmt.Errorf("email: commit tx: %w", err)
		}
		addrs[i].IsPrimary = true
		return &addrs[i], nil
	}
	return nil, ErrNotFound
}

// ListAddresses returns the actor's send-as addresses.
func (s *Service) ListAddresses(ctx context.Context, a Actor) ([]Address, error) {
	if err := s.prepare(ctx, a); err != nil {
		return nil, err
	}
	return s.store.ListAddresses(ctx, a.UserID)
}

// ListThreads returns the actor's threads (inbox).
func (s *Service) ListThreads(ctx context.Context, a Actor, limit int, before *uuid.UUID) ([]ThreadListItem, error) {
	if err := s.prepare(ctx, a); err != nil {
		return nil, err
	}
	return s.store.ListThreads(ctx, a.UserID, limit, before)
}

// GetThreadWithMessages returns a thread with its messages, if the actor may see it.
func (s *Service) GetThreadWithMessages(ctx context.Context, a Actor, id uuid.UUID, limit int, before *uuid.UUID) (*Thread, []MessageView, error) {
	if err := s.prepare(ctx, a); err != nil {
		return nil, nil, err
	}
	return s.store.GetThreadWithMessages(ctx, a.UserID, id, limit, before)
}

// Mailer hands a finished RFC 5322 message to a mail server. from is the envelope
// sender and rcpts every envelope recipient, blind copies included.
type Mailer interface {
	SendRaw(ctx context.Context, from string, rcpts []string, data []byte) error
}

// WithMailer wires the SMTP delivery. Without it Send refuses to run: a message
// that cannot leave must never be recorded as sent.
func (s *Service) WithMailer(m Mailer) *Service {
	s.mailer = m
	return s
}

const (
	maxRecipients = 50
	maxBodyBytes  = 1 << 20
	deliverTimout = 20 * time.Second
)

// Send delivers an outbound email and records the outcome.
//
// The message goes out over SMTP first; only then is it stored, as "sent" with
// its Message-ID when the server accepted it, or as "failed" with the reason when
// it did not. The caller is told which, so the interface can never claim a
// delivery that did not happen. The From address must be one of the actor's own
// verified addresses, so nobody can send as a colleague or a shared mailbox.
func (s *Service) Send(ctx context.Context, actor Actor, opts SendMessageOptions) (*Message, error) {
	if err := s.prepare(ctx, actor); err != nil {
		return nil, err
	}
	if s.mailer == nil {
		return nil, ErrNotConfigured
	}

	from, err := normalizeAddress(opts.FromAddress)
	if err != nil {
		return nil, err
	}
	owned, err := s.store.ListAddresses(ctx, actor.UserID)
	if err != nil {
		return nil, err
	}
	verified := false
	for _, a := range owned {
		if a.Verified && a.Address == from {
			verified = true
			break
		}
	}
	if !verified {
		return nil, fmt.Errorf("%w: %s is not one of your verified send-as addresses", ErrInvalid, from)
	}

	to, err := normalizeAddressList(opts.ToAddresses, maxRecipients)
	if err != nil {
		return nil, err
	}
	cc, err := normalizeAddressList(opts.CCAddresses, maxRecipients)
	if err != nil {
		return nil, err
	}
	bcc, err := normalizeAddressList(opts.BCCAddresses, maxRecipients)
	if err != nil {
		return nil, err
	}
	if len(to) == 0 {
		return nil, fmt.Errorf("%w: at least one recipient is required", ErrInvalid)
	}
	if len(to)+len(cc)+len(bcc) > maxRecipients {
		return nil, fmt.Errorf("%w: at most %d recipients are allowed", ErrInvalid, maxRecipients)
	}
	subject := headerValue(opts.Subject)
	if subject == "" {
		return nil, fmt.Errorf("%w: a subject is required", ErrInvalid)
	}
	if len(subject) > 500 {
		return nil, fmt.Errorf("%w: the subject is too long", ErrInvalid)
	}
	if strings.TrimSpace(opts.BodyText) == "" && strings.TrimSpace(opts.BodyHTML) == "" {
		return nil, fmt.Errorf("%w: the message has no body", ErrInvalid)
	}
	if len(opts.BodyText)+len(opts.BodyHTML) > maxBodyBytes {
		return nil, fmt.Errorf("%w: the message is too large", ErrInvalid)
	}

	// Replying into an existing thread needs the right to see it.
	var threadID uuid.UUID
	if opts.ThreadID != nil {
		ok, err := s.store.ThreadVisibleTo(ctx, actor.UserID, *opts.ThreadID)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, ErrNotFound
		}
		threadID = *opts.ThreadID
	}

	messageUUID, err := uuid.NewV7()
	if err != nil {
		return nil, fmt.Errorf("email: message id: %w", err)
	}
	now := s.now()
	messageID := "<" + messageUUID.String() + "@" + domainOf(from) + ">"

	raw, err := buildMIME(outboundMail{
		From: from, To: to, Cc: cc, Subject: subject,
		Text: opts.BodyText, HTML: opts.BodyHTML,
		MessageID: messageID, InReplyTo: headerValue(opts.InReplyTo), References: headerValue(opts.MsgReferences),
		Date: now,
	})
	if err != nil {
		return nil, err
	}

	rcpts := append(append(append([]string{}, to...), cc...), bcc...)
	sendCtx, cancel := context.WithTimeout(ctx, deliverTimout)
	defer cancel()
	sendErr := s.mailer.SendRaw(sendCtx, from, rcpts, raw)

	msg := &Message{
		ID: messageUUID, Direction: DirectionOutbound,
		FromAddress: from, ToAddresses: to, CCAddresses: cc, BCCAddresses: bcc,
		Subject: subject, BodyText: opts.BodyText, BodyHTML: opts.BodyHTML,
		MessageID: messageID, InReplyTo: headerValue(opts.InReplyTo), MsgReferences: headerValue(opts.MsgReferences),
		CreatedAt: now,
	}
	if sendErr != nil {
		msg.Status = StatusFailed
		msg.ErrorMessage = truncate(sendErr.Error(), 300)
		s.logger.Error("email: delivery failed", slog.String("message_id", messageID), slog.String("error", sendErr.Error()))
	} else {
		msg.Status = StatusSent
		msg.SentAt = &now
		msg.ProviderRef = messageID
	}

	if threadID == uuid.Nil {
		threadID, err = s.store.CreateThread(ctx, subject)
		if err != nil {
			return nil, s.afterSend(sendErr, err, messageID)
		}
	}
	msg.ThreadID = threadID
	if err := s.store.CreateMessage(ctx, s.store.DB(), msg); err != nil {
		return nil, s.afterSend(sendErr, err, messageID)
	}

	if sendErr != nil {
		return msg, fmt.Errorf("%w: the mail server did not accept the message; check the recipient addresses and try again", ErrDelivery)
	}
	return msg, nil
}

// afterSend reports a failure to record the message. When the mail was already
// accepted the user must hear that it went out even though history was not saved.
func (s *Service) afterSend(sendErr, storeErr error, messageID string) error {
	s.logger.Error("email: could not record message", slog.String("message_id", messageID), slog.String("error", storeErr.Error()))
	if sendErr == nil {
		return fmt.Errorf("%w: the message was sent but could not be saved to your mailbox; do not send it again", ErrDelivery)
	}
	return fmt.Errorf("%w: the mail server did not accept the message; check the recipient addresses and try again", ErrDelivery)
}

// InboundMessage is a parsed inbound email from the provider webhook.
type InboundMessage struct {
	MessageID     string
	InReplyTo     string
	MsgReferences string
	FromAddress   string
	ToAddresses   []string
	CCAddresses   []string
	Subject       string
	BodyText      string
	BodyHTML      string
	ReceivedAt    time.Time
	Attachments   []InboundAttachment
}

// InboundAttachment is an attachment from an inbound email.
type InboundAttachment struct {
	Filename    string
	ContentType string
	SizeBytes   int64
	StorageKey  string
}

// Receive stores an inbound email from the provider webhook.
func (s *Service) Receive(ctx context.Context, m InboundMessage) (*Message, error) {
	if m.MessageID == "" {
		return nil, ErrInvalid
	}

	// Deduplicate by message ID.
	if _, err := s.store.GetMessageByProviderRef(ctx, m.MessageID); err == nil {
		return nil, nil // already received
	} else if !errors.Is(err, ErrNotFound) {
		return nil, err
	}

	// Find or create the thread by References / In-Reply-To.
	threadID := uuid.Nil
	if m.InReplyTo != "" {
		if parent, err := s.store.GetThreadByMessageID(ctx, m.InReplyTo); err == nil {
			threadID = parent.ID
		}
	}
	if threadID == uuid.Nil {
		var err error
		threadID, err = s.store.CreateThread(ctx, m.Subject)
		if err != nil {
			return nil, err
		}
	}

	now := s.now()
	msg := &Message{
		ID:            uuid.Nil,
		ThreadID:      threadID,
		Direction:     DirectionInbound,
		FromAddress:   m.FromAddress,
		ToAddresses:   m.ToAddresses,
		CCAddresses:   m.CCAddresses,
		Subject:       m.Subject,
		BodyText:      m.BodyText,
		BodyHTML:      m.BodyHTML,
		MessageID:     m.MessageID,
		InReplyTo:     m.InReplyTo,
		MsgReferences: m.MsgReferences,
		ReceivedAt:    &now,
		Status:        StatusReceived,
		CreatedAt:     now,
	}
	msg.ID, _ = uuid.NewV7()

	tx, err := s.store.DB().Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("email: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := s.store.CreateMessage(ctx, tx, msg); err != nil {
		return nil, err
	}
	for _, att := range m.Attachments {
		a := &Attachment{
			ID:          uuid.Nil,
			MessageID:   msg.ID,
			Filename:    att.Filename,
			ContentType: att.ContentType,
			SizeBytes:   att.SizeBytes,
			StorageKey:  att.StorageKey,
			CreatedAt:   now,
		}
		a.ID, _ = uuid.NewV7()
		if err := s.store.CreateAttachment(ctx, tx, a); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("email: commit tx: %w", err)
	}

	// Emit an outbox event for realtime inbox updates.
	if s.outbox != nil {
		ev, err := outboxEvent(msg.ID, threadID, m.FromAddress, m.ToAddresses, m.Subject)
		if err == nil {
			if err := s.outbox(ctx, ev); err != nil {
				s.logger.Warn("email: outbox event failed", slog.String("error", err.Error()))
			}
		}
	}

	return msg, nil
}

// outboxEvent builds an outbox event for an email message.
func outboxEvent(messageID, threadID uuid.UUID, fromAddress string, toAddresses []string, subject string) (outbox.Event, error) {
	payload := map[string]any{
		"message_id":   messageID,
		"thread_id":    threadID,
		"from_address": fromAddress,
		"to_addresses": toAddresses,
		"subject":      subject,
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return outbox.Event{}, fmt.Errorf("email: marshal outbox payload: %w", err)
	}
	return outbox.Event{
		Type:          "core.email.message.v1",
		AggregateType: "email.message",
		AggregateID:   messageID,
		Payload:       b,
	}, nil
}
