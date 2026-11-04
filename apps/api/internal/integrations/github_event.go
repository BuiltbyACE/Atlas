package integrations

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// GitHubEvent is a durable record of a GitHub webhook delivery.
// Every event type (push, pull_request, check_run, workflow_run,
// deployment_status, installation) is stored here for replay and reconciliation.
type GitHubEvent struct {
	ID             uuid.UUID
	InstallationID int64
	DeliveryID     string
	EventType      string
	Action         string
	Repository     string
	SenderLogin    string
	SenderID       *int64
	Payload        json.RawMessage
	ProcessedAt    *time.Time
	CreatedAt      time.Time
}

// GitHubEventInput is the payload for recording a webhook event.
type GitHubEventInput struct {
	InstallationID int64
	DeliveryID     string
	EventType      string
	Action         string
	Repository     string
	SenderLogin    string
	SenderID       *int64
	Payload        []byte
}

// GitHubEventQuery narrows the event list.
type GitHubEventQuery struct {
	InstallationID *int64
	EventType      string
	Action         string
	Repository     string
	SenderLogin    string
	From           *time.Time
	To             *time.Time
}

func scanGitHubEvent(row rowScanner) (GitHubEvent, error) {
	var e GitHubEvent
	var senderID *int64
	var processedAt *time.Time
	var payload []byte
	err := row.Scan(&e.ID, &e.InstallationID, &e.DeliveryID, &e.EventType,
		&e.Action, &e.Repository, &e.SenderLogin, &senderID, &payload,
		&processedAt, &e.CreatedAt)
	if err != nil {
		return GitHubEvent{}, err
	}
	e.SenderID = senderID
	e.Payload = payload
	e.ProcessedAt = processedAt
	return e, nil
}

const gitHubEventCols = `
	e.id, e.installation_id, e.delivery_id, e.event_type, e.action,
	e.repository, e.sender_login, e.sender_id, e.payload,
	e.processed_at, e.created_at`

// CreateGitHubEvent records a webhook delivery in the durable event store.
func (s *PGStore) CreateGitHubEvent(ctx context.Context, in GitHubEventInput) (GitHubEvent, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return GitHubEvent{}, fmt.Errorf("new id: %w", err)
	}
	var e GitHubEvent
	err = s.db.QueryRow(ctx, `
		INSERT INTO integrations.github_events (
			id, installation_id, delivery_id, event_type, action,
			repository, sender_login, sender_id, payload, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		RETURNING `+gitHubEventCols,
		id, in.InstallationID, in.DeliveryID, in.EventType, in.Action,
		in.Repository, in.SenderLogin, in.SenderID, in.Payload, time.Now().UTC()).
		Scan(&e.ID, &e.InstallationID, &e.DeliveryID, &e.EventType, &e.Action,
			&e.Repository, &e.SenderLogin, &e.SenderID, &e.Payload,
			&e.ProcessedAt, &e.CreatedAt)
	if err != nil {
		return GitHubEvent{}, fmt.Errorf("create github event: %w", err)
	}
	return e, nil
}

// GetGitHubEvent returns one event by its SCOS id.
func (s *PGStore) GetGitHubEvent(ctx context.Context, id uuid.UUID) (*GitHubEvent, error) {
	e, err := scanGitHubEvent(s.db.QueryRow(ctx, `SELECT `+gitHubEventCols+` FROM integrations.github_events e WHERE e.id = $1`, id))
	if err != nil {
		return nil, err
	}
	return &e, nil
}

// GetGitHubEventByDeliveryID returns an event by GitHub delivery id.
func (s *PGStore) GetGitHubEventByDeliveryID(ctx context.Context, deliveryID string) (*GitHubEvent, error) {
	e, err := scanGitHubEvent(s.db.QueryRow(ctx, `SELECT `+gitHubEventCols+` FROM integrations.github_events e WHERE e.delivery_id = $1`, deliveryID))
	if err != nil {
		return nil, err
	}
	return &e, nil
}

// ListGitHubEvents returns events matching the query with keyset pagination.
func (s *PGStore) ListGitHubEvents(ctx context.Context, q GitHubEventQuery, limit int, before *Cursor) ([]GitHubEvent, error) {
	args := []any{}
	where := "TRUE"
	add := func(cond string, v any) {
		args = append(args, v)
		where += fmt.Sprintf(" AND "+cond, len(args))
	}
	if q.InstallationID != nil {
		add("e.installation_id = $%d", *q.InstallationID)
	}
	if q.EventType != "" {
		add("e.event_type = $%d", q.EventType)
	}
	if q.Action != "" {
		add("e.action = $%d", q.Action)
	}
	if q.Repository != "" {
		add("e.repository = $%d", q.Repository)
	}
	if q.SenderLogin != "" {
		add("e.sender_login = $%d", q.SenderLogin)
	}
	if q.From != nil {
		add("e.created_at >= $%d", *q.From)
	}
	if q.To != nil {
		add("e.created_at <= $%d", *q.To)
	}
	if before != nil {
		args = append(args, before.At, before.ID)
		where += fmt.Sprintf(" AND (e.created_at, e.id) < ($%d, $%d)", len(args)-1, len(args))
	}
	args = append(args, limit)
	rows, err := s.db.Query(ctx, `SELECT `+gitHubEventCols+` FROM integrations.github_events e WHERE `+where+
		` ORDER BY e.created_at DESC, e.id DESC LIMIT $`+fmt.Sprint(len(args)), args...)
	if err != nil {
		return nil, fmt.Errorf("list github events: %w", err)
	}
	defer rows.Close()
	out := []GitHubEvent{}
	for rows.Next() {
		e, err := scanGitHubEvent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// MarkProcessed sets the processed_at timestamp on an event.
func (s *PGStore) MarkProcessed(ctx context.Context, id uuid.UUID) error {
	now := time.Now().UTC()
	_, err := s.db.Exec(ctx, `UPDATE integrations.github_events SET processed_at = $2 WHERE id = $1`, id, now)
	if err != nil {
		return fmt.Errorf("mark processed: %w", err)
	}
	return nil
}
