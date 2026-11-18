"use client";

import { usePathname } from "next/navigation";
import { useCallback, useEffect, useMemo, useState, type ReactNode } from "react";
import { createPortal } from "react-dom";

import { cx } from "@/components/ui";

import {
  allActions,
  linkForPath,
  type NavModel,
} from "./navigation";
import { QuickJump, TopBar } from "./top-bar";
import { Sidebar } from "./sidebar";

const COLLAPSE_KEY = "atlas.sidebar.collapsed";
const PINNED_KEY = "atlas.nav.pinned";
const RECENT_KEY = "atlas.nav.recent";
const RECENT_LIMIT = 5;

/** Read a string array from localStorage, tolerating absent or corrupt values. */
function readList(key: string): string[] {
  try {
    const raw = window.localStorage.getItem(key);
    const parsed: unknown = raw ? JSON.parse(raw) : [];
    return Array.isArray(parsed) ? parsed.filter((v): v is string => typeof v === "string") : [];
  } catch {
    return [];
  }
}

function writeList(key: string, values: string[]) {
  try {
    window.localStorage.setItem(key, JSON.stringify(values));
  } catch {
    // Storage can be unavailable (private mode, quota); the preference simply
    // does not persist, which must never block navigation.
  }
}

/**
 * The authenticated workspace shell. It owns the three pieces of shell state
 * that outlive a page: the collapsed rail preference, pinned destinations and
 * recently visited destinations.
 *
 * Pinned and recent links are stored as hrefs but rendered only when the href
 * is present in the current Go navigation model. Revoking a permission removes
 * the destination from that model, so a stale pin can never become an
 * authorisation shortcut.
 */
export function AppShell({
  user,
  model,
  escalations,
  children,
}: {
  user: { id: string; name: string; email: string };
  model: NavModel;
  escalations?: string;
  children: ReactNode;
}) {
  const pathname = usePathname();
  const actions = useMemo(() => allActions(model), [model]);

  const [collapsed, setCollapsed] = useState(false);
  const [ready, setReady] = useState(false);
  const [menuOpen, setMenuOpen] = useState(false);
  const [jumpOpen, setJumpOpen] = useState(false);
  const [pinned, setPinned] = useState<string[]>([]);
  const [recent, setRecent] = useState<string[]>([]);

  // Restore preferences after mount so the server and first client render agree.
  useEffect(() => {
    setCollapsed(window.localStorage.getItem(COLLAPSE_KEY) === "1");
    setPinned(readList(PINNED_KEY));
    setRecent(readList(RECENT_KEY));
    setReady(true);
  }, []);

  useEffect(() => {
    if (!ready) return;
    try {
      window.localStorage.setItem(COLLAPSE_KEY, collapsed ? "1" : "0");
    } catch {
      // Non-fatal: the rail state still works for this page view.
    }
  }, [collapsed, ready]);

  // Record the destination being viewed, but only when Go still offers it.
  useEffect(() => {
    const item = linkForPath(model, pathname);
    if (!item) return;
    setRecent((previous) => {
      const next = [item.href, ...previous.filter((href) => href !== item.href)].slice(
        0,
        RECENT_LIMIT,
      );
      writeList(RECENT_KEY, next);
      return next;
    });
  }, [model, pathname]);

  useEffect(() => setMenuOpen(false), [pathname]);

  const togglePin = useCallback((href: string) => {
    setPinned((previous) => {
      const next = previous.includes(href)
        ? previous.filter((value) => value !== href)
        : [...previous, href];
      writeList(PINNED_KEY, next);
      return next;
    });
  }, []);

  const toggleCollapse = useCallback(() => setCollapsed((value) => !value), []);

  const sidebar = (
    <Sidebar
      model={model}
      userName={user.name}
      userEmail={user.email}
      escalations={escalations}
      collapsed={collapsed}
      collapsible
      pinned={pinned}
      recent={recent}
      onTogglePin={togglePin}
      onToggleCollapse={toggleCollapse}
      onOpenSearch={() => setJumpOpen(true)}
      onNavigate={() => setMenuOpen(false)}
    />
  );

  return (
    <div
      className={cx(
        "app-shell min-h-screen bg-canvas text-[15px] leading-[1.45] text-night lg:grid",
        collapsed
          ? "lg:grid-cols-[84px_minmax(0,1fr)]"
          : "lg:grid-cols-[264px_minmax(0,1fr)]",
      )}
    >
      <div data-atlas-region="sidebar" className="sticky top-0 hidden h-screen lg:block">
        {sidebar}
      </div>

      <div data-atlas-region="content" className="flex min-w-0 flex-col">
        <TopBar
          model={model}
          userId={user.id}
          actions={actions}
          escalations={escalations}
          onOpenMenu={() => setMenuOpen(true)}
          onOpenSearch={() => setJumpOpen(true)}
        />
        <main className="mx-auto w-full max-w-[1360px] px-4 pb-14 pt-5 sm:px-8 sm:pt-7">
          {children}
        </main>
      </div>

      {menuOpen && (
        <MobileNav onClose={() => setMenuOpen(false)}>
          <Sidebar
            model={model}
            userName={user.name}
            userEmail={user.email}
            escalations={escalations}
            collapsed={false}
            collapsible={false}
            pinned={pinned}
            recent={recent}
            onTogglePin={togglePin}
            onOpenSearch={() => {
              setMenuOpen(false);
              setJumpOpen(true);
            }}
            onNavigate={() => setMenuOpen(false)}
          />
        </MobileNav>
      )}

      {jumpOpen && (
        <QuickJump
          model={model}
          actions={actions}
          onClose={() => setJumpOpen(false)}
        />
      )}
    </div>
  );
}

function MobileNav({ children, onClose }: { children: ReactNode; onClose: () => void }) {
  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if (event.key === "Escape") onClose();
    };
    document.addEventListener("keydown", onKey);
    const previous = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    return () => {
      document.removeEventListener("keydown", onKey);
      document.body.style.overflow = previous;
    };
  }, [onClose]);

  return createPortal(
    <div className="app-shell font-sans lg:hidden">
      <button
        type="button"
        aria-label="Close navigation"
        onClick={onClose}
        className="fixed inset-0 z-40 animate-fade-in bg-[rgba(0,20,48,.38)]"
      />
      <div className="fixed inset-y-0 left-0 z-[41] w-[min(290px,86vw)] animate-fade-in shadow-drawer">
        {children}
      </div>
    </div>,
    document.body,
  );
}
