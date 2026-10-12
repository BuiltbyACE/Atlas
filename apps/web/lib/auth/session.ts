/**
 * Atlas web - server-side session helpers.
 *
 * Phase 1 placeholder. Real authentication is owned by Go (blueprint sections
 * 5.2/5.7, ADR-008): Go issues the session cookie and authorises every call.
 * These helpers only read the current user for server components and bounce
 * unauthenticated visitors to `/login`.
 *
 * The session cookie is forwarded automatically by `apiFetch` when running on
 * the server, so Go sees the request as the current user.
 */

import { redirect } from "next/navigation";

import { apiFetch } from "@/lib/api/client";

/** Shape of `GET /api/v1/auth/me` (until the generated client lands). */
export interface CurrentUser {
  id: string;
  email: string;
  name: string;
  /** Stable subject, e.g. `user:<uuid>`. */
  subject: string;
  person_id?: string;
  roles: string[];
}

/**
 * Read the current user, or `null` when there is no valid session.
 * Never throws for network/auth failures - callers decide what to do.
 */
export async function getCurrentUser(): Promise<CurrentUser | null> {
  try {
    return await apiFetch<CurrentUser>("/api/v1/auth/me", { cache: "no-store" });
  } catch {
    return null;
  }
}

/**
 * Require an authenticated user, redirecting to `/login` when there is none.
 * Intended for use in server components / layouts only.
 */
export async function requireUser(): Promise<CurrentUser> {
  const user = await getCurrentUser();
  if (!user) {
    redirect("/login");
  }
  return user;
}
