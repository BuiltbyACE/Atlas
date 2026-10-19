"use client";

import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { Fragment, useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { createPortal } from "react-dom";

import { BlobButton } from "@/components/blob-button";
import { Icon, type IconName } from "@/components/icons";
import { SignalRefresh } from "@/components/signal-refresh";
import { cx } from "@/components/ui";
import { userTopic } from "@/lib/realtime/topics";

import { ALL_LINKS, breadcrumbFor } from "./navigation";
import { NotificationsBell } from "./notifications-bell";
import { Sidebar } from "./sidebar";

/** Things to create, shared by the "New" menu and the quick-jump list. */
const CREATE_ACTIONS: Array<{ label: string; href: string; icon: IconName; hint: string }> = [
  { label: "New task", href: "/work/tasks?new=task", icon: "board", hint: "On the task board" },
  { label: "New project", href: "/work?new=project", icon: "folder", hint: "Owner, target and type" },
  { label: "File a daily report", href: "/daily-reports", icon: "clipboard", hint: "What moved today" },
  { label: "Request leave", href: "/people?new=leave", icon: "calendarPlus", hint: "Goes to an approver" },
];

/**
 * The sticky workspace bar (prototype `.ax-top`): breadcrumb, quick jump,
 * live indicator, notifications and the "New" menu. Below `lg` it also owns
 * the menu button that opens the sidebar as a drawer.
 */
export function TopBar({
  userId,
  userName,
  userEmail,
  escalations,
}: {
  userId: string;
  userName: string;
  userEmail: string;
  escalations?: string;
}) {
  const pathname = usePathname();
  const trail = breadcrumbFor(pathname);
  const [menuOpen, setMenuOpen] = useState(false);
  const [jumpOpen, setJumpOpen] = useState(false);

  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === "k") {
        event.preventDefault();
        setJumpOpen(true);
      }
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, []);

  useEffect(() => setMenuOpen(false), [pathname]);

  return (
    <header className="sticky top-0 z-20 flex min-h-[68px] flex-wrap items-center gap-x-3.5 gap-y-2.5 border-b border-line bg-canvas/90 px-4 py-2.5 backdrop-blur-md sm:px-8 lg:flex-nowrap lg:py-0">
      <button
        type="button"
        onClick={() => setMenuOpen(true)}
        aria-label="Open navigation"
        className="grid h-11 w-11 flex-none place-items-center rounded-control border border-line-strong bg-white text-night lg:hidden"
      >
        <Icon name="menu" />
      </button>

      <nav aria-label="Breadcrumb" className="flex min-w-0 items-center gap-2 whitespace-nowrap text-[14.5px] text-muted">
        <Link href="/dashboard" className="hover:text-night">
          Atlas
        </Link>
        {trail.map((crumb, index) => (
          <Fragment key={index}>
            <Icon name="chevronRight" className="h-3.5 w-3.5" />
            {crumb.href ? (
              <Link href={crumb.href} className="hover:text-night">
                {crumb.label}
              </Link>
            ) : index === trail.length - 1 ? (
              <b className="truncate font-semibold text-night" aria-current="page">
                {crumb.label}
              </b>
            ) : (
              <span>{crumb.label}</span>
            )}
          </Fragment>
        ))}
      </nav>

      <button
        type="button"
        onClick={() => setJumpOpen(true)}
        className="order-last flex h-[42px] w-full items-center gap-2.5 rounded-control border border-line-strong bg-white pl-3.5 pr-2.5 text-left text-[14.5px] text-[#7A879C] transition hover:border-[#B8C3D6] lg:order-none lg:ml-auto lg:w-auto lg:flex-[0_1_420px]"
      >
        <Icon name="search" className="text-muted" />
        <span className="flex-1 truncate">Jump to a page or action…</span>
        <kbd className="whitespace-nowrap rounded-md border border-b-2 border-line-strong px-1.5 py-px font-mono text-[11px] text-muted">
          Ctrl K
        </kbd>
      </button>

      <div className="ml-auto flex items-center gap-2.5 lg:ml-0">
        <span className="hidden sm:inline-flex">
          <SignalRefresh topic={userTopic(userId)} />
        </span>
        <NotificationsBell userId={userId} />
        <NewMenu />
      </div>

      {menuOpen && (
        <MobileNav onClose={() => setMenuOpen(false)}>
          <Sidebar
            userName={userName}
            userEmail={userEmail}
            escalations={escalations}
            onNavigate={() => setMenuOpen(false)}
          />
        </MobileNav>
      )}
      {jumpOpen && <QuickJump onClose={() => setJumpOpen(false)} />}
    </header>
  );
}

function NewMenu() {
  const [open, setOpen] = useState(false);
  const root = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!open) return;
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
  }, [open]);

  return (
    <div ref={root} className="relative">
      <BlobButton
        variant="primary"
        aria-haspopup="menu"
        aria-expanded={open}
        onClick={() => setOpen((value) => !value)}
      >
        <Icon name="plus" />
        New
      </BlobButton>
      {open && (
        <div
          role="menu"
          className="absolute right-0 top-[calc(100%+8px)] z-30 w-[260px] animate-fade-in rounded-[14px] border border-line bg-white p-1.5 shadow-pop"
        >
          {CREATE_ACTIONS.map((action) => (
            <Link
              key={action.href}
              role="menuitem"
              href={action.href}
              onClick={() => setOpen(false)}
              className="flex items-center gap-3 rounded-[10px] px-2.5 py-2 text-night transition hover:bg-[#F3F6FC]"
            >
              <span className="grid h-9 w-9 flex-none place-items-center rounded-[10px] bg-[#E8F0FF] text-atlas-ink">
                <Icon name={action.icon} />
              </span>
              <span>
                <b className="block text-sm font-semibold">{action.label}</b>
                <small className="block text-xs text-muted">{action.hint}</small>
              </span>
            </Link>
          ))}
        </div>
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

interface JumpItem {
  label: string;
  href: string;
  icon: IconName;
  group: string;
  keywords: string;
}

/** Ctrl/⌘ K: jump to any workspace page or start a common action. */
function QuickJump({ onClose }: { onClose: () => void }) {
  const router = useRouter();
  const [query, setQuery] = useState("");
  const [active, setActive] = useState(0);
  const input = useRef<HTMLInputElement>(null);

  const items = useMemo<JumpItem[]>(() => {
    const pages = ALL_LINKS.map((link) => ({
      label: link.label,
      href: link.href,
      icon: link.icon,
      group: link.section ?? "Workspace",
      keywords: `${link.section ?? ""} ${link.keywords ?? ""}`,
    }));
    const actions = CREATE_ACTIONS.map((action) => ({
      label: action.label,
      href: action.href,
      icon: action.icon,
      group: "Create",
      keywords: action.hint,
    }));
    const q = query.trim().toLowerCase();
    return [...pages, ...actions].filter(
      (item) => !q || `${item.label} ${item.group} ${item.keywords}`.toLowerCase().includes(q),
    );
  }, [query]);

  useEffect(() => {
    input.current?.focus();
    const previous = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    return () => {
      document.body.style.overflow = previous;
    };
  }, []);

  useEffect(() => setActive(0), [query]);

  function go(item: JumpItem | undefined) {
    if (!item) return;
    onClose();
    router.push(item.href);
  }

  return createPortal(
    <div className="app-shell font-sans">
      <button
        type="button"
        aria-label="Close"
        onClick={onClose}
        className="fixed inset-0 z-40 animate-fade-in bg-[rgba(0,20,48,.38)]"
      />
      <div
        role="dialog"
        aria-modal="true"
        aria-label="Jump to"
        className="fixed left-1/2 top-[12vh] z-[41] w-[min(560px,calc(100vw-32px))] -translate-x-1/2 animate-fade-in overflow-hidden rounded-card border border-line bg-white text-night shadow-pop"
      >
        <label className="flex items-center gap-2.5 border-b border-line px-4">
          <Icon name="search" className="text-muted" />
          <span className="sr-only">Jump to</span>
          <input
            ref={input}
            value={query}
            onChange={(event) => setQuery(event.target.value)}
            onKeyDown={(event) => {
              if (event.key === "ArrowDown") {
                event.preventDefault();
                setActive((i) => Math.min(i + 1, items.length - 1));
              } else if (event.key === "ArrowUp") {
                event.preventDefault();
                setActive((i) => Math.max(i - 1, 0));
              } else if (event.key === "Enter") {
                event.preventDefault();
                go(items[active]);
              } else if (event.key === "Escape") {
                onClose();
              }
            }}
            role="combobox"
            aria-expanded="true"
            aria-controls="quick-jump-list"
            aria-activedescendant={items[active] ? `jump-${active}` : undefined}
            placeholder="Jump to a page or action…"
            className="h-14 flex-1 bg-transparent text-base text-night outline-none placeholder:text-[#7A879C]"
          />
        </label>
        <ul id="quick-jump-list" role="listbox" className="max-h-[50vh] overflow-y-auto p-1.5 scroll-quiet">
          {items.length === 0 && (
            <li className="px-3 py-6 text-center text-sm text-muted">Nothing matches “{query}”.</li>
          )}
          {items.map((item, index) => (
            <li
              key={`${item.group}-${item.href}`}
              id={`jump-${index}`}
              role="option"
              aria-selected={index === active}
              onMouseEnter={() => setActive(index)}
              onClick={() => go(item)}
              className={cx(
                "flex cursor-pointer items-center gap-3 rounded-[10px] px-3 py-2.5",
                index === active ? "bg-[#EEF3FF]" : "",
              )}
            >
              <Icon name={item.icon} className="text-atlas-ink" />
              <span className="flex-1 text-[14.5px] font-medium">{item.label}</span>
              <span className="text-xs text-muted">{item.group}</span>
            </li>
          ))}
        </ul>
      </div>
    </div>,
    document.body,
  );
}
