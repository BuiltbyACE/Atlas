/**
 * Atlas web - browser-side calls for the sign-in journey (docs/FRONTEND.md §2-3).
 *
 * Thin, typed wrappers over `apiFetch`; every rule (credential checks, MFA,
 * lockout, session issue) lives in Go. Shapes mirror `contracts/openapi/
 * public.yaml` until the generated client lands (`generate:api`).
 *
 * The session cookie is HttpOnly and set by Go on the same origin, so nothing
 * here ever touches it. The `mfa_token` is a 5-minute challenge, not a
 * session: callers keep it in component state only.
 */

import { apiFetch } from "@/lib/api/client";
import type { CurrentUser } from "@/lib/auth/session";

/** Lifetime of the login MFA challenge (`MFAChallengeTTL` in atlas-api). */
export const MFA_CHALLENGE_TTL_MS = 5 * 60 * 1000;

/** Abort a request that has not answered in this long. */
const REQUEST_TIMEOUT_MS = 15_000;

/** `LoginResponse`: either a signed-in user or an MFA challenge. */
export interface LoginResponse {
  requires_mfa?: boolean;
  enrollment_required?: boolean;
  mfa_token?: string;
  user?: CurrentUser;
}

/** `MfaEnrollResponse`. */
export interface MfaEnrollResponse {
  secret: string;
  otpauth_url: string;
}

/** `MfaActivateResponse`. */
export interface MfaActivateResponse {
  recovery_codes: string[];
}

function post<T>(path: string, body?: unknown): Promise<T> {
  return apiFetch<T>(path, {
    method: "POST",
    body,
    cache: "no-store",
    signal: AbortSignal.timeout(REQUEST_TIMEOUT_MS),
  });
}

/** `POST /api/v1/auth/login` with an email or phone `identifier`. */
export function login(identifier: string, password: string): Promise<LoginResponse> {
  return post<LoginResponse>("/api/v1/auth/login", { identifier, password });
}

/** `POST /api/v1/auth/mfa/verify` with a TOTP or recovery code. */
export function verifyMfa(mfaToken: string, code: string): Promise<LoginResponse> {
  return post<LoginResponse>("/api/v1/auth/mfa/verify", { mfa_token: mfaToken, code });
}

/** `POST /api/v1/auth/mfa/enroll`: a fresh pending TOTP secret. */
export function enrollMfa(mfaToken: string): Promise<MfaEnrollResponse> {
  return post<MfaEnrollResponse>("/api/v1/auth/mfa/enroll", { mfa_token: mfaToken });
}

/** `POST /api/v1/auth/mfa/activate`: confirms enrolment and starts the session. */
export function activateMfa(mfaToken: string, code: string): Promise<MfaActivateResponse> {
  return post<MfaActivateResponse>("/api/v1/auth/mfa/activate", { mfa_token: mfaToken, code });
}

/** `POST /api/v1/auth/logout`: revokes this session and clears the cookie. */
export function logout(): Promise<{ status: string }> {
  return post<{ status: string }>("/api/v1/auth/logout");
}

/** One active sign-in, from `GET /api/v1/auth/sessions`. */
export interface SessionSummary {
  id: string;
  /** `true` for the session making this request. */
  current: boolean;
  ip?: string;
  user_agent?: string;
  created_at: string;
  last_seen_at: string;
  expires_at: string;
}

/** `GET /api/v1/auth/sessions`: every session this account still holds. */
export function getSessions(): Promise<{ sessions: SessionSummary[] }> {
  return apiFetch<{ sessions: SessionSummary[] }>("/api/v1/auth/sessions", {
    cache: "no-store",
    signal: AbortSignal.timeout(REQUEST_TIMEOUT_MS),
  });
}

/** `DELETE /api/v1/auth/sessions/{id}`: revoke one of your own sessions. */
export function revokeSession(id: string): Promise<{ status: string }> {
  return apiFetch<{ status: string }>(
    `/api/v1/auth/sessions/${encodeURIComponent(id)}`,
    { method: "DELETE", signal: AbortSignal.timeout(REQUEST_TIMEOUT_MS) },
  );
}

/**
 * `POST /api/v1/auth/sessions/revoke-all`: revoke every session you hold. Go
 * also clears this session's cookie, so the caller is signed out afterwards.
 */
export function revokeAllSessions(): Promise<{ revoked: number }> {
  return post<{ revoked: number }>("/api/v1/auth/sessions/revoke-all");
}

/**
 * `POST /api/v1/auth/invites/accept`: complete an invitation and set a
 * password. Public - the invitee has no account yet.
 */
export function acceptInvite(input: {
  token: string;
  full_name: string;
  password: string;
}): Promise<{ id: string; email: string }> {
  return post<{ id: string; email: string }>("/api/v1/auth/invites/accept", input);
}
