"use client";

import Link from "next/link";
import { useCallback, useEffect, useRef, useState } from "react";

import { Icon } from "@/components/icons";
import { EmptyState, cx } from "@/components/ui";
import {
  listNotifications,
  markAllNotificationsRead,
  markNotificationRead,
  unreadNotificationCount,
  type AtlasNotification,
} from "@/lib/api/atlas";
import { ApiError } from "@/lib/api/client";
import { timeAgo } from "@/lib/format";
import { useRealtimeSignals } from "@/lib/realtime/socket";
import { userTopic } from "@/lib/realtime/topics";

const POLL_MS = 60_000;

/** Where a notification's subject lives in the workspace, when we have a page. */
function hrefFor(item: AtlasNotification): string | null {
  const type = item.entity_type ?? "";
  const id = item.entity_id;
  if (type.includes("task") && id) return `/work/tasks/${id}`;
  if (type.includes("project") && id) return `/work/projects/${id}`;
  if (type.includes("escalation")) return "/work/escalations";
  if (type.includes("leave")) return "/people";
  if (type.includes("daily_report")) return "/daily-reports";
  return null;
}

/**
 * The bell (prototype `.ax-iconbtn` + `.ax-dot`), backed by the real in-app
 * notifications (`GET /api/v1/notifications`, `…/unread-count`). The count is
 * re-read on a slow poll, when the tab returns, and whenever a signal arrives
 * on the viewer's `user:{id}` topic (ADR-009: the signal only says "something
 * changed"). A viewer without a linked person gets a 4xx and no bell.
 */
export function NotificationsBell({ userId }: { userId: string }) {
  const [count, setCount] = useState<number | null>(null);
  const [hidden, setHidden] = useState(false);
  const [open, setOpen] = useState(false);
  const [items, setItems] = useState<AtlasNotification[] | null>(null);
  const [failed, setFailed] = useState(false);
  const root = useRef<HTMLDivElement>(null);

  const refreshCount = useCallback(async () => {
    try {
      setCount(await unreadNotificationCount());
    } catch (error) {
      if (error instanceof ApiError && error.status >= 400 && error.status < 500) {
        setHidden(true);
      }
    }
  }, []);

  const loadItems = useCallback(async () => {
    setFailed(false);
    try {
      const page = await listNotifications({ limit: 12 });
      setItems(page.items);
    } catch {
      setFailed(true);
      setItems([]);
    }
  }, []);

  useEffect(() => {
    void refreshCount();
    const timer = window.setInterval(refreshCount, POLL_MS);
    const onVisible = () => {
      if (document.visibilityState === "visible") void refreshCount();
    };
    document.addEventListener("visibilitychange", onVisible);
    return () => {
      window.clearInterval(timer);
      document.removeEventListener("visibilitychange", onVisible);
    };
  }, [refreshCount]);

  useRealtimeSignals(userTopic(userId), () => {
    void refreshCount();
    if (open) void loadItems();
  });

  useEffect(() => {
    if (!open) return;
    void loadItems();
    const onPointer = (event: PointerEvent) => {
      if (!root.current?.contains(event.target as Node)) setOpen(false);
    };
    const onKey = (event: KeyboardEvent) => {
      if (event.key === "Escape") setOpen(false);
    };
    document.addEventListener("pointerdown", onPointer);
    document.addEventListener("keydown", onKey);
    return () => {
      document.removeEventListener("pointerdown", onPointer);
      document.removeEventListener("keydown", onKey);
    };
  }, [open, loadItems]);

  async function markOne(item: AtlasNotification) {
    if (item.read_at) return;
    try {
      await markNotificationRead(item.id);
      setItems((list) =>
        list?.map((n) => (n.id === item.id ? { ...n, read_at: new Date().toISOString() } : n)) ?? null,
      );
      void refreshCount();
    } catch {
      // Leave it unread; the next poll shows the truth.
    }
  }

  async function markAll() {
    try {
      await markAllNotificationsRead();
      await Promise.all([refreshCount(), loadItems()]);
    } catch {
      setFailed(true);
    }
  }

  if (hidden) return null;
  const unread = count ?? 0;

  return (
    <div ref={root} className="relative">
      <button
        type="button"
        aria-label={unread > 0 ? `Notifications, ${unread} unread` : "Notifications"}
        aria-expanded={open}
        aria-haspopup="dialog"
        onClick={() => setOpen((value) => !value)}
        className="relative grid h-11 w-11 flex-none place-items-center rounded-control border border-line-strong bg-white text-night transition hover:border-[#C3CCDD]"
      >
        <Icon name="bell" />
        {unread > 0 && (
          <span
            aria-hidden="true"
            className="absolute right-[11px] top-2.5 h-[9px] w-[9px] rounded-full border-2 border-white bg-[#F04438]"
          />
        )}
      </button>

      {open && (
        <div
          role="dialog"
          aria-label="Notifications"
          className="absolute right-0 top-[calc(100%+8px)] z-30 w-[min(380px,calc(100vw-32px))] animate-fade-in overflow-hidden rounded-card border border-line bg-white shadow-pop"
        >
          <div className="flex items-center justify-between gap-3 border-b border-line px-[18px] py-3.5">
            <div>
              <h2 className="text-[15px] font-semibold text-night">Notifications</h2>
              <p className="text-[13px] text-muted">
                {unread > 0 ? `${unread} unread` : "You’re all caught up"}
              </p>
            </div>
            {unread > 0 && (
              <button
                type="button"
                onClick={markAll}
                className="rounded-[10px] px-2.5 py-1.5 text-[13.5px] font-semibold text-atlas-ink hover:bg-[#EEF3FF]"
              >
                Mark all read
              </button>
            )}
          </div>

          <div className="max-h-[420px] overflow-y-auto scroll-quiet">
            {items === null ? (
              <p className="px-[18px] py-6 text-sm text-muted">Loading…</p>
            ) : failed ? (
              <p className="px-[18px] py-6 text-sm text-muted">
                Notifications couldn’t be loaded. Try again shortly.
              </p>
            ) : items.length === 0 ? (
              <div className="p-4">
                <EmptyState icon="bell" iconTone="info" title="Nothing yet">
                  Escalations, leave decisions and missed reports land here.
                </EmptyState>
              </div>
            ) : (
              <ul>
                {items.map((item) => {
                  const href = hrefFor(item);
                  const body = (
                    <>
                      <span
                        aria-hidden="true"
                        className={cx(
                          "mt-1.5 h-2 w-2 flex-none rounded-full",
                          item.read_at ? "bg-transparent" : item.critical ? "bg-[#F04438]" : "bg-atlas",
                        )}
                      />
                      <span className="min-w-0 flex-1">
                        <b className={cx("block text-sm", item.read_at ? "font-medium text-muted" : "font-semibold text-night")}>
                          {item.subject || "Notification"}
                        </b>
                        {item.body && (
                          <span className="mt-0.5 line-clamp-2 block text-[13px] text-muted">
                            {item.body}
                          </span>
                        )}
                        <span className="mt-1 block text-xs text-[#7A879C]">
                          {timeAgo(item.created_at)}
                        </span>
                      </span>
                    </>
                  );
                  const className =
                    "flex w-full gap-3 border-t border-line px-[18px] py-3 text-left transition first:border-t-0 hover:bg-[#F7F9FC]";
                  return (
                    <li key={item.id}>
                      {href ? (
                        <Link
                          href={href}
                          className={className}
                          onClick={() => {
                            void markOne(item);
                            setOpen(false);
                          }}
                        >
                          {body}
                        </Link>
                      ) : (
                        <button type="button" className={className} onClick={() => markOne(item)}>
                          {body}
                        </button>
                      )}
                    </li>
                  );
                })}
              </ul>
            )}
          </div>
        </div>
      )}
    </div>
  );
}
