# ADR-008 — Session-cookie auth, MFA for privileged roles, short-lived realtime tokens

| | |
|---|---|
| **Status** | Proposed |
| **Date** | 2026-10-11 |
| **Deciders** | Internal & Technical CEO, Commercial CEO |
| **Phase** | Phase 0 |
| **Supersedes / Superseded by** | — |

## Context

§6.1 defines identity and authentication; §5.7 sets the communication contracts; §13.1 the security baseline. Account takeover of a CEO is a named threat (§13.3). The browser talks to Go over HTTPS and to Elixir over WebSocket, so two distinct credential paths are needed without adding CORS or third-party cookies.

## Decision

- **Sessions:** an **opaque random session token**, stored hashed with device/IP metadata, delivered as an `httpOnly`, `Secure`, `SameSite=Lax` cookie. A **CSRF token** is required on cookie-authenticated mutations. Idle timeout and absolute lifetime are **[CONFIG]**; users can see and revoke sessions; CEOs can revoke anyone's.
- **Passwords:** `argon2id`.
- **MFA:** **TOTP mandatory** for privileged roles (`ceo_commercial`, `ceo_internal`, `finance_officer`, `system_admin`, `auditor`); recovery codes stored hashed; WebAuthn/passkeys in a later phase.
- **Realtime tokens:** Go issues **5-minute Ed25519** tokens (`aud=atlas-realtime`) for Phoenix; Elixir verifies via Go's JWKS endpoint `/internal/v1/.well-known/jwks.json`.
- **Service identities:** `svc:orchestrator`, `svc:worker`, and one per integration, each with scoped permissions and rotatable credentials.
- **Lifecycle:** invite → activate → (suspend) → deactivate; deactivation revokes sessions and tokens immediately.

## Consequences

### Positive
- First-party cookies stay same-origin; no CORS/cookie complexity (ADR-007).
- Short-lived realtime tokens limit the blast radius of any token leak.
- MFA protects the highest-risk accounts (CEOs, finance, admin, auditor).

### Negative / trade-offs
- Two credential mechanisms (session cookie + realtime token) to implement and test.
- Ed25519 key rotation and JWKS caching/clock skew must be handled.
- TOTP adds friction for privileged users; the SSO decision (D3) may later change credential handling.

## Alternatives considered
- **JWT session cookies** — rejected: opaque server-side sessions allow immediate revocation.
- **Full OAuth/OIDC via an external IdP now** — deferred: depends on the SSO decision (D3).
- **Long-lived socket tokens** — rejected: larger leak window.
- **Browser API keys** — rejected: not suitable for user sessions.

## References
- `docs/ATLAS.md` §6.1
- `docs/ATLAS.md` §6.2
- `docs/ATLAS.md` §5.7
- `docs/ATLAS.md` §13.1
- `docs/ATLAS.md` §21 (D3)
