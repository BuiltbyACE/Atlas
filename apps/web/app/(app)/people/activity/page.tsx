import {
  Badge,
  Card,
  EmptyState,
  PageHeader,
  ProblemNotice,
  Stat,
  Table,
  Td,
  Th,
} from "@/components/ui";
import {
  getPresence,
  listSessions,
  type Presence,
  type SignInSession,
} from "@/lib/api/atlas";
import { ApiError } from "@/lib/api/client";
import { formatDay, formatDuration, formatTime, today } from "@/lib/format";

import { PresenceRoster } from "./presence-roster";

/**
 * People activity (blueprint §8.5): who is signed in now, and how long each
 * person was active on a chosen day.
 *
 * Read-only and derived entirely from the auth session rows Go already keeps;
 * nothing here writes. Two Go endpoints back it, both gated by
 * `people.session.read`:
 *
 *   - `GET /api/v1/people/presence` - live sessions seen within the presence
 *     window (a heartbeat keeps them fresh), rendered by `PresenceRoster`;
 *   - `GET /api/v1/people/sessions?from_date=&to_date=` - the day's sign-ins,
 *     rolled up per person here for display ("active from 09:12 to 17:40").
 */

export const dynamic = "force-dynamic";

/** One row of the per-person roll-up: a person's day in sign-ins. */
interface PersonActivity {
  person_id: string;
  full_name: string;
  sessions: number;
  total_seconds: number;
  first_at: string;
  last_at: string;
  online: boolean;
}

export default async function ActivityPage({
  searchParams,
}: {
  searchParams: Promise<{ date?: string }>;
}) {
  const { date } = await searchParams;
  const day = date && /^\d{4}-\d{2}-\d{2}$/.test(date) ? date : today();

  let presence: Presence | null = null;
  let presenceError: ApiError | null = null;
  try {
    presence = await getPresence();
  } catch (error) {
    if (!(error instanceof ApiError)) throw error;
    presenceError = error;
  }

  let sessions: SignInSession[] = [];
  let truncated = false;
  let sessionsError: ApiError | null = null;
  try {
    const page = await listSessions({ from_date: day, to_date: day, limit: 200 });
    sessions = page.items;
    truncated = page.has_more;
  } catch (error) {
    if (!(error instanceof ApiError)) throw error;
    sessionsError = error;
  }

  const rollup = summarise(sessions);
  const totalSeconds = rollup.reduce((sum, row) => sum + row.total_seconds, 0);
  const forbidden =
    presenceError?.status === 403 || sessionsError?.status === 403;

  return (
    <div className="space-y-6">
      <PageHeader
        title="Activity"
        description="Who is signed in now, and how long each person was active (blueprint §8.5)."
      />

      <div className="grid grid-cols-2 gap-4 sm:grid-cols-4">
        <Stat
          label="Online now"
          value={presence ? presence.online.length : "—"}
          tone={presence && presence.online.length > 0 ? "success" : "neutral"}
        />
        <Stat
          label="People active"
          value={sessionsError ? "—" : rollup.length}
          hint={formatDay(day)}
        />
        <Stat
          label="Sessions"
          value={sessionsError ? "—" : sessions.length}
          hint={formatDay(day)}
        />
        <Stat
          label="Total active"
          value={sessionsError ? "—" : formatDuration(totalSeconds)}
          hint={formatDay(day)}
        />
      </div>

      <Card
        title="Online now"
        description="Live sessions seen within the presence window; a heartbeat keeps them fresh."
      >
        {presenceError ? (
          <ProblemNotice
            error={presenceError}
            title={
              forbidden
                ? "You cannot see sign-in activity"
                : "Presence could not be loaded"
            }
          />
        ) : presence ? (
          <PresenceRoster initial={presence} />
        ) : null}
      </Card>

      <Card
        title={`Activity on ${formatDay(day)}`}
        description={
          truncated
            ? "Showing the first 200 sessions; narrow the day to see them all."
            : "Sign-ins that overlapped this day, rolled up per person."
        }
        actions={
          <form action="/people/activity" className="flex items-center gap-2">
            <input
              type="date"
              name="date"
              defaultValue={day}
              className="rounded-md border border-slate-300 bg-white px-2 py-1.5 text-sm shadow-sm outline-none focus:border-atlas focus:ring-2 focus:ring-atlas/20"
            />
            <button
              type="submit"
              className="rounded-md bg-atlas px-3 py-1.5 text-sm font-medium text-white transition hover:bg-atlas/90"
            >
              Show
            </button>
          </form>
        }
      >
        {sessionsError ? (
          <ProblemNotice
            error={sessionsError}
            title={
              forbidden
                ? "You cannot see sign-in activity"
                : "Activity could not be loaded"
            }
          />
        ) : sessions.length === 0 ? (
          <EmptyState>No sign-ins recorded on {formatDay(day)}.</EmptyState>
        ) : (
          <div className="space-y-6">
            <Table>
              <thead>
                <tr>
                  <Th>Person</Th>
                  <Th>Sessions</Th>
                  <Th>First in</Th>
                  <Th>Last active</Th>
                  <Th>Total active</Th>
                  <Th />
                </tr>
              </thead>
              <tbody>
                {rollup.map((row) => (
                  <tr key={row.person_id}>
                    <Td className="font-medium text-slate-900">
                      {row.full_name}
                    </Td>
                    <Td className="tabular-nums">{row.sessions}</Td>
                    <Td className="tabular-nums">{formatTime(row.first_at)}</Td>
                    <Td className="tabular-nums">{formatTime(row.last_at)}</Td>
                    <Td className="tabular-nums">
                      {formatDuration(row.total_seconds)}
                    </Td>
                    <Td>{row.online && <Badge tone="success">online</Badge>}</Td>
                  </tr>
                ))}
              </tbody>
            </Table>

            <div>
              <h3 className="mb-2 text-xs font-medium uppercase tracking-wide text-slate-500">
                Individual sessions
              </h3>
              <Table>
                <thead>
                  <tr>
                    <Th>Person</Th>
                    <Th>Started</Th>
                    <Th>Last active</Th>
                    <Th>Ended</Th>
                    <Th>Duration</Th>
                    <Th>IP</Th>
                    <Th />
                  </tr>
                </thead>
                <tbody>
                  {sessions.map((session) => (
                    <tr key={session.id}>
                      <Td>{session.full_name}</Td>
                      <Td className="tabular-nums">
                        {formatTime(session.started_at)}
                      </Td>
                      <Td className="tabular-nums">
                        {formatTime(session.last_active_at)}
                      </Td>
                      <Td className="tabular-nums">
                        {session.ended_at ? formatTime(session.ended_at) : "—"}
                      </Td>
                      <Td className="tabular-nums">
                        {formatDuration(session.active_seconds)}
                      </Td>
                      <Td className="font-mono text-xs text-slate-500">
                        {session.ip || "—"}
                      </Td>
                      <Td>
                        {session.online ? (
                          <Badge tone="success">online</Badge>
                        ) : (
                          <Badge tone="neutral">offline</Badge>
                        )}
                      </Td>
                    </tr>
                  ))}
                </tbody>
              </Table>
            </div>
          </div>
        )}
      </Card>
    </div>
  );
}

/**
 * Fold a day's sessions into one row per person. This is presentation maths over
 * what Go already decided (each session's `active_seconds`); the ordering keys
 * are ISO-8601 UTC strings, which compare lexicographically.
 */
function summarise(sessions: SignInSession[]): PersonActivity[] {
  const byPerson = new Map<string, PersonActivity>();
  for (const session of sessions) {
    const end = session.ended_at ?? session.last_active_at;
    const existing = byPerson.get(session.person_id);
    if (!existing) {
      byPerson.set(session.person_id, {
        person_id: session.person_id,
        full_name: session.full_name,
        sessions: 1,
        total_seconds: session.active_seconds,
        first_at: session.started_at,
        last_at: end,
        online: session.online,
      });
      continue;
    }
    existing.sessions += 1;
    existing.total_seconds += session.active_seconds;
    if (session.started_at < existing.first_at) existing.first_at = session.started_at;
    if (end > existing.last_at) existing.last_at = end;
    existing.online = existing.online || session.online;
  }
  return [...byPerson.values()].sort((a, b) => (a.last_at < b.last_at ? 1 : -1));
}
