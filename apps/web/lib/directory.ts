/**
 * Atlas web - a small person-id -> name directory.
 *
 * Work records carry person ids, not names. The only roster the Phase 2 API
 * exposes is the availability roster (§8.5), so that is the directory source.
 * It needs `people.person.read`; when the viewer lacks it (engineers and staff
 * by default) the call fails closed and callers fall back to a short id, so a
 * missing permission degrades the display without breaking the page. The same
 * roster feeds the people pickers; with it empty they fall back to an id field.
 */

import { getAvailability } from "@/lib/api/atlas";

/** One pickable person. */
export interface PersonOption {
  id: string;
  name: string;
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

/** The directory as a name-sorted list, for people pickers. */
export function rosterOf(directory: Map<string, string>): PersonOption[] {
  return [...directory.entries()]
    .map(([id, name]) => ({ id, name }))
    .sort((a, b) => a.name.localeCompare(b.name));
}

/** Display a person id through a directory, falling back to a short id. */
export function personName(
  directory: Map<string, string>,
  personId?: string | null,
): string {
  if (!personId) return "Unassigned";
  return directory.get(personId) ?? `Person ${personId.slice(0, 8)}`;
}
