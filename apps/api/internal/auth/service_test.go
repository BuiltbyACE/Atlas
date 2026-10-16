package auth

import (
	"context"
	"encoding/hex"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/safaristack/atlas/apps/api/internal/authz"
	"github.com/safaristack/atlas/apps/api/internal/platform/ed25519jwt"
)

// fakeStore is an in-memory auth.Store for exercising the service.
type fakeStore struct {
	accounts map[string]*Account // key: email, lowercase
	sessions map[string]*Session // key: hex(token hash)
	services map[string]*ServiceIdentity
	recovery map[string]bool // key: hex(recovery hash), true while unused
	attempts []time.Time

	// principalErr, when set, is returned by LoadPrincipal (for the
	// authorisation-check tests). permissions overrides the default set.
	principalErr error
	permissions  map[string]struct{}
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		accounts: map[string]*Account{},
		sessions: map[string]*Session{},
		services: map[string]*ServiceIdentity{},
		recovery: map[string]bool{},
	}
}

func (f *fakeStore) put(a *Account) *Account {
	f.accounts[strings.ToLower(a.Email)] = a
	return a
}

func (f *fakeStore) FindAccountByIdentifier(ctx context.Context, id Identifier) (*Account, error) {
	for _, a := range f.accounts {
		switch id.Kind {
		case IdentifierEmail:
			if strings.EqualFold(a.Email, id.Value) {
				return a, nil
			}
		case IdentifierPhone:
			if a.Phone == id.Value {
				return a, nil
			}
		}
	}
	return nil, nil
}

func (f *fakeStore) FindAccountByID(ctx context.Context, userID uuid.UUID) (*Account, error) {
	for _, a := range f.accounts {
		if a.ID == userID {
			return a, nil
		}
	}
	return nil, nil
}

func (f *fakeStore) LoadPrincipal(ctx context.Context, userID, sessionID uuid.UUID) (*authz.Principal, error) {
	if f.principalErr != nil {
		return nil, f.principalErr
	}
	permissions := f.permissions
	if permissions == nil {
		permissions = map[string]struct{}{"core.notification.read": {}}
	}
	return &authz.Principal{
		Kind:        authz.KindUser,
		Subject:     "user:" + userID.String(),
		UserID:      userID,
		SessionID:   sessionID,
		Roles:       []string{"staff"},
		Permissions: permissions,
	}, nil
}

func (f *fakeStore) CreateSession(ctx context.Context, s *Session) error {
	f.sessions[hex.EncodeToString(s.TokenHash)] = s
	return nil
}

func (f *fakeStore) SessionByTokenHash(ctx context.Context, hash []byte) (*Session, error) {
	return f.sessions[hex.EncodeToString(hash)], nil
}

func (f *fakeStore) TouchSession(ctx context.Context, id uuid.UUID, seen time.Time) error {
	for _, s := range f.sessions {
		if s.ID == id {
			s.LastSeenAt = seen
		}
	}
	return nil
}

func (f *fakeStore) RevokeSession(ctx context.Context, id, userID uuid.UUID) (bool, error) {
	for _, s := range f.sessions {
		if s.ID == id && s.UserID == userID {
			now := time.Now()
			s.RevokedAt = &now
			return true, nil
		}
	}
	return false, nil
}

func (f *fakeStore) RevokeAllSessions(ctx context.Context, userID uuid.UUID) (int64, error) {
	var n int64
	now := time.Now()
	for _, s := range f.sessions {
		if s.UserID == userID && s.RevokedAt == nil {
			s.RevokedAt = &now
			n++
		}
	}
	return n, nil
}

func (f *fakeStore) ListSessions(ctx context.Context, userID uuid.UUID) ([]Session, error) {
	var out []Session
	for _, s := range f.sessions {
		if s.UserID == userID {
			out = append(out, *s)
		}
	}
	return out, nil
}

func (f *fakeStore) MarkLastLogin(ctx context.Context, userID uuid.UUID, at time.Time) error {
	return nil
}

func (f *fakeStore) RecordLoginAttempt(ctx context.Context, identifier string, succeeded bool, ip netip.Addr, userAgent string) error {
	if !succeeded {
		f.attempts = append(f.attempts, time.Now())
	}
	return nil
}

func (f *fakeStore) CountRecentFailures(ctx context.Context, identifier string, since time.Time) (int, error) {
	n := 0
	for _, at := range f.attempts {
		if at.After(since) {
			n++
		}
	}
	return n, nil
}

func (f *fakeStore) SetMFASecret(ctx context.Context, userID uuid.UUID, sealed []byte) error {
	for _, a := range f.accounts {
		if a.ID == userID {
			a.MFASecretEnc = sealed
			a.MFARequired = true
		}
	}
	return nil
}

func (f *fakeStore) ActivateMFA(ctx context.Context, userID uuid.UUID, at time.Time, recoveryHashes [][]byte) error {
	for _, a := range f.accounts {
		if a.ID == userID {
			a.MFAActivated = true
		}
	}
	return nil
}

func (f *fakeStore) UseRecoveryCode(ctx context.Context, userID uuid.UUID, hash []byte) (bool, error) {
	key := hex.EncodeToString(hash)
	if !f.recovery[key] {
		return false, nil
	}
	f.recovery[key] = false // each code works once
	return true, nil
}

func (f *fakeStore) FindService(ctx context.Context, id string) (*ServiceIdentity, error) {
	return f.services[id], nil
}

func (f *fakeStore) CreateInvite(ctx context.Context, inv *Invite) error { return nil }

func (f *fakeStore) InviteByTokenHash(ctx context.Context, hash []byte) (*Invite, error) {
	return nil, nil
}

func (f *fakeStore) AcceptInvite(ctx context.Context, in AcceptInviteInput) (*AcceptedAccount, error) {
	return nil, nil
}

// ── harness ──────────────────────────────────────────────────

type harness struct {
	service *Service
	store   *fakeStore
	sealer  *Sealer
	now     time.Time
}

func newHarness(t *testing.T) *harness {
	t.Helper()

	encoded, err := GenerateEncryptionKey()
	if err != nil {
		t.Fatalf("GenerateEncryptionKey: %v", err)
	}
	sealer, err := NewSealer(encoded)
	if err != nil {
		t.Fatalf("NewSealer: %v", err)
	}
	key, err := ed25519jwt.Generate()
	if err != nil {
		t.Fatalf("ed25519jwt.Generate: %v", err)
	}

	store := newFakeStore()
	// The service clock is pinned, but ed25519jwt.Verify compares token expiry
	// with the wall clock, so the pinned time must be real.
	now := time.Now().UTC().Truncate(time.Second)

	service := NewService(store, sealer, key, ServiceConfig{
		SessionCookieName:    "atlas_session",
		SessionIdleTimeout:   12 * time.Hour,
		SessionAbsoluteTTL:   7 * 24 * time.Hour,
		MFAIssuer:            "SafariStack Atlas",
		MFAChallengeAudience: "atlas-mfa",
		MFAChallengeTTL:      5 * time.Minute,
		RealtimeIssuer:       "atlas",
		RealtimeAudience:     "atlas-realtime",
		RealtimeTTL:          5 * time.Minute,
		ServiceAudience:      "atlas-internal",
		LoginMaxFailures:     3,
		LoginWindow:          15 * time.Minute,
	}, nil)
	service.SetClock(func() time.Time { return now })

	return &harness{service: service, store: store, sealer: sealer, now: now}
}

func (h *harness) addAccount(t *testing.T, email, phone, password string, mfa bool) *Account {
	t.Helper()
	hash, err := HashPassword(password, testArgon2Params())
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	account := &Account{
		ID:           uuid.New(),
		PersonID:     uuid.New(),
		Email:        email,
		Phone:        phone,
		PasswordHash: hash,
		Status:       "active",
		MFARequired:  mfa,
		DisplayName:  "Test Person",
	}
	return h.store.put(account)
}

// addRecoveryCode registers a recovery code as unused for the account under
// test, mirroring core.recovery_codes.
func (h *harness) addRecoveryCode(code string) {
	h.store.recovery[hex.EncodeToString(recoveryHash(code))] = true
}

// ── tests ────────────────────────────────────────────────────

func TestLoginWithEmail(t *testing.T) {
	h := newHarness(t)
	h.addAccount(t, "asha@safaristack.co.ke", "+254700000001", "correct horse battery", false)

	out, err := h.service.Login(context.Background(), "asha@safaristack.co.ke", "correct horse battery", SessionMeta{})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if out.RequiresMFA {
		t.Fatal("did not expect an MFA challenge")
	}
	if out.RawToken == "" || out.Session == nil {
		t.Fatal("expected a session and a raw token")
	}
}

func TestLoginWithPhone(t *testing.T) {
	h := newHarness(t)
	h.addAccount(t, "asha@safaristack.co.ke", "+254700000001", "correct horse battery", false)

	out, err := h.service.Login(context.Background(), "0700 000 001", "correct horse battery", SessionMeta{})
	if err != nil {
		t.Fatalf("Login by national number: %v", err)
	}
	if out.Session == nil {
		t.Fatal("expected a session when signing in by phone")
	}
}

func TestLoginRejectsWrongPassword(t *testing.T) {
	h := newHarness(t)
	h.addAccount(t, "asha@safaristack.co.ke", "", "correct horse battery", false)

	if _, err := h.service.Login(context.Background(), "asha@safaristack.co.ke", "nope", SessionMeta{}); err == nil {
		t.Fatal("expected the wrong password to be rejected")
	}
}

func TestLoginThrottleAfterFailures(t *testing.T) {
	h := newHarness(t)
	h.addAccount(t, "asha@safaristack.co.ke", "", "correct horse battery", false)

	ctx := context.Background()
	for i := 0; i < 3; i++ {
		_, _ = h.service.Login(ctx, "asha@safaristack.co.ke", "nope", SessionMeta{})
	}
	_, err := h.service.Login(ctx, "asha@safaristack.co.ke", "correct horse battery", SessionMeta{})
	if err != ErrAccountLocked {
		t.Fatalf("err = %v, want ErrAccountLocked", err)
	}
}

func TestMFAChallengeAndVerify(t *testing.T) {
	h := newHarness(t)
	account := h.addAccount(t, "ceo@safaristack.co.ke", "", "correct horse battery", true)

	secret, err := GenerateTOTPSecret()
	if err != nil {
		t.Fatalf("GenerateTOTPSecret: %v", err)
	}
	sealed, err := h.sealer.Seal([]byte(secret))
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	account.MFASecretEnc = sealed
	account.MFAActivated = true

	out, err := h.service.Login(context.Background(), "ceo@safaristack.co.ke", "correct horse battery", SessionMeta{})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if !out.RequiresMFA || out.MFAToken == "" {
		t.Fatalf("expected an MFA challenge, got %+v", out)
	}

	key, err := decodeTOTPSecret(secret)
	if err != nil {
		t.Fatalf("decode secret: %v", err)
	}
	code := codeAt(key, h.now, totpDigits)

	session, raw, err := h.service.VerifyMFA(context.Background(), out.MFAToken, code, SessionMeta{})
	if err != nil {
		t.Fatalf("VerifyMFA: %v", err)
	}
	if session == nil || raw == "" {
		t.Fatal("expected a session after MFA verification")
	}
}

// A recovery code must still work when the stored TOTP secret cannot be
// decrypted — a rotated or missing ATLAS_ENCRYPTION_KEY. Before this, that case
// returned a 500 and locked the account out entirely.
func TestVerifyMFAFallsBackToRecoveryCode(t *testing.T) {
	h := newHarness(t)
	account := h.addAccount(t, "ceo@safaristack.co.ke", "", "correct horse battery", true)
	account.MFAActivated = true
	// Bytes that are not a valid sealed secret: sealer.Open fails.
	account.MFASecretEnc = []byte("not a valid sealed mfa secret")
	h.addRecoveryCode("ABCDEFGHIJKLMNOP")

	token, err := h.service.issueMFAToken(account.ID)
	if err != nil {
		t.Fatalf("issueMFAToken: %v", err)
	}

	session, raw, err := h.service.VerifyMFA(context.Background(), token, "abcd-efgh ijkl mnop", SessionMeta{})
	if err != nil {
		t.Fatalf("VerifyMFA with recovery code: %v", err)
	}
	if session == nil || raw == "" {
		t.Fatal("expected a session after recovery-code verification")
	}
}

// With the secret unreadable and an unknown recovery code, the failure is a
// normal invalid-code error — never a server error.
func TestVerifyMFAWrongRecoveryCodeIsRejected(t *testing.T) {
	h := newHarness(t)
	account := h.addAccount(t, "ceo@safaristack.co.ke", "", "correct horse battery", true)
	account.MFAActivated = true
	account.MFASecretEnc = []byte("not a valid sealed mfa secret")
	h.addRecoveryCode("ABCDEFGHIJKLMNOP")

	token, err := h.service.issueMFAToken(account.ID)
	if err != nil {
		t.Fatalf("issueMFAToken: %v", err)
	}

	_, _, err = h.service.VerifyMFA(context.Background(), token, "ZZZZZZZZZZZZZZZZ", SessionMeta{})
	if err != ErrInvalidTOTP {
		t.Fatalf("err = %v, want ErrInvalidTOTP", err)
	}
}

func TestSessionCookieInvalidForOtherToken(t *testing.T) {
	h := newHarness(t)
	h.addAccount(t, "asha@safaristack.co.ke", "", "correct horse battery", false)

	if _, _, err := h.service.ResolveSession(context.Background(), "not-a-real-token"); err == nil {
		t.Fatal("expected an unknown session token to be rejected")
	}
}
