import Link from "next/link";
import type { ReactNode } from "react";

import { Badge, Card, EmptyState, PageHeader, ProblemNotice, cx } from "@/components/ui";
import {
  getAvailability,
  listLeaveRequests,
  type Availability,
  type LeaveRequest,
  type Page,
} from "@/lib/api/atlas";
import { ApiError } from "@/lib/api/client";
import { formatDay, humanise, today } from "@/lib/format";

import { LeavePanel } from "./leave-panel";
import { OffboardForm } from "./offboard-form";

/**
 * People & accountability (blueprint §8.5): the availability roster for a day
 * (`GET /api/v1/people/availability`, gated by `people.person.read`) and the
 * leave queue (`GET /api/v1/people/leave-requests`).
 *
 * The day picker is a plain GET form - no client JS - so the roster stays a
 * server component. Leave actions live in `LeavePanel`.
 */

export const dynamic = "force-dynamic";

const LEAVE_FILTERS = ["requested", "approved", "declined", "cancelled"] as const;

export default async function PeoplePage({
  searchParams,
}: {
  searchParams: Promise<{ date?: string; leave?: string }>;
}) {
  const { date, leave } = await searchParams;
  const day = date && /^\d{4}-\d{2}-\d{2}$/.test(date) ? date : today();
  const leaveFilter = LEAVE_FILTERS.find((status) => status === leave);

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
    const page: Page<LeaveRequest> = await listLeaveRequests({
      status: leaveFilter,
      limit: 50,
    });
    leaveRequests = page.items;
  } catch (error) {
    if (!(error instanceof ApiError)) throw error;
    leaveError = error;
  }

  const available = availability?.items.filter((item) => item.available) ?? [];
  const away = availability?.items.filter((item) => !item.available) ?? [];

  return (
    <div className="space-y-6">
      <PageHeader
        title="People"
        description="Who is in today, and the leave queue that decides it (blueprint §8.5)."
      />

      <Card
        title="Availability"
        description={
          availability
            ? `${available.length} available · ${away.length} away on ${formatDay(availability.date)}`
            : undefined
        }
        actions={
          <form action="/people" className="flex items-center gap-2">
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
        {availabilityError ? (
          <ProblemNotice
            error={availabilityError}
            title={
              availabilityError.status === 403
                ? "You cannot read the roster"
                : "The roster could not be loaded"
            }
          />
        ) : (availability?.items.length ?? 0) === 0 ? (
          <EmptyState>No people on record.</EmptyState>
        ) : (
          <ul className="grid grid-cols-1 gap-2 sm:grid-cols-2 lg:grid-cols-3">
            {availability!.items.map((item) => (
              <li
                key={item.person_id}
                className={cx(
                  "rounded-lg border px-3 py-2",
                  item.available
                    ? "border-slate-200 bg-white"
                    : "border-amber-200 bg-amber-50",
                )}
              >
                <div className="flex items-center justify-between gap-2">
                  <span className="text-sm font-medium text-slate-900">
                    {item.full_name}
                  </span>
                  <Badge tone={item.available ? "success" : "warning"}>
                    {item.available ? "available" : "away"}
                  </Badge>
                </div>
                <p className="mt-0.5 text-xs text-slate-500">
                  {humanise(item.employment_type)}
                  {item.leave && (
                    <>
                      {" · "}
                      <span className="text-amber-700">
                        {humanise(item.leave.leave_type)} until{" "}
                        {formatDay(item.leave.to_date)}
                      </span>
                    </>
                  )}
                </p>
              </li>
            ))}
          </ul>
        )}
      </Card>

      <Card
        title="Leave requests"
        description="Decisions here change the roster above."
        actions={
          <div className="flex flex-wrap items-center gap-1">
            <FilterLink href="/people" active={!leaveFilter}>
              All
            </FilterLink>
            {LEAVE_FILTERS.map((status) => (
              <FilterLink
                key={status}
                href={`/people?leave=${status}`}
                active={leaveFilter === status}
              >
                {humanise(status)}
              </FilterLink>
            ))}
          </div>
        }
      >
        {leaveError ? (
          <ProblemNotice error={leaveError} title="Leave requests could not be loaded" />
        ) : (
          <LeavePanel requests={leaveRequests} />
        )}
      </Card>

      <Card
        title="Off-board a person"
        description="Ends employment, deactivates the login and revokes sessions and roles."
      >
        <OffboardForm />
      </Card>
    </div>
  );
}

function FilterLink({
  href,
  active,
  children,
}: {
  href: string;
  active: boolean;
  children: ReactNode;
}) {
  return (
    <Link
      href={href}
      className={cx(
        "rounded-full px-2.5 py-1 text-xs font-medium ring-1 ring-inset",
        active
          ? "bg-atlas text-white ring-atlas"
          : "bg-white text-slate-600 ring-slate-200 hover:bg-slate-50",
      )}
    >
      {children}
    </Link>
  );
}
