"use client";

import { useRouter } from "next/navigation";
import { useCallback, useEffect, useState } from "react";

import { Button } from "@/components/form";
import { Icon } from "@/components/icons";
import { Badge, Card, EmptyState, IconTile } from "@/components/ui";
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
          size="sm"
          variant="secondary"
          onClick={signOutEverywhere}
          disabled={signingOutAll || count === 0}
        >
          <Icon name="signOut" className="h-4 w-4" />
          {signingOutAll ? "Signing out…" : "Sign out everywhere"}
        </Button>
      }
    >
      {problem && (
        <p
          role="alert"
          className="mx-[22px] mb-3 rounded-control bg-[#FFF8EB] px-3.5 py-2.5 text-sm text-[#7A4300]"
        >
          {problem}
        </p>
      )}

      {sessions === null ? (
        <p className="px-[22px] pb-6 text-sm text-muted">Loading your sessions…</p>
      ) : sessions.length === 0 ? (
        <div className="px-[22px] pb-[22px]">
          <EmptyState icon="monitor" iconTone="info" title="No active sessions" />
        </div>
      ) : (
        <ul className="border-t border-line">
          {sessions.map((session) => (
            <li
              key={session.id}
              className="flex flex-wrap items-start gap-3.5 border-t border-line px-[22px] py-4 first:border-t-0"
            >
              <IconTile icon="monitor" tone={session.current ? "success" : "info"} size="sm" />
              <div className="min-w-0 flex-1">
                <div className="flex flex-wrap items-center gap-2">
                  <span className="truncate text-sm font-semibold text-night">
                    {session.user_agent || "Unknown device"}
                  </span>
                  {session.current && <Badge tone="success" dot>This device</Badge>}
                </div>
                <p className="mt-1 text-[13px] text-muted">
                  {session.ip ? `${session.ip} · ` : ""}
                  last active {formatDateTime(session.last_seen_at)}
                </p>
                <p className="mt-0.5 text-xs text-[#7A879C]">
                  signed in {formatDateTime(session.created_at)} · expires{" "}
                  {formatDateTime(session.expires_at)}
                </p>
              </div>
              {!session.current && (
                <Button
                  type="button"
                  size="sm"
                  variant="danger"
                  onClick={() => revoke(session.id)}
                  disabled={busyId === session.id}
                >
                  {busyId === session.id ? "Revoking…" : "Revoke"}
                </Button>
              )}
            </li>
          ))}
        </ul>
      )}
    </Card>
  );
}
