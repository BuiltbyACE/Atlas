import { Badge, Card, EmptyState, PageHeader, ProblemNotice, Table, Td, Th, statusTone } from "@/components/ui";
import { getTodayReport, listDailyReports, type DailyReport, type Page } from "@/lib/api/atlas";
import { ApiError } from "@/lib/api/client";
import { personDirectory } from "@/lib/directory";
import { formatDateTime, formatDay, humanise, shortId } from "@/lib/format";

import { ReportForm } from "./report-form";

/**
 * Daily reports (blueprint §8.5). The caller files their own report for today;
 * the same form updates it if it already exists (`GET /api/v1/daily-reports/
 * today` 404s until then, which is expected, not an error).
 *
 * A recent-reports read needs `people.daily_report.read`; when the viewer does
 * not hold it the section is simply omitted.
 */

export const dynamic = "force-dynamic";

export default async function DailyReportsPage() {
  let todayReport: DailyReport | null = null;
  try {
    todayReport = await getTodayReport();
  } catch (error) {
    // 404 means "not filed yet" - the form is what we want to show.
    if (!(error instanceof ApiError && error.status === 404)) {
      if (!(error instanceof ApiError)) throw error;
    }
  }

  let recent: DailyReport[] = [];
  let recentRefused = false;
  try {
    const page: Page<DailyReport> = await listDailyReports({ limit: 30 });
    recent = page.items;
  } catch (error) {
    if (error instanceof ApiError && error.status === 403) {
      recentRefused = true;
    } else if (!(error instanceof ApiError)) {
      throw error;
    }
  }

  const names = recentRefused ? new Map<string, string>() : await personDirectory();

  return (
    <div className="space-y-6">
      <PageHeader
        title="Daily report"
        description="What moved, what is blocked and what is next — filed in one place (blueprint §8.5)."
      />

      <Card
        title="Today"
        description={
          todayReport?.submitted_at
            ? `Filed ${formatDateTime(todayReport.submitted_at)} · version ${todayReport.version}`
            : "Nothing filed for today yet."
        }
      >
        <ReportForm existing={todayReport} />
      </Card>

      {recentRefused ? (
        <ProblemNotice
          error={{ message: "You do not have permission to read the team's reports." }}
          title="Team reports hidden"
        />
      ) : (
        <Card title="Recent reports" description={`${recent.length} most recent`} padded={false}>
          {recent.length === 0 ? (
            <div className="px-5 py-6">
              <EmptyState>No reports filed yet.</EmptyState>
            </div>
          ) : (
            <Table>
              <thead>
                <tr>
                  <Th>Day</Th>
                  <Th>Reporter</Th>
                  <Th>Status</Th>
                  <Th>Hours</Th>
                  <Th>Work done</Th>
                </tr>
              </thead>
              <tbody>
                {recent.map((report) => (
                  <tr key={report.id}>
                    <Td className="whitespace-nowrap text-slate-600">
                      {formatDay(report.report_date)}
                    </Td>
                    <Td className="text-slate-600">
                      {names.get(report.reporter_person_id) ??
                        shortId(report.reporter_person_id)}
                    </Td>
                    <Td>
                      <Badge tone={statusTone(report.status)}>{humanise(report.status)}</Badge>
                    </Td>
                    <Td className="tabular-nums text-slate-600">
                      {report.hours_worked ?? "—"}
                    </Td>
                    <Td className="max-w-md text-slate-600">
                      <span className="line-clamp-2">{report.work_done || "—"}</span>
                    </Td>
                  </tr>
                ))}
              </tbody>
            </Table>
          )}
        </Card>
      )}
    </div>
  );
}
