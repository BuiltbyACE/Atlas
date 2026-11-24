import { DayPicker } from "@/components/day-picker";
import { Icon } from "@/components/icons";
import {
  Card,
  ChipLink,
  Chips,
  Legend,
  Meter,
  PageHeader,
  ProblemNotice,
} from "@/components/ui";
import {
  getAvailability,
  listLeaveRequests,
  type Availability,
  type LeaveRequest,
} from "@/lib/api/atlas";
import { ApiError } from "@/lib/api/client";
import { requireUser } from "@/lib/auth/session";
import { personDirectory, rosterOf } from "@/lib/directory";
import { formatShortDay, today } from "@/lib/format";

import { LeaveQueue, RequestLeaveButton } from "./leave-panel";
import { OffboardForm } from "./offboard-form";
import { TeamGrid } from "./team-grid";

/**
 * Availability & leave (blueprint §8.5; prototype 06): the roster for a day
 * (`GET /api/v1/people/availability`, gated by `people.person.read`) and the
 * leave queue (`GET /api/v1/people/leave-requests`) that decides it.
 */

export const dynamic = "force-dynamic";

const LEAVE_FILTERS = [
  { value: "requested", label: "Pending" },
  { value: "approved", label: "Approved" },
  { value: "declined", label: "Declined" },
  { value: "cancelled", label: "Withdrawn" },
  { value: "all", label: "All" },
] as const;

export default async function PeoplePage({
  searchParams,
}: {
  searchParams: Promise<{ date?: string; leave?: string; new?: string }>;
}) {
  const { date, leave, new: create } = await searchParams;
  const now = today();
  const day = date && /^\d{4}-\d{2}-\d{2}$/.test(date) ? date : now;
  const leaveFilter = LEAVE_FILTERS.find((f) => f.value === leave)?.value ?? "requested";
  const user = await requireUser();

  let availability: Availability | null = null;
  let availabilityError: ApiError | null = null;
  try {
    availability = await getAvailability(day);
  } catch (error) {
    if (!(error instanceof ApiError)) throw error;
    availabilityError = error;
  }

  let leaveRequests: LeaveRequest[] = [];
  let leaveError: ApiError | null = null;
  try {
    leaveRequests = (
      await listLeaveRequests({
        status: leaveFilter === "all" ? undefined : leaveFilter,
        limit: 50,
      })
    ).items;
  } catch (error) {
    if (!(error instanceof ApiError)) throw error;
    leaveError = error;
  }

  // The roster (every non-exited person, whatever the day) doubles as the
  // name directory; reuse it rather than reading it twice.
  const names =
    availability
      ? new Map(availability.items.map((item) => [item.person_id, item.full_name]))
      : await personDirectory();

  const items = availability?.items ?? [];
  const available = items.filter((item) => item.available).length;
  const onLeave = items.filter((item) => !item.available && item.leave).length;
  const awayOther = items.length - available - onLeave;
  const leaveHref = (value: string) => {
    const query = new URLSearchParams();
    if (date) query.set("date", date);
    if (value !== "requested") query.set("leave", value);
    const qs = query.toString();
    return qs ? `/people?${qs}` : "/people";
  };

  return (
    <div className="space-y-6">
      <PageHeader
        title="Availability & leave"
        description="Who is in on a given day, and the leave queue that decides it."
        actions={<RequestLeaveButton autoOpen={create === "leave"} />}
      />

      <DayPicker day={day} today={now} />

      <div className="grid grid-cols-1 items-start gap-5 xl:grid-cols-[minmax(0,1.8fr)_minmax(0,1fr)]">
        <Card
          title="Team"
          description={
            availability
              ? `${available} available · ${items.length - available} away`
              : undefined
          }
          padded={false}
        >
          {availabilityError ? (
            <div className="px-[22px] pb-[22px]">
              <ProblemNotice
                error={availabilityError}
                title={
                  availabilityError.status === 403
                    ? "The team roster isn’t visible to you"
                    : "The roster could not be loaded"
                }
              />
            </div>
          ) : (
            <TeamGrid items={items} me={user.person_id} />
          )}
        </Card>

        <div className="flex flex-col gap-5">
          {availability && (
            <Card title="At a glance" description={formatShortDay(day)}>
              <div className="flex items-baseline gap-2.5">
                <b className="tabular text-[44px] font-semibold leading-none tracking-[-0.02em]">{available}</b>
                <span className="text-muted">of {items.length} people in</span>
              </div>
              <Meter
                className="my-4"
                label={`${available} available, ${onLeave} on leave, ${awayOther} away`}
                segments={[
                  { value: available, color: "#22A55A" },
                  { value: onLeave, color: "#1A6CF5" },
                  { value: awayOther, color: "#F5A524" },
                ]}
              />
              <Legend
                items={[
                  { label: "Available", value: available, color: "#22A55A" },
                  { label: "On approved leave", value: onLeave, color: "#1A6CF5" },
                  { label: "Away, other reasons", value: awayOther, color: "#F5A524" },
                ]}
              />
            </Card>
          )}

          <Card
            title="Leave queue"
            description={
              leaveFilter === "requested"
                ? "Requests waiting for a decision."
                : "Decisions here change the roster."
            }
          >
            <div className="mb-4">
              <Chips label="Filter leave requests">
                {LEAVE_FILTERS.map((f) => (
                  <ChipLink
                    key={f.value}
                    href={leaveHref(f.value)}
                    pressed={leaveFilter === f.value}
                    count={leaveFilter === f.value ? leaveRequests.length : undefined}
                  >
                    {f.label}
                  </ChipLink>
                ))}
              </Chips>
            </div>
            {leaveError ? (
              <ProblemNotice error={leaveError} title="Leave requests could not be loaded" />
            ) : (
              <LeaveQueue requests={leaveRequests} names={Object.fromEntries(names)} me={user.person_id} />
            )}
          </Card>
        </div>
      </div>

      <details className="group rounded-card border border-line bg-white shadow-card">
        <summary className="flex cursor-pointer list-none items-center gap-3 px-[22px] py-5 [&::-webkit-details-marker]:hidden">
          <span className="grid h-10 w-10 flex-none place-items-center rounded-control bg-[#FDE8E7] text-[#C0352B]">
            <Icon name="signOut" />
          </span>
          <span className="min-w-0 flex-1">
            <b className="block text-[17px] font-semibold">Off-board a person</b>
            <span className="block text-[13.5px] text-muted">
              Ends employment, deactivates the login and revokes sessions and roles.
            </span>
          </span>
          <Icon name="chevronDown" className="text-muted transition-transform group-open:rotate-180" />
        </summary>
        <div className="border-t border-line px-[22px] py-5">
          <OffboardForm people={rosterOf(names)} me={user.person_id} />
        </div>
      </details>
    </div>
  );
}
