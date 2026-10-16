package notifications

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/safaristack/atlas/apps/api/internal/identity"
	"github.com/safaristack/atlas/apps/api/internal/outbox"
)

// Cursor is a keyset position in a person's notification list: older than
// (At, ID). It is encoded opaquely for the wire by the HTTP layer.
type Cursor struct {
	At time.Time
	ID uuid.UUID
}

// Store is the persistence the notification engine needs. Service uses the
// read/write half; Dispatcher uses the queue half.
type Store interface {
	Recipient(ctx context.Context, personID uuid.UUID) (*Recipient, error)

	// AccountForPerson resolves a person to the user account a realtime nudge is
	// addressed to, or nil when the person has no account (§7.5). The engine
	// writes it into the requested event so the consumer can route the push.
	AccountForPerson(ctx context.Context, personID uuid.UUID) (*uuid.UUID, error)
	Preferences(ctx context.Context, personID uuid.UUID) (Preference, error)
	SavePreferences(ctx context.Context, personID uuid.UUID, p Preference) error

	// CreateWithEvent writes the notification rows and the outbox event in one
	// transaction. This is the entire write path and it never calls a provider.
	CreateWithEvent(ctx context.Context, ns []Notification, ev outbox.Event) error

	List(ctx context.Context, personID uuid.UUID, limit int, before *Cursor) ([]Notification, error)
	UnreadCount(ctx context.Context, personID uuid.UUID) (int, error)
	MarkRead(ctx context.Context, personID, id uuid.UUID) (bool, error)
	MarkAllRead(ctx context.Context, personID uuid.UUID) (int, error)

	// ClaimDue leases up to limit due notifications for one worker, oldest due
	// first, skipping rows another worker holds (FOR UPDATE SKIP LOCKED).
	ClaimDue(ctx context.Context, workerID string, lease time.Duration, limit int) ([]Notification, error)
	// Complete records a delivery outcome — an attempt row plus the new status —
	// and, when set, the matching event, in one transaction.
	Complete(ctx context.Context, n Notification, out Outcome, ev *outbox.Event) error
	// Defer pushes a claimed notification back without counting an attempt. It
	// is used when a provider's circuit breaker is open.
	Defer(ctx context.Context, id uuid.UUID, until time.Time) error
}

// PGStore is the Postgres-backed Store.
type PGStore struct{ db *pgxpool.Pool }

// NewPGStore builds a Store over a connection pool.
func NewPGStore(db *pgxpool.Pool) *PGStore { return &PGStore{db: db} }

// AccountForPerson resolves the person's account through the shared identity
// bridge, so the requested event carries a routable realtime address.
func (s *PGStore) AccountForPerson(ctx context.Context, personID uuid.UUID) (*uuid.UUID, error) {
	return identity.AccountForPerson(ctx, s.db, personID)
}

// notificationCols matches scanNotification, in order.
const notificationCols = `id, recipient_person_id, channel, COALESCE(template, ''),
	COALESCE(subject, ''), COALESCE(body, ''), payload, COALESCE(entity_type, ''),
	entity_id, status, attempts, max_attempts, COALESCE(last_error, ''), critical,
	COALESCE(idempotency_key, ''), deliver_after, created_at, sent_at, read_at`

func (s *PGStore) Recipient(ctx context.Context, personID uuid.UUID) (*Recipient, error) {
	var r Recipient
	err := s.db.QueryRow(ctx, `
		SELECT id, full_name, COALESCE(work_email::text, ''), status
		FROM people.persons WHERE id = $1`, personID).
		Scan(&r.PersonID, &r.FullName, &r.Email, &r.Status)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("notifications: recipient: %w", err)
	}
	return &r, nil
}

func (s *PGStore) Preferences(ctx context.Context, personID uuid.UUID) (Preference, error) {
	var (
		p          Preference
		start, end *int16
	)
	err := s.db.QueryRow(ctx, `
		SELECT person_id, in_app, email, whatsapp, sms,
		       quiet_start_minute, quiet_end_minute, timezone, updated_at
		FROM core.notification_preferences WHERE person_id = $1`, personID).
		Scan(&p.PersonID, &p.InApp, &p.Email, &p.WhatsApp, &p.SMS, &start, &end, &p.Timezone, &p.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return DefaultPreference(personID, defaultTimezone), nil
	}
	if err != nil {
		return Preference{}, fmt.Errorf("notifications: preferences: %w", err)
	}
	if start != nil {
		v := int(*start)
		p.QuietStart = &v
	}
	if end != nil {
		v := int(*end)
		p.QuietEnd = &v
	}
	return p, nil
}

func (s *PGStore) SavePreferences(ctx context.Context, personID uuid.UUID, p Preference) error {
	_, err := s.db.Exec(ctx, `
		INSERT INTO core.notification_preferences
		  (person_id, in_app, email, whatsapp, sms,
		   quiet_start_minute, quiet_end_minute, timezone)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (person_id) DO UPDATE SET
		  in_app = EXCLUDED.in_app,
		  email = EXCLUDED.email,
		  whatsapp = EXCLUDED.whatsapp,
		  sms = EXCLUDED.sms,
		  quiet_start_minute = EXCLUDED.quiet_start_minute,
		  quiet_end_minute = EXCLUDED.quiet_end_minute,
		  timezone = EXCLUDED.timezone`,
		personID, p.InApp, p.Email, p.WhatsApp, p.SMS,
		int16Ptr(p.QuietStart), int16Ptr(p.QuietEnd), p.Timezone)
	if err != nil {
		return fmt.Errorf("notifications: save preferences: %w", err)
	}
	return nil
}

// CreateWithEvent is the write path. It builds one multi-row INSERT (a single
// round trip regardless of channel count) and writes the outbox event through
// the same transaction, so the event and the rows commit together (ADR-005).
// ON CONFLICT DO NOTHING makes a repeated idempotency key a no-op (§8.10).
func (s *PGStore) CreateWithEvent(ctx context.Context, ns []Notification, ev outbox.Event) error {
	if len(ns) == 0 {
		return nil
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("notifications: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var (
		sb   strings.Builder
		args = make([]any, 0, len(ns)*17)
	)
	sb.WriteString(`INSERT INTO core.notifications (
		id, recipient_person_id, channel, template, subject, body, payload,
		entity_type, entity_id, status, attempts, max_attempts, critical,
		idempotency_key, deliver_after, sent_at, created_at) VALUES `)

	for i, n := range ns {
		if i > 0 {
			sb.WriteString(", ")
		}
		b := i * 17
		fmt.Fprintf(&sb, "($%d,$%d,$%d,$%d,$%d,$%d,$%d::jsonb,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d)",
			b+1, b+2, b+3, b+4, b+5, b+6, b+7, b+8, b+9, b+10, b+11, b+12, b+13, b+14, b+15, b+16, b+17)

		payload, err := marshalPayload(n.Payload)
		if err != nil {
			return err
		}
		args = append(args,
			n.ID, n.RecipientPersonID, string(n.Channel), nullString(n.Template),
			nullString(n.Subject), nullString(n.Body), payload, nullString(n.EntityType),
			uuidPtr(n.EntityID), string(n.Status), n.Attempts, n.MaxAttempts, n.Critical,
			nullString(n.IdempotencyKey), timePtr(n.DeliverAfter), timePtr(n.SentAt), n.CreatedAt)
	}

	sb.WriteString(` ON CONFLICT (idempotency_key, channel)
		WHERE idempotency_key IS NOT NULL DO NOTHING`)

	if _, err := tx.Exec(ctx, sb.String(), args...); err != nil {
		return fmt.Errorf("notifications: insert: %w", err)
	}
	if _, err := outbox.Write(ctx, tx, ev); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("notifications: commit: %w", err)
	}
	return nil
}

func (s *PGStore) List(ctx context.Context, personID uuid.UUID, limit int, before *Cursor) ([]Notification, error) {
	var (
		beforeTS *time.Time
		beforeID uuid.UUID = uuid.Nil
	)
	if before != nil {
		at := before.At.UTC()
		beforeTS = &at
		beforeID = before.ID
	}

	rows, err := s.db.Query(ctx, `
		SELECT `+notificationCols+`
		FROM core.notifications
		WHERE recipient_person_id = $1 AND channel = 'in_app'
		  AND ($2::timestamptz IS NULL OR (created_at, id) < ($2::timestamptz, $3::uuid))
		ORDER BY created_at DESC, id DESC
		LIMIT $4`, personID, timePtr(beforeTS), beforeID, limit)
	if err != nil {
		return nil, fmt.Errorf("notifications: list: %w", err)
	}
	defer rows.Close()

	out := make([]Notification, 0, limit)
	for rows.Next() {
		n, err := scanNotification(rows)
		if err != nil {
			return nil, fmt.Errorf("notifications: scan: %w", err)
		}
		out = append(out, n)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("notifications: list rows: %w", err)
	}
	return out, nil
}

func (s *PGStore) UnreadCount(ctx context.Context, personID uuid.UUID) (int, error) {
	var count int
	err := s.db.QueryRow(ctx, `
		SELECT count(*) FROM core.notifications
		WHERE recipient_person_id = $1 AND channel = 'in_app' AND read_at IS NULL`, personID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("notifications: unread count: %w", err)
	}
	return count, nil
}

func (s *PGStore) MarkRead(ctx context.Context, personID, id uuid.UUID) (bool, error) {
	tag, err := s.db.Exec(ctx, `
		UPDATE core.notifications
		SET read_at = COALESCE(read_at, now()),
		    status  = CASE WHEN status IN ('delivered', 'sent') THEN 'read' ELSE status END
		WHERE id = $1 AND recipient_person_id = $2 AND channel = 'in_app'`, id, personID)
	if err != nil {
		return false, fmt.Errorf("notifications: mark read: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

func (s *PGStore) MarkAllRead(ctx context.Context, personID uuid.UUID) (int, error) {
	tag, err := s.db.Exec(ctx, `
		UPDATE core.notifications
		SET read_at = now(), status = 'read'
		WHERE recipient_person_id = $1 AND channel = 'in_app' AND read_at IS NULL`, personID)
	if err != nil {
		return 0, fmt.Errorf("notifications: mark all read: %w", err)
	}
	return int(tag.RowsAffected()), nil
}

// ClaimDue is the dispatcher's work-stealing query. FOR UPDATE SKIP LOCKED lets
// several workers run at once without contention, and the claim lease means a
// worker that crashes mid-send does not strand its rows.
func (s *PGStore) ClaimDue(ctx context.Context, workerID string, lease time.Duration, limit int) ([]Notification, error) {
	rows, err := s.db.Query(ctx, `
		WITH due AS (
			SELECT id FROM core.notifications
			WHERE status = 'pending' AND channel <> 'in_app'
			  AND COALESCE(deliver_after, created_at) <= now()
			  AND (claimed_at IS NULL OR claimed_at < now() - make_interval(secs => $3))
			ORDER BY COALESCE(deliver_after, created_at)
			FOR UPDATE SKIP LOCKED
			LIMIT $1
		)
		UPDATE core.notifications n
		SET claimed_at = now(), claimed_by = $2
		FROM due, people.persons p
		WHERE n.id = due.id AND p.id = n.recipient_person_id
		RETURNING n.id, n.recipient_person_id, n.channel, COALESCE(n.template, ''),
			COALESCE(n.subject, ''), COALESCE(n.body, ''), n.payload, COALESCE(n.entity_type, ''),
			n.entity_id, n.status, n.attempts, n.max_attempts, COALESCE(n.last_error, ''),
			n.critical, COALESCE(n.idempotency_key, ''), n.deliver_after, n.created_at,
			n.sent_at, n.read_at, COALESCE(p.work_email::text, '')`,
		limit, workerID, lease.Seconds())
	if err != nil {
		return nil, fmt.Errorf("notifications: claim due: %w", err)
	}
	defer rows.Close()

	out := make([]Notification, 0, limit)
	for rows.Next() {
		n, err := scanClaim(rows)
		if err != nil {
			return nil, fmt.Errorf("notifications: claim scan: %w", err)
		}
		out = append(out, n)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("notifications: claim rows: %w", err)
	}
	return out, nil
}

func (s *PGStore) Complete(ctx context.Context, n Notification, out Outcome, ev *outbox.Event) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("notifications: complete begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	attempt := n.Attempts + 1
	attemptStatus := "failed"
	if out.Success {
		attemptStatus = "sent"
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO core.notification_attempts
			(notification_id, channel, provider, provider_ref, status, error)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		n.ID, string(n.Channel), nullString(out.Provider), nullString(out.ProviderRef),
		attemptStatus, nullString(out.Err)); err != nil {
		return fmt.Errorf("notifications: record attempt: %w", err)
	}

	switch {
	case out.Success:
		_, err = tx.Exec(ctx, `
			UPDATE core.notifications
			SET status = 'sent', sent_at = now(), attempts = $2, last_error = NULL,
			    claimed_at = NULL, claimed_by = NULL, deliver_after = NULL
			WHERE id = $1`, n.ID, attempt)
	case out.NextAttempt != nil:
		_, err = tx.Exec(ctx, `
			UPDATE core.notifications
			SET attempts = $2, last_error = $3, claimed_at = NULL, claimed_by = NULL,
			    deliver_after = $4
			WHERE id = $1`, n.ID, attempt, nullString(out.Err), *out.NextAttempt)
	default:
		_, err = tx.Exec(ctx, `
			UPDATE core.notifications
			SET status = 'failed', attempts = $2, last_error = $3,
			    claimed_at = NULL, claimed_by = NULL
			WHERE id = $1`, n.ID, attempt, nullString(out.Err))
	}
	if err != nil {
		return fmt.Errorf("notifications: complete update: %w", err)
	}

	if ev != nil {
		if _, err := outbox.Write(ctx, tx, *ev); err != nil {
			return err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("notifications: complete commit: %w", err)
	}
	return nil
}

func (s *PGStore) Defer(ctx context.Context, id uuid.UUID, until time.Time) error {
	_, err := s.db.Exec(ctx, `
		UPDATE core.notifications
		SET deliver_after = $2, claimed_at = NULL, claimed_by = NULL
		WHERE id = $1`, id, until)
	if err != nil {
		return fmt.Errorf("notifications: defer: %w", err)
	}
	return nil
}

// ── scanning helpers ─────────────────────────────────────────

func scanNotification(row pgx.Row) (Notification, error) {
	var (
		n                            Notification
		channel, status              string
		payload                      []byte
		deliverAfter, sentAt, readAt *time.Time
	)
	if err := row.Scan(
		&n.ID, &n.RecipientPersonID, &channel, &n.Template, &n.Subject, &n.Body, &payload,
		&n.EntityType, &n.EntityID, &status, &n.Attempts, &n.MaxAttempts, &n.LastError,
		&n.Critical, &n.IdempotencyKey, &deliverAfter, &n.CreatedAt, &sentAt, &readAt,
	); err != nil {
		return Notification{}, err
	}
	return finishScan(&n, channel, status, payload, deliverAfter, sentAt, readAt), nil
}

func scanClaim(row pgx.Row) (Notification, error) {
	var (
		n                            Notification
		channel, status              string
		payload                      []byte
		deliverAfter, sentAt, readAt *time.Time
	)
	if err := row.Scan(
		&n.ID, &n.RecipientPersonID, &channel, &n.Template, &n.Subject, &n.Body, &payload,
		&n.EntityType, &n.EntityID, &status, &n.Attempts, &n.MaxAttempts, &n.LastError,
		&n.Critical, &n.IdempotencyKey, &deliverAfter, &n.CreatedAt, &sentAt, &readAt,
		&n.RecipientEmail,
	); err != nil {
		return Notification{}, err
	}
	return finishScan(&n, channel, status, payload, deliverAfter, sentAt, readAt), nil
}

func finishScan(n *Notification, channel, status string, payload []byte, deliverAfter, sentAt, readAt *time.Time) Notification {
	n.Channel = Channel(channel)
	n.Status = Status(status)
	if len(payload) > 0 {
		_ = json.Unmarshal(payload, &n.Payload)
	}
	n.DeliverAfter = deliverAfter
	n.SentAt = sentAt
	n.ReadAt = readAt
	return *n
}

// ── tiny value helpers ───────────────────────────────────────

func nullString(s string) any {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return s
}

func marshalPayload(payload map[string]any) (any, error) {
	if len(payload) == 0 {
		return nil, nil
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("notifications: marshal payload: %w", err)
	}
	return string(raw), nil
}

func timePtr(t *time.Time) any {
	if t == nil {
		return nil
	}
	return *t
}

func int16Ptr(v *int) any {
	if v == nil {
		return nil
	}
	return int16(*v)
}

func uuidPtr(id *uuid.UUID) any {
	if id == nil {
		return nil
	}
	return *id
}
