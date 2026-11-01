package integrations

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// Writer is the integrations write model (§8.9). Go owns every rule: an
// integration in error state cannot be edited, and credential references are
// locations, never secrets.
type Writer interface {
	// AllocateRef allocates a human reference for a new record.
	AllocateRef(ctx context.Context, pType string, pYear int) (string, error)

	// Integrations.
	CreateIntegration(ctx context.Context, i Integration) (Integration, error)
	UpdateIntegration(ctx context.Context, i Integration, expectedVersion int) (Integration, error)

	// Outbound log: records an outbound call attempt (§8.9).
	WriteOutboundLog(ctx context.Context, o OutboundLogEntry) (OutboundLogEntry, error)
}

var _ Writer = (*PGStore)(nil)

// AllocateRef reserves the next human reference for a given type and year.
func (s *PGStore) AllocateRef(ctx context.Context, refType string, year int) (string, error) {
	var ref string
	if err := s.db.QueryRow(ctx, `SELECT core.next_reference($1, $2)`, refType, year).Scan(&ref); err != nil {
		return "", fmt.Errorf("integrations: allocate ref %q: %w", refType, err)
	}
	return ref, nil
}

func writeErr(op string, err error) error {
	return fmt.Errorf("integrations: %s: %w", op, err)
}

func nullText(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// nullTextPtr returns a pointer to the string, or nil when empty.
func nullTextPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// ── integrations ──────────────────────────────────────────────────────────

const integrationInsertCols = `
	i.ref, i.name, i.kind, i.direction, i.owner_person_id, i.credential_ref,
	i.scopes, i.status, i.version`

func (s *PGStore) CreateIntegration(ctx context.Context, i Integration) (Integration, error) {
	if !validIntegrationKind(i.Kind) {
		return Integration{}, withCode(errInvalidField("kind"), CodeInvalidIntegrationKind)
	}
	if !validDirection(i.Direction) {
		return Integration{}, withCode(errInvalidField("direction"), CodeInvalidDirection)
	}
	if !validIntegrationStatus(i.Status) {
		i.Status = IntegrationActive
	}
	if !validRef(i.Ref) {
		return Integration{}, withCode(errInvalidField("ref"), CodeDuplicateRef)
	}
	ref, err := s.AllocateRef(ctx, "integration", time.Now().UTC().Year())
	if err != nil {
		return Integration{}, writeErr("allocate ref", err)
	}
	i.Ref = ref
	i.Version = 1
	scopes, err := json.Marshal(i.Scopes)
	if err != nil {
		return Integration{}, writeErr("marshal scopes", err)
	}
	err = s.db.QueryRow(ctx, `
		INSERT INTO integrations.integrations (ref, name, kind, direction, owner_person_id, credential_ref, scopes, status, version)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		RETURNING id, created_at, updated_at`,
		i.Ref, i.Name, string(i.Kind), string(i.Direction), i.OwnerPersonID,
		i.CredentialRef, scopes, string(i.Status), i.Version).
		Scan(&i.ID, &i.CreatedAt, &i.UpdatedAt)
	if err != nil {
		return Integration{}, writeErr("create integration", err)
	}
	return i, nil
}

func (s *PGStore) UpdateIntegration(ctx context.Context, i Integration, expectedVersion int) (Integration, error) {
	current, err := s.GetIntegration(ctx, i.ID)
	if err != nil {
		return Integration{}, err
	}
	if current == nil {
		return Integration{}, ErrNotFound
	}
	if current.Version != expectedVersion {
		return Integration{}, withCode(ErrConflict, CodeIntegrationNotEditable)
	}
	if !IntegrationEditable(current.Status) {
		return Integration{}, withCode(ErrForbidden, CodeIntegrationNotEditable)
	}
	i.Ref = current.Ref
	i.Version = current.Version + 1
	scopes, err := json.Marshal(i.Scopes)
	if err != nil {
		return Integration{}, writeErr("marshal scopes", err)
	}
	err = s.db.QueryRow(ctx, `
		UPDATE integrations.integrations
		SET name = $1, kind = $2, direction = $3, owner_person_id = $4, credential_ref = $5,
		    scopes = $6, status = $7, version = $8
		WHERE id = $9
		RETURNING updated_at`,
		i.Name, string(i.Kind), string(i.Direction), i.OwnerPersonID,
		i.CredentialRef, scopes, string(i.Status), i.Version, i.ID).
		Scan(&i.UpdatedAt)
	if err != nil {
		return Integration{}, writeErr("update integration", err)
	}
	return i, nil
}

// WriteOutboundLog records an outbound call attempt (§8.9).
func (s *PGStore) WriteOutboundLog(ctx context.Context, o OutboundLogEntry) (OutboundLogEntry, error) {
	err := s.db.QueryRow(ctx, `
		INSERT INTO integrations.outbound_log (provider, endpoint, method, status_code, error_message, attempt, max_attempts, last_attempt_at, circuit_state)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		RETURNING id, created_at`,
		o.Provider, o.Endpoint, o.Method, o.StatusCode, o.ErrorMessage,
		o.Attempt, o.MaxAttempts, o.LastAttemptAt, o.CircuitState).
		Scan(&o.ID, &o.CreatedAt)
	if err != nil {
		return OutboundLogEntry{}, writeErr("write outbound log", err)
	}
	return o, nil
}
