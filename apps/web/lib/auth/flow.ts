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
