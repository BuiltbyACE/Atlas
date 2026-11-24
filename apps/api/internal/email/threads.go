package email

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// visibleThreadSQL is the rule for who may see a thread: the caller must hold a
// verified address that appears on at least one of its messages, as the sender
// or as a To/Cc recipient. %s is the placeholder holding the caller's user id
// and the thread alias in scope must be "t".
//
// Bcc is deliberately not matched: a blind copy must not reveal the thread to
// anyone but its sender.
const visibleThreadSQL = `EXISTS (
	SELECT 1
	FROM core.email_addresses a
	JOIN core.email_messages vm ON vm.thread_id = t.id
	WHERE a.user_id = %s AND a.verified
	  AND (lower(vm.from_address) = lower(a.address)
	       OR lower(a.address) = ANY (
	            SELECT lower(x) FROM unnest(vm.to_addresses || vm.cc_addresses) AS x)))`

// buildListThreadsQuery returns the inbox query and how many arguments it
// expects, in order: user id, [cursor thread id], limit. Placeholders are built
// here, in one place, because a literal "$%d" left in a query string is sent to
// Postgres verbatim and fails every call (it did in production).
func buildListThreadsQuery(withCursor bool) (query string, argCount int) {
	argCount = 1
	where := fmt.Sprintf(visibleThreadSQL, "$1")
	if withCursor {
		argCount++
		where += fmt.Sprintf(
			" AND t.updated_at < (SELECT updated_at FROM core.email_threads WHERE id = $%d)", argCount)
	}
	argCount++
	query = `
		SELECT t.id, t.subject, t.updated_at,
		       COALESCE(mc.message_count, 0) AS message_count,
		       COALESCE(mc.unread_count, 0) AS unread_count
		FROM core.email_threads t
		JOIN LATERAL (
			SELECT COUNT(*) AS message_count,
			       COUNT(*) FILTER (WHERE status = 'received' AND received_at IS NOT NULL) AS unread_count
			FROM core.email_messages m WHERE m.thread_id = t.id
		) mc ON TRUE
		WHERE ` + where + `
		ORDER BY t.updated_at DESC
		LIMIT ` + fmt.Sprintf("$%d", argCount)
	return query, argCount
}

// buildThreadMessagesQuery returns the conversation query and its argument
// count, in order: thread id, [cursor created_at source message id], limit.
func buildThreadMessagesQuery(withCursor bool) (query string, argCount int) {
	argCount = 1
	where := "m.thread_id = $1"
	if withCursor {
		argCount++
		where += fmt.Sprintf(" AND m.created_at < (SELECT created_at FROM core.email_messages WHERE id = $%d)", argCount)
	}
	argCount++
	query = `
		SELECT m.id, m.direction, m.from_address, m.to_addresses, m.cc_addresses,
		       m.subject, m.body_text, m.body_html, m.sent_at, m.received_at,
		       m.status, m.created_at,
		       a.id, a.filename, a.content_type, a.size_bytes, a.storage_key
		FROM core.email_messages m
		LEFT JOIN core.email_attachments a ON a.message_id = m.id
		WHERE ` + where + `
		ORDER BY m.created_at DESC, m.id DESC, a.created_at
		LIMIT ` + fmt.Sprintf("$%d", argCount)
	return query, argCount
}

// ListThreads returns the caller's threads newest first, paginated by cursor.
func (s *PGStore) ListThreads(ctx context.Context, userID uuid.UUID, limit int, before *uuid.UUID) ([]ThreadListItem, error) {
	query, _ := buildListThreadsQuery(before != nil)
	args := []any{userID}
	if before != nil {
		args = append(args, *before)
	}
	args = append(args, limit)

	rows, err := s.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("email: list threads: %w", err)
	}
	defer rows.Close()

	threads := make([]ThreadListItem, 0)
	for rows.Next() {
		var t ThreadListItem
		if err := rows.Scan(&t.ID, &t.Subject, &t.UpdatedAt, &t.MessageCount, &t.UnreadCount); err != nil {
			return nil, fmt.Errorf("email: scan thread: %w", err)
		}
		threads = append(threads, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("email: threads: %w", err)
	}
	return threads, nil
}

// ThreadVisibleTo reports whether the user may see the thread.
func (s *PGStore) ThreadVisibleTo(ctx context.Context, userID, threadID uuid.UUID) (bool, error) {
	var ok bool
	err := s.db.QueryRow(ctx,
		`SELECT `+fmt.Sprintf(visibleThreadSQL, "$1")+`
		 FROM core.email_threads t WHERE t.id = $2`, userID, threadID).Scan(&ok)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, fmt.Errorf("email: thread visibility: %w", err)
	}
	return ok, nil
}

// GetThreadWithMessages loads a thread with its messages for the conversation
// view. A thread the caller cannot see is reported as not found, so its
// existence is not revealed.
func (s *PGStore) GetThreadWithMessages(ctx context.Context, userID, id uuid.UUID, limit int, before *uuid.UUID) (*Thread, []MessageView, error) {
	visible, err := s.ThreadVisibleTo(ctx, userID, id)
	if err != nil {
		return nil, nil, err
	}
	if !visible {
		return nil, nil, ErrNotFound
	}
	t, err := s.GetThread(ctx, id)
	if err != nil {
		return nil, nil, err
	}

	query, _ := buildThreadMessagesQuery(before != nil)
	args := []any{id}
	if before != nil {
		args = append(args, *before)
	}
	args = append(args, limit)

	rows, err := s.db.Query(ctx, query, args...)
	if err != nil {
		return nil, nil, fmt.Errorf("email: get thread messages: %w", err)
	}
	defer rows.Close()

	messages := make([]MessageView, 0)
	index := map[uuid.UUID]int{}
	for rows.Next() {
		var m MessageView
		var attID *uuid.UUID
		var attFilename, attContentType, attStorageKey *string
		var attSize *int64
		if err := rows.Scan(
			&m.ID, &m.Direction, &m.FromAddress, &m.ToAddresses, &m.CCAddresses,
			&m.Subject, &m.BodyText, &m.BodyHTML, &m.SentAt, &m.ReceivedAt,
			&m.Status, &m.CreatedAt,
			&attID, &attFilename, &attContentType, &attSize, &attStorageKey,
		); err != nil {
			return nil, nil, fmt.Errorf("email: scan message: %w", err)
		}
		// One row per attachment: fold them back onto their message.
		i, seen := index[m.ID]
		if !seen {
			m.ThreadID = id
			m.Attachments = []Attachment{}
			messages = append(messages, m)
			i = len(messages) - 1
			index[m.ID] = i
		}
		if attID != nil {
			messages[i].Attachments = append(messages[i].Attachments, Attachment{
				ID:          *attID,
				MessageID:   m.ID,
				Filename:    deref(attFilename),
				ContentType: deref(attContentType),
				SizeBytes:   derefInt(attSize),
				StorageKey:  deref(attStorageKey),
			})
		}
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("email: messages: %w", err)
	}
	return t, messages, nil
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func derefInt(n *int64) int64 {
	if n == nil {
		return 0
	}
	return *n
}

// EnsureLoginAddress records the account holder's login email as a verified
// send-as address. The login email is already proven by sign-in, so it needs no
// separate verification, and it belongs to its account whoever registered it
// first (an unverified claim by someone else is taken over).
func (s *PGStore) EnsureLoginAddress(ctx context.Context, userID uuid.UUID, loginEmail string) error {
	address := strings.ToLower(strings.TrimSpace(loginEmail))
	if address == "" {
		return nil
	}
	id, err := uuid.NewV7()
	if err != nil {
		return fmt.Errorf("email: address id: %w", err)
	}
	_, err = s.db.Exec(ctx, `
		INSERT INTO core.email_addresses (id, user_id, address, verified, is_primary)
		VALUES ($1, $2, $3, true,
		        NOT EXISTS (SELECT 1 FROM core.email_addresses WHERE user_id = $2 AND is_primary))
		ON CONFLICT (address) DO UPDATE
		   SET user_id = EXCLUDED.user_id, verified = true
		 WHERE core.email_addresses.user_id = EXCLUDED.user_id
		    OR core.email_addresses.verified = false`, id, userID, address)
	if err != nil {
		return fmt.Errorf("email: ensure login address: %w", err)
	}
	return nil
}

// errDuplicateAddress is returned when an address is already registered.
var errDuplicateAddress = errors.New("email: address already registered")
