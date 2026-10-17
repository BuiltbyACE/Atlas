"use client";

import { useEffect, useState } from "react";

import { Badge, EmptyState } from "@/components/ui";
import { getPresence, type Presence } from "@/lib/api/atlas";
import { formatDuration, formatTime } from "@/lib/format";

/** How often the live roster re-reads the presence endpoint. */
const POLL_MS = 30_000;

/**
 * The "online now" roster (blueprint §8.5). The server page hands over an
 * initial reading so the first paint needs no client fetch; from there this
 * re-reads `GET /api/v1/people/presence` every 30 seconds (and on tab focus) so
 * the panel reflects people arriving and leaving without a manual reload.
 *
 * Real, not mocked: a person appears here while their open client is sending
 * heartbeats (`PresenceHeartbeat`) and drops off the presence window after they
 * leave. A failed refresh flips the pill to "reconnecting" and keeps the last
 * good reading rather than emptying the roster.
 */
export function PresenceRoster({ initial }: { initial: Presence }) {
  const [presence, setPresence] = useState<Presence>(initial);
  const [stale, setStale] = useState(false);

  useEffect(() => {
    let disposed = false;

    const refresh = async () => {
      try {
        const next = await getPresence();
        if (disposed) return;
        setPresence(next);
        setStale(false);
      } catch {
        if (!disposed) setStale(true);
      }
    };

    const timer = window.setInterval(refresh, POLL_MS);
    const onVisible = () => {
      if (document.visibilityState === "visible") void refresh();
    };
    document.addEventListener("visibilitychange", onVisible);

    return () => {
      disposed = true;
      window.clearInterval(timer);
      document.removeEventListener("visibilitychange", onVisible);
    };
  }, []);

  const online = presence.online;

  return (
    <div className="space-y-3">
      <p className="flex items-center gap-2 text-xs text-slate-500">
        <Badge tone={stale ? "warning" : "success"}>
          {stale ? "reconnecting" : "live"}
        </Badge>
        <span>
          {online.length} online · updated {formatTime(presence.generated_at)}
        </span>
      </p>

      {online.length === 0 ? (
        <EmptyState>Nobody is signed in and active right now.</EmptyState>
      ) : (
        <ul className="grid grid-cols-1 gap-2 sm:grid-cols-2 lg:grid-cols-3">
          {online.map((session) => (
            <li
              key={session.id}
              className="rounded-lg border border-slate-200 bg-white px-3 py-2"
            >
              <div className="flex items-center justify-between gap-2">
                <span className="truncate text-sm font-medium text-slate-900">
                  {session.full_name}
                </span>
                <Badge tone="success">online</Badge>
              </div>
              <p className="mt-0.5 text-xs text-slate-500">
                since {formatTime(session.started_at)} ·{" "}
                {formatDuration(session.active_seconds)}
              </p>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
