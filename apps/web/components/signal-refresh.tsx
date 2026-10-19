"use client";

import { useRouter } from "next/navigation";
import { useEffect, useRef, useState } from "react";

import { cx } from "@/components/ui";
import { useRealtimeSignals, useRealtimeStatus } from "@/lib/realtime/socket";

/** Coalesce a burst of signals (a sweep, a bulk resolve) into one refetch. */
const REFRESH_DEBOUNCE_MS = 400;

/**
 * Subscribes to a Phoenix topic and re-renders the current route on every
 * signal. ADR-009 / blueprint §5.2: signals carry identifiers only, so the
 * correct response is always a refetch - which `router.refresh()` performs for
 * the server components on this route. Go authorises that refetch.
 *
 * The pill is honest: it reads "Offline" when the socket is down or the channel
 * could not join (for example when the viewer lacks the topic's read
 * permission), recovers on its own when the channel rejoins, and the page keeps
 * working from its last server render meanwhile. `quiet` hides the pill while
 * live and only shows it when something is wrong.
 */
export function SignalRefresh({
  topic,
  label = "Live",
  quiet = false,
}: {
  topic: string;
  label?: string;
  quiet?: boolean;
}) {
  const router = useRouter();
  const socketStatus = useRealtimeStatus();
  const [joinFailed, setJoinFailed] = useState(false);
  const timer = useRef<number | undefined>(undefined);

  useEffect(() => () => window.clearTimeout(timer.current), []);

  useRealtimeSignals(
    topic,
    () => {
      window.clearTimeout(timer.current);
      timer.current = window.setTimeout(() => router.refresh(), REFRESH_DEBOUNCE_MS);
    },
    {
      onError: () => setJoinFailed(true),
      onJoined: () => setJoinFailed(false),
    },
  );

  const offline = joinFailed || socketStatus === "closed";
  if (quiet && !offline) return null;

  return <LivePill state={offline ? "offline" : socketStatus === "open" ? "live" : "connecting"} label={label} title={offline ? "Realtime unavailable — the page still works; reload to see changes" : `Subscribed to ${topic}`} />;
}

export function LivePill({
  state,
  label = "Live",
  title,
}: {
  state: "live" | "connecting" | "offline";
  label?: string;
  title?: string;
}) {
  return (
    <span
      title={title}
      className={cx(
        "inline-flex h-8 items-center gap-2 whitespace-nowrap rounded-full px-3 text-[13px] font-semibold",
        state === "live" && "bg-[#E5F4EB] text-[#166534]",
        state === "connecting" && "bg-[#EEF1F6] text-muted",
        state === "offline" && "bg-[#FFF1D6] text-[#8A4B00]",
      )}
    >
      <i
        aria-hidden="true"
        className={cx(
          "h-2 w-2 rounded-full",
          state === "live" && "bg-[#22A55A] shadow-[0_0_0_3px_rgba(34,165,90,.22)]",
          state === "connecting" && "animate-pulse bg-[#8A97AD]",
          state === "offline" && "bg-[#F5A524]",
        )}
      />
      {state === "offline" ? "Offline" : state === "connecting" ? "Connecting" : label}
    </span>
  );
}
