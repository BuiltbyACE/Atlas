"use client";

import { useRouter } from "next/navigation";
import { useCallback, useEffect, useState } from "react";

import { Button } from "@/components/form";
import { Badge, Card, EmptyState } from "@/components/ui";
import { ApiError } from "@/lib/api/client";
import {
  getSessions,
  revokeAllSessions,
  revokeSession,
  type SessionSummary,
} from "@/lib/auth/flow";
import { formatDateTime } from "@/lib/format";

/** Turn a failure into something a person can act on. Go owns the rules. */
function describe(error: unknown): string {
  if (error instanceof ApiError) {
    return error.problem.detail ?? error.message;
  }
  return "Atlas can't be reached right now. Check your connection and try again.";
}

/**
 * Every device this account is signed in on (`GET /api/v1/auth/sessions`).
 *
 * Sessions are owned by Go (ADR-008): this panel only lists them and asks Go to
 * revoke one or all. Revoking another device takes effect immediately; "sign
 * out everywhere" also clears this session's cookie, so it leaves for /login.
 * The current device is not offered for individual revocation - use the
 * sidebar's "Sign out" for that - so the roster never strands the viewer.
 */
export function SessionsPanel() {
  const router = useRouter();
  const [sessions, setSessions] = useState<SessionSummary[] | null>(null);
  const [problem, setProblem] = useState<string | null>(null);
  const [busyId, setBusyId] = useState<string | null>(null);
  const [signingOutAll, setSigningOutAll] = useState(false);

  const load = useCallback(async () => {
    setProblem(null);
    try {
      const result = await getSessions();
      setSessions(result.sessions);
    } catch (error) {
      setSessions([]);
      setProblem(describe(error));
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  async function revoke(id: string) {
    if (busyId) return;
    setBusyId(id);
    setProblem(null);
    try {
      await revokeSession(id);
      await load();
    } catch (error) {
      setProblem(describe(error));
    } finally {
      setBusyId(null);
    }
  }

  async function signOutEverywhere() {
    if (signingOutAll) return;
    setSigningOutAll(true);
    try {
      await revokeAllSessions();
    } catch {
      // Go clears the cookie regardless; /login expects no session.
    }
    router.replace("/login");
  }

  const count = sessions?.length ?? 0;

  return (
    <Card
      title="Sessions and devices"
      description="Where this account is signed in. Revoking a device signs it out immediately."
      padded={false}
      actions={
        <Button
          type="button"
          variant="secondary"
          onClick={signOutEverywhere}
          disabled={signingOutAll || count === 0}
        >
          {signingOutAll ? "Signing out." : "Sign out everywhere"}
        </Button>
      }
    >
      {problem && (
        <p
          role="alert"
          className="border-b border-amber-100 bg-amber-50 px-5 py-2.5 text-sm text-amber-900"
        >
          {problem}
        </p>
      )}

      {sessions === null ? (
        <p className="px-5 py-6 text-sm text-slate-500">Loading your sessions.</p>
      ) : sessions.length === 0 ? (
        <div className="px-5 py-4">
          <EmptyState>No active sessions.</EmptyState>
        </div>
      ) : (
        <ul className="divide-y divide-slate-100">
          {sessions.map((session) => (
            <li
              key={session.id}
              className="flex flex-wrap items-start justify-between gap-3 px-5 py-4"
            >
              <div className="min-w-0">
                <div className="flex items-center gap-2">
                  <span className="truncate text-sm font-medium text-slate-800">
                    {session.user_agent || "Unknown device"}
                  </span>
                  {session.current && <Badge tone="success">This device</Badge>}
                </div>
                <p className="mt-1 text-xs text-slate-500">
                  {session.ip ? `${session.ip} · ` : ""}
                  last active {formatDateTime(session.last_seen_at)}
                </p>
                <p className="mt-0.5 text-xs text-slate-400">
                  signed in {formatDateTime(session.created_at)} · expires{" "}
                  {formatDateTime(session.expires_at)}
                </p>
              </div>
              {!session.current && (
                <Button
                  type="button"
                  variant="danger"
                  onClick={() => revoke(session.id)}
                  disabled={busyId === session.id}
                >
                  {busyId === session.id ? "Revoking." : "Revoke"}
                </Button>
              )}
            </li>
          ))}
        </ul>
      )}
    </Card>
  );
}
