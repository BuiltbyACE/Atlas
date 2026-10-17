/**
 * Atlas web - shared presentational primitives.
 *
 * Deliberately dumb: no data fetching, no permissions, no business rules. Both
 * server and client components import these, so the module stays hook-free and
 * free of server-only APIs.
 */

import type { ReactNode } from "react";

/** Join class names, dropping falsey values. */
export function cx(...values: Array<string | false | null | undefined>): string {
  return values.filter(Boolean).join(" ");
}

// ── page chrome ─────────────────────────────────────────────────────────────

export function PageHeader({
  title,
  description,
  actions,
}: {
  title: string;
  description?: ReactNode;
  actions?: ReactNode;
}) {
  return (
    <header className="flex flex-wrap items-start justify-between gap-4">
      <div className="min-w-0">
        <h1 className="text-xl font-semibold tracking-tight text-slate-900">
          {title}
        </h1>
        {description && (
          <p className="mt-1 max-w-2xl text-sm text-slate-500">{description}</p>
        )}
      </div>
      {actions && <div className="flex shrink-0 items-center gap-2">{actions}</div>}
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
}: {
  title?: ReactNode;
  description?: ReactNode;
  actions?: ReactNode;
  children?: ReactNode;
  className?: string;
  padded?: boolean;
}) {
  return (
    <section
      className={cx(
        "rounded-xl border border-slate-200 bg-white shadow-sm",
        className,
      )}
    >
      {(title || actions) && (
        <div className="flex flex-wrap items-center justify-between gap-2 border-b border-slate-100 px-5 py-3">
          <div className="min-w-0">
            {title && (
              <h2 className="text-sm font-semibold text-slate-900">{title}</h2>
            )}
            {description && (
              <p className="mt-0.5 text-xs text-slate-500">{description}</p>
            )}
          </div>
          {actions && <div className="flex items-center gap-2">{actions}</div>}
        </div>
      )}
      <div className={cx(padded && "px-5 py-4")}>{children}</div>
    </section>
  );
}

// ── badges ──────────────────────────────────────────────────────────────────

export type Tone =
  | "neutral"
  | "info"
  | "success"
  | "warning"
  | "danger"
  | "accent";

const TONE_CLASSES: Record<Tone, string> = {
  neutral: "bg-slate-100 text-slate-700 ring-slate-200",
  info: "bg-sky-50 text-sky-700 ring-sky-200",
  success: "bg-emerald-50 text-emerald-700 ring-emerald-200",
  warning: "bg-amber-50 text-amber-800 ring-amber-200",
  danger: "bg-red-50 text-red-700 ring-red-200",
  accent: "bg-atlas/10 text-atlas ring-atlas/20",
};

export function Badge({
  children,
  tone = "neutral",
  className,
}: {
  children: ReactNode;
  tone?: Tone;
  className?: string;
}) {
  return (
    <span
      className={cx(
        "inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium ring-1 ring-inset",
        TONE_CLASSES[tone],
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
      return "success";
    case "in_progress":
    case "review":
    case "planning":
    case "requested":
    case "pending":
      return "info";
    case "blocked":
    case "paused":
    case "warning":
    case "late":
    case "overdue":
      return "warning";
    case "cancelled":
    case "declined":
    case "exited":
    case "missed":
      return "danger";
    case "on_leave":
    case "not_started":
    case "todo":
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
      return "warning";
    case "medium":
      return "info";
    default:
      return "neutral";
  }
}

// ── states ──────────────────────────────────────────────────────────────────

export function EmptyState({
  children,
  className,
}: {
  children: ReactNode;
  className?: string;
}) {
  return (
    <div
      className={cx(
        "rounded-lg border border-dashed border-slate-300 bg-slate-50 px-4 py-8 text-center text-sm text-slate-500",
        className,
      )}
    >
      {children}
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
      className="rounded-lg border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-900"
    >
      <p className="font-medium">{title}</p>
      <p className="mt-1 text-amber-800">{error.message}</p>
      {(error.code || error.requestId) && (
        <p className="mt-2 font-mono text-xs text-amber-700">
          {error.code && <span>{error.code}</span>}
          {error.code && error.requestId && " · "}
          {error.requestId && <span>ref {error.requestId}</span>}
        </p>
      )}
    </div>
  );
}

export function Stat({
  label,
  value,
  hint,
  tone = "neutral",
}: {
  label: string;
  value: ReactNode;
  hint?: ReactNode;
  tone?: Tone;
}) {
  return (
    <div className="rounded-xl border border-slate-200 bg-white p-4 shadow-sm">
      <p className="text-xs font-medium uppercase tracking-wide text-slate-500">
        {label}
      </p>
      <p
        className={cx(
          "mt-1 text-2xl font-semibold tabular-nums",
          tone === "danger" && "text-red-600",
          tone === "warning" && "text-amber-600",
          tone === "success" && "text-emerald-600",
          tone === "neutral" && "text-slate-900",
          tone === "info" && "text-slate-900",
          tone === "accent" && "text-atlas",
        )}
      >
        {value}
      </p>
      {hint && <p className="mt-1 text-xs text-slate-500">{hint}</p>}
    </div>
  );
}

// ── table ───────────────────────────────────────────────────────────────────

export function Table({ children }: { children: ReactNode }) {
  return (
    <div className="overflow-x-auto">
      <table className="w-full border-collapse text-left text-sm">
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
        "border-b border-slate-200 px-3 py-2 text-xs font-medium uppercase tracking-wide text-slate-500",
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
    <td className={cx("border-b border-slate-100 px-3 py-2 align-top", className)}>
      {children}
    </td>
  );
}
