"use client";

import Image from "next/image";
import Link from "next/link";
import { usePathname } from "next/navigation";

import { Blobs } from "@/components/blob-button";
import { Icon } from "@/components/icons";
import { SignOutButton } from "@/components/sign-out-button";
import { cx } from "@/components/ui";
import { initials } from "@/lib/format";

import {
  COMING_NEXT,
  NAV_SECTIONS,
  SETTINGS_LINK,
  linkForPath,
  type NavLink,
} from "./navigation";

/**
 * The navy workspace sidebar (prototype `.ax-side`). The current page is
 * marked with `aria-current="page"`; modules not built yet are muted labels.
 * Rendered fixed on desktop and inside the mobile menu drawer below `lg`.
 */
export function Sidebar({
  userName,
  userEmail,
  escalations,
  onNavigate,
}: {
  userName: string;
  userEmail: string;
  /** Open-escalations count for the badge ("99+" when capped); hidden at 0. */
  escalations?: string;
  /** Called after a link is followed (closes the mobile drawer). */
  onNavigate?: () => void;
}) {
  const pathname = usePathname();
  const current = linkForPath(pathname)?.href;

  return (
    <aside
      aria-label="Main navigation"
      className="on-navy flex h-full flex-col overflow-y-auto bg-navy px-3.5 pb-4 pt-[22px] text-navy-text scroll-quiet"
    >
      <Link
        href="/dashboard"
        onClick={onNavigate}
        className="flex items-center gap-3 px-2.5 pb-[18px] pt-0.5 text-white"
      >
        <Image
          src="/brand/safaristack-mark.png"
          alt=""
          width={331}
          height={299}
          className="h-auto w-[42px]"
          priority
        />
        <span>
          <b className="block text-[21px] font-semibold leading-tight text-white">Atlas</b>
          <small className="mt-0.5 block text-[12.5px] text-navy-soft">
            SafariStack Company OS
          </small>
        </span>
      </Link>

      <nav aria-label="Sections" className="flex flex-col">
        {NAV_SECTIONS.map((section, index) => (
          <div key={section.title ?? index}>
            {section.title && <SectionTitle>{section.title}</SectionTitle>}
            <div className="flex flex-col gap-0.5">
              {section.links.map((link) => (
                <NavItem
                  key={link.href}
                  link={link}
                  active={current === link.href}
                  badge={link.badge === "escalations" ? escalations : undefined}
                  onNavigate={onNavigate}
                />
              ))}
            </div>
          </div>
        ))}

        <SectionTitle>Coming next</SectionTitle>
        <div className="flex flex-col gap-0.5">
          {COMING_NEXT.map((item) => (
            <span
              key={item.label}
              title="Not built yet"
              className="flex min-h-10 items-center gap-3 whitespace-nowrap rounded-[11px] px-3 text-[15px] text-navy-dim"
            >
              <Icon name={item.icon} className="opacity-80" />
              {item.label}
              <span className="ml-auto rounded-full border border-navy-soft/30 px-2 py-px text-[10.5px] uppercase tracking-[0.08em] text-[#8EA4C8]">
                Soon
              </span>
            </span>
          ))}
        </div>
      </nav>

      <div className="mt-auto flex flex-col gap-2 border-t border-white/10 pt-3.5">
        <NavItem
          link={SETTINGS_LINK}
          active={current === SETTINGS_LINK.href}
          onNavigate={onNavigate}
        />
        <div className="flex items-center gap-2.5 rounded-[14px] bg-white/[0.06] py-2 pl-2.5 pr-2">
          <span
            aria-hidden="true"
            className="grid h-8 w-8 flex-none place-items-center rounded-full bg-sky text-xs font-semibold text-navy"
          >
            {initials(userName)}
          </span>
          <span className="min-w-0">
            <b className="block truncate text-sm font-semibold text-white">{userName}</b>
            <small className="block truncate text-xs text-navy-soft" title={userEmail}>
              {userEmail}
            </small>
          </span>
          <SignOutButton compact />
        </div>
      </div>
    </aside>
  );
}

function SectionTitle({ children }: { children: string }) {
  return (
    <p className="mx-3 mb-1.5 mt-5 text-[11px] font-semibold uppercase tracking-[0.12em] text-navy-dim">
      {children}
    </p>
  );
}

function NavItem({
  link,
  active,
  badge,
  onNavigate,
}: {
  link: NavLink;
  active: boolean;
  badge?: string;
  onNavigate?: () => void;
}) {
  return (
    <Link
      href={link.href}
      onClick={onNavigate}
      aria-current={active ? "page" : undefined}
      className={cx(
        active
          ? "flex min-h-10 items-center gap-3 whitespace-nowrap rounded-[11px] bg-white px-3 text-[15px] font-semibold text-navy shadow-[0_6px_18px_rgba(0,0,0,.18)]"
          : "blob-btn blob-btn--fast blob-btn--nav",
      )}
    >
      <Icon name={link.icon} className={active ? "text-atlas" : "opacity-80"} />
      {link.label}
      {badge && (
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
}
