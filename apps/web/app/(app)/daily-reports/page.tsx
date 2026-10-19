import {
  Badge,
  Card,
  DateBlock,
  EmptyState,
  PageHeader,
  Table,
  Td,
  Th,
  statusTone,
} from "@/components/ui";
import {
  getTodayReport,
  listDailyReports,
  listTasks,
  type DailyReport,
  type Task,
} from "@/lib/api/atlas";
import { ApiError } from "@/lib/api/client";
import { requireUser } from "@/lib/auth/session";
import { personDirectory, personName } from "@/lib/directory";
import { dayParts, formatDay, statusLabel, today } from "@/lib/format";

import { DailyReportWorkspace } from "./report-form";

/**
 * Daily reports (blueprint §8.5; prototype 08). The caller files their own
 * report for today; the same form updates it if it already exists (`GET
 * /api/v1/daily-reports/today` 404s until then, which is expected, not an
 * error). Beside it: the caller's open tasks to report against, and their
 * recent reports. Below, the team's reports for viewers who may read them
 * (`people.daily_report.read`).
 */

export const dynamic = "force-dynamic";

const OPEN = new Set(["todo", "in_progress", "blocked", "review"]);

/** A read that may be refused: 403/404 become `null`, anything else throws. */
async function maybe<T>(promise: Promise<T>): Promise<T | null> {
  try {
    return await promise;
  } catch (error) {
    if (error instanceof ApiError) return null;
    throw error;
  }
}

export default async function DailyReportsPage() {
  const user = await requireUser();
  const me = user.person_id;

  const [todayReport, mine, team, myTasks] = await Promise.all([
    maybe(getTodayReport()),
    me ? maybe(listDailyReports({ reporter_person_id: me, limit: 7 })) : Promise.resolve(null),
    maybe(listDailyReports({ limit: 30 })),
    me ? maybe(listTasks({ assignee_person_id: me, limit: 100 })) : Promise.resolve(null),
  ]);

  const openMine: Task[] = (myTasks?.items ?? []).filter((task) => OPEN.has(task.status));
  const unowned =
    openMine.length === 0
      ? await maybe(listTasks({ limit: 200 })).then((page) =>
          page ? page.items.filter((t) => OPEN.has(t.status) && !t.assignee_person_id).length : null,
        )
      : null;

  const others = (team?.items ?? []).filter((report) => report.reporter_person_id !== me);
  const names = others.length > 0 ? await personDirectory() : new Map<string, string>();
  const recent = (mine?.items ?? []).filter((report) => report.submitted_at);

  return (
    <div className="space-y-6">
      <PageHeader
        title="Daily report"
        description="What moved, what is blocked and what is next — filed in one place."
      />

      <DailyReportWorkspace
        existing={todayReport}
        today={today()}
        myTasks={openMine}
        unowned={unowned}
        hasPerson={Boolean(me)}
      />

      <div className="grid grid-cols-1 items-start gap-5 xl:grid-cols-[minmax(0,1fr)_minmax(0,1.8fr)]">
        <Card title="Your recent reports" description="Your last filed reports, newest first.">
          {recent.length === 0 ? (
            <EmptyState icon="clipboard" iconTone="info" title="Nothing filed yet">
              Your filed reports appear here, newest first.
            </EmptyState>
          ) : (
            <ul className="flex flex-col gap-2.5">
              {recent.map((report) => (
                <RecentReport key={report.id} report={report} />
              ))}
            </ul>
          )}
        </Card>

        {team !== null && others.length > 0 && (
          <Card title="Team reports" description={`The ${others.length} most recent from others`} padded={false}>
            <Table>
              <thead>
                <tr>
                  <Th>Day</Th>
                  <Th>Reporter</Th>
                  <Th>Status</Th>
                  <Th>Hours</Th>
                  <Th>What moved</Th>
                </tr>
              </thead>
              <tbody>
                {others.map((report) => (
                  <tr key={report.id}>
                    <Td className="whitespace-nowrap text-muted">{formatDay(report.report_date)}</Td>
                    <Td className="whitespace-nowrap font-medium">{personName(names, report.reporter_person_id)}</Td>
                    <Td>
                      <Badge tone={statusTone(report.status)} dot>
                        {report.status === "submitted" ? "Filed" : statusLabel(report.status)}
                      </Badge>
                    </Td>
                    <Td className="tabular text-muted">{report.hours_worked ?? "—"}</Td>
                    <Td className="max-w-md text-muted">
                      <span className="line-clamp-2">{report.work_done || "—"}</span>
                    </Td>
                  </tr>
                ))}
              </tbody>
            </Table>
          </Card>
        )}
      </div>
    </div>
  );
}

function RecentReport({ report }: { report: DailyReport }) {
  const { weekday, day } = dayParts(report.report_date);
  const firstLine = (report.work_done ?? "").split("\n").find((line) => line.trim()) ?? "—";
  return (
    <li className="flex items-start gap-3 rounded-[14px] border border-line p-3">
      <DateBlock top={weekday} day={day} />
      <span className="min-w-0 flex-1">
        <b className="block truncate text-[14.5px] font-semibold">{firstLine}</b>
        <span className="mt-1.5 flex flex-wrap gap-1.5">
          <Badge tone={statusTone(report.status)} dot>
            {report.status === "late" ? "Filed late" : report.status === "submitted" ? "Filed" : statusLabel(report.status)}
          </Badge>
          {report.hours_worked != null && <Badge tone="neutral">{report.hours_worked}h</Badge>}
          {report.blockers && <Badge tone="warning" dot>Blocker</Badge>}
        </span>
      </span>
    </li>
  );
}
