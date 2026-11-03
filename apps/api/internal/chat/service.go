// Package chat serves Atlas's messaging platform (§8.5).
package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/safaristack/atlas/apps/api/internal/outbox"
)

// Service is the chat service.
type Service struct {
	store  Store
	logger *slog.Logger
	now    func() time.Time

	outbox func(ctx context.Context, e outbox.Event) error
}

// NewService builds the chat service.
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

// WithOutbox wires the outbox event writer.
func (s *Service) WithOutbox(write func(ctx context.Context, e outbox.Event) error) *Service {
	s.outbox = write
	return s
}

// CreateConversationOptions is input for starting a conversation.
type CreateConversationOptions struct {
	Type        string   // "direct" or "group"
	Name        string   // for group conversations
	CreatedBy   uuid.UUID
	Participant []uuid.UUID // other participants (creator is auto-added)
}

// CreateConversation starts a 1:1 or group conversation.
func (s *Service) CreateConversation(ctx context.Context, opts CreateConversationOptions) (*Conversation, error) {
	if opts.Type != "direct" && opts.Type != "group" {
		return nil, ErrInvalid
	}
	if opts.Type == "direct" && len(opts.Participant) != 1 {
		return nil, ErrInvalid
	}

	convID, err := uuid.NewV7()
	if err != nil {
		return nil, fmt.Errorf("chat: conversation id: %w", err)
	}

	conv := &Conversation{
		ID:        convID,
		Type:      opts.Type,
		Name:      opts.Name,
		CreatedBy: opts.CreatedBy,
	}

	tx, err := s.store.DB().Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("chat: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := s.store.CreateConversation(ctx, tx, conv); err != nil {
		return nil, err
	}

	// Add creator and participants
	allParticipants := append([]uuid.UUID{opts.CreatedBy}, opts.Participant...)
	for _, p := range allParticipants {
		if err := s.store.AddParticipant(ctx, tx, convID, p); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("chat: commit tx: %w", err)
	}

	if s.outbox != nil {
		ev, err := outboxEvent("core.chat.conversation.created.v1", "chat.conversation", convID,
			map[string]any{"conversation_id": convID, "type": opts.Type, "created_by": opts.CreatedBy})
		if err == nil {
			if err := s.outbox(ctx, ev); err != nil {
				s.logger.Warn("chat: outbox event failed", slog.String("error", err.Error()))
			}
		}
	}

	return conv, nil
}

// SendMessageOptions is input for sending a chat message.
type SendMessageOptions struct {
	ConversationID uuid.UUID
	SenderID       uuid.UUID
	Body           string
	BodyType       string // "text", "file", "system"
	ReplyToMessage *uuid.UUID
	Attachments    []MessageAttachmentInput
}

// MessageAttachmentInput describes a file attached to a message.
type MessageAttachmentInput struct {
	DocumentID   *uuid.UUID
	Filename     string
	ContentType  string
	SizeBytes    int64
}

// SendMessage sends a message in a conversation.
func (s *Service) SendMessage(ctx context.Context, opts SendMessageOptions) (*Message, error) {
	if opts.Body == "" && len(opts.Attachments) == 0 {
		return nil, ErrInvalid
	}

	// Verify sender is a participant
	ok, err := s.store.IsParticipant(ctx, opts.ConversationID, opts.SenderID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrForbidden
	}

	msgID, err := uuid.NewV7()
	if err != nil {
		return nil, fmt.Errorf("chat: message id: %w", err)
	}

	now := s.now()
	msg := &Message{
		ID:             msgID,
		ConversationID: opts.ConversationID,
		SenderID:       opts.SenderID,
		Body:           opts.Body,
		BodyType:       opts.BodyType,
		ReplyToMessage: opts.ReplyToMessage,
		CreatedAt:      now,
	}

	tx, err := s.store.DB().Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("chat: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := s.store.CreateMessage(ctx, tx, msg); err != nil {
		return nil, err
	}
	for _, att := range opts.Attachments {
		a := &MessageAttachment{
			ID:          uuid.Nil,
			MessageID:   msgID,
			DocumentID:  *att.DocumentID,
			Filename:    att.Filename,
			ContentType: att.ContentType,
			SizeBytes:   att.SizeBytes,
			CreatedAt:   now,
		}
		a.ID, _ = uuid.NewV7()
		if err := s.store.CreateMessageAttachment(ctx, tx, a); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("chat: commit tx: %w", err)
	}

	if s.outbox != nil {
		ev, err := outboxEvent("core.chat.message.created.v1", "chat.message", msgID,
			map[string]any{"conversation_id": opts.ConversationID, "sender_id": opts.SenderID})
		if err == nil {
			if err := s.outbox(ctx, ev); err != nil {
				s.logger.Warn("chat: outbox event failed", slog.String("error", err.Error()))
			}
		}
	}

	return msg, nil
}

// ListConversations returns the user's conversations.
func (s *Service) ListConversations(ctx context.Context, userID uuid.UUID, limit int, before *uuid.UUID) ([]ConversationView, error) {
	return s.store.ListConversations(ctx, userID, limit, before)
}

// ListMessages returns messages for a conversation.
func (s *Service) ListMessages(ctx context.Context, userID uuid.UUID, convID uuid.UUID, limit int, before *uuid.UUID) ([]MessageView, error) {
	ok, err := s.store.IsParticipant(ctx, convID, userID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrForbidden
	}
	return s.store.ListMessages(ctx, convID, limit, before)
}

// EditMessage edits a message body (sender only).
func (s *Service) EditMessage(ctx context.Context, userID uuid.UUID, msgID uuid.UUID, body string) error {
	m, err := s.store.GetMessage(ctx, msgID)
	if err != nil {
		return err
	}
	if m.SenderID != userID {
		return ErrForbidden
	}
	return s.store.UpdateMessage(ctx, s.store.DB(), msgID, body)
}

// DeleteMessage soft-deletes a message (sender only).
func (s *Service) DeleteMessage(ctx context.Context, userID uuid.UUID, msgID uuid.UUID) error {
	m, err := s.store.GetMessage(ctx, msgID)
	if err != nil {
		return err
	}
	if m.SenderID != userID {
		return ErrForbidden
	}
	return s.store.DeleteMessage(ctx, s.store.DB(), msgID)
}

// outboxEvent builds an outbox event.
func outboxEvent(eventType, aggregateType string, aggregateID uuid.UUID, payload map[string]any) (outbox.Event, error) {
	b, err := jsonMarshal(payload)
	if err != nil {
		return outbox.Event{}, fmt.Errorf("chat: marshal outbox payload: %w", err)
	}
	return outbox.Event{
		Type:          eventType,
		AggregateType: aggregateType,
		AggregateID:   aggregateID,
		Payload:       b,
	}, nil
}

// jsonMarshal is a local variable so tests can stub it.
var jsonMarshal = func(v any) ([]byte, error) {
	return jsonMarshalStd(v)
}

var jsonMarshalStd = func(v any) ([]byte, error) {
	return json.Marshal(v)
}
