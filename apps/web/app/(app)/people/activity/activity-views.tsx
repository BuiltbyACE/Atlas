"use client";

import { Fragment, useEffect, useMemo, useState } from "react";

import { Icon } from "@/components/icons";
import { Avatar, Badge, EmptyState, Td, Th, Table, cx } from "@/components/ui";
import type { SignInSession } from "@/lib/api/atlas";
import { formatDuration, formatTime, initials } from "@/lib/format";

/** One person's day in sign-ins, folded from their sessions. */
interface PersonDay {
  personId: string;
  name: string;
  sessions: SignInSession[];
  totalSeconds: number;
  firstAt: string;
  lastAt: string;
  online: boolean;
}

/**
 * Fold a day's sessions into one row per person. Presentation maths over what
 * Go already decided (each session's `active_seconds`); ISO-8601 UTC strings
 * compare lexicographically.
 */
function byPerson(sessions: SignInSession[]): PersonDay[] {
  const map = new Map<string, PersonDay>();
  for (const session of sessions) {
    const end = session.ended_at ?? session.last_active_at;
    const row = map.get(session.person_id);
    if (!row) {
      map.set(session.person_id, {
        personId: session.person_id,
        name: session.full_name,
        sessions: [session],
        totalSeconds: session.active_seconds,
        firstAt: session.started_at,
        lastAt: end,
        online: session.online,
      });
      continue;
    }
    row.sessions.push(session);
    row.totalSeconds += session.active_seconds;
    if (session.started_at < row.firstAt) row.firstAt = session.started_at;
    if (end > row.lastAt) row.lastAt = end;
    row.online = row.online || session.online;
  }
  for (const row of map.values()) {
    row.sessions.sort((a, b) => (a.started_at < b.started_at ? -1 : 1));
  }
  return [...map.values()].sort((a, b) => (a.lastAt < b.lastAt ? 1 : -1));
}

/** "now", re-read every minute so the timeline's marker moves. */
function useNow(): number {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    const timer = window.setInterval(() => setNow(Date.now()), 60_000);
    return () => window.clearInterval(timer);
  }, []);
  return now;
}

const MAX_LANES = 12;

/**
 * When people were signed in across the day (prototype 07 "Day timeline").
 * Rendered in the browser so the hours are the viewer's own. A session's span
 * is sign-in to sign-out, or to its last heartbeat (now, while it is live).
 */
export function DayTimeline({
  sessions,
  day,
  me,
}: {
  sessions: SignInSession[];
  day: string;
  me?: string;
}) {
  const now = useNow();
  const people = useMemo(() => byPerson(sessions), [sessions]);

  const dayStart = new Date(`${day}T00:00:00`).getTime();
  const dayEnd = dayStart + 24 * 3_600_000;
  const spans = people.slice(0, MAX_LANES).map((person) => ({
    person,
    segments: person.sessions
      .map((session) => {
        const start = new Date(session.started_at).getTime();
        const end = session.ended_at
          ? new Date(session.ended_at).getTime()
          : session.online
            ? now
            : new Date(session.last_active_at).getTime();
        return { id: session.id, start: Math.max(start, dayStart), end: Math.min(end, dayEnd), online: session.online };
      })
      .filter((s) => s.end > s.start),
  }));

  const earliest = Math.min(
    ...spans.flatMap((s) => s.segments.map((seg) => seg.start)),
    dayStart + 6 * 3_600_000,
  );
  const startHour = Math.floor((earliest - dayStart) / 3_600_000 / 3) * 3;
  const windowStart = dayStart + startHour * 3_600_000;
  const windowMs = dayEnd - windowStart;
  const pct = (t: number) => ((t - windowStart) / windowMs) * 100;
  const ticks = Array.from({ length: (24 - startHour) / 3 + 1 }, (_, i) => startHour + i * 3);
  const showNow = now >= windowStart && now <= dayEnd;
  const nowLabel = new Date(now).toLocaleTimeString("en-GB", { hour: "2-digit", minute: "2-digit" });
  const lanePct = 100 / (ticks.length - 1);

  if (people.length === 0) {
    return (
      <EmptyState icon="clock" iconTone="info" title="No sign-ins on this day" className="min-h-[180px]">
        Lanes appear here as people sign in.
      </EmptyState>
    );
  }

  return (
    <div>
      <div className="relative ml-0 h-[22px] text-xs text-muted sm:ml-[150px]" aria-hidden="true">
        {ticks.map((hour, index) => (
          <span
            key={hour}
            className={cx(
              "absolute top-0",
              index === 0 ? "" : index === ticks.length - 1 ? "-translate-x-full" : "-translate-x-1/2",
            )}
            style={{ left: `${(index / (ticks.length - 1)) * 100}%` }}
          >
            {String(hour).padStart(2, "0")}:00
          </span>
        ))}
      </div>

      {spans.map(({ person, segments }) => {
        const label = `${person.name} signed in ${segments
          .map((s) => `from ${formatTime(new Date(s.start).toISOString())} to ${s.online ? "now" : formatTime(new Date(s.end).toISOString())}`)
          .join(", ")}`;
        return (
          <div
            key={person.personId}
            className="grid min-h-[52px] grid-cols-1 items-center gap-2 border-t border-line py-2.5 sm:grid-cols-[150px_minmax(0,1fr)] sm:gap-0 sm:py-0"
          >
            <span className="flex min-w-0 items-center gap-2.5 text-[14.5px] font-medium">
              <Avatar initials={initials(person.name)} size="sm" variant={person.personId === me ? "me" : "default"} />
              <span className="truncate">{person.name}</span>
            </span>
            <div
              role="img"
              aria-label={label}
              className="relative h-[30px] rounded-[10px]"
              style={{
                background: `repeating-linear-gradient(90deg,#F2F4F8 0,#F2F4F8 calc(${lanePct}% - 1px),#E3E7EE calc(${lanePct}% - 1px),#E3E7EE ${lanePct}%)`,
              }}
            >
              {segments.map((segment) => (
                <span
                  key={segment.id}
                  className={cx(
                    "absolute bottom-1 top-1 min-w-2 rounded-[7px]",
                    segment.online ? "bg-atlas shadow-[0_4px_10px_rgba(26,108,245,.3)]" : "bg-[#7FA7F0]",
                  )}
                  style={{ left: `${pct(segment.start)}%`, width: `${Math.max(pct(segment.end) - pct(segment.start), 0.6)}%` }}
                />
              ))}
              {showNow && (
                <span className="absolute -bottom-1.5 -top-1.5 w-0.5 rounded-sm bg-[#22A55A]" style={{ left: `${pct(now)}%` }} />
              )}
            </div>
          </div>
        );
      })}

      <div className="flex flex-wrap gap-[18px] pt-3.5 text-[13px] text-muted sm:pl-[150px]">
        <span className="inline-flex items-center gap-1.5">
          <i aria-hidden="true" className="h-2 w-3 rounded-[3px] bg-atlas" />
          Signed in now
        </span>
        <span className="inline-flex items-center gap-1.5">
          <i aria-hidden="true" className="h-2 w-3 rounded-[3px] bg-[#7FA7F0]" />
          Earlier session
        </span>
        {showNow && (
          <span className="inline-flex items-center gap-1.5">
            <i aria-hidden="true" className="h-3 w-[3px] rounded-sm bg-[#22A55A]" />
            Now ({nowLabel})
          </span>
        )}
        {people.length > MAX_LANES && <span>and {people.length - MAX_LANES} more people in the table below</span>}
      </div>
    </div>
  );
}

/** Per-person roll-up with each person's sessions one click away. */
export function ActivityTable({ sessions, me }: { sessions: SignInSession[]; me?: string }) {
  const people = useMemo(() => byPerson(sessions), [sessions]);
  const [open, setOpen] = useState<Set<string>>(() => new Set(people.length === 1 ? [people[0].personId] : []));

  function toggle(id: string) {
    setOpen((set) => {
      const next = new Set(set);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }

  return (
    <Table>
      <thead>
        <tr>
          <Th>Person</Th>
          <Th>Sessions</Th>
          <Th>First in</Th>
          <Th>Last active</Th>
          <Th>Total active</Th>
          <Th>Status</Th>
          <Th>
            <span className="sr-only">Details</span>
          </Th>
        </tr>
      </thead>
      <tbody>
        {people.map((person) => {
          const expanded = open.has(person.personId);
          return (
            <Fragment key={person.personId}>
              <tr>
                <Td>
                  <span className="flex items-center gap-2.5 font-semibold">
                    <Avatar initials={initials(person.name)} variant={person.personId === me ? "me" : "default"} />
                    {person.name}
                  </span>
                </Td>
                <Td className="tabular">{person.sessions.length}</Td>
                <Td className="tabular">{formatTime(person.firstAt)}</Td>
                <Td className="tabular">{formatTime(person.lastAt)}</Td>
                <Td className="tabular font-semibold">{formatDuration(person.totalSeconds)}</Td>
                <Td>
                  {person.online ? (
                    <Badge tone="success" dot>Online</Badge>
                  ) : (
                    <Badge tone="neutral" dot>Offline</Badge>
                  )}
                </Td>
                <Td className="text-right">
                  <button
                    type="button"
                    aria-expanded={expanded}
                    onClick={() => toggle(person.personId)}
                    className="inline-flex h-9 items-center gap-1.5 whitespace-nowrap rounded-[10px] px-2.5 text-[13.5px] font-semibold text-atlas-ink hover:bg-[#EEF3FF]"
                  >
                    {expanded ? "Hide sessions" : "Show sessions"}
                    <Icon name="chevronDown" className={cx("h-4 w-4 transition-transform", expanded && "rotate-180")} />
                  </button>
                </Td>
              </tr>
              {expanded && (
                <tr>
                  <td colSpan={7} className="border-b border-line bg-[#FAFBFC] p-0">
                    <p className="mb-1 ml-16 mr-[22px] mt-2.5 text-xs font-semibold uppercase tracking-[0.09em] text-muted">
                      Individual sessions · {person.name}
                    </p>
                    <div className="overflow-x-auto">
                      <table className="w-full border-collapse text-sm">
                        <thead>
                          <tr className="text-left text-[11.5px] uppercase tracking-[0.06em] text-muted">
                            <th className="py-2 pl-16 pr-[22px] font-medium">Started</th>
                            <th className="px-[22px] py-2 font-medium">Last active</th>
                            <th className="px-[22px] py-2 font-medium">Ended</th>
                            <th className="px-[22px] py-2 font-medium">Duration</th>
                            <th className="px-[22px] py-2 font-medium">Address</th>
                            <th className="px-[22px] py-2 font-medium">Status</th>
                          </tr>
                        </thead>
                        <tbody>
                          {person.sessions.map((session) => (
                            <tr key={session.id} className="whitespace-nowrap">
                              <td className="tabular py-2 pb-2.5 pl-16 pr-[22px]">{formatTime(session.started_at)}</td>
                              <td className="tabular px-[22px] py-2 pb-2.5">{formatTime(session.last_active_at)}</td>
                              <td className="tabular px-[22px] py-2 pb-2.5 text-muted">
                                {session.ended_at ? formatTime(session.ended_at) : session.online ? "Still signed in" : "Timed out"}
                              </td>
                              <td className="tabular px-[22px] py-2 pb-2.5">{formatDuration(session.active_seconds)}</td>
                              <td className="px-[22px] py-2 pb-2.5">
                                <span className="font-mono text-xs">{session.ip || "—"}</span>
                              </td>
                              <td className="px-[22px] py-2 pb-2.5">
                                {session.online ? (
                                  <Badge tone="success" dot>Online</Badge>
                                ) : (
                                  <Badge tone="neutral" dot>Ended</Badge>
                                )}
                              </td>
                            </tr>
                          ))}
                        </tbody>
                      </table>
                    </div>
                  </td>
                </tr>
              )}
            </Fragment>
          );
        })}
      </tbody>
    </Table>
  );
}
