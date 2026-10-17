package auth

import (
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/safaristack/atlas/apps/api/internal/authz"
	"github.com/safaristack/atlas/apps/api/internal/platform/httpx"
)

// HTTPConfig are the cookie knobs the handlers need.
type HTTPConfig struct {
	CookieName    string
	CookiePath    string
	SecureCookies bool
}

// Handler serves the §11 authentication surface.
type Handler struct {
	service *Service
	cfg     HTTPConfig
	logger  *slog.Logger
}

// NewHandler builds the auth HTTP handler.
func NewHandler(service *Service, cfg HTTPConfig, logger *slog.Logger) *Handler {
	if logger == nil {
		logger = slog.Default()
	}
	if cfg.CookiePath == "" {
		cfg.CookiePath = "/"
	}
	return &Handler{service: service, cfg: cfg, logger: logger}
}

// ── request and response shapes ──────────────────────────────

type loginRequest struct {
	Identifier string `json:"identifier"`
	Password   string `json:"password"`
}

type loginResponse struct {
	RequiresMFA        bool          `json:"requires_mfa"`
	EnrollmentRequired bool          `json:"enrollment_required,omitempty"`
	MFAToken           string        `json:"mfa_token,omitempty"`
	User               *userResponse `json:"user,omitempty"`
}

type userResponse struct {
	ID       string   `json:"id"`
	Email    string   `json:"email"`
	Name     string   `json:"name"`
	Subject  string   `json:"subject"`
	PersonID string   `json:"person_id,omitempty"`
	Roles    []string `json:"roles"`
}

type mfaVerifyRequest struct {
	MFAToken string `json:"mfa_token"`
	Code     string `json:"code"`
}

type mfaEnrollResponse struct {
	Secret     string `json:"secret"`
	OTPAuthURL string `json:"otpauth_url"`
}

type mfaActivateResponse struct {
	RecoveryCodes []string `json:"recovery_codes"`
}

type sessionResponse struct {
	ID         string    `json:"id"`
	Current    bool      `json:"current"`
	IP         string    `json:"ip,omitempty"`
	UserAgent  string    `json:"user_agent,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	LastSeenAt time.Time `json:"last_seen_at"`
	ExpiresAt  time.Time `json:"expires_at"`
}

type realtimeResponse struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
}

type inviteRequest struct {
	Email      string      `json:"email"`
	FullName   string      `json:"full_name"`
	Employment string      `json:"employment_type"`
	RoleGrants []GrantSpec `json:"role_grants"`
	TTLHours   int         `json:"ttl_hours"`
}

type inviteResponse struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
	Email     string    `json:"email"`
}

type acceptInviteRequest struct {
	Token    string `json:"token"`
	FullName string `json:"full_name"`
	Password string `json:"password"`
}

// ── handlers ─────────────────────────────────────────────────

// Login handles POST /api/v1/auth/login.
func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		writeDecodeError(w, r, err, "The request body must be a JSON object with identifier and password.")
		return
	}

	outcome, err := h.service.Login(r.Context(), req.Identifier, req.Password, sessionMeta(r))
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	if outcome.RequiresMFA {
		httpx.WriteJSON(w, http.StatusOK, loginResponse{
			RequiresMFA:        true,
			EnrollmentRequired: outcome.EnrollmentRequired,
			MFAToken:           outcome.MFAToken,
		})
		return
	}

	h.setSessionCookie(w, outcome.RawToken, outcome.Session.ExpiresAt)
	principal, err := h.service.store.LoadPrincipal(r.Context(), outcome.Session.UserID, outcome.Session.ID)
	if err != nil {
		h.logger.ErrorContext(r.Context(), "load principal after login", "error", err.Error())
		httpx.WriteJSON(w, http.StatusOK, loginResponse{})
		return
	}
	httpx.WriteJSON(w, http.StatusOK, loginResponse{User: userView(principal)})
}

// MFAVerify handles POST /api/v1/auth/mfa/verify.
func (h *Handler) MFAVerify(w http.ResponseWriter, r *http.Request) {
	var req mfaVerifyRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		writeDecodeError(w, r, err, "The request body must be a JSON object with mfa_token and code.")
		return
	}

	session, raw, err := h.service.VerifyMFA(r.Context(), req.MFAToken, req.Code, sessionMeta(r))
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	h.setSessionCookie(w, raw, session.ExpiresAt)
	principal, err := h.service.store.LoadPrincipal(r.Context(), session.UserID, session.ID)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, loginResponse{User: userView(principal)})
}

// MFAEnroll handles POST /api/v1/auth/mfa/enroll. It is authenticated by the
// MFA challenge token from login, so enrolment happens before the session does.
func (h *Handler) MFAEnroll(w http.ResponseWriter, r *http.Request) {
	var req mfaVerifyRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		writeDecodeError(w, r, err, "The request body must include the mfa_token.")
		return
	}

	userID, err := h.service.ResolveMFAChallenge(req.MFAToken)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	secret, uri, err := h.service.PrepareMFA(r.Context(), userID, "")
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, mfaEnrollResponse{Secret: secret, OTPAuthURL: uri})
}

// MFAActivate handles POST /api/v1/auth/mfa/activate.
func (h *Handler) MFAActivate(w http.ResponseWriter, r *http.Request) {
	var req mfaVerifyRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		writeDecodeError(w, r, err, "The request body must be a JSON object with mfa_token and code.")
		return
	}

	userID, err := h.service.ResolveMFAChallenge(req.MFAToken)
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	codes, err := h.service.ConfirmMFA(r.Context(), userID, req.Code)
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	session, raw, err := h.service.StartSession(r.Context(), userID, true, sessionMeta(r))
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	h.setSessionCookie(w, raw, session.ExpiresAt)
	httpx.WriteJSON(w, http.StatusCreated, mfaActivateResponse{RecoveryCodes: codes})
}

// Logout handles POST /api/v1/auth/logout.
func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	principal, ok := authz.PrincipalFrom(r.Context())
	if !ok || !principal.IsUser() {
		httpx.WriteProblem(w, r, httpx.Unauthenticated("Authentication is required."))
		return
	}
	if err := h.service.Logout(r.Context(), principal.SessionID, principal.UserID); err != nil {
		h.writeError(w, r, err)
		return
	}
	h.clearSessionCookie(w)
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "signed_out"})
}

// Me handles GET /api/v1/auth/me.
func (h *Handler) Me(w http.ResponseWriter, r *http.Request) {
	principal, ok := authz.PrincipalFrom(r.Context())
	if !ok {
		httpx.WriteProblem(w, r, httpx.Unauthenticated("Authentication is required."))
		return
	}
	httpx.WriteJSON(w, http.StatusOK, userView(principal))
}

// Heartbeat handles POST /api/v1/auth/heartbeat: a cheap, authenticated ping an
// open client sends while a tab is visible. The request is what refreshes the
// session's last_seen_at — the gate resolves and touches the session before the
// handler runs — so this only confirms the caller is a signed-in person (a
// service token has no session to keep alive). It backs the people activity
// view, which reads "online" as a session seen within the presence window.
func (h *Handler) Heartbeat(w http.ResponseWriter, r *http.Request) {
	principal, ok := authz.PrincipalFrom(r.Context())
	if !ok || !principal.IsUser() {
		httpx.WriteProblem(w, r, httpx.Unauthenticated("Authentication is required."))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Sessions handles GET /api/v1/auth/sessions.
func (h *Handler) Sessions(w http.ResponseWriter, r *http.Request) {
	principal, ok := authz.PrincipalFrom(r.Context())
	if !ok || !principal.IsUser() {
		httpx.WriteProblem(w, r, httpx.Unauthenticated("Authentication is required."))
		return
	}

	sessions, err := h.service.Sessions(r.Context(), principal.UserID)
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	views := make([]sessionResponse, 0, len(sessions))
	for _, s := range sessions {
		ip := ""
		if s.IP.IsValid() {
			ip = s.IP.String()
		}
		views = append(views, sessionResponse{
			ID:         s.ID.String(),
			Current:    s.ID == principal.SessionID,
			IP:         ip,
			UserAgent:  s.UserAgent,
			CreatedAt:  s.CreatedAt,
			LastSeenAt: s.LastSeenAt,
			ExpiresAt:  s.ExpiresAt,
		})
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"sessions": views})
}

// RevokeSession handles DELETE /api/v1/auth/sessions/{id}.
func (h *Handler) RevokeSession(w http.ResponseWriter, r *http.Request) {
	principal, ok := authz.PrincipalFrom(r.Context())
	if !ok || !principal.IsUser() {
		httpx.WriteProblem(w, r, httpx.Unauthenticated("Authentication is required."))
		return
	}

	id, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		httpx.WriteProblem(w, r, httpx.BadRequest("The session id must be a UUID."))
		return
	}

	revoked, err := h.service.RevokeSession(r.Context(), id, principal.UserID)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	if !revoked {
		httpx.WriteProblem(w, r, httpx.NotFound("No such session."))
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "revoked"})
}

// RevokeAllSessions handles POST /api/v1/auth/sessions/revoke-all.
func (h *Handler) RevokeAllSessions(w http.ResponseWriter, r *http.Request) {
	principal, ok := authz.PrincipalFrom(r.Context())
	if !ok || !principal.IsUser() {
		httpx.WriteProblem(w, r, httpx.Unauthenticated("Authentication is required."))
		return
	}

	count, err := h.service.RevokeAllSessions(r.Context(), principal.UserID)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	h.clearSessionCookie(w)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"revoked": count})
}

// RealtimeToken handles POST /api/v1/auth/realtime-token.
func (h *Handler) RealtimeToken(w http.ResponseWriter, r *http.Request) {
	principal, ok := authz.PrincipalFrom(r.Context())
	if !ok || !principal.IsUser() {
		httpx.WriteProblem(w, r, httpx.Unauthenticated("Authentication is required."))
		return
	}

	token, expires, err := h.service.RealtimeToken(principal)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, realtimeResponse{Token: token, ExpiresAt: expires})
}

// CreateInvite handles POST /api/v1/auth/invites.
func (h *Handler) CreateInvite(w http.ResponseWriter, r *http.Request) {
	principal, _ := authz.PrincipalFrom(r.Context())

	var req inviteRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		writeDecodeError(w, r, err, "The request body must be an invite object.")
		return
	}

	var ttl time.Duration
	if req.TTLHours > 0 {
		ttl = time.Duration(req.TTLHours) * time.Hour
	}

	inviter := principal.UserID
	token, invite, err := h.service.Invite(r.Context(), req.Email, req.FullName, req.Employment, req.RoleGrants, &inviter, ttl)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, inviteResponse{Token: token, ExpiresAt: invite.ExpiresAt, Email: invite.Email})
}

// AcceptInvite handles POST /api/v1/auth/invites/accept.
func (h *Handler) AcceptInvite(w http.ResponseWriter, r *http.Request) {
	var req acceptInviteRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		writeDecodeError(w, r, err, "The request body must be an invite acceptance object.")
		return
	}

	account, err := h.service.AcceptInvite(r.Context(), req.Token, req.FullName, req.Password)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, map[string]string{
		"id":    account.AccountID.String(),
		"email": account.Email,
	})
}

// JWKS handles GET /internal/v1/.well-known/jwks.json.
func (h *Handler) JWKS(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "public, max-age=300")
	httpx.WriteJSON(w, http.StatusOK, h.service.JWKS())
}

// ── helpers ──────────────────────────────────────────────────

func (h *Handler) setSessionCookie(w http.ResponseWriter, value string, expires time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     h.cfg.CookieName,
		Value:    value,
		Path:     h.cfg.CookiePath,
		HttpOnly: true,
		Secure:   h.cfg.SecureCookies,
		SameSite: http.SameSiteLaxMode,
		Expires:  expires,
		MaxAge:   int(time.Until(expires).Seconds()),
	})
}

func (h *Handler) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     h.cfg.CookieName,
		Value:    "",
		Path:     h.cfg.CookiePath,
		HttpOnly: true,
		Secure:   h.cfg.SecureCookies,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

func userView(p *authz.Principal) *userResponse {
	return &userResponse{
		ID:       p.UserID.String(),
		Email:    p.Email,
		Name:     p.Name,
		Subject:  p.Subject,
		PersonID: uuidStringOrEmpty(p.PersonID),
		Roles:    p.Roles,
	}
}

// writeError maps an auth error to a problem response, keeping the mapping in
// one place so every endpoint answers abuse the same way.
func (h *Handler) writeError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ErrInvalidCredentials),
		errors.Is(err, ErrInvalidIdentifier),
		errors.Is(err, ErrAccountInactive),
		errors.Is(err, ErrInvalidTOTP),
		errors.Is(err, ErrTokenInvalid):
		httpx.WriteProblem(w, r, httpx.Unauthenticated("The credentials supplied are not valid."))
	case errors.Is(err, ErrAccountLocked):
		httpx.WriteProblem(w, r, httpx.New(http.StatusTooManyRequests, httpx.CodeRateLimited, "Too many attempts. Try again later."))
	case errors.Is(err, ErrPasswordTooShort):
		httpx.WriteProblem(w, r, httpx.BadRequest("The password must be at least 12 characters."))
	case errors.Is(err, ErrMFAAlreadyActive):
		httpx.WriteProblem(w, r, httpx.Conflict("Multi-factor authentication is already active."))
	case errors.Is(err, ErrMFANotEnrolled):
		httpx.WriteProblem(w, r, httpx.New(http.StatusBadRequest, httpx.CodeMFARequired, "Multi-factor authentication has not been enrolled."))
	case errors.Is(err, ErrInviteInvalid):
		httpx.WriteProblem(w, r, httpx.BadRequest("That invitation is not valid."))
	default:
		httpx.WriteError(w, r, h.logger, err)
	}
}

// writeDecodeError answers a body that could not be decoded. A wrong media
// type keeps its 415 so the caller learns to send JSON; any other failure gets
// the endpoint's own explanation of the expected body.
func writeDecodeError(w http.ResponseWriter, r *http.Request, err error, detail string) {
	var p httpx.Problem
	if errors.As(err, &p) && p.Status == http.StatusUnsupportedMediaType {
		httpx.WriteProblem(w, r, p)
		return
	}
	httpx.WriteProblem(w, r, httpx.BadRequest(detail))
}

// sessionMeta collects the client context recorded with a session.
func sessionMeta(r *http.Request) SessionMeta {
	return SessionMeta{IP: clientAddr(r), UserAgent: truncate(r.UserAgent(), 400)}
}

func clientAddr(r *http.Request) netip.Addr {
	host := r.RemoteAddr
	if h, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		host = h
	}
	addr, err := netip.ParseAddr(strings.TrimSpace(host))
	if err != nil {
		return netip.Addr{}
	}
	return addr
}

func parseUUID(value string) (uuid.UUID, error) {
	return uuid.Parse(strings.TrimSpace(value))
}

// uuidStringOrEmpty renders a UUID, or "" for the zero value.
func uuidStringOrEmpty(id uuid.UUID) string {
	if id == uuid.Nil {
		return ""
	}
	return id.String()
}

func truncate(value string, max int) string {
	if len(value) <= max {
		return value
	}
	return value[:max]
}
