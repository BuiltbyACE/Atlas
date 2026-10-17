"use client";

import { useRouter } from "next/navigation";
import { useState } from "react";

import { useRealtimeSignals } from "@/lib/realtime/socket";

/**
 * Subscribes to a Phoenix topic and re-renders the current route on every
 * signal. ADR-009 / blueprint §5.2: signals carry identifiers only, so the
 * correct response is always a refetch - which `router.refresh()` performs for
 * the server components on this route. Go authorises that refetch.
 *
 * The pill is honest: it flips to "Offline" when the channel cannot join (for
 * example when the viewer lacks the topic's read permission), and the page keeps
 * working from its last server render.
 */
export function SignalRefresh({
  topic,
  label = "Live",
}: {
  topic: string;
  label?: string;
}) {
  const router = useRouter();
  const [offline, setOffline] = useState(false);

  useRealtimeSignals(
    topic,
    () => {
      router.refresh();
    },
    { onError: () => setOffline(true) },
  );

  return (
    <span
      title={offline ? "Realtime unavailable" : `Subscribed to ${topic}`}
      className={
        "inline-flex items-center gap-1.5 rounded-full px-2 py-0.5 text-xs font-medium ring-1 ring-inset " +
        (offline
          ? "bg-slate-100 text-slate-500 ring-slate-200"
          : "bg-emerald-50 text-emerald-700 ring-emerald-200")
      }
    >
      <span
        aria-hidden="true"
        className={
          "h-1.5 w-1.5 rounded-full " +
          (offline ? "bg-slate-400" : "animate-pulse bg-emerald-500")
        }
      />
      {offline ? "Offline" : label}
    </span>
  );
}
