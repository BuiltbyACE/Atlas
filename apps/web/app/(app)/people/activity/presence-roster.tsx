"use client";

import { useEffect, useState } from "react";

import { Avatar, Badge, EmptyState } from "@/components/ui";
import { getPresence, type Presence } from "@/lib/api/atlas";
import { formatDuration, formatTime, initials } from "@/lib/format";

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
export function PresenceRoster({ initial, me }: { initial: Presence; me?: string }) {
  const [presence, setPresence] = useState<Presence>(initial);
  const [stale, setStale] = useState(false);

  useEffect(() => setPresence(initial), [initial]);

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

  // One row per person: a person signed in on two devices is one person here.
  const online = [...new Map(presence.online.map((s) => [s.person_id, s])).values()];

  return (
    <>
      <p className="flex flex-wrap items-center gap-2.5 px-[22px] pb-3.5 text-[13.5px] text-muted">
        <Badge tone={stale ? "warning" : "success"} dot>
          {stale ? "Reconnecting" : "Live"}
        </Badge>
        {online.length} online · updated {formatTime(presence.generated_at)}
      </p>

      {online.length === 0 ? (
        <div className="px-[22px] pb-4">
          <EmptyState icon="signal" iconTone="info" title="Nobody is active right now">
            People appear here while Atlas is open in their browser.
          </EmptyState>
        </div>
      ) : (
        <ul className="flex max-h-[360px] flex-col gap-2.5 overflow-y-auto px-[22px] pb-4 scroll-quiet">
          {online.map((session) => (
            <li key={session.id} className="flex items-center gap-3 rounded-[14px] border border-line px-3.5 py-3">
              <span className="rounded-full p-[2px] ring-2 ring-[#22A55A]">
                <Avatar initials={initials(session.full_name)} size="lg" variant={session.person_id === me ? "me" : "default"} />
              </span>
              <span className="min-w-0">
                <b className="block truncate text-[15px] font-semibold">
                  {session.full_name}
                  {session.person_id === me && <span className="font-normal text-muted"> (you)</span>}
                </b>
                <small className="block text-[13px] text-muted">
                  Since {formatTime(session.started_at)} · {formatDuration(session.active_seconds)}
                </small>
              </span>
              <span className="ml-auto">
                <Badge tone="success" dot>Online</Badge>
              </span>
            </li>
          ))}
        </ul>
      )}
      <p className="px-[22px] pb-5 text-[13px] text-muted">
        People drop off this list a few minutes after their last heartbeat.
      </p>
    </>
  );
}
