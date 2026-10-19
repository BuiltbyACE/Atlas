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
 * (blueprint sections 5.7 and 7.5, ADR-008). The orchestrator verifies the
 * token on every (re)connect, so the socket reads its params through a closure
 * and the token is refreshed before it expires and whenever the socket errors.
 * A socket built with one fixed token would be refused on its first reconnect
 * after five minutes and stay down for the life of the tab.
 */

import { useEffect, useRef, useSyncExternalStore } from "react";
import { Socket, type Channel } from "phoenix";

import { EXEC_DASHBOARD_TOPIC, userTopic } from "@/lib/realtime/topics";

/** Event name the orchestrator pushes signals on for every topic. */
const SIGNAL_EVENT = "signal";

/** Refresh the token comfortably inside its 5-minute lifetime. */
const TOKEN_REFRESH_MS = 4 * 60_000;
/** Never ask Go for tokens faster than this while the socket is flapping. */
const TOKEN_RETRY_MIN_MS = 10_000;

// The topic names live in `./topics` so server components can import them too;
// re-exported here for existing callers.
export { EXEC_DASHBOARD_TOPIC, userTopic };

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

/** The shared socket's connection state, for an honest "Live" indicator. */
export type RealtimeStatus = "connecting" | "open" | "closed";

interface RealtimeTokenResponse {
  /** Short-lived JWT for the Phoenix socket. */
  token: string;
  /** ISO-8601 expiry, when returned. */
  expires_at?: string;
}

/**
 * Where the socket lives.
 *
 * Production: Caddy proxies `/socket` to the orchestrator, so the same-origin
 * default is right. Development: Next.js rewrites cannot proxy WebSockets, so a
 * relative endpoint would hit the Next dev server and never connect (the
 * dashboard then sits on "Offline"). In development a relative or missing
 * endpoint is therefore pointed at the orchestrator on :4000 of the same host.
 * `http(s)://` is accepted and normalised to `ws(s)://`.
 */
function realtimeEndpoint(): string {
  const configured = process.env.NEXT_PUBLIC_REALTIME_URL?.trim() || "/socket";
  if (/^https?:\/\//i.test(configured)) return configured.replace(/^http/i, "ws");
  if (configured.startsWith("/") && process.env.NODE_ENV === "development") {
    return `ws://${window.location.hostname}:4000${configured}`;
  }
  return configured;
}

/** Fetch a fresh short-lived realtime token from Go. */
export async function fetchRealtimeToken(): Promise<RealtimeTokenResponse> {
  // Imported lazily to avoid a static server/client dependency cycle.
  const { apiFetch } = await import("@/lib/api/client");
  return apiFetch<RealtimeTokenResponse>("/api/v1/auth/realtime-token", {
    method: "POST",
  });
}

// ── connection state ────────────────────────────────────────────────────────

let status: RealtimeStatus = "connecting";
const statusListeners = new Set<() => void>();

function setStatus(next: RealtimeStatus): void {
  if (status === next) return;
  status = next;
  for (const listener of statusListeners) listener();
}

function subscribeStatus(listener: () => void): () => void {
  statusListeners.add(listener);
  return () => statusListeners.delete(listener);
}

/** React hook: the shared socket's connection state. */
export function useRealtimeStatus(): RealtimeStatus {
  return useSyncExternalStore(
    subscribeStatus,
    () => status,
    () => "connecting",
  );
}

// ── the shared socket ───────────────────────────────────────────────────────

let socket: Socket | null = null;
let connecting: Promise<Socket> | null = null;
let token = "";
let tokenFetchedAt = 0;
let refreshTimer: number | undefined;

async function refreshToken(): Promise<void> {
  if (Date.now() - tokenFetchedAt < TOKEN_RETRY_MIN_MS) return;
  tokenFetchedAt = Date.now();
  try {
    token = (await fetchRealtimeToken()).token;
  } catch {
    // Signed out or Go unreachable: keep the old token; the socket stays
    // closed and the indicator says so.
  }
}

/** Lazily create and connect the shared socket (browser only). */
export async function getSocket(): Promise<Socket> {
  if (typeof window === "undefined") {
    throw new Error("getSocket() may only be called in the browser.");
  }
  if (socket) return socket;
  if (connecting) return connecting;

  connecting = (async () => {
    setStatus("connecting");
    try {
      token = (await fetchRealtimeToken()).token;
      tokenFetchedAt = Date.now();
    } catch (error) {
      setStatus("closed");
      throw error;
    }

    // `params` is read on every (re)connect, so it must be a closure.
    const created = new Socket(realtimeEndpoint(), { params: () => ({ token }) });
    created.onOpen(() => setStatus("open"));
    created.onClose(() => setStatus("closed"));
    created.onError(() => {
      setStatus("closed");
      // The likeliest cause of a refused reconnect is an expired token.
      void refreshToken();
    });
    created.connect();

    refreshTimer = window.setInterval(() => {
      tokenFetchedAt = 0;
      void refreshToken();
    }, TOKEN_REFRESH_MS);

    socket = created;
    return created;
  })();

  try {
    return await connecting;
  } finally {
    connecting = null;
  }
}

/** Disconnect and forget the shared socket (e.g. on sign out). */
export function disconnectSocket(): void {
  window.clearInterval(refreshTimer);
  for (const entry of topics.values()) entry.closed = true;
  topics.clear();
  if (socket) {
    socket.disconnect();
    socket = null;
  }
  token = "";
  setStatus("closed");
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

interface Subscriber {
  onSignal: SignalHandler;
  onError?: RealtimeErrorHandler;
  onJoined?: () => void;
}

interface TopicEntry {
  channel: Channel | null;
  subscribers: Set<Subscriber>;
  state: "joining" | "joined" | "failed";
  closed: boolean;
}

/**
 * One channel per topic, shared by every subscriber. Phoenix closes an
 * existing channel when the same topic is joined again on the same socket, so
 * two components listening to `user:{id}` (the shell's live pill and the
 * notifications bell, say) must share a channel rather than evict each other.
 */
const topics = new Map<string, TopicEntry>();

async function openChannel(topic: string, entry: TopicEntry): Promise<void> {
  const fail = (reason: unknown) => {
    entry.state = "failed";
    for (const sub of entry.subscribers) sub.onError?.(reason);
  };
  try {
    const shared = await getSocket();
    if (entry.closed) return;

    const channel = shared.channel(topic, {});
    entry.channel = channel;
    channel.on(SIGNAL_EVENT, (payload: unknown) => {
      const signal = parseSignal(topic, payload);
      if (!signal) return;
      for (const sub of entry.subscribers) sub.onSignal(signal);
    });
    channel.onError((reason: unknown) => fail(reason));
    // The join push is re-sent on every rejoin, so "ok" also marks recovery.
    channel
      .join()
      .receive("ok", () => {
        entry.state = "joined";
        for (const sub of entry.subscribers) sub.onJoined?.();
      })
      .receive("error", (response: unknown) => fail(response))
      .receive("timeout", () => fail(new Error("channel join timeout")));
  } catch (error) {
    fail(error);
  }
}

/**
 * Join a topic and call `onSignal` for each signal. Returns an unsubscribe
 * function; the channel is left when its last subscriber goes. `onJoined`
 * fires on every successful (re)join, so a caller can tell a recovered channel
 * from a failed one.
 *
 * The handler receives identifiers only; refetch the entity from the Go API.
 */
export function joinTopic(
  topic: string,
  onSignal: SignalHandler,
  onError?: RealtimeErrorHandler,
  onJoined?: () => void,
): () => void {
  const subscriber: Subscriber = { onSignal, onError, onJoined };
  let entry = topics.get(topic);
  if (!entry) {
    entry = { channel: null, subscribers: new Set(), state: "joining", closed: false };
    topics.set(topic, entry);
    entry.subscribers.add(subscriber);
    void openChannel(topic, entry);
  } else {
    entry.subscribers.add(subscriber);
    // A late subscriber learns the channel's current state at once.
    const current = entry;
    queueMicrotask(() => {
      if (current.state === "joined") onJoined?.();
      if (current.state === "failed") onError?.(new Error("channel unavailable"));
    });
  }

  const joined = entry;
  return () => {
    joined.subscribers.delete(subscriber);
    if (joined.subscribers.size === 0) {
      joined.closed = true;
      joined.channel?.leave();
      if (topics.get(topic) === joined) topics.delete(topic);
    }
  };
}

interface UseRealtimeSignalsOptions {
  /** Set to false to stay disconnected (e.g. until auth is ready). */
  enabled?: boolean;
  /** Called when the channel fails to join or errs. */
  onError?: RealtimeErrorHandler;
  /** Called each time the channel (re)joins. */
  onJoined?: () => void;
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
  const { enabled = true, onError, onJoined } = options;

  const handlerRef = useRef(handler);
  const errorRef = useRef(onError);
  const joinedRef = useRef(onJoined);

  useEffect(() => {
    handlerRef.current = handler;
  }, [handler]);

  useEffect(() => {
    errorRef.current = onError;
  }, [onError]);

  useEffect(() => {
    joinedRef.current = onJoined;
  }, [onJoined]);

  useEffect(() => {
    if (!enabled) return;
    return joinTopic(
      topic,
      (signal) => handlerRef.current(signal),
      (error) => errorRef.current?.(error),
      () => joinedRef.current?.(),
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
