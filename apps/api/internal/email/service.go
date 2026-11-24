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
	"time"

	"github.com/google/uuid"

	"github.com/safaristack/atlas/apps/api/internal/outbox"
)

// Service is the email pipeline: send, receive, and address management.
type Service struct {
	store  Store
	logger *slog.Logger
	now    func() time.Time

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

// Send sends an outbound email. It creates the thread/message records and
// returns the message. The caller is responsible for SMTP delivery.
func (s *Service) Send(ctx context.Context, opts SendMessageOptions) (*Message, error) {
	if opts.FromAddress == "" {
		return nil, ErrInvalid
	}
	if len(opts.ToAddresses) == 0 {
		return nil, ErrInvalid
	}

	threadID := uuid.Nil
	if opts.ThreadID != nil {
		threadID = *opts.ThreadID
	} else {
		var err error
		threadID, err = s.store.CreateThread(ctx, opts.Subject)
		if err != nil {
			return nil, err
		}
	}

	messageID, err := uuid.NewV7()
	if err != nil {
		return nil, fmt.Errorf("email: message id: %w", err)
	}

	now := s.now()
	msg := &Message{
		ID:            messageID,
		ThreadID:      threadID,
		Direction:     DirectionOutbound,
		FromAddress:   opts.FromAddress,
		ToAddresses:   opts.ToAddresses,
		CCAddresses:   opts.CCAddresses,
		BCCAddresses:  opts.BCCAddresses,
		Subject:       opts.Subject,
		BodyText:      opts.BodyText,
		BodyHTML:      opts.BodyHTML,
		MessageID:     messageID.String(),
		InReplyTo:     opts.InReplyTo,
		MsgReferences: opts.MsgReferences,
		SentAt:        &now,
		Status:        StatusSent,
		CreatedAt:     now,
	}

	// Begin a transaction so thread + message + attachments commit together.
	tx, err := s.store.DB().Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("email: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := s.store.CreateMessage(ctx, tx, msg); err != nil {
		return nil, err
	}
	for _, att := range opts.Attachments {
		a := &Attachment{
			ID:          uuid.Nil,
			MessageID:   messageID,
			Filename:    att.Filename,
			ContentType: att.ContentType,
			SizeBytes:   att.SizeBytes,
			StorageKey:  att.StorageKey,
			CreatedAt:   now,
		}
		a.ID, err = uuid.NewV7()
		if err != nil {
			return nil, fmt.Errorf("email: attachment id: %w", err)
		}
		if err := s.store.CreateAttachment(ctx, tx, a); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("email: commit tx: %w", err)
	}

	// Emit an outbox event so realtime subscribers learn about the new email.
	if s.outbox != nil {
		ev, err := outboxEvent(messageID, threadID, opts.FromAddress, opts.ToAddresses, opts.Subject)
		if err == nil {
			if err := s.outbox(ctx, ev); err != nil {
				s.logger.Warn("email: outbox event failed", slog.String("error", err.Error()))
			}
		} else {
			s.logger.Warn("email: build outbox event failed", slog.String("error", err.Error()))
		}
	}

	return msg, nil
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
