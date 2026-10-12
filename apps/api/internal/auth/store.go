package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/safaristack/atlas/apps/api/internal/authz"
)

// epoch is the COALESCE fallback for NULL timestamps; a value at or before it
// means "no timestamp".
var epoch = time.Unix(0, 0).UTC()

// IdentifierKind distinguishes the two login identifiers §6.1 supports.
type IdentifierKind string

const (
	IdentifierEmail IdentifierKind = "email"
	IdentifierPhone IdentifierKind = "phone"
)

// Identifier is a resolved login identifier.
type Identifier struct {
	Kind  IdentifierKind
	Value string
}

// Account is the subset of core.user_accounts the sign-in flow needs.
type Account struct {
	ID           uuid.UUID
	PersonID     uuid.UUID
	Email        string
	Phone        string
	PasswordHash string
	Status       string
	MFARequired  bool
	MFASecretEnc []byte
	MFAActivated bool
	DisplayName  string
}

// Session is a row of core.sessions.
type Session struct {
	ID          uuid.UUID
	UserID      uuid.UUID
	TokenHash   []byte
	MFAVerified bool
	IP          netip.Addr
	UserAgent   string
	CreatedAt   time.Time
	LastSeenAt  time.Time
	ExpiresAt   time.Time
	RevokedAt   *time.Time
}

// SessionMeta is the client context recorded on a new session (§6.1).
type SessionMeta struct {
	IP        netip.Addr
	UserAgent string
}

// ServiceIdentity is a non-human principal (svc:orchestrator, …).
type ServiceIdentity struct {
	ID          string
	Permissions []string
	Credential  []byte
	DisabledAt  *time.Time
}

// GrantSpec is a role grant carried on an invite.
type GrantSpec struct {
	RoleKey   string     `json:"role_key"`
	ScopeType string     `json:"scope_type"`
	ScopeID   *uuid.UUID `json:"scope_id,omitempty"`
}

// Invite is a row of core.account_invites.
type Invite struct {
	ID         uuid.UUID
	Email      string
	Phone      string
	FullName   string
	Employment string
	RoleGrants []GrantSpec
	TokenHash  []byte
	InvitedBy  *uuid.UUID
	ExpiresAt  time.Time
	AcceptedAt *time.Time
	RevokedAt  *time.Time
	CreatedAt  time.Time
}

// AcceptedAccount is the result of accepting an invite.
type AcceptedAccount struct {
	AccountID uuid.UUID
	PersonID  uuid.UUID
	Email     string
}

// AcceptInviteInput carries the caller's decisions when accepting an invite.
type AcceptInviteInput struct {
	TokenHash    []byte
	FullName     string
	Phone        string
	PasswordHash string
	MFARequired  bool
	Now          time.Time
}

// Store is the persistence the auth service needs. It is an interface so the
// service can be unit-tested without a database.
type Store interface {
	FindAccountByIdentifier(ctx context.Context, id Identifier) (*Account, error)
	FindAccountByID(ctx context.Context, userID uuid.UUID) (*Account, error)
	LoadPrincipal(ctx context.Context, userID, sessionID uuid.UUID) (*authz.Principal, error)

	CreateSession(ctx context.Context, s *Session) error
	SessionByTokenHash(ctx context.Context, hash []byte) (*Session, error)
	TouchSession(ctx context.Context, id uuid.UUID, seen time.Time) error
	RevokeSession(ctx context.Context, id, userID uuid.UUID) (bool, error)
	RevokeAllSessions(ctx context.Context, userID uuid.UUID) (int64, error)
	ListSessions(ctx context.Context, userID uuid.UUID) ([]Session, error)
	MarkLastLogin(ctx context.Context, userID uuid.UUID, at time.Time) error

	RecordLoginAttempt(ctx context.Context, identifier string, succeeded bool, ip netip.Addr, userAgent string) error
	CountRecentFailures(ctx context.Context, identifier string, since time.Time) (int, error)

	SetMFASecret(ctx context.Context, userID uuid.UUID, sealed []byte) error
	ActivateMFA(ctx context.Context, userID uuid.UUID, at time.Time, recoveryHashes [][]byte) error
	UseRecoveryCode(ctx context.Context, userID uuid.UUID, hash []byte) (bool, error)

	FindService(ctx context.Context, id string) (*ServiceIdentity, error)

	CreateInvite(ctx context.Context, inv *Invite) error
	InviteByTokenHash(ctx context.Context, hash []byte) (*Invite, error)
	AcceptInvite(ctx context.Context, in AcceptInviteInput) (*AcceptedAccount, error)
}

// PGStore is the pgx-backed implementation of Store.
type PGStore struct {
	db *pgxpool.Pool
}

// NewPGStore wraps a connection pool.
func NewPGStore(db *pgxpool.Pool) *PGStore { return &PGStore{db: db} }

// ── accounts and authorization ───────────────────────────────

// FindAccountByIdentifier returns nil (with no error) when nothing matches.
func (s *PGStore) FindAccountByIdentifier(ctx context.Context, id Identifier) (*Account, error) {
	const base = `
		SELECT a.id, a.person_id, a.email, COALESCE(a.phone_e164, ''),
		       COALESCE(a.password_hash, ''), a.status, a.mfa_required,
		       COALESCE(a.mfa_totp_secret_enc, ''::bytea),
		       (a.mfa_totp_activated_at IS NOT NULL),
		       p.full_name
		FROM core.user_accounts a
		JOIN people.persons p ON p.id = a.person_id
		WHERE `

	var query, arg string
	switch id.Kind {
	case IdentifierPhone:
		query, arg = base+"a.phone_e164 = $1", id.Value
	case IdentifierEmail:
		query, arg = base+"a.email = $1", id.Value
	default:
		return nil, fmt.Errorf("auth: unknown identifier kind %q", id.Kind)
	}

	var a Account
	err := s.db.QueryRow(ctx, query, arg).Scan(
		&a.ID, &a.PersonID, &a.Email, &a.Phone, &a.PasswordHash, &a.Status,
		&a.MFARequired, &a.MFASecretEnc, &a.MFAActivated, &a.DisplayName,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("auth: find account: %w", err)
	}
	if len(a.MFASecretEnc) == 0 {
		a.MFASecretEnc = nil
	}
	return &a, nil
}

// FindAccountByID returns nil (with no error) when nothing matches.
func (s *PGStore) FindAccountByID(ctx context.Context, userID uuid.UUID) (*Account, error) {
	var a Account
	err := s.db.QueryRow(ctx, `
		SELECT a.id, a.person_id, a.email, COALESCE(a.phone_e164, ''),
		       COALESCE(a.password_hash, ''), a.status, a.mfa_required,
		       COALESCE(a.mfa_totp_secret_enc, ''::bytea),
		       (a.mfa_totp_activated_at IS NOT NULL),
		       p.full_name
		FROM core.user_accounts a
		JOIN people.persons p ON p.id = a.person_id
		WHERE a.id = $1`, userID).Scan(
		&a.ID, &a.PersonID, &a.Email, &a.Phone, &a.PasswordHash, &a.Status,
		&a.MFARequired, &a.MFASecretEnc, &a.MFAActivated, &a.DisplayName,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("auth: find account by id: %w", err)
	}
	if len(a.MFASecretEnc) == 0 {
		a.MFASecretEnc = nil
	}
	return &a, nil
}

// LoadPrincipal assembles the effective permission set for a user.
func (s *PGStore) LoadPrincipal(ctx context.Context, userID, sessionID uuid.UUID) (*authz.Principal, error) {
	var email, name string
	err := s.db.QueryRow(ctx, `
		SELECT a.email, p.full_name
		FROM core.user_accounts a
		JOIN people.persons p ON p.id = a.person_id
		WHERE a.id = $1`, userID).Scan(&email, &name)
	if err != nil {
		return nil, fmt.Errorf("auth: load principal: %w", err)
	}

	p := &authz.Principal{
		Kind:        authz.KindUser,
		Subject:     "user:" + userID.String(),
		UserID:      userID,
		Email:       email,
		Name:        name,
		Permissions: map[string]struct{}{},
		SessionID:   sessionID,
	}

	grants, err := s.db.Query(ctx, `
		SELECT role_key, scope_type,
		       COALESCE(scope_id, '00000000-0000-0000-0000-000000000000'::uuid)
		FROM core.role_grants
		WHERE user_id = $1 AND revoked_at IS NULL`, userID)
	if err != nil {
		return nil, fmt.Errorf("auth: load grants: %w", err)
	}
	defer grants.Close()
	for grants.Next() {
		var g authz.Grant
		if err := grants.Scan(&g.RoleKey, &g.ScopeType, &g.ScopeID); err != nil {
			return nil, fmt.Errorf("auth: scan grant: %w", err)
		}
		p.Grants = append(p.Grants, g)
		p.Roles = append(p.Roles, g.RoleKey)
	}
	if err := grants.Err(); err != nil {
		return nil, fmt.Errorf("auth: grants: %w", err)
	}

	perms, err := s.db.Query(ctx, `
		SELECT DISTINCT rp.permission_key
		FROM core.role_grants g
		JOIN core.role_permissions rp ON rp.role_key = g.role_key
		WHERE g.user_id = $1 AND g.revoked_at IS NULL`, userID)
	if err != nil {
		return nil, fmt.Errorf("auth: load permissions: %w", err)
	}
	defer perms.Close()
	for perms.Next() {
		var key string
		if err := perms.Scan(&key); err != nil {
			return nil, fmt.Errorf("auth: scan permission: %w", err)
		}
		p.Permissions[key] = struct{}{}
	}
	if err := perms.Err(); err != nil {
		return nil, fmt.Errorf("auth: permissions: %w", err)
	}
	return p, nil
}

// ── sessions ─────────────────────────────────────────────────

func (s *PGStore) CreateSession(ctx context.Context, sess *Session) error {
	_, err := s.db.Exec(ctx, `
		INSERT INTO core.sessions
			(id, user_id, token_hash, mfa_verified, ip, user_agent, created_at, last_seen_at, expires_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		sess.ID, sess.UserID, sess.TokenHash, sess.MFAVerified, nullableAddr(sess.IP),
		sess.UserAgent, sess.CreatedAt, sess.LastSeenAt, sess.ExpiresAt)
	if err != nil {
		return fmt.Errorf("auth: create session: %w", err)
	}
	return nil
}

func (s *PGStore) SessionByTokenHash(ctx context.Context, hash []byte) (*Session, error) {
	var (
		sess      Session
		ipText    string
		revokedAt time.Time
	)
	err := s.db.QueryRow(ctx, `
		SELECT id, user_id, mfa_verified, COALESCE(host(ip), ''), COALESCE(user_agent, ''),
		       created_at, last_seen_at, expires_at, COALESCE(revoked_at, to_timestamp(0))
		FROM core.sessions
		WHERE token_hash = $1`, hash).
		Scan(&sess.ID, &sess.UserID, &sess.MFAVerified, &ipText, &sess.UserAgent,
			&sess.CreatedAt, &sess.LastSeenAt, &sess.ExpiresAt, &revokedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("auth: session by token: %w", err)
	}
	sess.TokenHash = hash
	if ipText != "" {
		if addr, perr := netip.ParseAddr(ipText); perr == nil {
			sess.IP = addr
		}
	}
	if revokedAt.After(epoch) {
		revoked := revokedAt
		sess.RevokedAt = &revoked
	}
	return &sess, nil
}

func (s *PGStore) TouchSession(ctx context.Context, id uuid.UUID, seen time.Time) error {
	_, err := s.db.Exec(ctx, `UPDATE core.sessions SET last_seen_at = $2 WHERE id = $1`, id, seen)
	if err != nil {
		return fmt.Errorf("auth: touch session: %w", err)
	}
	return nil
}

func (s *PGStore) RevokeSession(ctx context.Context, id, userID uuid.UUID) (bool, error) {
	tag, err := s.db.Exec(ctx, `
		UPDATE core.sessions SET revoked_at = now()
		WHERE id = $1 AND user_id = $2 AND revoked_at IS NULL`, id, userID)
	if err != nil {
		return false, fmt.Errorf("auth: revoke session: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

func (s *PGStore) RevokeAllSessions(ctx context.Context, userID uuid.UUID) (int64, error) {
	tag, err := s.db.Exec(ctx, `
		UPDATE core.sessions SET revoked_at = now()
		WHERE user_id = $1 AND revoked_at IS NULL`, userID)
	if err != nil {
		return 0, fmt.Errorf("auth: revoke all sessions: %w", err)
	}
	return tag.RowsAffected(), nil
}

func (s *PGStore) ListSessions(ctx context.Context, userID uuid.UUID) ([]Session, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, user_id, mfa_verified, COALESCE(host(ip), ''), COALESCE(user_agent, ''),
		       created_at, last_seen_at, expires_at
		FROM core.sessions
		WHERE user_id = $1 AND revoked_at IS NULL AND expires_at > now()
		ORDER BY last_seen_at DESC`, userID)
	if err != nil {
		return nil, fmt.Errorf("auth: list sessions: %w", err)
	}
	defer rows.Close()

	var out []Session
	for rows.Next() {
		var (
			sess   Session
			ipText string
		)
		if err := rows.Scan(&sess.ID, &sess.UserID, &sess.MFAVerified, &ipText, &sess.UserAgent,
			&sess.CreatedAt, &sess.LastSeenAt, &sess.ExpiresAt); err != nil {
			return nil, fmt.Errorf("auth: scan session: %w", err)
		}
		if ipText != "" {
			if addr, perr := netip.ParseAddr(ipText); perr == nil {
				sess.IP = addr
			}
		}
		out = append(out, sess)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("auth: sessions: %w", err)
	}
	return out, nil
}

func (s *PGStore) MarkLastLogin(ctx context.Context, userID uuid.UUID, at time.Time) error {
	_, err := s.db.Exec(ctx, `UPDATE core.user_accounts SET last_login_at = $2 WHERE id = $1`, userID, at)
	if err != nil {
		return fmt.Errorf("auth: mark last login: %w", err)
	}
	return nil
}

// ── lockout telemetry ────────────────────────────────────────

func (s *PGStore) RecordLoginAttempt(ctx context.Context, identifier string, succeeded bool, ip netip.Addr, userAgent string) error {
	_, err := s.db.Exec(ctx, `
		INSERT INTO core.login_attempts (identifier, succeeded, ip, user_agent)
		VALUES ($1, $2, $3, $4)`, identifier, succeeded, nullableAddr(ip), userAgent)
	if err != nil {
		return fmt.Errorf("auth: record login attempt: %w", err)
	}
	return nil
}

func (s *PGStore) CountRecentFailures(ctx context.Context, identifier string, since time.Time) (int, error) {
	var n int
	err := s.db.QueryRow(ctx, `
		SELECT count(*) FROM core.login_attempts
		WHERE identifier = $1 AND succeeded = false AND attempted_at > $2`, identifier, since).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("auth: count login failures: %w", err)
	}
	return n, nil
}

// ── MFA ──────────────────────────────────────────────────────

func (s *PGStore) SetMFASecret(ctx context.Context, userID uuid.UUID, sealed []byte) error {
	_, err := s.db.Exec(ctx, `
		UPDATE core.user_accounts
		SET mfa_totp_secret_enc = $2, mfa_required = true
		WHERE id = $1`, userID, sealed)
	if err != nil {
		return fmt.Errorf("auth: set mfa secret: %w", err)
	}
	return nil
}

func (s *PGStore) ActivateMFA(ctx context.Context, userID uuid.UUID, at time.Time, recoveryHashes [][]byte) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("auth: activate mfa begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `
		UPDATE core.user_accounts SET mfa_totp_activated_at = $2, mfa_required = true WHERE id = $1`,
		userID, at); err != nil {
		return fmt.Errorf("auth: activate mfa: %w", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM core.recovery_codes WHERE user_id = $1`, userID); err != nil {
		return fmt.Errorf("auth: clear recovery codes: %w", err)
	}
	for _, hash := range recoveryHashes {
		id, err := uuid.NewV7()
		if err != nil {
			return fmt.Errorf("auth: recovery id: %w", err)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO core.recovery_codes (id, user_id, code_hash) VALUES ($1, $2, $3)`,
			id, userID, hash); err != nil {
			return fmt.Errorf("auth: insert recovery code: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("auth: activate mfa commit: %w", err)
	}
	return nil
}

func (s *PGStore) UseRecoveryCode(ctx context.Context, userID uuid.UUID, hash []byte) (bool, error) {
	tag, err := s.db.Exec(ctx, `
		UPDATE core.recovery_codes SET used_at = now()
		WHERE user_id = $1 AND code_hash = $2 AND used_at IS NULL`, userID, hash)
	if err != nil {
		return false, fmt.Errorf("auth: use recovery code: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

// ── service identities ───────────────────────────────────────

func (s *PGStore) FindService(ctx context.Context, id string) (*ServiceIdentity, error) {
	var (
		svc      ServiceIdentity
		disabled time.Time
	)
	err := s.db.QueryRow(ctx, `
		SELECT id, permissions, credential_hash, COALESCE(disabled_at, to_timestamp(0))
		FROM core.service_identities WHERE id = $1`, id).
		Scan(&svc.ID, &svc.Permissions, &svc.Credential, &disabled)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("auth: find service: %w", err)
	}
	if disabled.After(epoch) {
		svc.DisabledAt = &disabled
	}
	return &svc, nil
}

// ── invites ──────────────────────────────────────────────────

func (s *PGStore) CreateInvite(ctx context.Context, inv *Invite) error {
	grants, err := json.Marshal(inv.RoleGrants)
	if err != nil {
		return fmt.Errorf("auth: marshal invite grants: %w", err)
	}
	_, err = s.db.Exec(ctx, `
		INSERT INTO core.account_invites
			(id, email, phone_e164, full_name, employment_type, role_grants, token_hash, invited_by, expires_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		inv.ID, inv.Email, nullIfEmpty(inv.Phone), nullIfEmpty(inv.FullName),
		orDefault(inv.Employment, "full_time"), grants, inv.TokenHash, inv.InvitedBy, inv.ExpiresAt)
	if err != nil {
		return fmt.Errorf("auth: create invite: %w", err)
	}
	return nil
}

func (s *PGStore) InviteByTokenHash(ctx context.Context, hash []byte) (*Invite, error) {
	var (
		inv        Invite
		phone      *string
		fullName   *string
		grantsJSON []byte
		accepted   time.Time
		revoked    time.Time
	)
	err := s.db.QueryRow(ctx, `
		SELECT id, email, phone_e164, full_name, employment_type, role_grants,
		       token_hash, invited_by, expires_at,
		       COALESCE(accepted_at, to_timestamp(0)), COALESCE(revoked_at, to_timestamp(0)), created_at
		FROM core.account_invites WHERE token_hash = $1`, hash).
		Scan(&inv.ID, &inv.Email, &phone, &fullName, &inv.Employment, &grantsJSON,
			&inv.TokenHash, &inv.InvitedBy, &inv.ExpiresAt, &accepted, &revoked, &inv.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("auth: invite by token: %w", err)
	}
	if phone != nil {
		inv.Phone = *phone
	}
	if fullName != nil {
		inv.FullName = *fullName
	}
	if accepted.After(epoch) {
		inv.AcceptedAt = &accepted
	}
	if revoked.After(epoch) {
		inv.RevokedAt = &revoked
	}
	if len(grantsJSON) > 0 {
		if err := json.Unmarshal(grantsJSON, &inv.RoleGrants); err != nil {
			return nil, fmt.Errorf("auth: decode invite grants: %w", err)
		}
	}
	return &inv, nil
}

// AcceptInvite creates the person and account behind an invite and applies its
// role grants, all in one transaction.
func (s *PGStore) AcceptInvite(ctx context.Context, in AcceptInviteInput) (*AcceptedAccount, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("auth: accept invite begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var (
		inviteID   uuid.UUID
		email      string
		phone      string
		inviteName string
		employment string
		grantsJSON []byte
		invitedBy  *uuid.UUID
	)
	err = tx.QueryRow(ctx, `
		SELECT id, email, COALESCE(phone_e164, ''), COALESCE(full_name, ''),
		       employment_type, role_grants, invited_by
		FROM core.account_invites
		WHERE token_hash = $1 AND accepted_at IS NULL AND revoked_at IS NULL AND expires_at > $2
		FOR UPDATE`, in.TokenHash, in.Now).
		Scan(&inviteID, &email, &phone, &inviteName, &employment, &grantsJSON, &invitedBy)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrTokenInvalid
	}
	if err != nil {
		return nil, fmt.Errorf("auth: find invite: %w", err)
	}

	name := in.FullName
	if name == "" {
		name = inviteName
	}
	if name == "" {
		return nil, fmt.Errorf("auth: a full name is required to accept this invite")
	}
	contactPhone := in.Phone
	if contactPhone == "" {
		contactPhone = phone
	}

	personID, err := uuid.NewV7()
	if err != nil {
		return nil, fmt.Errorf("auth: person id: %w", err)
	}
	accountID, err := uuid.NewV7()
	if err != nil {
		return nil, fmt.Errorf("auth: account id: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO people.persons (id, full_name, work_email, phone_e164, employment_type, status)
		VALUES ($1, $2, $3, $4, $5, 'active')`,
		personID, name, email, nullIfEmpty(contactPhone), orDefault(employment, "full_time")); err != nil {
		return nil, fmt.Errorf("auth: create person: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO core.user_accounts (id, person_id, email, phone_e164, password_hash, mfa_required, status)
		VALUES ($1, $2, $3, $4, $5, $6, 'active')`,
		accountID, personID, email, nullIfEmpty(contactPhone), in.PasswordHash, in.MFARequired); err != nil {
		return nil, fmt.Errorf("auth: create account: %w", err)
	}

	grantedBy := accountID
	if invitedBy != nil {
		grantedBy = *invitedBy
	}

	var specs []GrantSpec
	if len(grantsJSON) > 0 {
		if err := json.Unmarshal(grantsJSON, &specs); err != nil {
			return nil, fmt.Errorf("auth: decode invite grants: %w", err)
		}
	}
	for _, spec := range specs {
		grantID, err := uuid.NewV7()
		if err != nil {
			return nil, fmt.Errorf("auth: grant id: %w", err)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO core.role_grants (id, user_id, role_key, scope_type, scope_id, granted_by)
			VALUES ($1, $2, $3, $4, $5, $6)`,
			grantID, accountID, spec.RoleKey, spec.ScopeType, spec.ScopeID, grantedBy); err != nil {
			return nil, fmt.Errorf("auth: assign role %q: %w", spec.RoleKey, err)
		}
	}

	if _, err := tx.Exec(ctx, `UPDATE core.account_invites SET accepted_at = $2 WHERE id = $1`, inviteID, in.Now); err != nil {
		return nil, fmt.Errorf("auth: mark invite accepted: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("auth: accept invite commit: %w", err)
	}
	return &AcceptedAccount{AccountID: accountID, PersonID: personID, Email: email}, nil
}

// ── helpers ──────────────────────────────────────────────────

func nullableAddr(a netip.Addr) any {
	if !a.IsValid() {
		return nil
	}
	return a
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}
