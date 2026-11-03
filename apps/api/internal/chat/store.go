// Package chat serves Atlas's messaging platform (§8.5).
//
// Conversations and messages are stored durably so history persists.
// Realtime delivery runs through the existing orchestrator WebSocket
// (/socket); this schema is the durable read/write model.
package chat

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Sentinel errors.
var (
	ErrNotFound  = errors.New("chat: not found")
	ErrInvalid   = errors.New("chat: invalid")
	ErrForbidden = errors.New("chat: forbidden")
)

// Conversation is a chat thread (direct or group).
type Conversation struct {
	ID         uuid.UUID `json:"id"`
	Type       string    `json:"type"` // "direct" or "group"
	Name       string    `json:"name,omitempty"`
	CreatedBy  uuid.UUID `json:"created_by"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// Message is one chat message.
type Message struct {
	ID             uuid.UUID `json:"id"`
	ConversationID uuid.UUID `json:"conversation_id"`
	SenderID       uuid.UUID `json:"sender_id"`
	Body           string    `json:"body"`
	BodyType       string    `json:"body_type"` // "text", "file", "system"
	ReplyToMessage *uuid.UUID `json:"reply_to_message,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	EditedAt       *time.Time `json:"edited_at,omitempty"`
	DeletedAt      *time.Time `json:"deleted_at,omitempty"`
}

// MessageView is a message shaped for the client, with sender info.
type MessageView struct {
	ID             uuid.UUID `json:"id"`
	ConversationID uuid.UUID `json:"conversation_id"`
	SenderID       uuid.UUID `json:"sender_id"`
	SenderName     string    `json:"sender_name,omitempty"`
	Body           string    `json:"body"`
	BodyType       string    `json:"body_type"`
	ReplyToMessage *uuid.UUID `json:"reply_to_message,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	EditedAt       *time.Time `json:"edited_at,omitempty"`
}

// ConversationView is a conversation with last message preview.
type ConversationView struct {
	ID             uuid.UUID   `json:"id"`
	Type           string      `json:"type"`
	Name           string      `json:"name,omitempty"`
	CreatedBy      uuid.UUID   `json:"created_by"`
	CreatedAt      time.Time   `json:"created_at"`
	UpdatedAt      time.Time   `json:"updated_at"`
	LastMessage    *MessageView `json:"last_message,omitempty"`
	UnreadCount    int         `json:"unread_count"`
}

// Store is the persistence interface for chat.
type Store interface {
	// DB returns the underlying connection pool.
	DB() *pgxpool.Pool

	// Conversations.
	GetConversation(ctx context.Context, id uuid.UUID) (*Conversation, error)
	ListConversations(ctx context.Context, userID uuid.UUID, limit int, before *uuid.UUID) ([]ConversationView, error)
	CreateConversation(ctx context.Context, ex Execer, conv *Conversation) error
	AddParticipant(ctx context.Context, ex Execer, convID, userID uuid.UUID) error
	GetParticipants(ctx context.Context, convID uuid.UUID) ([]uuid.UUID, error)
	IsParticipant(ctx context.Context, convID, userID uuid.UUID) (bool, error)

	// Messages.
	CreateMessage(ctx context.Context, ex Execer, m *Message) error
	GetMessage(ctx context.Context, id uuid.UUID) (*Message, error)
	ListMessages(ctx context.Context, convID uuid.UUID, limit int, before *uuid.UUID) ([]MessageView, error)
	UpdateMessage(ctx context.Context, ex Execer, id uuid.UUID, body string) error
	DeleteMessage(ctx context.Context, ex Execer, id uuid.UUID) error

	// Attachments.
	CreateMessageAttachment(ctx context.Context, ex Execer, a *MessageAttachment) error
}

// MessageAttachment is a file attached to a chat message.
type MessageAttachment struct {
	ID             uuid.UUID `json:"id"`
	MessageID      uuid.UUID `json:"message_id"`
	DocumentID     uuid.UUID `json:"document_id,omitempty"`
	Filename       string    `json:"filename"`
	ContentType    string    `json:"content_type"`
	SizeBytes      int64     `json:"size_bytes"`
	CreatedAt      time.Time `json:"created_at"`
}

// Execer is the transaction interface used by the store.
type Execer interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Row is the row scanner interface.
type Row interface {
	Scan(dest ...any) error
}

// PGStore is the Postgres-backed chat store.
type PGStore struct {
	db *pgxpool.Pool
}

// NewPGStore builds a store over the pool.
func NewPGStore(db *pgxpool.Pool) *PGStore { return &PGStore{db: db} }

// DB returns the underlying connection pool.
func (s *PGStore) DB() *pgxpool.Pool { return s.db }

// ── conversations ──────────────────────────────────────────────────────────

// GetConversation returns a conversation by ID.
func (s *PGStore) GetConversation(ctx context.Context, id uuid.UUID) (*Conversation, error) {
	var c Conversation
	if err := s.db.QueryRow(ctx, `
		SELECT id, type, name, created_by, created_at, updated_at
		FROM core.chat_conversations WHERE id = $1`, id).Scan(
		&c.ID, &c.Type, &c.Name, &c.CreatedBy, &c.CreatedAt, &c.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("chat: get conversation: %w", err)
	}
	return &c, nil
}

// ListConversations returns a user's conversations newest first.
func (s *PGStore) ListConversations(ctx context.Context, userID uuid.UUID, limit int, before *uuid.UUID) ([]ConversationView, error) {
	args := []any{userID}
	where := "cp.user_id = $1"
	if before != nil {
		args = append(args, *before)
		where += fmt.Sprintf(" AND c.updated_at < (SELECT updated_at FROM core.chat_conversations WHERE id = $%d)", len(args))
	}
	args = append(args, limit)
	rows, err := s.db.Query(ctx, `
		SELECT c.id, c.type, c.name, c.created_by, c.created_at, c.updated_at,
		       lm.id, lm.sender_id, lm.body, lm.body_type, lm.created_at,
		       p.full_name,
		       (SELECT COUNT(*) FROM core.chat_messages m2
		        WHERE m2.conversation_id = c.id
		          AND m2.created_at > COALESCE(cp.last_read_at, '1970-01-01')) AS unread_count
		FROM core.chat_conversations c
		JOIN core.chat_participants cp ON cp.conversation_id = c.id AND cp.user_id = $1
		JOIN LATERAL (
			SELECT id, sender_id, body, body_type, created_at
			FROM core.chat_messages m3
			WHERE m3.conversation_id = c.id
			ORDER BY created_at DESC LIMIT 1
		) lm ON TRUE
		JOIN LATERAL (
			SELECT full_name FROM core.user_accounts ua
			JOIN people.persons p ON p.id = ua.person_id
			WHERE ua.id = lm.sender_id
		) p ON TRUE
		WHERE ` + where + `
		ORDER BY c.updated_at DESC
		LIMIT $%d`, args...)
	if err != nil {
		return nil, fmt.Errorf("chat: list conversations: %w", err)
	}
	defer rows.Close()

	var conversations []ConversationView
	for rows.Next() {
		var c ConversationView
		var lmID *uuid.UUID
		var lmSenderID *uuid.UUID
		var lmBody *string
		var lmBodyType *string
		var lmCreated *time.Time
		var lmSenderName *string
		if err := rows.Scan(
			&c.ID, &c.Type, &c.Name, &c.CreatedBy, &c.CreatedAt, &c.UpdatedAt,
			&lmID, &lmSenderID, &lmBody, &lmBodyType, &lmCreated,
			&lmSenderName, &c.UnreadCount,
		); err != nil {
			return nil, fmt.Errorf("chat: scan conversation: %w", err)
		}
		if lmID != nil {
			c.LastMessage = &MessageView{
				ID:             *lmID,
				ConversationID: c.ID,
				SenderID:       *lmSenderID,
				Body:           *lmBody,
				BodyType:       *lmBodyType,
				CreatedAt:      *lmCreated,
				SenderName:     *lmSenderName,
			}
		}
		conversations = append(conversations, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("chat: conversations: %w", err)
	}
	return conversations, nil
}

// CreateConversation inserts a new conversation.
func (s *PGStore) CreateConversation(ctx context.Context, ex Execer, conv *Conversation) error {
	if _, err := ex.Exec(ctx, `
		INSERT INTO core.chat_conversations (id, type, name, created_by)
		VALUES ($1, $2, $3, $4)`, conv.ID, conv.Type, conv.Name, conv.CreatedBy); err != nil {
		return fmt.Errorf("chat: create conversation: %w", err)
	}
	return nil
}

// AddParticipant adds a user to a conversation.
func (s *PGStore) AddParticipant(ctx context.Context, ex Execer, convID, userID uuid.UUID) error {
	if _, err := ex.Exec(ctx, `
		INSERT INTO core.chat_participants (conversation_id, user_id)
		VALUES ($1, $2)
		ON CONFLICT DO NOTHING`, convID, userID); err != nil {
		return fmt.Errorf("chat: add participant: %w", err)
	}
	return nil
}

// GetParticipants returns all participant IDs for a conversation.
func (s *PGStore) GetParticipants(ctx context.Context, convID uuid.UUID) ([]uuid.UUID, error) {
	rows, err := s.db.Query(ctx, `
		SELECT user_id FROM core.chat_participants WHERE conversation_id = $1`, convID)
	if err != nil {
		return nil, fmt.Errorf("chat: get participants: %w", err)
	}
	defer rows.Close()

	var participants []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("chat: scan participant: %w", err)
		}
		participants = append(participants, id)
	}
	return participants, nil
}

// IsParticipant checks if a user is in a conversation.
func (s *PGStore) IsParticipant(ctx context.Context, convID, userID uuid.UUID) (bool, error) {
	var exists bool
	if err := s.db.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM core.chat_participants
			WHERE conversation_id = $1 AND user_id = $2)`, convID, userID).Scan(&exists); err != nil {
		return false, fmt.Errorf("chat: is participant: %w", err)
	}
	return exists, nil
}

// ── messages ───────────────────────────────────────────────────────────────

// CreateMessage inserts a new message.
func (s *PGStore) CreateMessage(ctx context.Context, ex Execer, m *Message) error {
	if _, err := ex.Exec(ctx, `
		INSERT INTO core.chat_messages
		(id, conversation_id, sender_id, body, body_type, reply_to_message, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7)`,
		m.ID, m.ConversationID, m.SenderID, m.Body, m.BodyType, m.ReplyToMessage, m.CreatedAt); err != nil {
		return fmt.Errorf("chat: create message: %w", err)
	}
	return nil
}

// GetMessage returns a message by ID.
func (s *PGStore) GetMessage(ctx context.Context, id uuid.UUID) (*Message, error) {
	var m Message
	var editedAt, deletedAt *time.Time
	if err := s.db.QueryRow(ctx, `
		SELECT id, conversation_id, sender_id, body, body_type, reply_to_message,
		       created_at, edited_at, deleted_at
		FROM core.chat_messages WHERE id = $1`, id).Scan(
		&m.ID, &m.ConversationID, &m.SenderID, &m.Body, &m.BodyType, &m.ReplyToMessage,
		&m.CreatedAt, &editedAt, &deletedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("chat: get message: %w", err)
	}
	m.EditedAt = editedAt
	m.DeletedAt = deletedAt
	return &m, nil
}

// ListMessages returns messages for a conversation newest first.
func (s *PGStore) ListMessages(ctx context.Context, convID uuid.UUID, limit int, before *uuid.UUID) ([]MessageView, error) {
	args := []any{convID}
	where := "m.conversation_id = $1 AND m.deleted_at IS NULL"
	if before != nil {
		args = append(args, *before)
		where += fmt.Sprintf(" AND m.created_at < $%d", len(args))
	}
	args = append(args, limit)
	rows, err := s.db.Query(ctx, `
		SELECT m.id, m.sender_id, m.body, m.body_type, m.created_at, m.edited_at,
		       p.full_name
		FROM core.chat_messages m
		JOIN core.user_accounts ua ON ua.id = m.sender_id
		JOIN people.persons p ON p.id = ua.person_id
		WHERE ` + where + `
		ORDER BY m.created_at DESC
		LIMIT $%d`, args...)
	if err != nil {
		return nil, fmt.Errorf("chat: list messages: %w", err)
	}
	defer rows.Close()

	var messages []MessageView
	for rows.Next() {
		var m MessageView
		var editedAt *time.Time
		if err := rows.Scan(
			&m.ID, &m.SenderID, &m.Body, &m.BodyType, &m.CreatedAt, &editedAt,
			&m.SenderName,
		); err != nil {
			return nil, fmt.Errorf("chat: scan message: %w", err)
		}
		m.ConversationID = convID
		m.EditedAt = editedAt
		messages = append(messages, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("chat: messages: %w", err)
	}
	return messages, nil
}

// UpdateMessage edits a message body.
func (s *PGStore) UpdateMessage(ctx context.Context, ex Execer, id uuid.UUID, body string) error {
	now := time.Now().UTC()
	if _, err := ex.Exec(ctx, `
		UPDATE core.chat_messages
		SET body = $2, edited_at = $3
		WHERE id = $1`, id, body, now); err != nil {
		return fmt.Errorf("chat: update message: %w", err)
	}
	return nil
}

// DeleteMessage soft-deletes a message.
func (s *PGStore) DeleteMessage(ctx context.Context, ex Execer, id uuid.UUID) error {
	now := time.Now().UTC()
	if _, err := ex.Exec(ctx, `
		UPDATE core.chat_messages
		SET deleted_at = $2
		WHERE id = $1`, id, now); err != nil {
		return fmt.Errorf("chat: delete message: %w", err)
	}
	return nil
}

// ── attachments ────────────────────────────────────────────────────────────

// CreateMessageAttachment inserts a message attachment.
func (s *PGStore) CreateMessageAttachment(ctx context.Context, ex Execer, a *MessageAttachment) error {
	if _, err := ex.Exec(ctx, `
		INSERT INTO core.chat_message_attachments
		(id, message_id, document_id, filename, content_type, size_bytes, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7)`,
		a.ID, a.MessageID, a.DocumentID, a.Filename, a.ContentType, a.SizeBytes, a.CreatedAt); err != nil {
		return fmt.Errorf("chat: create attachment: %w", err)
	}
	return nil
}

// Compile-time check that PGStore implements Store.
var _ Store = (*PGStore)(nil)
