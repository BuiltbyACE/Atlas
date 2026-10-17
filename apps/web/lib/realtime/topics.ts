/**
 * Atlas web - realtime topic names.
 *
 * Kept in a plain module (no `"use client"`) so BOTH server components and the
 * Phoenix client can import the same strings. Importing a constant from a
 * `"use client"` module into a server component yields a client reference, not
 * the value, so the topic names must live here.
 */

/** The CEO executive dashboard topic (blueprint §7.5, §8.8). */
export const EXEC_DASHBOARD_TOPIC = "exec:dashboard";

/** A single user's notifications/approvals/escalations topic. */
export function userTopic(userId: string): string {
  return `user:${userId}`;
}

/** A project's task/evidence topic (joined after Go's authz check, §7.5). */
export function projectTopic(projectId: string): string {
  return `work:project:${projectId}`;
}
