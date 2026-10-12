package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/safaristack/atlas/apps/api/internal/authz"
	"github.com/safaristack/atlas/apps/api/internal/platform/ed25519jwt"
)

// Additional errors returned by the service.
var (
	// ErrAccountLocked is returned after too many failed attempts.
	ErrAccountLocked = errors.New("auth: too many failed attempts; try again later")
	// ErrInvalidIdentifier is returned for a value that is neither an email
	// nor a plausible phone number.
	ErrInvalidIdentifier = errors.New("auth: identifier must be an email or phone number")
	// ErrInvalidTOTP is returned for a wrong or expired TOTP code.
	ErrInvalidTOTP = errors.New("auth: invalid one-time code")
	// ErrMFAAlreadyActive is returned when enrolment runs twice.
	ErrMFAAlreadyActive = errors.New("auth: multi-factor is already active")
	// ErrMFANotEnrolled is returned when verification runs before enrolment.
	ErrMFANotEnrolled = errors.New("auth: multi-factor has not been enrolled")
	// ErrInviteInvalid is returned for an expired, used or unknown invite.
	ErrInviteInvalid = errors.New("auth: invitation is not valid")
	// ErrPasswordTooShort is returned when a new password fails policy.
	ErrPasswordTooShort = errors.New("auth: password must be at least 12 characters")
)

// defaultPhoneCountry is prepended to a national number that omits its country
// code (Kenya, the default region for SafariStack). [DECIDE] D-country.
const defaultPhoneCountry = "+254"

// minPasswordLength is the local policy floor for new passwords (§13.1).
const minPasswordLength = 12

// ServiceConfig holds the auth knobs the service needs.
type ServiceConfig struct {
	SessionCookieName    string
	SessionIdleTimeout   time.Duration
	SessionAbsoluteTTL   time.Duration
	MFAIssuer            string
	MFAChallengeAudience string
	MFAChallengeTTL      time.Duration
	RealtimeIssuer       string
	RealtimeAudience     string
	RealtimeTTL          time.Duration
	ServiceAudience      string
	LoginMaxFailures     int
	LoginWindow          time.Duration
	RecoveryCodeCount    int
}

// Service applies the sign-in, session, MFA and token rules.
type Service struct {
	store  Store
	sealer *Sealer
	key    *ed25519jwt.Key
	cfg    ServiceConfig
	logger *slog.Logger
	now    func() time.Time
}

// NewService wires the dependencies. A nil logger is replaced by the default.
func NewService(store Store, sealer *Sealer, key *ed25519jwt.Key, cfg ServiceConfig, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	if cfg.RecoveryCodeCount == 0 {
		cfg.RecoveryCodeCount = 10
	}
	if cfg.MFAChallengeAudience == "" {
		cfg.MFAChallengeAudience = "atlas-mfa"
	}
	if cfg.MFAChallengeTTL == 0 {
		cfg.MFAChallengeTTL = 5 * time.Minute
	}
	if cfg.LoginWindow == 0 {
		cfg.LoginWindow = 15 * time.Minute
	}
	return &Service{
		store:  store,
		sealer: sealer,
		key:    key,
		cfg:    cfg,
		logger: logger,
		now:    time.Now,
	}
}

// SetClock overrides time, for tests.
func (s *Service) SetClock(now func() time.Time) { s.now = now }

// ── identifiers ──────────────────────────────────────────────

// ParseIdentifier resolves user input into the kind the store looks up.
func ParseIdentifier(raw string) (Identifier, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return Identifier{}, ErrInvalidIdentifier
	}
	if strings.Contains(trimmed, "@") {
		return Identifier{Kind: IdentifierEmail, Value: strings.ToLower(trimmed)}, nil
	}
	phone, err := NormalizePhone(trimmed)
	if err != nil {
		return Identifier{}, err
	}
	return Identifier{Kind: IdentifierPhone, Value: phone}, nil
}

// NormalizePhone returns an E.164 number, accepting a leading `+`, a national
// `0…`, or a bare country code. It is deliberately conservative: a value that
// does not look like a phone number is rejected rather than guessed at.
func NormalizePhone(raw string) (string, error) {
	var cleaned strings.Builder
	for _, r := range raw {
		switch {
		case r >= '0' && r <= '9':
			cleaned.WriteRune(r)
		case r == '+':
			cleaned.WriteRune(r)
		}
	}
	value := cleaned.String()
	switch {
	case value == "":
		return "", ErrInvalidIdentifier
	case strings.HasPrefix(value, "+"):
		// already international
	case strings.HasPrefix(value, "0"):
		value = defaultPhoneCountry + value[1:]
	case strings.HasPrefix(value, strings.TrimPrefix(defaultPhoneCountry, "+")):
		value = "+" + value
	default:
		value = "+" + value
	}
	if len(value) < 8 || len(value) > 16 {
		return "", ErrInvalidIdentifier
	}
	return value, nil
}

// ── sign-in ──────────────────────────────────────────────────

// LoginOutcome is either a started session or an MFA challenge.
type LoginOutcome struct {
	RequiresMFA        bool
	MFAToken           string
	EnrollmentRequired bool
	Session            *Session
	RawToken           string
}

// Login verifies an identifier (email or phone) and a password. When the
// account has MFA, it returns a challenge instead of a session.
func (s *Service) Login(ctx context.Context, identifier, password string, meta SessionMeta) (*LoginOutcome, error) {
	id, err := ParseIdentifier(identifier)
	if err != nil {
		return nil, ErrInvalidCredentials
	}

	if s.cfg.LoginMaxFailures > 0 {
		failures, err := s.store.CountRecentFailures(ctx, id.Value, s.now().Add(-s.cfg.LoginWindow))
		if err != nil {
			return nil, err
		}
		if failures >= s.cfg.LoginMaxFailures {
			return nil, ErrAccountLocked
		}
	}

	account, err := s.store.FindAccountByIdentifier(ctx, id)
	if err != nil {
		return nil, err
	}
	if account == nil || account.PasswordHash == "" {
		s.recordAttempt(ctx, id.Value, false, meta)
		return nil, ErrInvalidCredentials
	}

	ok, err := VerifyPassword(password, account.PasswordHash)
	if err != nil {
		// A malformed hash is an operator problem, not a user one: log it and
		// fail the sign-in without telling the caller which it was.
		s.logger.ErrorContext(ctx, "stored password hash is unreadable",
			"user_id", account.ID.String(), "error", err.Error())
		s.recordAttempt(ctx, id.Value, false, meta)
		return nil, ErrInvalidCredentials
	}
	if !ok {
		s.recordAttempt(ctx, id.Value, false, meta)
		return nil, ErrInvalidCredentials
	}
	if account.Status != "active" {
		s.recordAttempt(ctx, id.Value, false, meta)
		return nil, ErrAccountInactive
	}

	s.recordAttempt(ctx, id.Value, true, meta)
	if err := s.store.MarkLastLogin(ctx, account.ID, s.now()); err != nil {
		s.logger.WarnContext(ctx, "could not record last login", "user_id", account.ID.String(), "error", err.Error())
	}

	if account.MFARequired {
		token, err := s.issueMFAToken(account.ID)
		if err != nil {
			return nil, err
		}
		return &LoginOutcome{
			RequiresMFA:        true,
			MFAToken:           token,
			EnrollmentRequired: !account.MFAActivated,
		}, nil
	}

	session, raw, err := s.StartSession(ctx, account.ID, true, meta)
	if err != nil {
		return nil, err
	}
	return &LoginOutcome{Session: session, RawToken: raw}, nil
}

func (s *Service) recordAttempt(ctx context.Context, identifier string, ok bool, meta SessionMeta) {
	if err := s.store.RecordLoginAttempt(ctx, identifier, ok, meta.IP, meta.UserAgent); err != nil {
		s.logger.WarnContext(ctx, "could not record login attempt", "error", err.Error())
	}
}

// ── sessions ─────────────────────────────────────────────────

// StartSession mints an opaque token, stores its hash and returns the plaintext.
func (s *Service) StartSession(ctx context.Context, userID uuid.UUID, mfaVerified bool, meta SessionMeta) (*Session, string, error) {
	raw, err := newOpaqueToken()
	if err != nil {
		return nil, "", err
	}
	id, err := uuid.NewV7()
	if err != nil {
		return nil, "", fmt.Errorf("auth: session id: %w", err)
	}
	now := s.now()
	session := &Session{
		ID:          id,
		UserID:      userID,
		TokenHash:   HashToken(raw),
		MFAVerified: mfaVerified,
		IP:          meta.IP,
		UserAgent:   meta.UserAgent,
		CreatedAt:   now,
		LastSeenAt:  now,
		ExpiresAt:   now.Add(s.cfg.SessionAbsoluteTTL),
	}
	if err := s.store.CreateSession(ctx, session); err != nil {
		return nil, "", err
	}
	return session, raw, nil
}

// ResolveSession validates a cookie token, refreshes the idle timer and loads
// the principal. Every failure is a single ErrTokenInvalid.
func (s *Service) ResolveSession(ctx context.Context, rawToken string) (*Session, *authz.Principal, error) {
	if strings.TrimSpace(rawToken) == "" {
		return nil, nil, ErrTokenInvalid
	}
	session, err := s.store.SessionByTokenHash(ctx, HashToken(rawToken))
	if err != nil {
		return nil, nil, err
	}
	if session == nil {
		return nil, nil, ErrTokenInvalid
	}

	now := s.now()
	switch {
	case session.RevokedAt != nil:
		return nil, nil, ErrTokenInvalid
	case !now.Before(session.ExpiresAt):
		return nil, nil, ErrTokenInvalid
	case s.cfg.SessionIdleTimeout > 0 && now.Sub(session.LastSeenAt) > s.cfg.SessionIdleTimeout:
		return nil, nil, ErrTokenInvalid
	}

	if err := s.store.TouchSession(ctx, session.ID, now); err != nil {
		return nil, nil, err
	}
	principal, err := s.store.LoadPrincipal(ctx, session.UserID, session.ID)
	if err != nil {
		return nil, nil, err
	}
	return session, principal, nil
}

// Logout revokes one session, scoped to its owner.
func (s *Service) Logout(ctx context.Context, sessionID, userID uuid.UUID) error {
	if _, err := s.store.RevokeSession(ctx, sessionID, userID); err != nil {
		return err
	}
	return nil
}

// Sessions lists a user's active sessions.
func (s *Service) Sessions(ctx context.Context, userID uuid.UUID) ([]Session, error) {
	return s.store.ListSessions(ctx, userID)
}

// RevokeSession ends one of the caller's sessions.
func (s *Service) RevokeSession(ctx context.Context, sessionID, userID uuid.UUID) (bool, error) {
	return s.store.RevokeSession(ctx, sessionID, userID)
}

// RevokeAllSessions ends every session a user holds.
func (s *Service) RevokeAllSessions(ctx context.Context, userID uuid.UUID) (int64, error) {
	return s.store.RevokeAllSessions(ctx, userID)
}

// ── multi-factor ─────────────────────────────────────────────

func (s *Service) issueMFAToken(userID uuid.UUID) (string, error) {
	now := s.now()
	return s.key.Sign(ed25519jwt.Claims{
		Issuer:    s.cfg.RealtimeIssuer,
		Subject:   "user:" + userID.String(),
		Audience:  s.cfg.MFAChallengeAudience,
		IssuedAt:  now,
		ExpiresAt: now.Add(s.cfg.MFAChallengeTTL),
		Purpose:   "mfa",
	})
}

// ResolveMFAChallenge returns the user id behind a challenge token.
func (s *Service) ResolveMFAChallenge(token string) (uuid.UUID, error) {
	claims, err := ed25519jwt.Verify(token, s.key.PublicKey())
	if err != nil {
		return uuid.Nil, ErrTokenInvalid
	}
	aud, _ := claims["aud"].(string)
	purpose, _ := claims["purpose"].(string)
	if aud != s.cfg.MFAChallengeAudience || purpose != "mfa" {
		return uuid.Nil, ErrTokenInvalid
	}
	sub, _ := claims["sub"].(string)
	userID, err := uuid.Parse(strings.TrimPrefix(sub, "user:"))
	if err != nil {
		return uuid.Nil, ErrTokenInvalid
	}
	return userID, nil
}

// PrepareMFA generates and seals a pending TOTP secret for a user, returning
// the secret and its provisioning URI. The user confirms with ConfirmMFA.
func (s *Service) PrepareMFA(ctx context.Context, userID uuid.UUID, label string) (string, string, error) {
	account, err := s.store.FindAccountByID(ctx, userID)
	if err != nil {
		return "", "", err
	}
	if account == nil {
		return "", "", ErrTokenInvalid
	}
	if account.MFAActivated {
		return "", "", ErrMFAAlreadyActive
	}

	secret, err := GenerateTOTPSecret()
	if err != nil {
		return "", "", err
	}
	sealed, err := s.sealer.Seal([]byte(secret))
	if err != nil {
		return "", "", err
	}
	if err := s.store.SetMFASecret(ctx, userID, sealed); err != nil {
		return "", "", err
	}

	accountLabel := label
	if accountLabel == "" {
		accountLabel = account.Email
	}
	return secret, TOTPURI(s.cfg.MFAIssuer, accountLabel, secret), nil
}

// ConfirmMFA verifies the first TOTP code, activates MFA and issues recovery
// codes. It is idempotent only in the sense that a second call fails cleanly.
func (s *Service) ConfirmMFA(ctx context.Context, userID uuid.UUID, code string) ([]string, error) {
	account, err := s.store.FindAccountByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if account == nil {
		return nil, ErrTokenInvalid
	}
	if account.MFAActivated {
		return nil, ErrMFAAlreadyActive
	}
	if len(account.MFASecretEnc) == 0 {
		return nil, ErrMFANotEnrolled
	}

	secret, err := s.sealer.Open(account.MFASecretEnc)
	if err != nil {
		return nil, fmt.Errorf("auth: open mfa secret: %w", err)
	}
	if !VerifyTOTP(string(secret), code, s.now()) {
		return nil, ErrInvalidTOTP
	}

	codes, hashes, err := generateRecoveryCodes(s.cfg.RecoveryCodeCount)
	if err != nil {
		return nil, err
	}
	if err := s.store.ActivateMFA(ctx, userID, s.now(), hashes); err != nil {
		return nil, err
	}
	return codes, nil
}

// VerifyMFA completes a login challenge with a TOTP code or a recovery code.
func (s *Service) VerifyMFA(ctx context.Context, challengeToken, code string, meta SessionMeta) (*Session, string, error) {
	userID, err := s.ResolveMFAChallenge(challengeToken)
	if err != nil {
		return nil, "", err
	}
	account, err := s.store.FindAccountByID(ctx, userID)
	if err != nil {
		return nil, "", err
	}
	if account == nil {
		return nil, "", ErrTokenInvalid
	}
	if !account.MFAActivated || len(account.MFASecretEnc) == 0 {
		return nil, "", ErrMFANotEnrolled
	}

	secret, err := s.sealer.Open(account.MFASecretEnc)
	if err != nil {
		return nil, "", fmt.Errorf("auth: open mfa secret: %w", err)
	}

	trimmed := strings.TrimSpace(code)
	if !VerifyTOTP(string(secret), trimmed, s.now()) {
		used, err := s.store.UseRecoveryCode(ctx, userID, recoveryHash(trimmed))
		if err != nil {
			return nil, "", err
		}
		if !used {
			return nil, "", ErrInvalidTOTP
		}
	}

	return s.StartSession(ctx, userID, true, meta)
}

// ── tokens ───────────────────────────────────────────────────

// RealtimeToken mints the 5-minute Ed25519 token Phoenix verifies via JWKS.
func (s *Service) RealtimeToken(principal *authz.Principal) (string, time.Time, error) {
	if !principal.IsUser() {
		return "", time.Time{}, errors.New("auth: realtime tokens are issued to users")
	}
	now := s.now()
	expires := now.Add(s.cfg.RealtimeTTL)

	scopes := []string{"user:" + principal.UserID.String()}
	for _, role := range principal.Roles {
		scopes = append(scopes, "role:"+role)
	}

	token, err := s.key.Sign(ed25519jwt.Claims{
		Issuer:    s.cfg.RealtimeIssuer,
		Subject:   principal.Subject,
		Audience:  s.cfg.RealtimeAudience,
		IssuedAt:  now,
		ExpiresAt: expires,
		Scopes:    scopes,
	})
	if err != nil {
		return "", time.Time{}, err
	}
	return token, expires, nil
}

// ResolveServiceToken verifies an internal token MintServiceToken produced.
func (s *Service) ResolveServiceToken(ctx context.Context, token string) (*authz.Principal, error) {
	claims, err := ed25519jwt.Verify(token, s.key.PublicKey())
	if err != nil {
		return nil, ErrTokenInvalid
	}
	if aud, _ := claims["aud"].(string); aud != s.cfg.ServiceAudience {
		return nil, ErrTokenInvalid
	}
	sub, _ := claims["sub"].(string)
	if !strings.HasPrefix(sub, "svc:") {
		return nil, ErrTokenInvalid
	}

	identity, err := s.store.FindService(ctx, sub)
	if err != nil {
		return nil, err
	}
	if identity == nil || identity.DisabledAt != nil {
		return nil, ErrTokenInvalid
	}

	permissions := make(map[string]struct{}, len(identity.Permissions))
	for _, p := range identity.Permissions {
		permissions[p] = struct{}{}
	}
	return &authz.Principal{Kind: authz.KindService, Subject: sub, Permissions: permissions}, nil
}

// MintServiceToken issues an internal token for a service identity. It backs
// `atlas-cli` and the smoke test; production callers can present the same
// subject so the route layer treats them identically.
func (s *Service) MintServiceToken(ctx context.Context, serviceID string, fallbackPermissions []string) (string, time.Time, error) {
	permissions := fallbackPermissions
	if identity, err := s.store.FindService(ctx, serviceID); err == nil && identity != nil {
		permissions = identity.Permissions
	}
	if len(permissions) == 0 {
		return "", time.Time{}, fmt.Errorf("auth: service %q has no permissions", serviceID)
	}

	now := s.now()
	expires := now.Add(s.cfg.RealtimeTTL)
	token, err := s.key.Sign(ed25519jwt.Claims{
		Issuer:    s.cfg.RealtimeIssuer,
		Subject:   serviceID,
		Audience:  s.cfg.ServiceAudience,
		IssuedAt:  now,
		ExpiresAt: expires,
		Scopes:    permissions,
	})
	if err != nil {
		return "", time.Time{}, err
	}
	return token, expires, nil
}

// JWKS is the public key set Elixir fetches to verify realtime tokens.
func (s *Service) JWKS() ed25519jwt.JWKS { return s.key.JWKS() }

// ── invites ──────────────────────────────────────────────────

// Invite creates a single-use invitation. The plaintext token is returned here
// and never stored.
func (s *Service) Invite(ctx context.Context, email, fullName, employment string, grants []GrantSpec, invitedBy *uuid.UUID, ttl time.Duration) (string, *Invite, error) {
	trimmed := strings.ToLower(strings.TrimSpace(email))
	if !strings.Contains(trimmed, "@") {
		return "", nil, ErrInvalidIdentifier
	}
	raw, err := newOpaqueToken()
	if err != nil {
		return "", nil, err
	}
	id, err := uuid.NewV7()
	if err != nil {
		return "", nil, err
	}
	now := s.now()
	if ttl <= 0 {
		ttl = 72 * time.Hour
	}
	invite := &Invite{
		ID:         id,
		Email:      trimmed,
		FullName:   strings.TrimSpace(fullName),
		Employment: orDefault(employment, "full_time"),
		RoleGrants: grants,
		TokenHash:  HashToken(raw),
		InvitedBy:  invitedBy,
		ExpiresAt:  now.Add(ttl),
		CreatedAt:  now,
	}
	if err := s.store.CreateInvite(ctx, invite); err != nil {
		return "", nil, err
	}
	return raw, invite, nil
}

// AcceptInvite turns a token plus a chosen password into an account.
func (s *Service) AcceptInvite(ctx context.Context, token, fullName, password string) (*AcceptedAccount, error) {
	if strings.TrimSpace(token) == "" {
		return nil, ErrInviteInvalid
	}
	if len(password) < minPasswordLength {
		return nil, ErrPasswordTooShort
	}

	invite, err := s.store.InviteByTokenHash(ctx, HashToken(token))
	if err != nil {
		return nil, err
	}
	if invite == nil || invite.AcceptedAt != nil || invite.RevokedAt != nil || !s.now().Before(invite.ExpiresAt) {
		return nil, ErrInviteInvalid
	}

	hash, err := HashPassword(password, DefaultArgon2Params())
	if err != nil {
		return nil, err
	}

	name := strings.TrimSpace(fullName)
	if name == "" {
		name = invite.FullName
	}

	account, err := s.store.AcceptInvite(ctx, AcceptInviteInput{
		TokenHash:    HashToken(token),
		FullName:     name,
		PasswordHash: hash,
		MFARequired:  false,
		Now:          s.now(),
	})
	if err != nil {
		return nil, err
	}
	return account, nil
}

// ── helpers ──────────────────────────────────────────────────

// HashToken returns the SHA-256 of a bearer token. Only hashes are stored.
func HashToken(raw string) []byte {
	sum := sha256.Sum256([]byte(raw))
	return sum[:]
}

func recoveryHash(code string) []byte {
	normalized := strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(code), "-", ""))
	return HashToken(normalized)
}

func newOpaqueToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("auth: read random token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func generateRecoveryCodes(count int) ([]string, [][]byte, error) {
	codes := make([]string, 0, count)
	hashes := make([][]byte, 0, count)
	encoder := base32.StdEncoding.WithPadding(base32.NoPadding)
	for i := 0; i < count; i++ {
		buf := make([]byte, 10)
		if _, err := rand.Read(buf); err != nil {
			return nil, nil, fmt.Errorf("auth: recovery code: %w", err)
		}
		code := encoder.EncodeToString(buf)
		codes = append(codes, code)
		hashes = append(hashes, recoveryHash(code))
	}
	return codes, hashes, nil
}
