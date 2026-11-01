"use client";

import { useEffect, useState } from "react";

import {
  Badge,
  Card,
  EmptyState,
  PageHeader,
  ProblemNotice,
  Stat,
  StatGrid,
  Table,
  Td,
  Th,
  statusTone,
} from "@/components/ui";
import { useToast } from "@/components/toast";
import { requireUser } from "@/lib/auth/session";
import {
  getTodayReport,
  listDailyReports,
  listLeaveRequests,
  listPendingApprovals,
  listTasks,
  listTimeEntries,
  type Approval,
  type DailyReport,
  type LeaveRequest,
  type Task,
  type TimeEntry,
} from "@/lib/api/atlas";
import { ApiError } from "@/lib/api/client";
import { personDirectory, personName } from "@/lib/directory";
import { formatDay, formatMinor, humanise, isOverdue } from "@/lib/format";
import { useMutation } from "@/lib/use-mutation";

/**
 * My Work dashboard. A personal overview for employees: my open tasks, my
 * today's daily report status, my pending leave requests, my recent time
 * entries, and approvals waiting for my decision.
 */
export const dynamic = "force-dynamic";

async function safe<T>(promise: Promise<T>, fallback: T): Promise<T> {
  try {
    return await promise;
  } catch (error) {
    if (error instanceof ApiError) return fallback;
    throw error;
  }
}

export default async function MyWorkPage() {
  const user = await requireUser();
  const me = user.person_id;

  const [tasks, setTasks] = useState<Task[]>([]);
  const [todayReport, setTodayReport] = useState<DailyReport | null>(null);
  const [leaveRequests, setLeaveRequests] = useState<LeaveRequest[]>([]);
  const [timeEntries, setTimeEntries] = useState<TimeEntry[]>([]);
  const [pendingApprovals, setPendingApprovals] = useState<Approval[]>([]);
  const [error, setError] = useState<ApiError | null>(null);
  const [names, setNames] = useState<Record<string, string>>({});

  useEffect(() => {
    const load = async () => {
      const [
        tasksResult,
        reportResult,
        leaveResult,
        timeResult,
        approvalsResult,
        dirResult,
      ] = await Promise.allSettled([
        me
          ? listTasks({ assignee_person_id: me, limit: 50 })
          : Promise.reject(new Error("no person")),
        me ? getTodayReport() : Promise.reject(new Error("no person")),
        me
          ? listLeaveRequests({ person_id: me, limit: 20 })
          : Promise.reject(new Error("no person")),
        me
          ? listTimeEntries({ person_id: me, limit: 20 })
          : Promise.reject(new Error("no person")),
        listPendingApprovals({ limit: 20 }),
        personDirectory(),
      ]);

      if (tasksResult.status === "fulfilled")
        setTasks(tasksResult.value.items);
      if (reportResult.status === "fulfilled") setTodayReport(reportResult.value);
      if (leaveResult.status === "fulfilled")
        setLeaveRequests(leaveResult.value.items);
      if (timeResult.status === "fulfilled")
        setTimeEntries(timeResult.value.items);
      if (approvalsResult.status === "fulfilled")
        setPendingApprovals(approvalsResult.value.items);
      if (dirResult.status === "fulfilled") {
        const dir = dirResult.value;
        const nameMap: Record<string, string> = {};
        dir.forEach((name, id) => {
          nameMap[id] = name;
        });
        setNames(nameMap);
      }
    };

    load().catch((err) => {
      if (err instanceof ApiError) setError(err);
    });
  }, [me]);

  const openTasks = tasks.filter(
    (t) => t.status !== "done" && t.status !== "cancelled",
  );
  const overdueTasks = openTasks.filter((t) => isOverdue(t.due_at, t.status));
  const blockedTasks = openTasks.filter((t) => t.blocked);
  const pendingLeave = leaveRequests.filter((l) => l.status === "requested");

  const totalMinutes = timeEntries.reduce((sum, e) => sum + e.minutes, 0);

  return (
    <div className="space-y-6">
      <PageHeader
        title="My Work"
        description="Your tasks, reports, leave, time and approvals at a glance."
      />

      {error && (
        <ProblemNotice
          error={error}
          title={
            error.status === 403
              ? "You do not have permission to view your work dashboard"
              : "My Work could not be loaded"
          }
        />
      )}

      <StatGrid label="My work at a glance">
        <Stat
          label="Open tasks"
          value={openTasks.length.toString()}
          icon="board"
          iconTone="info"
          hint={`${blockedTasks.length} blocked · ${overdueTasks.length} overdue`}
        />
        <Stat
          label="Today's report"
          value={
            todayReport?.submitted_at ? "Filed" : "Not filed"
          }
          icon="clipboard"
          iconTone={
            todayReport?.submitted_at ? "success" : "warning"
          }
          hint={todayReport?.status ?? "Not yet filed"}
        />
        <Stat
          label="Pending leave"
          value={pendingLeave.length.toString()}
          icon="calendar"
          iconTone="neutral"
          hint="Waiting for approval"
        />
        <Stat
          label="Hours logged"
          value={(totalMinutes / 60).toFixed(1)}
          icon="clock"
          iconTone="success"
          hint="Time entries this period"
        />
        <Stat
          label="Approvals for you"
          value={pendingApprovals.length.toString()}
          icon="shieldCheck"
          iconTone="warning"
          hint="Waiting for your decision"
        />
      </StatGrid>

      <div className="grid grid-cols-1 gap-5 lg:grid-cols-3">
        {/* My Tasks */}
        <Card title="My Tasks" description={openTasks.length.toString()}>
          {openTasks.length === 0 ? (
            <EmptyState
              icon="board"
              iconTone="success"
              title="No open tasks"
            >
              You're all caught up.
            </EmptyState>
          ) : (
            <Table>
              <thead>
                <tr>
                  <Th>Task</Th>
                  <Th>Status</Th>
                  <Th className="text-right">Due</Th>
                </tr>
              </thead>
              <tbody>
                {openTasks.slice(0, 5).map((t) => (
                  <tr key={t.id}>
                    <Td>
                      <span className="font-medium text-night">
                        {t.title}
                      </span>
                    </Td>
                    <Td>
                      <Badge tone={statusTone(t.status)} dot>
                        {humanise(t.status)}
                      </Badge>
                    </Td>
                    <Td className="tabular text-right text-muted">
                      {t.due_at
                        ? isOverdue(t.due_at, t.status)
                          ? "Overdue"
                          : formatDay(t.due_at)
                        : "—"}
                    </Td>
                  </tr>
                ))}
              </tbody>
            </Table>
          )}
        </Card>

        {/* Daily Report */}
        <Card title="Today's Report" description="Your daily standup.">
          {todayReport ? (
            <Table>
              <tbody>
                <tr>
                  <Th className="text-muted w-[30%]">Status</Th>
                  <Td className="tabular">
                    <Badge tone={statusTone(todayReport.status)}>
                      {humanise(todayReport.status)}
                    </Badge>
                  </Td>
                </tr>
                <tr>
                  <Th className="text-muted">Hours</Th>
                  <Td className="tabular text-right">
                    {todayReport.hours_worked ?? "—"}
                  </Td>
                </tr>
                <tr>
                  <Th className="text-muted">Work done</Th>
                  <Td className="tabular text-muted text-[13px]">
                    {todayReport.work_done ?? "—"}
                  </Td>
                </tr>
              </tbody>
            </Table>
          ) : (
            <div className="p-[22px]">
              <EmptyState
                icon="clipboard"
                iconTone="warning"
                title="No report filed yet"
              >
                File today's report to track your progress.
              </EmptyState>
            </div>
          )}
        </Card>

        {/* Leave Requests */}
        <Card title="My Leave" description={pendingLeave.length.toString()}>
          {pendingLeave.length === 0 ? (
            <EmptyState
              icon="calendar"
              iconTone="success"
              title="No pending leave"
            >
              All your leave requests are decided.
            </EmptyState>
          ) : (
            <Table>
              <thead>
                <tr>
                  <Th>Type</Th>
                  <Th>Period</Th>
                  <Th>Status</Th>
                </tr>
              </thead>
              <tbody>
                {pendingLeave.slice(0, 5).map((l) => (
                  <tr key={l.id}>
                    <Td className="tabular text-muted">
                      {humanise(l.leave_type)}
                    </Td>
                    <Td className="tabular text-muted">
                      {formatDay(l.from_date)} – {formatDay(l.to_date)}
                    </Td>
                    <Td>
                      <Badge tone="info">{humanise(l.status)}</Badge>
                    </Td>
                  </tr>
                ))}
              </tbody>
            </Table>
          )}
        </Card>
      </div>

      <div className="grid grid-cols-1 gap-5 lg:grid-cols-2">
        {/* Time Entries */}
        <Card title="Recent Time Entries" description={timeEntries.length.toString()}>
          {timeEntries.length === 0 ? (
            <EmptyState
              icon="clock"
              iconTone="info"
              title="No time entries yet"
            >
              Log time against your tasks to track your effort.
            </EmptyState>
          ) : (
            <Table>
              <thead>
                <tr>
                  <Th>Date</Th>
                  <Th>Task</Th>
                  <Th className="text-right">Minutes</Th>
                </tr>
              </thead>
              <tbody>
                {timeEntries.slice(0, 5).map((e) => (
                  <tr key={e.id}>
                    <Td className="tabular text-muted">
                      {formatDay(e.entry_date)}
                    </Td>
                    <Td className="tabular text-muted font-mono text-[13px]">
                      {e.task_id.slice(0, 8)}
                    </Td>
                    <Td className="tabular text-right">{e.minutes}</Td>
                  </tr>
                ))}
              </tbody>
            </Table>
          )}
        </Card>

        {/* Pending Approvals */}
        <Card title="Approvals for You" description={pendingApprovals.length.toString()}>
          {pendingApprovals.length === 0 ? (
            <EmptyState
              icon="shieldCheck"
              iconTone="success"
              title="No pending approvals"
            >
              No approvals are waiting for your decision.
            </EmptyState>
          ) : (
            <Table>
              <thead>
                <tr>
                  <Th>Ref</Th>
                  <Th>Entity</Th>
                  <Th className="text-right">Amount</Th>
                </tr>
              </thead>
              <tbody>
                {pendingApprovals.slice(0, 5).map((a) => (
                  <tr key={a.id}>
                    <Td className="tabular text-muted font-mono text-[13px]">
                      {a.ref}
                    </Td>
                    <Td className="tabular text-muted">
                      {a.entity_type} {a.entity_id.slice(0, 8)}
                    </Td>
                    <Td className="tabular text-right">
                      {formatMinor(a.amount_minor, a.currency)}
                    </Td>
                  </tr>
                ))}
              </tbody>
            </Table>
          )}
        </Card>
      </div>
    </div>
  );
}
