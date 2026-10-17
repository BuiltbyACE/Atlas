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
export function shortId(id: string): string {
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
