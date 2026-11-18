"use client";

import Image from "next/image";
import Link from "next/link";
import { usePathname } from "next/navigation";
import { useEffect, useMemo, useState, type ReactNode } from "react";

import { Blobs } from "@/components/blob-button";
import { Icon } from "@/components/icons";
import { SignOutButton } from "@/components/sign-out-button";
import { cx } from "@/components/ui";
import { initials } from "@/lib/format";

import { linkForPath, type NavItem, type NavModel } from "./navigation";

const CLOSED_KEY = "atlas.nav.closed";

function readClosed(): string[] {
  try {
    const raw = window.localStorage.getItem(CLOSED_KEY);
    const parsed: unknown = raw ? JSON.parse(raw) : [];
    return Array.isArray(parsed) ? parsed.filter((v): v is string => typeof v === "string") : [];
  } catch {
    return [];
  }
}

/**
 * The navy workspace sidebar (prototype `.ax-side`).
 *
 * It renders only the destinations Go put in the navigation response. Sections
 * are collapsible groups; the desktop sidebar can collapse to an icon rail with
 * tooltips; below `lg` the same component fills the drawer. Pinned, recent and
 * account controls are separate from the permission-driven module list.
 *
 * Every link is still just a link: Go enforces each destination's permission on
 * the request, so the sidebar can hide but never guard.
 */
export function Sidebar({
  model,
  userName,
  userEmail,
  escalations,
  collapsed = false,
  collapsible = false,
  pinned = [],
  recent = [],
  onTogglePin,
  onToggleCollapse,
  onOpenSearch,
  onNavigate,
}: {
  model: NavModel;
  userName: string;
  userEmail: string;
  /** Open-escalations count for the badge ("99+" when capped); hidden at 0. */
  escalations?: string;
  collapsed?: boolean;
  /** Show the desktop collapse control. Off inside the mobile drawer. */
  collapsible?: boolean;
  pinned?: string[];
  recent?: string[];
  onTogglePin?: (href: string) => void;
  onToggleCollapse?: () => void;
  onOpenSearch?: () => void;
  /** Called after a link is followed (closes the mobile drawer). */
  onNavigate?: () => void;
}) {
  const pathname = usePathname();
  const current = linkForPath(model, pathname)?.href;
  const [query, setQuery] = useState("");
  const [closed, setClosed] = useState<string[]>([]);

  useEffect(() => setClosed(readClosed()), []);

  const toggleGroup = (key: string) => {
    setClosed((previous) => {
      const next = previous.includes(key)
        ? previous.filter((value) => value !== key)
        : [...previous, key];
      try {
        window.localStorage.setItem(CLOSED_KEY, JSON.stringify(next));
      } catch {
        // Non-fatal: the group still opens for this page view.
      }
      return next;
    });
  };

  const byHref = useMemo(
    () => new Map(model.items.map((item) => [item.href, item])),
    [model],
  );
  const pinnedItems = pinned
    .map((href) => byHref.get(href))
    .filter((item): item is NavItem => Boolean(item));
  const recentItems = recent
    .map((href) => byHref.get(href))
    .filter((item): item is NavItem => item !== undefined && !pinned.includes(item.href));

  const accountItems =
    model.sections.find((section) => section.key === "account")?.items ?? [];
  const moduleSections = model.sections.filter((section) => section.key !== "account");

  const q = query.trim().toLowerCase();
  const searchResults = q
    ? model.items.filter((item) =>
        `${item.label} ${item.keywords ?? ""}`.toLowerCase().includes(q),
      )
    : [];

  const badgeFor = (item: NavItem) =>
    item.badge === "escalations" ? escalations : undefined;

  return (
    <aside
      aria-label="Main navigation"
      className={cx(
        "on-navy flex h-full flex-col overflow-y-auto bg-navy pb-4 pt-[22px] text-navy-text scroll-quiet",
        collapsed ? "px-2" : "px-3.5",
      )}
    >
      <div className={cx("flex items-center pb-[18px]", collapsed ? "justify-center" : "gap-2")}>
        <Link
          href="/dashboard"
          onClick={onNavigate}
          className={cx(
            "flex items-center gap-3 text-white",
            collapsed ? "justify-center px-1" : "min-w-0 px-2.5 pt-0.5",
          )}
          title="Atlas"
        >
          <Image
            src="/brand/safaristack-mark.png"
            alt=""
            width={331}
            height={299}
            className="h-auto w-[42px] flex-none"
            priority
          />
          {!collapsed && (
            <span className="min-w-0">
              <b className="block text-[21px] font-semibold leading-tight text-white">Atlas</b>
              <small className="mt-0.5 block text-[12.5px] text-navy-soft">
                SafariStack Company OS
              </small>
            </span>
          )}
        </Link>
        {collapsible && !collapsed && (
          <button
            type="button"
            onClick={onToggleCollapse}
            aria-label="Collapse sidebar"
            className="ml-auto grid h-8 w-8 flex-none place-items-center rounded-[9px] text-navy-soft transition hover:bg-white/10 hover:text-white focus-visible:outline focus-visible:outline-2 focus-visible:outline-sky"
          >
            <Icon name="chevronLeft" />
          </button>
        )}
      </div>

      {collapsed ? (
        collapsible && (
          <button
            type="button"
            onClick={onToggleCollapse}
            aria-label="Expand sidebar"
            className="mx-auto mb-3 grid h-8 w-8 place-items-center rounded-[9px] text-navy-soft transition hover:bg-white/10 hover:text-white focus-visible:outline focus-visible:outline-2 focus-visible:outline-sky"
          >
            <Icon name="chevronRight" />
          </button>
        )
      ) : (
        <div className="mb-2 flex items-center gap-1.5">
          <button
            type="button"
            onClick={onOpenSearch}
            className="flex min-h-9 flex-1 items-center gap-2 rounded-[9px] border border-white/10 bg-white/[0.06] px-2.5 text-left text-[13px] text-navy-soft transition hover:border-white/20 hover:text-white focus-visible:outline focus-visible:outline-2 focus-visible:outline-sky"
          >
            <Icon name="search" className="h-4 w-4" />
            Search navigation
          </button>
        </div>
      )}

      <nav aria-label="Sections" className="flex flex-col">
        {!collapsed && q ? (
          <div>
            <SectionTitle>Search results</SectionTitle>
            <div className="flex flex-col gap-0.5">
              {searchResults.length === 0 ? (
                <p className="px-3 py-3 text-[13.5px] text-navy-dim">
                  Nothing matches “{query}”.
                </p>
              ) : (
                searchResults.map((item) => (
                  <NavItemRow
                    key={item.href}
                    item={item}
                    active={current === item.href}
                    badge={badgeFor(item)}
                    onNavigate={onNavigate}
                  />
                ))
              )}
            </div>
          </div>
        ) : (
          <>
            {!collapsed && pinnedItems.length > 0 && (
              <NavGroup title="Pinned">
                {pinnedItems.map((item) => (
                  <NavItemRow
                    key={`pinned-${item.href}`}
                    item={item}
                    active={current === item.href}
                    badge={badgeFor(item)}
                    pinned
                    onTogglePin={onTogglePin}
                    onNavigate={onNavigate}
                  />
                ))}
              </NavGroup>
            )}
            {!collapsed && recentItems.length > 0 && (
              <NavGroup title="Recent">
                {recentItems.map((item) => (
                  <NavItemRow
                    key={`recent-${item.href}`}
                    item={item}
                    active={current === item.href}
                    badge={badgeFor(item)}
                    pinned={pinned.includes(item.href)}
                    onTogglePin={onTogglePin}
                    onNavigate={onNavigate}
                  />
                ))}
              </NavGroup>
            )}

            {moduleSections.map((section, index) => {
              const isClosed = closed.includes(section.key);
              return (
                <div
                  key={section.key || index}
                  className={collapsed ? "border-t border-white/10 pt-1 first:border-t-0" : ""}
                >
                  {section.title && !collapsed && (
                    <SectionTitle
                      collapsible
                      open={!isClosed}
                      onToggle={() => toggleGroup(section.key)}
                      controls={`nav-group-${section.key}`}
                    >
                      {section.title}
                    </SectionTitle>
                  )}
                  <div
                    id={`nav-group-${section.key}`}
                    hidden={!collapsed && Boolean(section.title) && isClosed}
                    className="flex flex-col gap-0.5"
                  >
                    {section.items.map((item) => (
                      <NavItemRow
                        key={item.href}
                        item={item}
                        active={current === item.href}
                        badge={badgeFor(item)}
                        collapsed={collapsed}
                        pinned={pinned.includes(item.href)}
                        onTogglePin={onTogglePin}
                        onNavigate={onNavigate}
                      />
                    ))}
                  </div>
                </div>
              );
            })}
          </>
        )}
      </nav>

      <div className="mt-auto flex flex-col gap-2 border-t border-white/10 pt-3.5">
        {accountItems.map((item) => (
          <NavItemRow
            key={item.href}
            item={item}
            active={current === item.href}
            collapsed={collapsed}
            onNavigate={onNavigate}
          />
        ))}
        <div
          className={cx(
            "flex items-center gap-2.5 rounded-[14px] bg-white/[0.06] py-2 pl-2.5 pr-2",
            collapsed && "flex-col px-1.5",
          )}
        >
          <span
            aria-hidden="true"
            className="grid h-8 w-8 flex-none place-items-center rounded-full bg-sky text-xs font-semibold text-navy"
          >
            {initials(userName)}
          </span>
          {!collapsed && (
            <span className="min-w-0">
              <b className="block truncate text-sm font-semibold text-white">{userName}</b>
              <small className="block truncate text-xs text-navy-soft" title={userEmail}>
                {userEmail}
              </small>
            </span>
          )}
          <span className={collapsed ? "" : "ml-auto"}>
            <SignOutButton compact />
          </span>
        </div>
      </div>
    </aside>
  );
}

function NavGroup({ title, children }: { title: string; children: ReactNode }) {
  return (
    <div>
      <SectionTitle>{title}</SectionTitle>
      <div className="flex flex-col gap-0.5">{children}</div>
    </div>
  );
}

function SectionTitle({
  children,
  collapsible = false,
  open = true,
  onToggle,
  controls,
}: {
  children: string;
  collapsible?: boolean;
  open?: boolean;
  onToggle?: () => void;
  controls?: string;
}) {
  const label = (
    <>
      {children}
      {collapsible && (
        <Icon
          name="chevronDown"
          className={cx("h-3 w-3 transition-transform", open ? "" : "-rotate-90")}
        />
      )}
    </>
  );
  const classes =
    "mx-3 mb-1.5 mt-5 flex w-[calc(100%-1.5rem)] items-center justify-between gap-2 text-left text-[11px] font-semibold uppercase tracking-[0.12em] text-navy-dim";
  if (!collapsible) {
    return <p className={classes}>{label}</p>;
  }
  return (
    <button
      type="button"
      onClick={onToggle}
      aria-expanded={open}
      aria-controls={controls}
      className={cx(
        classes,
        "rounded-[7px] transition hover:text-white focus-visible:outline focus-visible:outline-2 focus-visible:outline-sky",
      )}
    >
      {label}
    </button>
  );
}

function NavItemRow({
  item,
  active,
  badge,
  collapsed = false,
  pinned = false,
  onTogglePin,
  onNavigate,
}: {
  item: NavItem;
  active: boolean;
  badge?: string;
  collapsed?: boolean;
  pinned?: boolean;
  onTogglePin?: (href: string) => void;
  onNavigate?: () => void;
}) {
  const link = (
    <Link
      href={item.href}
      onClick={onNavigate}
      aria-current={active ? "page" : undefined}
      title={collapsed ? item.label : undefined}
      className={cx(
        active
          ? "flex min-h-10 items-center gap-3 whitespace-nowrap rounded-[11px] bg-white px-3 text-[15px] font-semibold text-navy shadow-[0_6px_18px_rgba(0,0,0,.18)]"
          : "blob-btn blob-btn--fast blob-btn--nav",
        collapsed && "justify-center px-2",
        onTogglePin && "flex-1",
      )}
    >
      <Icon name={item.icon} className={active ? "text-atlas" : "opacity-80"} />
      {collapsed ? (
        <span className="sr-only">{item.label}</span>
      ) : (
        <span className="min-w-0 flex-1 truncate">{item.label}</span>
      )}
      {badge && !collapsed && (
        <span
          className="ml-auto min-w-7 rounded-full bg-[#FFB020] px-2 py-0.5 text-center text-xs font-bold text-[#3A2600]"
          aria-label={`${badge} open`}
        >
          {badge}
        </span>
      )}
      {!active && <Blobs />}
    </Link>
  );

  if (!onTogglePin || collapsed) return link;

  return (
    <div className="group flex items-center gap-0.5">
      {link}
      <button
        type="button"
        onClick={() => onTogglePin(item.href)}
        aria-label={pinned ? `Unpin ${item.label}` : `Pin ${item.label}`}
        aria-pressed={pinned}
        className={cx(
          "grid h-7 w-7 flex-none place-items-center rounded-[7px] transition focus-visible:outline focus-visible:outline-2 focus-visible:outline-sky",
          pinned
            ? "text-sky opacity-100"
            : "text-navy-soft opacity-0 group-hover:opacity-100 group-focus-within:opacity-100",
        )}
      >
        <Icon name="pin" className="h-4 w-4" />
      </button>
    </div>
  );
}
