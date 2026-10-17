"use client";

import { useEffect } from "react";

import { heartbeat } from "@/lib/api/atlas";

/** How often an open client refreshes its session activity. */
const HEARTBEAT_INTERVAL_MS = 60_000;

/**
 * Keeps the current session's activity warm while a signed-in client is open.
 *
 * The people activity view (§8.5) reads "online" as a session seen within the
 * presence window (2 minutes by default), so a tab that only navigates
 * occasionally would otherwise drift to "away". This pings
 * `POST /api/v1/auth/heartbeat` on mount and every minute. Failures are
 * swallowed: a 401 means the session ended, and the next navigation will bounce
 * to /login.
 *
 * Renders nothing.
 */
export function PresenceHeartbeat() {
  useEffect(() => {
    let disposed = false;

    const ping = () => {
      if (disposed) return;
      // The response is irrelevant; the request itself refreshes last_seen_at.
      void heartbeat().catch(() => undefined);
    };

    ping();
    const timer = window.setInterval(ping, HEARTBEAT_INTERVAL_MS);
    // Coming back from sleep or a hidden tab, refresh at once rather than
    // waiting up to a minute to be seen again.
    const onVisible = () => {
      if (document.visibilityState === "visible") ping();
    };
    document.addEventListener("visibilitychange", onVisible);

    return () => {
      disposed = true;
      window.clearInterval(timer);
      document.removeEventListener("visibilitychange", onVisible);
    };
  }, []);

  return null;
}
