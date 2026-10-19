import { DayPicker } from "@/components/day-picker";
import { Icon } from "@/components/icons";
import {
  Card,
  EmptyState,
  PageHeader,
  ProblemNotice,
  Stat,
  StatGrid,
} from "@/components/ui";
import {
  getPresence,
  listSessions,
  type Presence,
  type SignInSession,
} from "@/lib/api/atlas";
import { ApiError } from "@/lib/api/client";
import { requireUser } from "@/lib/auth/session";
import { formatDay, formatDuration, formatLongDay, today } from "@/lib/format";

import { ActivityTable, DayTimeline } from "./activity-views";
import { PresenceRoster } from "./presence-roster";

/**
 * People activity (blueprint §8.5; prototype 07): who is signed in now, when
 * people were signed in on a day, and how long each was active.
 *
 * Read-only and derived entirely from the auth session rows Go already keeps;
 * nothing here writes. Two Go endpoints back it, both gated by
 * `people.session.read`:
 *
 *   - `GET /api/v1/people/presence` - live sessions seen within the presence
 *     window (a heartbeat keeps them fresh), rendered by `PresenceRoster`;
 *   - `GET /api/v1/people/sessions?from_date=&to_date=` - the day's sign-ins,
 *     drawn as a timeline and rolled up per person.
 */

export const dynamic = "force-dynamic";

export default async function ActivityPage({
  searchParams,
}: {
  searchParams: Promise<{ date?: string }>;
}) {
  const { date } = await searchParams;
  const now = today();
  const day = date && /^\d{4}-\d{2}-\d{2}$/.test(date) ? date : now;
  const user = await requireUser();

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

  const forbidden = presenceError?.status === 403 || sessionsError?.status === 403;
  const peopleActive = new Set(sessions.map((s) => s.person_id)).size;
  const totalSeconds = sessions.reduce((sum, s) => sum + s.active_seconds, 0);
  const onlineNames = presence
    ? [...new Set(presence.online.map((s) => s.full_name))]
    : [];

  if (forbidden) {
    return (
      <div className="space-y-6">
        <PageHeader title="Activity" description="Who is signed in now, and how long each person was active." />
        <ProblemNotice
          error={(presenceError ?? sessionsError)!}
          title="Sign-in activity isn’t visible to you"
        />
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <PageHeader
        title="Activity"
        description="Who is signed in now, and how long each person was active."
      />

      <DayPicker day={day} today={now} />

      <StatGrid label={`Activity totals for ${formatDay(day)}`}>
        <Stat
          label="Online now"
          value={presence ? onlineNames.length : "—"}
          tone={onlineNames.length > 0 ? "success" : "neutral"}
          hint={
            onlineNames.length === 0
              ? "Nobody active right now"
              : onlineNames.length <= 2
                ? onlineNames.join(" and ")
                : `${onlineNames.slice(0, 2).join(", ")} and ${onlineNames.length - 2} more`
          }
          icon="signal"
          iconTone="success"
        />
        <Stat label="People active" value={sessionsError ? "—" : peopleActive} hint={`on ${formatDay(day)}`} icon="people" iconTone="info" />
        <Stat label="Sessions" value={sessionsError ? "—" : sessions.length} hint={`on ${formatDay(day)}`} icon="monitor" iconTone="violet" />
        <Stat
          label="Total active"
          value={sessionsError ? "—" : formatDuration(totalSeconds)}
          hint={`across everyone on ${formatDay(day)}`}
          icon="clock"
          iconTone="warning"
        />
      </StatGrid>

      <div className="grid grid-cols-1 items-stretch gap-5 xl:grid-cols-[minmax(0,1fr)_minmax(0,1.9fr)]">
        <Card title="Online now" description="Sessions that sent a heartbeat recently." padded={false}>
          {presenceError ? (
            <div className="px-[22px] pb-[22px]">
              <ProblemNotice error={presenceError} title="Presence could not be loaded" />
            </div>
          ) : presence ? (
            <PresenceRoster initial={presence} me={user.person_id} />
          ) : null}
        </Card>

        <Card title="Day timeline" description={`When people were signed in on ${formatLongDay(day)}.`}>
          {sessionsError ? (
            <ProblemNotice error={sessionsError} title="Activity could not be loaded" />
          ) : (
            <DayTimeline sessions={sessions} day={day} me={user.person_id} />
          )}
        </Card>
      </div>

      <Card
        title={`Activity on ${formatDay(day)}`}
        description={
          truncated
            ? "Showing the first 200 sessions of this day."
            : "Sign-ins that overlapped this day, rolled up per person."
        }
        padded={false}
      >
        {sessionsError ? (
          <div className="px-[22px] pb-[22px]">
            <ProblemNotice error={sessionsError} title="Activity could not be loaded" />
          </div>
        ) : sessions.length === 0 ? (
          <div className="px-[22px] pb-[22px]">
            <EmptyState icon="clock" iconTone="info" title="No sign-ins recorded" className="min-h-[180px]">
              Nobody was signed in on {formatDay(day)}.
            </EmptyState>
          </div>
        ) : (
          <ActivityTable sessions={sessions} me={user.person_id} />
        )}
      </Card>

      <p className="flex items-center gap-2.5 text-[13.5px] text-muted">
        <Icon name="shield" className="h-4 w-4" />
        Atlas records sign-in sessions only — never keystrokes, screens or messages.
      </p>
    </div>
  );
}
