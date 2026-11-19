/**
 * Atlas web - shared presentational primitives.
 *
 * Deliberately dumb: no data fetching, no permissions, no business rules. Both
 * server and client components import these, so the module stays hook-free and
 * free of server-only APIs.
 *
 * The look follows the workspace prototypes (`images/Atlas Design – 0x`):
 * paper cards on a canvas, pill badges, KPI tiles and dashed empty states.
 */

import Link from "next/link";
import type { ReactNode } from "react";

import { Icon, type IconName } from "@/components/icons";

/** Join class names, dropping falsey values. */
export function cx(...values: Array<string | false | null | undefined>): string {
  return values.filter(Boolean).join(" ");
}

// ── page chrome ─────────────────────────────────────────────────────────────

export function PageHeader({
  title,
  description,
  actions,
  eyebrow,
}: {
  title: ReactNode;
  description?: ReactNode;
  actions?: ReactNode;
  eyebrow?: ReactNode;
}) {
  return (
    <header className="flex flex-wrap items-end justify-between gap-x-6 gap-y-4">
      <div className="min-w-0">
        {eyebrow && <div className="mb-2">{eyebrow}</div>}
        <h1 className="text-[28px] font-semibold leading-tight tracking-[-0.015em] text-night sm:text-[32px]">
          {title}
        </h1>
        {description && (
          <div className="mt-2 max-w-[62ch] text-[15.5px] text-muted">{description}</div>
        )}
      </div>
      {actions && <div className="flex flex-wrap items-center gap-2.5">{actions}</div>}
    </header>
  );
}

export function Card({
  title,
  description,
  actions,
  children,
  className,
  padded = true,
  id,
}: {
  title?: ReactNode;
  description?: ReactNode;
  actions?: ReactNode;
  children?: ReactNode;
  className?: string;
  padded?: boolean;
  id?: string;
}) {
  const hasHeader = Boolean(title || actions);
  return (
    <section
      id={id}
      className={cx(
        "rounded-card border border-line bg-white shadow-card",
        className,
      )}
    >
      {hasHeader && (
        <div className="flex flex-wrap items-start justify-between gap-3 px-[22px] pb-3.5 pt-5">
          <div className="min-w-0">
            {title && <h2 className="text-[17px] font-semibold text-night">{title}</h2>}
            {description && (
              <div className="mt-0.5 text-[13.5px] text-muted">{description}</div>
            )}
          </div>
          {actions && <div className="flex flex-wrap items-center gap-2">{actions}</div>}
        </div>
      )}
      <div className={cx(padded && (hasHeader ? "px-[22px] pb-[22px]" : "p-[22px]"))}>
        {children}
      </div>
    </section>
  );
}

/** Small uppercase label used over numbers and in definition lists. */
export function Label({ children, className }: { children: ReactNode; className?: string }) {
  return (
    <p
      className={cx(
        "text-xs font-semibold uppercase tracking-[0.09em] text-muted",
        className,
      )}
    >
      {children}
    </p>
  );
}

// ── badges ──────────────────────────────────────────────────────────────────

export type Tone =
  | "neutral"
  | "info"
  | "success"
  | "warning"
  | "danger"
  | "accent"
  | "orange"
  | "violet"
  | "navy";

/** Pill colours (prototype `.p-*`). */
const PILL: Record<Tone, string> = {
  neutral: "bg-[#EEF1F6] text-[#3B4A63]",
  info: "bg-[#E8F0FF] text-[#1849A9]",
  accent: "bg-[#E8F0FF] text-[#1849A9]",
  success: "bg-[#E3F4EA] text-[#17663A]",
  warning: "bg-[#FFF1D6] text-[#8A4B00]",
  danger: "bg-[#FDE8E7] text-[#B42318]",
  orange: "bg-[#FFEBDD] text-[#B23C00]",
  violet: "bg-[#EFEBFF] text-[#5B3CC4]",
  navy: "bg-navy text-white",
};

/** Icon-tile colours (prototype `.i-*`). */
const TILE: Record<Tone, string> = {
  neutral: "bg-[#EEF1F6] text-[#3B4A63]",
  info: "bg-[#E8F0FF] text-atlas-ink",
  accent: "bg-[#E8F0FF] text-atlas-ink",
  success: "bg-[#E3F4EA] text-[#17803D]",
  warning: "bg-[#FFF1D6] text-[#B45309]",
  danger: "bg-[#FDE8E7] text-[#C0352B]",
  orange: "bg-[#FFEBDD] text-[#C2410C]",
  violet: "bg-[#EFEBFF] text-[#5B3CC4]",
  navy: "bg-navy text-white",
};

/** Solid swatches for dots, bars and timeline marks. */
export const SWATCH: Record<Tone, string> = {
  neutral: "#8A97AD",
  info: "#1A6CF5",
  accent: "#1A6CF5",
  success: "#22A55A",
  warning: "#F5A524",
  danger: "#D92D20",
  orange: "#E8590C",
  violet: "#7A5AF8",
  navy: "#00204D",
};

export function Badge({
  children,
  tone = "neutral",
  dot = false,
  className,
}: {
  children: ReactNode;
  tone?: Tone;
  /** Lead with a small dot in the text colour (status pills). */
  dot?: boolean;
  className?: string;
}) {
  return (
    <span
      className={cx(
        "inline-flex h-6 items-center gap-1.5 whitespace-nowrap rounded-full px-2.5 text-[12.5px] font-semibold leading-none",
        PILL[tone],
        className,
      )}
    >
      {dot && <i aria-hidden="true" className="h-1.5 w-1.5 rounded-full bg-current" />}
      {children}
    </span>
  );
}

/** A dark, square-ish tag (prototype `.tb-tag`), e.g. a task ref. */
export function Tag({ children, className }: { children: ReactNode; className?: string }) {
  return (
    <span
      className={cx(
        "inline-flex h-[22px] items-center rounded-md bg-navy px-2 font-mono text-[11px] font-medium tracking-[0.02em] text-white",
        className,
      )}
    >
      {children}
    </span>
  );
}

/** Map a workflow status to a badge tone (shared by work + people views). */
export function statusTone(status: string): Tone {
  switch (status) {
    case "active":
    case "available":
    case "completed":
    case "done":
    case "approved":
    case "submitted":
    case "reviewed":
    case "resolved":
      return "success";
    case "in_progress":
    case "planning":
    case "due":
    case "draft":
      return "info";
    case "review":
    case "requested":
    case "pending":
    case "paused":
    case "late":
    case "on_leave":
    case "open":
      return "warning";
    case "blocked":
    case "declined":
    case "exited":
    case "missed":
    case "overdue":
      return "danger";
    case "cancelled":
    case "not_started":
    case "todo":
    case "excused":
      return "neutral";
    default:
      return "neutral";
  }
}

export function priorityTone(priority: string): Tone {
  switch (priority) {
    case "urgent":
      return "danger";
    case "high":
      return "orange";
    case "medium":
      return "info";
    default:
      return "neutral";
  }
}

// ── avatars and tiles ───────────────────────────────────────────────────────

type AvatarVariant = "default" | "me" | "empty" | "system";
type AvatarSize = "sm" | "md" | "lg";

const AVATAR_SIZE: Record<AvatarSize, string> = {
  sm: "h-[26px] w-[26px] text-[11px]",
  md: "h-8 w-8 text-xs",
  lg: "h-[42px] w-[42px] text-[13px]",
};

const AVATAR_VARIANT: Record<AvatarVariant, string> = {
  default: "bg-[#DCE8FF] text-navy",
  me: "bg-sky text-navy",
  empty: "border-[1.5px] border-dashed border-[#A9B5C9] bg-transparent text-[#7A879C]",
  system: "bg-navy text-white",
};

/** Initials in a circle. `me` marks the viewer, `empty` an unassigned slot. */
export function Avatar({
  initials,
  variant = "default",
  size = "md",
  className,
}: {
  initials: ReactNode;
  variant?: AvatarVariant;
  size?: AvatarSize;
  className?: string;
}) {
  return (
    <span
      aria-hidden="true"
      className={cx(
        "inline-grid flex-none place-items-center rounded-full font-semibold tracking-[0.02em]",
        AVATAR_SIZE[size],
        AVATAR_VARIANT[variant],
        className,
      )}
    >
      {initials}
    </span>
  );
}

/** A rounded square holding an icon, tinted by tone (KPI and list markers). */
export function IconTile({
  icon,
  tone = "info",
  size = "md",
  round = false,
  className,
}: {
  icon: IconName;
  tone?: Tone;
  size?: "sm" | "md" | "lg";
  round?: boolean;
  className?: string;
}) {
  return (
    <span
      aria-hidden="true"
      className={cx(
        "grid flex-none place-items-center",
        size === "sm" && "h-8 w-8",
        size === "md" && "h-10 w-10",
        size === "lg" && "h-[42px] w-[42px]",
        round ? "rounded-full" : "rounded-control",
        TILE[tone],
        className,
      )}
    >
      <Icon name={icon} className={size === "sm" ? "h-4 w-4" : undefined} />
    </span>
  );
}

// ── chips (filters) ─────────────────────────────────────────────────────────

const CHIP =
  "inline-flex h-[38px] items-center gap-2 rounded-full border px-[15px] text-sm font-medium transition";

function chipClass(pressed: boolean): string {
  return cx(
    CHIP,
    pressed
      ? "border-navy bg-navy text-white"
      : "border-line-strong bg-white text-night hover:border-[#B8C3D6]",
  );
}

function ChipCount({ count, pressed }: { count: ReactNode; pressed: boolean }) {
  return (
    <span
      className={cx(
        "inline-grid h-5 min-w-[22px] place-items-center rounded-full px-1.5 text-xs font-semibold",
        pressed ? "bg-white/15 text-white" : "bg-[#EEF1F6] text-muted",
      )}
    >
      {count}
    </span>
  );
}

/** A filter chip that navigates (server-rendered filters). */
export function ChipLink({
  href,
  pressed,
  count,
  children,
}: {
  href: string;
  pressed: boolean;
  count?: ReactNode;
  children: ReactNode;
}) {
  return (
    <Link href={href} aria-current={pressed ? "true" : undefined} className={chipClass(pressed)}>
      {children}
      {count !== undefined && <ChipCount count={count} pressed={pressed} />}
    </Link>
  );
}

/** A filter chip that toggles client state. */
export function ChipButton({
  pressed,
  count,
  children,
  onClick,
  small = false,
}: {
  pressed: boolean;
  count?: ReactNode;
  children: ReactNode;
  onClick: () => void;
  small?: boolean;
}) {
  return (
    <button
      type="button"
      aria-pressed={pressed}
      onClick={onClick}
      className={cx(chipClass(pressed), small && "h-9 px-[13px] text-[13.5px]")}
    >
      {children}
      {count !== undefined && <ChipCount count={count} pressed={pressed} />}
    </button>
  );
}

export function Chips({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div role="group" aria-label={label} className="flex flex-wrap gap-2">
      {children}
    </div>
  );
}

// ── numbers ─────────────────────────────────────────────────────────────────

const VALUE_TONE: Partial<Record<Tone, string>> = {
  danger: "text-[#C0352B]",
  warning: "text-[#B45309]",
  success: "text-[#17803D]",
  accent: "text-atlas",
};

/** A KPI tile (prototype `.ax-kpi`). */
export function Stat({
  label,
  value,
  hint,
  tone = "neutral",
  icon,
  iconTone,
  foot,
}: {
  label: string;
  value: ReactNode;
  hint?: ReactNode;
  tone?: Tone;
  icon?: IconName;
  iconTone?: Tone;
  foot?: ReactNode;
}) {
  return (
    <article className="rounded-card border border-line bg-white px-[22px] py-5 shadow-card">
      <div className="flex items-center justify-between gap-2.5">
        <Label>{label}</Label>
        {icon && <IconTile icon={icon} tone={iconTone ?? "info"} />}
      </div>
      <p
        className={cx(
          "tabular mt-3.5 text-[40px] font-semibold leading-none tracking-[-0.02em]",
          VALUE_TONE[tone] ?? "text-night",
        )}
      >
        {value}
      </p>
      {hint && <p className="mt-2 text-[13.5px] text-muted">{hint}</p>}
      {foot && (
        <div className="mt-4 flex min-h-[39px] flex-wrap items-center justify-between gap-2 border-t border-dashed border-line-strong pt-3.5 text-[13px]">
          {foot}
        </div>
      )}
    </article>
  );
}

/** A grid of KPI tiles: four across on wide screens. */
export function StatGrid({ children, label }: { children: ReactNode; label?: string }) {
  return (
    <section
      aria-label={label}
      className="grid grid-cols-1 gap-5 sm:grid-cols-2 xl:grid-cols-4"
    >
      {children}
    </section>
  );
}

/** A stacked horizontal bar (prototype `.db-bar`). */
export function Meter({
  segments,
  label,
  className,
}: {
  segments: Array<{ value: number; color: string }>;
  label: string;
  className?: string;
}) {
  const total = segments.reduce((sum, s) => sum + s.value, 0);
  return (
    <div
      role="img"
      aria-label={label}
      className={cx("flex h-3 overflow-hidden rounded-full bg-[#EEF1F6]", className)}
    >
      {total > 0 &&
        segments
          .filter((s) => s.value > 0)
          .map((s, i) => (
            <span
              key={i}
              className="block h-full"
              style={{ width: `${(s.value / total) * 100}%`, background: s.color }}
            />
          ))}
    </div>
  );
}

/** A legend row list under a meter (prototype `.db-legend`). */
export function Legend({
  items,
}: {
  items: Array<{ label: ReactNode; value: ReactNode; color: string }>;
}) {
  return (
    <ul>
      {items.map((item, i) => (
        <li
          key={i}
          className="flex items-center gap-2.5 border-t border-line py-[11px] text-[14.5px] first:border-t-0"
        >
          <i aria-hidden="true" className="h-2.5 w-2.5 flex-none rounded-[3px]" style={{ background: item.color }} />
          {item.label}
          <b className="tabular ml-auto font-semibold">{item.value}</b>
        </li>
      ))}
    </ul>
  );
}

/** A navy calendar block: weekday over day number (prototype `.wl-date`). */
export function DateBlock({
  top,
  day,
  muted = false,
}: {
  top: ReactNode;
  day: ReactNode;
  muted?: boolean;
}) {
  return (
    <span
      className={cx(
        "w-[50px] flex-none rounded-[10px] py-[5px] text-center",
        muted ? "bg-[#EEF1F6] text-muted" : "bg-navy text-white",
      )}
    >
      <small
        className={cx(
          "block text-[10.5px] uppercase tracking-[0.08em]",
          muted ? "text-muted" : "text-navy-soft",
        )}
      >
        {top}
      </small>
      <b className="block text-lg font-semibold leading-tight">{day}</b>
    </span>
  );
}

// ── links ───────────────────────────────────────────────────────────────────

/** A bold inline link with a trailing arrow (prototype `.ax-link`). */
export function ArrowLink({
  href,
  children,
  className,
}: {
  href: string;
  children: ReactNode;
  className?: string;
}) {
  return (
    <Link
      href={href}
      className={cx(
        "inline-flex items-center gap-1.5 whitespace-nowrap font-semibold text-atlas-ink hover:underline",
        className,
      )}
    >
      {children}
      <Icon name="arrowRight" className="h-4 w-4" />
    </Link>
  );
}

// ── states ──────────────────────────────────────────────────────────────────

export function EmptyState({
  children,
  title,
  icon,
  iconTone = "success",
  action,
  className,
}: {
  children?: ReactNode;
  title?: ReactNode;
  icon?: IconName;
  iconTone?: Tone;
  action?: ReactNode;
  className?: string;
}) {
  return (
    <div
      className={cx(
        "flex flex-col items-center justify-center gap-1.5 rounded-[14px] border-[1.5px] border-dashed border-line-strong bg-[#FBFBFC] px-5 py-7 text-center text-sm text-muted",
        className,
      )}
    >
      {icon && <IconTile icon={icon} tone={iconTone} round className="mb-1" />}
      {title && <b className="text-[15px] font-semibold text-night">{title}</b>}
      {children && <span className="max-w-[46ch]">{children}</span>}
      {action && <div className="mt-1.5">{action}</div>}
    </div>
  );
}

/** The minimum a caller must supply to render a problem notice. `ApiError`
 *  satisfies it: its `message`, `code` and `requestId` come from the problem
 *  body Go returned. */
export interface ProblemLike {
  message: string;
  code?: string;
  requestId?: string;
}

/**
 * Renders an RFC 9457 problem returned by Go. Server pages catch `ApiError` and
 * pass it here; the code and `request_id` are shown so they can be quoted in a
 * report.
 */
export function ProblemNotice({
  error,
  title = "We could not load this",
}: {
  error: ProblemLike;
  title?: string;
}) {
  return (
    <div
      role="alert"
      className="flex gap-3 rounded-control border border-[#FFE0A8] bg-[#FFF8EB] px-4 py-3 text-sm text-[#7A4300]"
    >
      <Icon name="alert" className="mt-0.5 text-[#B45309]" />
      <div className="min-w-0">
        <p className="font-semibold">{title}</p>
        <p className="mt-0.5">{error.message}</p>
        {(error.code || error.requestId) && (
          <p className="mt-1.5 break-all font-mono text-xs text-[#8A5A14]">
            {error.code && <span>{error.code}</span>}
            {error.code && error.requestId && " · "}
            {error.requestId && <span>ref {error.requestId}</span>}
          </p>
        )}
      </div>
    </div>
  );
}

/** An inline red error line (prototype `.fm-err`), for a form refusal. */
export function InlineError({ children }: { children: ReactNode }) {
  return (
    <p
      role="alert"
      className="rounded-[10px] bg-[#FDECEC] px-3 py-2.5 text-[13.5px] font-medium text-[#B42318]"
    >
      {children}
    </p>
  );
}

// ── table ───────────────────────────────────────────────────────────────────

export function Table({ children }: { children: ReactNode }) {
  return (
    <div className="overflow-x-auto">
      <table className="w-full border-collapse text-left text-sm [&_tbody_tr:last-child_td]:border-b-0">
        {children}
      </table>
    </div>
  );
}

export function Th({
  children,
  className,
}: {
  children?: ReactNode;
  className?: string;
}) {
  return (
    <th
      scope="col"
      className={cx(
        "whitespace-nowrap border-y border-line bg-[#FBFBFC] px-[22px] py-2.5 text-xs font-semibold uppercase tracking-[0.08em] text-muted",
        className,
      )}
    >
      {children}
    </th>
  );
}

export function Td({
  children,
  className,
}: {
  children?: ReactNode;
  className?: string;
}) {
  return (
    <td className={cx("border-b border-line px-[22px] py-3.5 align-middle", className)}>
      {children}
    </td>
  );
}
