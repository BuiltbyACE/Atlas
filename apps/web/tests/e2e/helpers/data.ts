/**
 * Fixture helpers. Every run creates its own uniquely suffixed records so the
 * suite can be re-run against the same disposable database without colliding
 * with what an earlier attempt left behind.
 */

let counter = 0;

/** A short, collision-resistant suffix for names created by a test. */
export function unique(prefix: string): string {
  counter += 1;
  return `${prefix} ${Date.now().toString(36)}-${counter}`;
}

/** Today as the `YYYY-MM-DD` a date input expects. */
export function todayIso(): string {
  const now = new Date();
  const local = new Date(now.getTime() - now.getTimezoneOffset() * 60_000);
  return local.toISOString().slice(0, 10);
}
