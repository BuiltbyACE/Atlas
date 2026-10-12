"use client";

/**
 * Atlas web - Phoenix Channels realtime helper.
 *
 * Connects the browser to the Elixir orchestrator's `/socket` (Phoenix
 * Channels) and exposes hooks for joining a topic.
 *
 * ADR-009 / blueprint section 5.2: **real-time messages are signals, not
 * data.** A signal carries an entity identifier and a change type
 * (`{ type, entity, id, change }`) - never business fields. On receipt the
 * client MUST refetch from the Go API, which authorises the request as the
 * current user. This keeps Elixir from becoming a second, unauthorised data
 * path and keeps permissions in Go.
 *
 * Auth: the socket uses a short-lived realtime token (5 minute TTL,
 * `aud=atlas-realtime`) issued by Go at `POST /api/v1/auth/realtime-token`
 * (blueprint sections 5.7 and 7.5, ADR-008).
 */

import { useEffect, useRef } from "react";
import { Socket, type Channel } from "phoenix";

/** Event name the orchestrator pushes signals on for every topic. */
const SIGNAL_EVENT = "signal";

/** Well-known topic for the CEO executive dashboard (blueprint section 7.5). */
export const EXEC_DASHBOARD_TOPIC = "exec:dashboard";

/** Topic for a single user's notifications, approvals and escalations. */
export function userTopic(userId: string): string {
  return `user:${userId}`;
}

export type ChangeType = "created" | "updated" | "deleted" | "changed";

/**
 * A realtime signal. Identifier + change type only - the client refetches the
 * real record from Go.
 */
export interface RealtimeSignal {
  /** Topic the signal arrived on. */
  topic: string;
  /** Dotted change name, e.g. `work.task.changed`. */
  type: string;
  /** Entity kind, e.g. `task`. */
  entity?: string;
  /** Identifier of the changed entity. */
  id: string;
  /** What happened to the entity. */
  change?: ChangeType | string;
  /** Monotonic server sequence, when provided. */
  seq?: number;
}

/** Called for every signal; refetch the entity from Go inside this handler. */
export type SignalHandler = (signal: RealtimeSignal) => void;
export type RealtimeErrorHandler = (error: unknown) => void;

interface RealtimeTokenResponse {
  /** Short-lived JWT for the Phoenix socket. */
  token: string;
  /** ISO-8601 expiry, when returned. */
  expires_at?: string;
}

function realtimeEndpoint(): string {
  return process.env.NEXT_PUBLIC_REALTIME_URL ?? "/socket";
}

/** Fetch a fresh short-lived realtime token from Go. */
export async function fetchRealtimeToken(): Promise<RealtimeTokenResponse> {
  // Imported lazily to avoid a static server/client dependency cycle.
  const { apiFetch } = await import("@/lib/api/client");
  return apiFetch<RealtimeTokenResponse>("/api/v1/auth/realtime-token", {
    method: "POST",
  });
}

let socket: Socket | null = null;

/** Lazily create and connect the shared socket (browser only). */
export async function getSocket(): Promise<Socket> {
  if (typeof window === "undefined") {
    throw new Error("getSocket() may only be called in the browser.");
  }
  if (socket) return socket;

  const { token } = await fetchRealtimeToken();
  socket = new Socket(realtimeEndpoint(), { params: { token } });
  socket.connect();
  return socket;
}

/** Disconnect and forget the shared socket (e.g. on sign out). */
export function disconnectSocket(): void {
  if (socket) {
    socket.disconnect();
    socket = null;
  }
}

/**
 * Validate a raw channel payload. Signals MUST carry an `id` and a `type`;
 * anything else is ignored so that a malformed push can never be mistaken for
 * business data.
 */
function parseSignal(topic: string, payload: unknown): RealtimeSignal | null {
  if (!payload || typeof payload !== "object") return null;
  const raw = payload as Record<string, unknown>;
  if (typeof raw.id !== "string" || raw.id.length === 0) return null;
  if (typeof raw.type !== "string" || raw.type.length === 0) return null;

  return {
    topic,
    type: raw.type,
    id: raw.id,
    entity: typeof raw.entity === "string" ? raw.entity : undefined,
    change: typeof raw.change === "string" ? raw.change : undefined,
    seq: typeof raw.seq === "number" ? raw.seq : undefined,
  };
}

/**
 * Join a topic and call `onSignal` for each signal. Returns an unsubscribe
 * function that leaves the channel.
 *
 * The handler receives identifiers only; refetch the entity from the Go API.
 */
export function joinTopic(
  topic: string,
  onSignal: SignalHandler,
  onError?: RealtimeErrorHandler,
): () => void {
  let channel: Channel | null = null;
  let left = false;

  void (async () => {
    try {
      const shared = await getSocket();
      if (left) return;

      channel = shared.channel(topic, {});
      channel.on(SIGNAL_EVENT, (payload: unknown) => {
        const signal = parseSignal(topic, payload);
        if (signal) onSignal(signal);
      });
      channel
        .join()
        .receive("error", (response: unknown) => onError?.(response))
        .receive("timeout", () => onError?.(new Error("channel join timeout")));
    } catch (error) {
      onError?.(error);
    }
  })();

  return () => {
    left = true;
    channel?.leave();
    channel = null;
  };
}

interface UseRealtimeSignalsOptions {
  /** Set to false to stay disconnected (e.g. until auth is ready). */
  enabled?: boolean;
  /** Called when the channel fails to join or errs. */
  onError?: RealtimeErrorHandler;
}

/**
 * React hook: subscribe to `topic` while the component is mounted. The latest
 * `handler` is always used without re-joining the channel.
 *
 * @example
 * useRealtimeSignals(userTopic(me.id), (signal) => {
 *   // signal.id + signal.change only -> refetch from Go:
 *   void refreshNotifications(signal.id);
 * });
 */
export function useRealtimeSignals(
  topic: string,
  handler: SignalHandler,
  options: UseRealtimeSignalsOptions = {},
): void {
  const { enabled = true, onError } = options;

  const handlerRef = useRef(handler);
  const errorRef = useRef(onError);

  useEffect(() => {
    handlerRef.current = handler;
  }, [handler]);

  useEffect(() => {
    errorRef.current = onError;
  }, [onError]);

  useEffect(() => {
    if (!enabled) return;
    return joinTopic(
      topic,
      (signal) => handlerRef.current(signal),
      (error) => errorRef.current?.(error),
    );
  }, [topic, enabled]);
}

/** Subscribe to the CEO `exec:dashboard` topic (blueprint section 7.5). */
export function useExecDashboard(
  handler: SignalHandler,
  options?: UseRealtimeSignalsOptions,
): void {
  useRealtimeSignals(EXEC_DASHBOARD_TOPIC, handler, options);
}

/** Subscribe to the current user's `user:{id}` topic. */
export function useUserChannel(
  userId: string,
  handler: SignalHandler,
  options?: UseRealtimeSignalsOptions,
): void {
  useRealtimeSignals(userTopic(userId), handler, options);
}
