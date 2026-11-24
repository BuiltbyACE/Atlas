// Package email serves Atlas's two-way email pipeline (§8.4).
//
// Every message sent or received through the system is stored durably as a
// message record grouped into threads. Outbound goes through the SMTP adapter
// already used for invitation emails; inbound arrives through a webhook from
// the inbound mail provider (§6.2).
package email

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Sentinel errors. The HTTP layer maps these onto problem responses.
var (
	ErrNotFound  = errors.New("email: not found")
	ErrInvalid   = errors.New("email: invalid")
	ErrForbidden = errors.New("email: forbidden")
)

// MessageDirection is the direction of an email message.
type MessageDirection string

const (
	DirectionInbound  MessageDirection = "inbound"
	DirectionOutbound MessageDirection = "outbound"
)

// MessageStatus is the delivery status of a message.
type MessageStatus string

const (
	StatusReceived  MessageStatus = "received"
	StatusSent      MessageStatus = "sent"
	StatusDelivered MessageStatus = "delivered"
	StatusFailed    MessageStatus = "failed"
)

// Thread is a group of related email messages.
type Thread struct {
	ID        uuid.UUID `json:"id"`
	Subject   string    `json:"subject"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Message is one email message, inbound or outbound.
type Message struct {
	ID            uuid.UUID        `json:"id"`
	ThreadID      uuid.UUID        `json:"thread_id"`
	Direction     MessageDirection `json:"direction"`
	FromAddress   string           `json:"from_address"`
	ToAddresses   []string         `json:"to_addresses"`
	CCAddresses   []string         `json:"cc_addresses"`
	BCCAddresses  []string         `json:"bcc_addresses"`
	Subject       string           `json:"subject"`
	BodyText      string           `json:"body_text"`
	BodyHTML      string           `json:"body_html"`
	MessageID     string           `json:"message_id"`
	InReplyTo     string           `json:"in_reply_to,omitempty"`
	MsgReferences string           `json:"msg_references,omitempty"`
	ReceivedAt    *time.Time       `json:"received_at,omitempty"`
	SentAt        *time.Time       `json:"sent_at,omitempty"`
	Status        MessageStatus    `json:"status"`
	ProviderRef   string           `json:"provider_ref,omitempty"`
	ErrorMessage  string           `json:"error_message,omitempty"`
	CreatedAt     time.Time        `json:"created_at"`
}

// Attachment is a file attached to an email message.
type Attachment struct {
	ID          uuid.UUID `json:"id"`
	MessageID   uuid.UUID `json:"message_id"`
	Filename    string    `json:"filename"`
	ContentType string    `json:"content_type"`
	SizeBytes   int64     `json:"size_bytes"`
	StorageKey  string    `json:"storage_key"`
	CreatedAt   time.Time `json:"created_at"`
}

// Address is a user's verified "send as" email identity.
type Address struct {
	ID        uuid.UUID `json:"id"`
	UserID    uuid.UUID `json:"user_id"`
	Address   string    `json:"address"`
	Verified  bool      `json:"verified"`
	IsPrimary bool      `json:"is_primary"`
	CreatedAt time.Time `json:"created_at"`
}

// ThreadListItem is a thread summary for the inbox list view.
type ThreadListItem struct {
	ID           uuid.UUID    `json:"id"`
	Subject      string       `json:"subject"`
	UpdatedAt    time.Time    `json:"updated_at"`
	MessageCount int          `json:"message_count"`
	UnreadCount  int          `json:"unread_count"`
	Latest       *MessageView `json:"latest,omitempty"`
}

// MessageView is a message shaped for the conversation view.
type MessageView struct {
	ID          uuid.UUID        `json:"id"`
	ThreadID    uuid.UUID        `json:"thread_id"`
	Direction   MessageDirection `json:"direction"`
	FromAddress string           `json:"from_address"`
	ToAddresses []string         `json:"to_addresses"`
	CCAddresses []string         `json:"cc_addresses"`
	Subject     string           `json:"subject"`
	BodyText    string           `json:"body_text"`
	BodyHTML    string           `json:"body_html"`
	SentAt      *time.Time       `json:"sent_at,omitempty"`
	ReceivedAt  *time.Time       `json:"received_at,omitempty"`
	Status      MessageStatus    `json:"status"`
	Attachments []Attachment     `json:"attachments"`
	CreatedAt   time.Time        `json:"created_at"`
}

// Store is the persistence interface for email.
type Store interface {
	// DB returns the underlying connection pool, for callers that need to
	// begin transactions.
	DB() *pgxpool.Pool

	// Threads.
	GetThread(ctx context.Context, id uuid.UUID) (*Thread, error)
	ListThreads(ctx context.Context, userID uuid.UUID, limit int, before *uuid.UUID) ([]ThreadListItem, error)
	GetThreadWithMessages(ctx context.Context, userID, id uuid.UUID, limit int, before *uuid.UUID) (*Thread, []MessageView, error)

	// Messages.
	CreateThread(ctx context.Context, subject string) (uuid.UUID, error)
	CreateMessage(ctx context.Context, ex Execer, m *Message) error
	GetMessage(ctx context.Context, id uuid.UUID) (*Message, error)
	GetMessageByProviderRef(ctx context.Context, messageID string) (*Message, error)
	GetThreadByMessageID(ctx context.Context, messageID string) (*Thread, error)
	UpdateMessageStatus(ctx context.Context, ex Execer, id uuid.UUID, status MessageStatus, providerRef string, errorMessage string) error
	GetMessageAttachments(ctx context.Context, messageID uuid.UUID) ([]Attachment, error)

	// Addresses.
	ListAddresses(ctx context.Context, userID uuid.UUID) ([]Address, error)
	GetAddress(ctx context.Context, id uuid.UUID) (*Address, error)
	CreateAddress(ctx context.Context, ex Execer, userID uuid.UUID, address string, isPrimary bool) (*Address, error)
	// EnsureLoginAddress makes the account holder's login email a verified
	// send-as address, so every user can read and send mail from day one.
	EnsureLoginAddress(ctx context.Context, userID uuid.UUID, loginEmail string) error
	SetPrimaryAddress(ctx context.Context, ex Execer, userID uuid.UUID, addressID uuid.UUID) error

	// Attachments.
	CreateAttachment(ctx context.Context, ex Execer, a *Attachment) error
}

// Execer is the transaction interface used by the store.
// pgxpool.Pool and pgx.Tx both satisfy it.
type Execer interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// PGStore is the Postgres-backed email store.
type PGStore struct {
	db *pgxpool.Pool
}

// NewPGStore builds a store over the pool.
func NewPGStore(db *pgxpool.Pool) *PGStore { return &PGStore{db: db} }

// DB returns the underlying connection pool.
func (s *PGStore) DB() *pgxpool.Pool { return s.db }

// ── threads ────────────────────────────────────────────────────────────────

// GetThread returns a thread by ID.
func (s *PGStore) GetThread(ctx context.Context, id uuid.UUID) (*Thread, error) {
	var t Thread
	err := s.db.QueryRow(ctx, `
		SELECT id, subject, created_at, updated_at
		FROM core.email_threads
		WHERE id = $1`, id).Scan(
		&t.ID, &t.Subject, &t.CreatedAt, &t.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("email: get thread: %w", err)
	}
	return &t, nil
}

// ── messages ───────────────────────────────────────────────────────────────

// CreateThread inserts a new empty thread.
func (s *PGStore) CreateThread(ctx context.Context, subject string) (uuid.UUID, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return uuid.Nil, fmt.Errorf("email: thread id: %w", err)
	}
	if _, err := s.db.Exec(ctx, `
		INSERT INTO core.email_threads (id, subject)
		VALUES ($1, $2)`, id, subject); err != nil {
		return uuid.Nil, fmt.Errorf("email: create thread: %w", err)
	}
	return id, nil
}

// CreateMessage inserts a new message inside a transaction.
func (s *PGStore) CreateMessage(ctx context.Context, ex Execer, m *Message) error {
	if _, err := ex.Exec(ctx, `
		INSERT INTO core.email_messages
		(id, thread_id, direction, from_address, to_addresses, cc_addresses, bcc_addresses,
		 subject, body_text, body_html, message_id, in_reply_to, msg_references,
		 received_at, sent_at, status, provider_ref, error_message, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19)`,
		m.ID, m.ThreadID, m.Direction, m.FromAddress, m.ToAddresses, m.CCAddresses, m.BCCAddresses,
		m.Subject, m.BodyText, m.BodyHTML, m.MessageID, m.InReplyTo, m.MsgReferences,
		m.ReceivedAt, m.SentAt, m.Status, m.ProviderRef, m.ErrorMessage, m.CreatedAt,
	); err != nil {
		return fmt.Errorf("email: create message: %w", err)
	}
	return nil
}

// GetMessage returns a message by ID.
func (s *PGStore) GetMessage(ctx context.Context, id uuid.UUID) (*Message, error) {
	var m Message
	var receivedAt, sentAt *time.Time
	if err := s.db.QueryRow(ctx, `
		SELECT id, thread_id, direction, from_address, to_addresses, cc_addresses, bcc_addresses,
		       subject, body_text, body_html, message_id, in_reply_to, msg_references,
		       received_at, sent_at, status, provider_ref, error_message, created_at
		FROM core.email_messages WHERE id = $1`, id).Scan(
		&m.ID, &m.ThreadID, &m.Direction, &m.FromAddress, &m.ToAddresses, &m.CCAddresses, &m.BCCAddresses,
		&m.Subject, &m.BodyText, &m.BodyHTML, &m.MessageID, &m.InReplyTo, &m.MsgReferences,
		&receivedAt, &sentAt, &m.Status, &m.ProviderRef, &m.ErrorMessage, &m.CreatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("email: get message: %w", err)
	}
	m.ReceivedAt = receivedAt
	m.SentAt = sentAt
	return &m, nil
}

// GetMessageByProviderRef returns a message by its provider message ID.
func (s *PGStore) GetMessageByProviderRef(ctx context.Context, messageID string) (*Message, error) {
	var m Message
	var receivedAt, sentAt *time.Time
	if err := s.db.QueryRow(ctx, `
		SELECT id, thread_id, direction, from_address, to_addresses, cc_addresses, bcc_addresses,
		       subject, body_text, body_html, message_id, in_reply_to, msg_references,
		       received_at, sent_at, status, provider_ref, error_message, created_at
		FROM core.email_messages WHERE message_id = $1`, messageID).Scan(
		&m.ID, &m.ThreadID, &m.Direction, &m.FromAddress, &m.ToAddresses, &m.CCAddresses, &m.BCCAddresses,
		&m.Subject, &m.BodyText, &m.BodyHTML, &m.MessageID, &m.InReplyTo, &m.MsgReferences,
		&receivedAt, &sentAt, &m.Status, &m.ProviderRef, &m.ErrorMessage, &m.CreatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("email: get message by ref: %w", err)
	}
	m.ReceivedAt = receivedAt
	m.SentAt = sentAt
	return &m, nil
}

// GetThreadByMessageID returns the thread containing a message with the given provider message ID.
func (s *PGStore) GetThreadByMessageID(ctx context.Context, messageID string) (*Thread, error) {
	var t Thread
	if err := s.db.QueryRow(ctx, `
		SELECT t.id, t.subject, t.created_at, t.updated_at
		FROM core.email_threads t
		JOIN core.email_messages m ON m.thread_id = t.id
		WHERE m.message_id = $1`, messageID).Scan(
		&t.ID, &t.Subject, &t.CreatedAt, &t.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("email: get thread by message id: %w", err)
	}
	return &t, nil
}

// UpdateMessageStatus updates a message's delivery status.
func (s *PGStore) UpdateMessageStatus(ctx context.Context, ex Execer, id uuid.UUID, status MessageStatus, providerRef string, errorMessage string) error {
	if _, err := ex.Exec(ctx, `
		UPDATE core.email_messages
		SET status = $2, provider_ref = $3, error_message = $4
		WHERE id = $1`, id, status, providerRef, errorMessage); err != nil {
		return fmt.Errorf("email: update message status: %w", err)
	}
	return nil
}

// GetMessageAttachments returns attachments for a message.
func (s *PGStore) GetMessageAttachments(ctx context.Context, messageID uuid.UUID) ([]Attachment, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, message_id, filename, content_type, size_bytes, storage_key, created_at
		FROM core.email_attachments WHERE message_id = $1 ORDER BY created_at`, messageID)
	if err != nil {
		return nil, fmt.Errorf("email: get attachments: %w", err)
	}
	defer rows.Close()

	var attachments []Attachment
	for rows.Next() {
		var a Attachment
		if err := rows.Scan(&a.ID, &a.MessageID, &a.Filename, &a.ContentType, &a.SizeBytes, &a.StorageKey, &a.CreatedAt); err != nil {
			return nil, fmt.Errorf("email: scan attachment: %w", err)
		}
		attachments = append(attachments, a)
	}
	return attachments, nil
}

// ── addresses ──────────────────────────────────────────────────────────────

// ListAddresses returns a user's verified "send as" addresses.
func (s *PGStore) ListAddresses(ctx context.Context, userID uuid.UUID) ([]Address, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, user_id, address, verified, is_primary, created_at
		FROM core.email_addresses WHERE user_id = $1 ORDER BY is_primary DESC, address`, userID)
	if err != nil {
		return nil, fmt.Errorf("email: list addresses: %w", err)
	}
	defer rows.Close()

	var addresses = make([]Address, 0)
	for rows.Next() {
		var a Address
		if err := rows.Scan(&a.ID, &a.UserID, &a.Address, &a.Verified, &a.IsPrimary, &a.CreatedAt); err != nil {
			return nil, fmt.Errorf("email: scan address: %w", err)
		}
		addresses = append(addresses, a)
	}
	return addresses, nil
}

// GetAddress returns an address by ID.
func (s *PGStore) GetAddress(ctx context.Context, id uuid.UUID) (*Address, error) {
	var a Address
	if err := s.db.QueryRow(ctx, `
		SELECT id, user_id, address, verified, is_primary, created_at
		FROM core.email_addresses WHERE id = $1`, id).Scan(
		&a.ID, &a.UserID, &a.Address, &a.Verified, &a.IsPrimary, &a.CreatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("email: get address: %w", err)
	}
	return &a, nil
}

// CreateAddress inserts a new send-as address inside a transaction/execer.
func (s *PGStore) CreateAddress(ctx context.Context, ex Execer, userID uuid.UUID, address string, isPrimary bool) (*Address, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return nil, fmt.Errorf("email: address id: %w", err)
	}
	a := &Address{ID: id, UserID: userID, Address: strings.ToLower(strings.TrimSpace(address)), Verified: false, IsPrimary: isPrimary, CreatedAt: time.Now().UTC()}
	if _, err := ex.Exec(ctx, `
		INSERT INTO core.email_addresses (id, user_id, address, verified, is_primary, created_at)
		VALUES ($1,$2,$3,$4,$5,$6)`, a.ID, a.UserID, a.Address, a.Verified, a.IsPrimary, a.CreatedAt); err != nil {
		var pg *pgconn.PgError
		if errors.As(err, &pg) && pg.Code == "23505" {
			return nil, errDuplicateAddress
		}
		return nil, fmt.Errorf("email: create address: %w", err)
	}
	return a, nil
}

// SetPrimaryAddress marks one address as primary and unsets others.
func (s *PGStore) SetPrimaryAddress(ctx context.Context, ex Execer, userID uuid.UUID, addressID uuid.UUID) error {
	if _, err := ex.Exec(ctx, `UPDATE core.email_addresses SET is_primary = FALSE WHERE user_id = $1`, userID); err != nil {
		return fmt.Errorf("email: unset primary: %w", err)
	}
	if _, err := ex.Exec(ctx, `UPDATE core.email_addresses SET is_primary = TRUE WHERE id = $1 AND user_id = $2`, addressID, userID); err != nil {
		return fmt.Errorf("email: set primary: %w", err)
	}
	return nil
}

// ── attachments ────────────────────────────────────────────────────────────

// CreateAttachment inserts an attachment inside a transaction/execer.
func (s *PGStore) CreateAttachment(ctx context.Context, ex Execer, a *Attachment) error {
	if _, err := ex.Exec(ctx, `
		INSERT INTO core.email_attachments (id, message_id, filename, content_type, size_bytes, storage_key, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7)`,
		a.ID, a.MessageID, a.Filename, a.ContentType, a.SizeBytes, a.StorageKey, a.CreatedAt); err != nil {
		return fmt.Errorf("email: create attachment: %w", err)
	}
	return nil
}

// Compile-time check that PGStore implements Store.
var _ Store = (*PGStore)(nil)
