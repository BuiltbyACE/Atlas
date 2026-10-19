/**
 * Atlas web - small presentation formatters.
 *
 * Pure, dependency-free and safe on the server and the client. Nothing here is
 * business logic; Go decides values, these only make them readable.
 */

/** `2026-09-14T08:31:00Z` -> `14 Sep 2026`. */
export function formatDate(value?: string | null): string {
  if (!value) return "—";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "—";
  return date.toLocaleDateString("en-GB", {
    day: "2-digit",
    month: "short",
    year: "numeric",
  });
}

/** `2026-09-14T08:31:00Z` -> `14 Sep 2026, 11:31`. */
export function formatDateTime(value?: string | null): string {
  if (!value) return "—";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "—";
  return date.toLocaleString("en-GB", {
    day: "2-digit",
    month: "short",
    year: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  });
}

/** `2026-09-14T08:31:00Z` -> `11:31` (local time), for same-day spans. */
export function formatTime(value?: string | null): string {
  if (!value) return "—";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "—";
  return date.toLocaleTimeString("en-GB", {
    hour: "2-digit",
    minute: "2-digit",
  });
}

/**
 * A span in seconds as `3h 42m` / `45m` / `30s`. Presentation only: Go computes
 * `active_seconds`, this only makes it readable.
 */
export function formatDuration(seconds?: number | null): string {
  if (seconds === undefined || seconds === null || seconds < 0) return "—";
  const total = Math.round(seconds);
  const hours = Math.floor(total / 3600);
  const minutes = Math.floor((total % 3600) / 60);
  if (hours > 0) return `${hours}h ${minutes}m`;
  if (minutes > 0) return `${minutes}m`;
  return `${total}s`;
}

/**
 * Money as Go stores it: integer minor units plus a currency (ADR-010, §6.6).
 * `formatMinor(150000)` -> `KES 1,500.00`. Zero-decimal currencies (JPY, …) are
 * rendered without a fractional part. Presentation only; Go owns the amounts.
 */
export function formatMinor(minor?: number | null, currency = "KES"): string {
  if (minor === undefined || minor === null) return "—";
  const code = (currency || "KES").toUpperCase();
  const fraction = ZERO_DECIMAL_CURRENCIES.has(code) ? 0 : 2;
  try {
    return new Intl.NumberFormat("en-GB", {
      style: "currency",
      currency: code,
      minimumFractionDigits: fraction,
      maximumFractionDigits: fraction,
    }).format(minor / 10 ** fraction);
  } catch {
    // An unrecognised currency code must not break a page; show the number.
    return `${code} ${(minor / 10 ** fraction).toLocaleString("en-GB", {
      minimumFractionDigits: fraction,
      maximumFractionDigits: fraction,
    })}`;
  }
}

const ZERO_DECIMAL_CURRENCIES = new Set(["JPY", "KRW", "VND", "CLP", "XAF", "XOF"]);

/**
 * Money in minor units, compacted for a KPI tile: `formatMinorCompact(12_400_000)`
 * -> `KES 124K`, `1_240_000_000` -> `KES 12.4M`. Presentation only.
 */
export function formatMinorCompact(
  minor?: number | null,
  currency = "KES",
): string {
  if (minor === undefined || minor === null) return "—";
  const code = (currency || "KES").toUpperCase();
  const major = minor / 10 ** (ZERO_DECIMAL_CURRENCIES.has(code) ? 0 : 2);
  const abs = Math.abs(major);
  for (const [size, suffix] of [
    [1_000_000_000, "B"],
    [1_000_000, "M"],
    [1_000, "K"],
  ] as const) {
    if (abs >= size) {
      return `${code} ${(major / size).toFixed(1).replace(/\.0$/, "")}${suffix}`;
    }
  }
  return `${code} ${major.toLocaleString("en-GB", { maximumFractionDigits: 0 })}`;
}

/** A plain calendar date `2026-09-14` kept as-is (no timezone shift). */
export function formatDay(value?: string | null): string {
  if (!value) return "—";
  if (/^\d{4}-\d{2}-\d{2}$/.test(value)) return formatDate(`${value}T00:00:00Z`);
  return formatDate(value);
}

/** Today as `YYYY-MM-DD` in UTC - matches the API's calendar-date handling. */
export function today(): string {
  return new Date().toISOString().slice(0, 10);
}

/** Whole days from today (negative = in the past). */
export function daysFromToday(value?: string | null): number | null {
  if (!value) return null;
  const date = new Date(value.length === 10 ? `${value}T00:00:00Z` : value);
  if (Number.isNaN(date.getTime())) return null;
  const ms = date.getTime() - Date.now();
  return Math.round(ms / 86_400_000);
}

/** "in 3 days" / "2 days ago" / "today", for deadlines. */
export function dueLabel(value?: string | null): string | null {
  const days = daysFromToday(value);
  if (days === null) return null;
  if (days === 0) return "due today";
  if (days === 1) return "due tomorrow";
  if (days === -1) return "1 day overdue";
  if (days < 0) return `${Math.abs(days)} days overdue`;
  return `due in ${days} days`;
}

/** `snake_case` / `kebab-case` -> `Title Case`. */
export function humanise(value: string): string {
  return value
    .replace(/[_-]+/g, " ")
    .replace(/\s+/g, " ")
    .trim()
    .replace(/\b\w/g, (c) => c.toUpperCase());
}

/** Initials for an avatar bubble. */
export function initials(name: string): string {
  const parts = name.trim().split(/\s+/).filter(Boolean);
  if (parts.length === 0) return "?";
  if (parts.length === 1) return parts[0].slice(0, 2).toUpperCase();
  return (parts[0][0] + parts[parts.length - 1][0]).toUpperCase();
}

/** Shorten a UUID for display when a name is not available. */
export function shortId(id?: string | null): string {
  if (!id) return "—";
  return id.length > 8 ? id.slice(0, 8) : id;
}

/**
 * Convert a date-input value (`YYYY-MM-DD`) to the RFC 3339 timestamp Go's work
 * API requires (`optTime` parses `time.RFC3339`). An empty value is `null`
 * (clear the field); an already-timestamped value passes through.
 */
export function toApiTime(value: string): string | null {
  const trimmed = value.trim();
  if (!trimmed) return null;
  if (/^\d{4}-\d{2}-\d{2}$/.test(trimmed)) return `${trimmed}T00:00:00Z`;
  return trimmed;
}

/** An RFC 3339 timestamp -> the `YYYY-MM-DD` a `<input type="date">` wants. */
export function dateInput(value?: string | null): string {
  if (!value) return "";
  return value.slice(0, 10);
}

/** Display labels for workflow states, where the enum reads awkwardly. */
const STATUS_LABELS: Record<string, string> = {
  todo: "To do",
  in_progress: "In progress",
  review: "In review",
  requested: "Pending",
  not_started: "Not started",
  on_leave: "On leave",
};

/** `in_progress` -> `In progress`; anything unmapped is humanised. */
export function statusLabel(status: string): string {
  return STATUS_LABELS[status] ?? humanise(status);
}

/** `annual` -> `Annual leave`. */
export function leaveTypeLabel(type: string): string {
  return type === "other" ? "Other leave" : `${humanise(type)} leave`;
}

/** The first word of a display name, for greetings. */
export function firstName(name: string): string {
  return name.trim().split(/\s+/)[0] ?? name;
}

/** Shift a `YYYY-MM-DD` calendar day by whole days (no timezone drift). */
export function addDays(day: string, days: number): string {
  const date = new Date(`${day}T00:00:00Z`);
  date.setUTCDate(date.getUTCDate() + days);
  return date.toISOString().slice(0, 10);
}

/** Inclusive length of a calendar-day span: 1 Oct -> 3 Oct is 3 days. */
export function daySpan(from: string, to: string): number {
  const ms =
    new Date(`${to.slice(0, 10)}T00:00:00Z`).getTime() -
    new Date(`${from.slice(0, 10)}T00:00:00Z`).getTime();
  return Math.round(ms / 86_400_000) + 1;
}

/** A calendar day in words: `Sunday, 18 October 2026`. */
export function formatLongDay(day: string): string {
  return new Date(`${day.slice(0, 10)}T00:00:00Z`).toLocaleDateString("en-GB", {
    weekday: "long",
    day: "numeric",
    month: "long",
    year: "numeric",
    timeZone: "UTC",
  });
}

/** `18 Oct` - a short day for cards and agendas. */
export function formatShortDay(value?: string | null): string {
  if (!value) return "—";
  return new Date(`${value.slice(0, 10)}T00:00:00Z`).toLocaleDateString("en-GB", {
    day: "numeric",
    month: "short",
    timeZone: "UTC",
  });
}

/** `Sun` / `18` parts for a calendar block. */
export function dayParts(value: string): { weekday: string; day: string } {
  const date = new Date(`${value.slice(0, 10)}T00:00:00Z`);
  return {
    weekday: date.toLocaleDateString("en-GB", { weekday: "short", timeZone: "UTC" }),
    day: String(date.getUTCDate()),
  };
}

/** A relative label for a past moment: `just now`, `5m ago`, `yesterday`. */
export function timeAgo(value?: string | null): string {
  if (!value) return "—";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "—";
  const seconds = Math.round((Date.now() - date.getTime()) / 1000);
  if (seconds < 60) return "just now";
  if (seconds < 3600) return `${Math.floor(seconds / 60)}m ago`;
  if (seconds < 86_400) return `${Math.floor(seconds / 3600)}h ago`;
  const days = Math.floor(seconds / 86_400);
  if (days === 1) return "yesterday";
  if (days < 7) return `${days} days ago`;
  return formatDate(value);
}

/** `due_at` has passed on an item that is still open. */
export function isOverdue(due?: string | null, status?: string): boolean {
  if (!due) return false;
  if (status === "done" || status === "cancelled" || status === "completed") return false;
  return new Date(due).getTime() < Date.now();
}
