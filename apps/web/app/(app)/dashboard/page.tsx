import Link from "next/link";

import { SignalRefresh } from "@/components/signal-refresh";
import { Card, EmptyState, PageHeader, ProblemNotice, Stat, Table, Td, Th } from "@/components/ui";
import { getExecutiveSummary } from "@/lib/api/atlas";
import { ApiError } from "@/lib/api/client";
import { formatDateTime, humanise } from "@/lib/format";
import { EXEC_DASHBOARD_TOPIC } from "@/lib/realtime/topics";

/**
 * Executive dashboard v1 (blueprint §8.8).
 *
 * One read model, computed by Go (`GET /api/v1/dashboard/executive`, gated by
 * `intel.dashboard.read` which both CEOs hold). The tiles arrive live over the
 * `exec:dashboard` Phoenix topic as signals only; `SignalRefresh` refetches.
 *
 * If the viewer is not a CEO the route is 403 and we say so plainly rather than
 * showing empty tiles - the failure is a permission fact, not a data gap.
 */

export const dynamic = "force-dynamic";

const PEOPLE_ORDER = ["active", "on_leave", "onboarding", "exited"] as const;

export default async function DashboardPage() {
  let summary;
  try {
    summary = await getExecutiveSummary();
  } catch (error) {
    if (!(error instanceof ApiError)) throw error;
    return (
      <div className="space-y-6">
        <PageHeader
          title="Executive dashboard"
          description="Staff status, approvals, overdue work and recent audit highlights."
        />
        <ProblemNotice
          error={error}
          title={
            error.status === 403
              ? "This dashboard is for the executive principals"
              : "The dashboard could not be loaded"
          }
        />
      </div>
    );
  }

  const { people, approvals, work, recent_audit: audit, generated_at } = summary;
  const byStatus = people.by_status ?? {};

  return (
    <div className="space-y-6">
      <PageHeader
        title="Executive dashboard"
        description={
          <>
            As at {formatDateTime(generated_at)}. Signals arrive on{" "}
            <code className="font-mono text-xs">exec:dashboard</code>; each one
            refetches from Go.
          </>
        }
        actions={<SignalRefresh topic={EXEC_DASHBOARD_TOPIC} />}
      />

      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
        <Stat label="People" value={people.total} hint="Non-system persons on record" />
        <Stat
          label="Overdue work"
          value={work.overdue}
          tone={work.overdue > 0 ? "danger" : "neutral"}
          hint="Open tasks past their deadline"
        />
        <Stat
          label="Open escalations"
          value={work.open_escalations}
          tone={work.open_escalations > 0 ? "warning" : "neutral"}
          hint="Raised by the overdue sweep"
        />
        <Stat
          label="Approvals waiting"
          value={approvals.pending}
          tone={approvals.overdue > 0 ? "danger" : "neutral"}
          hint={`${approvals.overdue} overdue · ${approvals.escalated} escalated`}
        />
      </div>

      <div className="grid grid-cols-1 gap-4 lg:grid-cols-3">
        <Card title="Staff status">
          {people.total === 0 ? (
            <EmptyState>No people on record yet.</EmptyState>
          ) : (
            <ul className="space-y-2">
              {PEOPLE_ORDER.map((status) => (
                <li key={status} className="flex items-center justify-between text-sm">
                  <span className="text-slate-600">{humanise(status)}</span>
                  <span className="font-medium tabular-nums text-slate-900">
                    {byStatus[status] ?? 0}
                  </span>
                </li>
              ))}
            </ul>
          )}
        </Card>

        <Card
          title="Pending approvals by role"
          description="Where the queue is sitting."
        >
          {Object.keys(approvals.pending_by_role ?? {}).length === 0 ? (
            <EmptyState>Nothing waiting on anyone.</EmptyState>
          ) : (
            <ul className="space-y-2">
              {Object.entries(approvals.pending_by_role).map(([role, count]) => (
                <li key={role} className="flex items-center justify-between text-sm">
                  <span className="text-slate-600">{humanise(role)}</span>
                  <span className="font-medium tabular-nums text-slate-900">
                    {count}
                  </span>
                </li>
              ))}
            </ul>
          )}
        </Card>

        <Card title="Shortcuts">
          <ul className="space-y-2 text-sm">
            <li>
              <Link href="/work" className="text-atlas hover:underline">
                Projects &amp; tasks
              </Link>
            </li>
            <li>
              <Link href="/work/escalations" className="text-atlas hover:underline">
                Open escalations
              </Link>
            </li>
            <li>
              <Link href="/daily-reports" className="text-atlas hover:underline">
                Daily reports
              </Link>
            </li>
            <li>
              <Link href="/people" className="text-atlas hover:underline">
                Availability &amp; leave
              </Link>
            </li>
          </ul>
        </Card>
      </div>

      <Card title="Recent audit highlights" padded={false}>
        {audit.length === 0 ? (
          <div className="px-5 py-4">
            <EmptyState>No audit events yet.</EmptyState>
          </div>
        ) : (
          <Table>
            <thead>
              <tr>
                <Th>When</Th>
                <Th>Action</Th>
                <Th>Entity</Th>
                <Th>Actor</Th>
              </tr>
            </thead>
            <tbody>
              {audit.map((entry) => (
                <tr key={entry.id}>
                  <Td className="whitespace-nowrap text-slate-500">
                    {formatDateTime(entry.occurred_at)}
                  </Td>
                  <Td className="font-mono text-xs">{entry.action}</Td>
                  <Td>{humanise(entry.entity_type)}</Td>
                  <Td className="text-slate-500">
                    {entry.actor_type}:{entry.actor_id.slice(0, 8)}
                  </Td>
                </tr>
              ))}
            </tbody>
          </Table>
        )}
      </Card>
    </div>
  );
}
