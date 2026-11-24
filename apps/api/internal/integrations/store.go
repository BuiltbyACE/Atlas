package integrations

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/safaristack/atlas/apps/api/internal/outbox"
)

// rowScanner is what pgx's Row and Rows have in common.
type rowScanner interface {
	Scan(dest ...any) error
}

// Store is the integrations read model (§8.9). Writes go through Writer,
// added when the integration lifecycle lands.
type Store interface {
	// Integrations.
	ListIntegrations(ctx context.Context, q IntegrationQuery, limit int, before *Cursor) ([]Integration, error)
	GetIntegration(ctx context.Context, id uuid.UUID) (*Integration, error)

	// Webhook inbox.
	ListWebhookInbox(ctx context.Context, q WebhookInboxQuery, limit int, before *Cursor) ([]WebhookInboxEntry, error)
	GetWebhookInbox(ctx context.Context, id uuid.UUID) (*WebhookInboxEntry, error)

	// Outbound log.
	ListOutboundLog(ctx context.Context, q OutboundLogQuery, limit int, before *Cursor) ([]OutboundLogEntry, error)
	GetOutboundLog(ctx context.Context, id uuid.UUID) (*OutboundLogEntry, error)

	// Delivery claim/release for webhook idempotency (§11).
	ClaimDelivery(ctx context.Context, deliveryID, eventType string) (bool, error)
	ReleaseDelivery(ctx context.Context, deliveryID string) error
	WriteEvent(ctx context.Context, ev outbox.Event) error

	// GitHub installations (§8.9 Engineering Intelligence).
	CreateInstallation(ctx context.Context, in InstallationInput) (Installation, error)
	GetInstallation(ctx context.Context, id uuid.UUID) (*Installation, error)
	GetInstallationByGitHubID(ctx context.Context, installationID int64) (*Installation, error)
	ListInstallations(ctx context.Context, q InstallationQuery, limit int, before *Cursor) ([]Installation, error)
	UpdateInstallationStatus(ctx context.Context, id uuid.UUID, status InstallationStatus) (Installation, error)
	LinkPersonToInstallation(ctx context.Context, id uuid.UUID, personID uuid.UUID) (Installation, error)

	// GitHub user mappings (§8.9 Engineering Intelligence).
	CreateUserMapping(ctx context.Context, in UserMappingInput) (UserMapping, error)
	GetUserMapping(ctx context.Context, id uuid.UUID) (*UserMapping, error)
	GetUserMappingByGitHubID(ctx context.Context, githubUserID int64) (*UserMapping, error)
	ListUserMappings(ctx context.Context, q UserMappingQuery) ([]UserMapping, error)
	SetPrimaryMapping(ctx context.Context, id uuid.UUID) (UserMapping, error)
	DeleteUserMapping(ctx context.Context, id uuid.UUID) error

	// GitHub repositories (§8.9 Engineering Intelligence).
	CreateRepository(ctx context.Context, in RepositoryInput) (Repository, error)
	GetRepository(ctx context.Context, id uuid.UUID) (*Repository, error)
	GetRepositoryByGitHubID(ctx context.Context, installationID int64, githubRepoID int64) (*Repository, error)
	ListRepositories(ctx context.Context, q RepositoryQuery) ([]Repository, error)
	LinkRepositoryToProject(ctx context.Context, id uuid.UUID, projectID uuid.UUID) (Repository, error)
	UpdateLastPushedAt(ctx context.Context, id uuid.UUID, pushedAt time.Time) (Repository, error)

	// GitHub events (§8.9 Engineering Intelligence).
	CreateGitHubEvent(ctx context.Context, in GitHubEventInput) (GitHubEvent, error)
	GetGitHubEvent(ctx context.Context, id uuid.UUID) (*GitHubEvent, error)
	GetGitHubEventByDeliveryID(ctx context.Context, deliveryID string) (*GitHubEvent, error)
	ListGitHubEvents(ctx context.Context, q GitHubEventQuery, limit int, before *Cursor) ([]GitHubEvent, error)
	MarkProcessed(ctx context.Context, id uuid.UUID) error
}

// PGStore is the Postgres-backed read model.
type PGStore struct{ db *pgxpool.Pool }

// NewPGStore builds the read model over a pool.
func NewPGStore(db *pgxpool.Pool) *PGStore { return &PGStore{db: db} }

// ── integrations ──────────────────────────────────────────────────────────

// integrationCols is the integration projection.
const integrationCols = `
	i.id, i.ref, i.name, i.kind, i.direction, i.owner_person_id, COALESCE(p.full_name, ''),
	i.credential_ref, i.scopes, i.status, i.last_success, i.error_rate,
	i.version, i.created_at, i.updated_at`

func scanIntegration(row rowScanner) (Integration, error) {
	var i Integration
	var owner *uuid.UUID
	var lastSuccess *time.Time
	var scopes []byte
	err := row.Scan(&i.ID, &i.Ref, &i.Name, &i.Kind, &i.Direction, &owner, &i.OwnerName,
		&i.CredentialRef, &scopes, &i.Status, &lastSuccess, &i.ErrorRate,
		&i.Version, &i.CreatedAt, &i.UpdatedAt)
	if err != nil {
		return Integration{}, err
	}
	if owner != nil {
		i.OwnerPersonID = owner
	}
	if lastSuccess != nil {
		i.LastSuccess = lastSuccess
	}
	if err := json.Unmarshal(scopes, &i.Scopes); err != nil {
		return Integration{}, err
	}
	return i, nil
}

func (s *PGStore) ListIntegrations(ctx context.Context, q IntegrationQuery, limit int, before *Cursor) ([]Integration, error) {
	args := []any{}
	where := "TRUE"
	add := func(cond string, v any) {
		args = append(args, v)
		where += fmt.Sprintf(" AND "+cond, len(args))
	}
	if q.Kind != nil {
		add("i.kind = $%d", string(*q.Kind))
	}
	if q.Direction != nil {
		add("i.direction = $%d", string(*q.Direction))
	}
	if q.Status != nil {
		add("i.status = $%d", string(*q.Status))
	}
	if q.OwnerID != nil {
		add("i.owner_person_id = $%d", *q.OwnerID)
	}
	if search := trim(q.Search); search != "" {
		add("(i.name ILIKE '%%' || $%[1]d || '%%' OR i.ref ILIKE '%%' || $%[1]d || '%%')", search)
	}
	if before != nil {
		args = append(args, before.At, before.ID)
		where += fmt.Sprintf(" AND (i.created_at, i.id) < ($%d, $%d)", len(args)-1, len(args))
	}
	args = append(args, limit)
	rows, err := s.db.Query(ctx, `SELECT `+integrationCols+` FROM integrations.integrations i
		LEFT JOIN people.persons p ON p.id = i.owner_person_id WHERE `+where+
		` ORDER BY i.created_at DESC, i.id DESC LIMIT $`+fmt.Sprint(len(args)), args...)
	if err != nil {
		return nil, fmt.Errorf("integrations: list integrations: %w", err)
	}
	defer rows.Close()
	out := []Integration{}
	for rows.Next() {
		i, err := scanIntegration(rows)
		if err != nil {
			return nil, fmt.Errorf("integrations: scan integration: %w", err)
		}
		out = append(out, i)
	}
	return out, rows.Err()
}

func (s *PGStore) GetIntegration(ctx context.Context, id uuid.UUID) (*Integration, error) {
	i, err := scanIntegration(s.db.QueryRow(ctx, `SELECT `+integrationCols+` FROM integrations.integrations i
		LEFT JOIN people.persons p ON p.id = i.owner_person_id WHERE i.id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("integrations: get integration: %w", err)
	}
	return &i, nil
}

// ── webhook inbox ───────────────────────────────────────────────────────────

const webhookInboxCols = `
	w.id, w.provider, w.event_type, w.delivery_id, w.payload, w.headers,
	w.received_at, w.processed_at, w.processing_status, w.error_message`

func scanWebhookInbox(row rowScanner) (WebhookInboxEntry, error) {
	var w WebhookInboxEntry
	var payload, headers []byte
	var processedAt *time.Time
	err := row.Scan(&w.ID, &w.Provider, &w.EventType, &w.DeliveryID, &payload,
		&headers, &w.ReceivedAt, &processedAt, &w.ProcessingStatus, &w.ErrorMessage)
	if err != nil {
		return WebhookInboxEntry{}, err
	}
	w.Payload = payload
	w.Headers = headers
	w.ProcessedAt = processedAt
	return w, nil
}

func (s *PGStore) ListWebhookInbox(ctx context.Context, q WebhookInboxQuery, limit int, before *Cursor) ([]WebhookInboxEntry, error) {
	args := []any{}
	where := "TRUE"
	add := func(cond string, v any) {
		args = append(args, v)
		where += fmt.Sprintf(" AND "+cond, len(args))
	}
	if q.Provider != nil {
		add("w.provider = $%d", *q.Provider)
	}
	if q.EventType != nil {
		add("w.event_type = $%d", *q.EventType)
	}
	if q.DeliveryID != nil {
		add("w.delivery_id = $%d", *q.DeliveryID)
	}
	if q.ProcessingStatus != nil {
		add("w.processing_status = $%d", string(*q.ProcessingStatus))
	}
	if q.From != nil {
		add("w.received_at >= $%d", *q.From)
	}
	if q.To != nil {
		add("w.received_at <= $%d", *q.To)
	}
	if search := trim(q.Search); search != "" {
		add("(w.provider ILIKE '%%' || $%[1]d || '%%' OR w.event_type ILIKE '%%' || $%[1]d || '%%' OR w.delivery_id ILIKE '%%' || $%[1]d || '%%')", search)
	}
	if before != nil {
		args = append(args, before.At, before.ID)
		where += fmt.Sprintf(" AND (w.received_at, w.id) < ($%d, $%d)", len(args)-1, len(args))
	}
	args = append(args, limit)
	rows, err := s.db.Query(ctx, `SELECT `+webhookInboxCols+` FROM integrations.webhook_inbox w WHERE `+where+
		` ORDER BY w.received_at DESC, w.id DESC LIMIT $`+fmt.Sprint(len(args)), args...)
	if err != nil {
		return nil, fmt.Errorf("integrations: list webhook inbox: %w", err)
	}
	defer rows.Close()
	out := []WebhookInboxEntry{}
	for rows.Next() {
		w, err := scanWebhookInbox(rows)
		if err != nil {
			return nil, fmt.Errorf("integrations: scan webhook inbox: %w", err)
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

func (s *PGStore) GetWebhookInbox(ctx context.Context, id uuid.UUID) (*WebhookInboxEntry, error) {
	w, err := scanWebhookInbox(s.db.QueryRow(ctx, `SELECT `+webhookInboxCols+` FROM integrations.webhook_inbox w WHERE w.id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("integrations: get webhook inbox: %w", err)
	}
	return &w, nil
}

// ClaimDelivery records a delivery ID and reports whether it is new.
// A unique conflict on delivery_id means the delivery was already seen.
func (s *PGStore) ClaimDelivery(ctx context.Context, deliveryID, eventType string) (bool, error) {
	tag, err := s.db.Exec(ctx, `
		INSERT INTO integrations.github_deliveries (delivery_id, event_type)
		VALUES ($1, $2)
		ON CONFLICT (delivery_id) DO NOTHING`, deliveryID, eventType)
	if err != nil {
		return false, fmt.Errorf("integrations: claim delivery: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

// ReleaseDelivery removes a delivery claim so a processing failure can be retried.
func (s *PGStore) ReleaseDelivery(ctx context.Context, deliveryID string) error {
	if _, err := s.db.Exec(ctx, `DELETE FROM integrations.github_deliveries WHERE delivery_id = $1`, deliveryID); err != nil {
		return fmt.Errorf("integrations: release delivery: %w", err)
	}
	return nil
}

// WriteEvent appends an event to the outbox.
func (s *PGStore) WriteEvent(ctx context.Context, ev outbox.Event) error {
	if _, err := outbox.Write(ctx, s.db, ev); err != nil {
		return fmt.Errorf("integrations: %w", err)
	}
	return nil
}

// ── outbound log ────────────────────────────────────────────────────────────

const outboundLogCols = `
	o.id, o.provider, o.endpoint, o.method, o.status_code, o.error_message,
	o.attempt, o.max_attempts, o.last_attempt_at, o.circuit_state, o.created_at`

func scanOutboundLog(row rowScanner) (OutboundLogEntry, error) {
	var o OutboundLogEntry
	var statusCode *int
	err := row.Scan(&o.ID, &o.Provider, &o.Endpoint, &o.Method, &statusCode,
		&o.ErrorMessage, &o.Attempt, &o.MaxAttempts, &o.LastAttemptAt,
		&o.CircuitState, &o.CreatedAt)
	if err != nil {
		return OutboundLogEntry{}, err
	}
	o.StatusCode = statusCode
	return o, nil
}

func (s *PGStore) ListOutboundLog(ctx context.Context, q OutboundLogQuery, limit int, before *Cursor) ([]OutboundLogEntry, error) {
	args := []any{}
	where := "TRUE"
	add := func(cond string, v any) {
		args = append(args, v)
		where += fmt.Sprintf(" AND "+cond, len(args))
	}
	if q.Provider != nil {
		add("o.provider = $%d", *q.Provider)
	}
	if q.Endpoint != nil {
		add("o.endpoint = $%d", *q.Endpoint)
	}
	if q.CircuitState != nil {
		add("o.circuit_state = $%d", string(*q.CircuitState))
	}
	if q.From != nil {
		add("o.last_attempt_at >= $%d", *q.From)
	}
	if q.To != nil {
		add("o.last_attempt_at <= $%d", *q.To)
	}
	if search := trim(q.Search); search != "" {
		add("(o.provider ILIKE '%%' || $%[1]d || '%%' OR o.endpoint ILIKE '%%' || $%[1]d || '%%' OR o.error_message ILIKE '%%' || $%[1]d || '%%')", search)
	}
	if before != nil {
		args = append(args, before.At, before.ID)
		where += fmt.Sprintf(" AND (o.last_attempt_at, o.id) < ($%d, $%d)", len(args)-1, len(args))
	}
	args = append(args, limit)
	rows, err := s.db.Query(ctx, `SELECT `+outboundLogCols+` FROM integrations.outbound_log o WHERE `+where+
		` ORDER BY o.last_attempt_at DESC, o.id DESC LIMIT $`+fmt.Sprint(len(args)), args...)
	if err != nil {
		return nil, fmt.Errorf("integrations: list outbound log: %w", err)
	}
	defer rows.Close()
	out := []OutboundLogEntry{}
	for rows.Next() {
		o, err := scanOutboundLog(rows)
		if err != nil {
			return nil, fmt.Errorf("integrations: scan outbound log: %w", err)
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

func (s *PGStore) GetOutboundLog(ctx context.Context, id uuid.UUID) (*OutboundLogEntry, error) {
	o, err := scanOutboundLog(s.db.QueryRow(ctx, `SELECT `+outboundLogCols+` FROM integrations.outbound_log o WHERE o.id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("integrations: get outbound log: %w", err)
	}
	return &o, nil
}

// sortIntegrations sorts integrations by created_at descending.
func sortIntegrations(integrations []Integration) {
	sort.SliceStable(integrations, func(i, j int) bool {
		return integrations[i].CreatedAt.After(integrations[j].CreatedAt)
	})
}

// sortWebhookInbox sorts webhook inbox entries by received_at descending.
func sortWebhookInbox(entries []WebhookInboxEntry) {
	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].ReceivedAt.After(entries[j].ReceivedAt)
	})
}

// sortOutboundLog sorts outbound log entries by last_attempt_at descending.
func sortOutboundLog(entries []OutboundLogEntry) {
	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].LastAttemptAt.After(entries[j].LastAttemptAt)
	})
}
