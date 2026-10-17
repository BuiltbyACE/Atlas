/**
 * Atlas web - a small person-id -> name directory.
 *
 * Work records carry person ids, not names. The only roster the Phase 2 API
 * exposes is the availability roster (§8.5), so that is the directory source.
 * It needs `people.person.read`; when the viewer lacks it the call fails closed
 * and callers fall back to a short id, so a missing permission degrades the
 * display without breaking the page.
 */

import { getAvailability } from "@/lib/api/atlas";

export type PersonDirectory = Map<string, string>;

/** A read-only view of the directory with a name-or-fallback helper. */
export interface Directory {
  name(id?: string | null): string;
  size: number;
}

export function directoryOf(names: Map<string, string>): {
  name: (id?: string | null) => string;
  size: number;
} {
  return {
    has: () => undefined,
    size: names.size,
    name: (id?: string | null) => {
      if (!id) return "Unassigned";
      return names.get(id) ?? `${id.slice(0, 8)}…`;
    },
  } as never;
}

/**
 * Load the roster and return a lookup. Failures are swallowed: an empty map
 * simply means every person renders as a short id.
 */
export async function personDirectory(): Promise<Map<string, string>> {
  try {
    const availability = await getAvailability();
    return new Map(
      availability.items.map((item) => [item.person_id, item.full_name]),
    );
  } catch {
    return new Map();
  }
}

/** Display a person id through a directory, falling back to a short id. */
export function personName(
  directory: Map<string, string>,
  personId?: string | null,
): string {
  if (!personId) return "Unassigned";
  return directory.get(personId) ?? `${personId.slice(0, 8)}…`;
}
