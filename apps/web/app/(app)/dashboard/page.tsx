import Image from "next/image";
import Link from "next/link";
import type { ReactNode } from "react";

import { BlobLink } from "@/components/blob-button";
import { ButtonLink, buttonClass } from "@/components/form";
import { Icon, type IconName } from "@/components/icons";
import { SignalRefresh } from "@/components/signal-refresh";
import {
  ArrowLink,
  Avatar,
  Badge,
  Card,
  EmptyState,
  IconTile,
  Label,
  Legend,
  Meter,
  ProblemNotice,
  Stat,
  StatGrid,
  Table,
  Td,
  Th,
  statusTone,
  type Tone,
} from "@/components/ui";
import { columnLabel } from "@/components/work/board";
import {
  getExecutiveSummary,
  getTodayReport,
  listEscalations,
  listProjects,
  listTasks,
  type AuditHighlight,
  type DailyReport,
  type Escalation,
  type ExecutiveSummary,
  type Page,
  type Project,
  type Task,
} from "@/lib/api/atlas";
import { ApiError } from "@/lib/api/client";
import { requireUser } from "@/lib/auth/session";
import {
  daysFromToday,
  firstName,
  formatDateTime,
  formatDay,
  humanise,
  isOverdue,
  timeAgo,
} from "@/lib/format";
import { EXEC_DASHBOARD_TOPIC } from "@/lib/realtime/topics";

import { Greeting } from "./greeting";

/**
 * The dashboard (blueprint §8.8; prototype 01).
 *
 * For the executive principals the tiles come from one read model computed by
 * Go (`GET /api/v1/dashboard/executive`, `intel.dashboard.read`), kept live by
 * signals on `exec:dashboard`. Everyone else gets the same page built from the
 * work they can read, without the company-wide tiles: a 403 on the executive
 * summary is a permission fact, so those cards are simply not shown.
 *
 * "Needs attention" is a list of facts read from Go's records (open
 * escalations, overdue or ownerless work, a report not yet filed) with a link
 * to where each is dealt with. Nothing is decided here.
 */

export const dynamic = "force-dynamic";

const OPEN_TASK = new Set(["todo", "in_progress", "blocked", "review"]);
const OPEN_PROJECT = new Set(["planning", "active", "paused"]);

async function maybe<T>(promise: Promise<T>): Promise<T | null> {
  try {
    return await promise;
  } catch (error) {
    if (error instanceof ApiError) return null;
    throw error;
  }
}

interface Attention {
  key: string;
  icon: IconName;
  tone: Tone;
  title: string;
  detail: string;
  href: string;
  action: string;
}

function plural(n: number, one: string, many: string): string {
  return `${n} ${n === 1 ? one : many}`;
}

function listNames(names: string[], max = 2): string {
  if (names.length <= max) return names.join(" · ");
  return `${names.slice(0, max).join(" · ")} and ${names.length - max} more`;
}

export default async function DashboardPage() {
  const user = await requireUser();

  let summary: ExecutiveSummary | null = null;
  let summaryError: ApiError | null = null;
  try {
    summary = await getExecutiveSummary();
  } catch (error) {
    if (!(error instanceof ApiError)) throw error;
    summaryError = error;
  }

  const [taskPage, projectPage, escalationPage, todayReport] = await Promise.all([
    maybe(listTasks({ limit: 200 })),
    maybe(listProjects({ limit: 200 })),
    maybe(listEscalations({ status: "open", limit: 100 })),
    user.person_id ? maybe(getTodayReport()) : Promise.resolve(null),
  ]);

  const tasks = taskPage?.items ?? [];
  const projects = projectPage?.items ?? [];
  const open = tasks.filter((task) => OPEN_TASK.has(task.status));
  const overdue = open.filter((task) => isOverdue(task.due_at, task.status));
  const blocked = open.filter((task) => task.blocked);
  const mine = user.person_id ? open.filter((task) => task.assignee_person_id === user.person_id) : [];

  const attention = buildAttention({
    tasks: open,
    overdue,
    blocked,
    projects,
    escalations: escalationPage,
    summary,
    todayReport,
    canReport: Boolean(user.person_id),
  });

  const openEscalations = summary?.work.open_escalations ?? escalationPage?.items.length ?? 0;
  const approvalsPending = summary?.approvals.pending ?? 0;
  const lead = leadSentence(attention.length, overdue.length, approvalsPending, Boolean(summary));

  return (
    <div className="space-y-5">
      <section aria-labelledby="dashboard-hello" className="relative mb-6 mt-[86px] lg:mt-[108px]">
        <Image
          src="/brand/savanna-horizon.png"
          alt=""
          aria-hidden="true"
          width={1040}
          height={205}
          priority
          className="pointer-events-none absolute bottom-[calc(100%-2px)] right-[4%] h-[90px] w-auto lg:h-[122px]"
        />
        <div className="on-navy relative flex flex-wrap items-end justify-between gap-x-8 gap-y-6 rounded-[24px] bg-navy px-[22px] py-[26px] text-white sm:px-10 sm:pb-9 sm:pt-[34px]">
          <div id="dashboard-hello">
            <Greeting firstName={firstName(user.name)} />
            <p className="mt-3 max-w-[58ch] text-[16.5px] text-navy-lead">{lead}</p>
            {summary && (
              <div className="mt-4">
                <SignalRefresh topic={EXEC_DASHBOARD_TOPIC} quiet />
              </div>
            )}
          </div>
          <div className="flex flex-wrap gap-2.5">
            <BlobLink href="/work/tasks?new=task" variant="light">
              <Icon name="plus" />
              New task
            </BlobLink>
            <BlobLink href="/work?new=project" variant="onNavy">
              New project
            </BlobLink>
            <BlobLink href="/daily-reports" variant="onNavy">
              Daily report
            </BlobLink>
          </div>
        </div>
      </section>

      {summary ? (
        <StatGrid label="Key numbers">
          <Stat
            label="People"
            value={summary.people.total}
            hint="Non-system persons on record"
            icon="people"
            iconTone="info"
            foot={
              <>
                <Badge tone="success" dot>
                  {summary.people.by_status.active ?? 0} active
                </Badge>
                <span className="text-muted">{summary.people.by_status.on_leave ?? 0} on leave</span>
              </>
            }
          />
          <OverdueStat count={summary.work.overdue} />
          <EscalationStat count={summary.work.open_escalations} />
          <Stat
            label="Approvals waiting"
            value={summary.approvals.pending}
            tone={summary.approvals.overdue > 0 ? "danger" : "neutral"}
            hint={summary.approvals.pending === 0 ? "Nothing waiting on anyone" : "Waiting for a decision"}
            icon="shieldCheck"
            iconTone="violet"
            foot={
              <span className="text-muted">
                {summary.approvals.overdue} overdue · {summary.approvals.escalated} escalated
              </span>
            }
          />
        </StatGrid>
      ) : (
        <StatGrid label="Key numbers">
          <Stat
            label="Your open tasks"
            value={user.person_id ? mine.length : "—"}
            hint={user.person_id ? "Assigned to you" : "No person record linked"}
            icon="user"
            iconTone="info"
            foot={<ArrowLink href={`/work/tasks${user.person_id ? `?assignee_person_id=${user.person_id}` : ""}`}>My tasks</ArrowLink>}
          />
          <OverdueStat count={overdue.length} />
          <EscalationStat count={openEscalations} capped={escalationPage?.has_more} />
          <Stat
            label="Blocked"
            value={blocked.length}
            tone={blocked.length > 0 ? "warning" : "neutral"}
            hint="Waiting on a blocker"
            icon="alert"
            iconTone={blocked.length > 0 ? "warning" : "success"}
            foot={blocked.length > 0 ? <ArrowLink href="/work/workload">Workload</ArrowLink> : <Badge tone="success" dot>Nothing stuck</Badge>}
          />
        </StatGrid>
      )}

      <div className="grid grid-cols-1 gap-5 xl:grid-cols-[minmax(0,1.75fr)_minmax(0,1fr)]">
        <Card
          title="Needs attention"
          description="Built from today’s records. Clear these to keep work moving."
          actions={
            attention.length > 0 && (
              <span className="grid h-6 min-w-7 place-items-center rounded-full bg-navy px-2 text-[12.5px] font-bold text-white">
                {attention.length}
              </span>
            )
          }
          padded={false}
        >
          {attention.length === 0 ? (
            <div className="px-[22px] pb-[22px]">
              <EmptyState icon="check" title="Nothing needs you right now" className="min-h-[196px]">
                Overdue work, open escalations and ownerless tasks show up here as they happen.
              </EmptyState>
            </div>
          ) : (
            <ul className="px-3 pb-2.5">
              {attention.map((item) => (
                <li key={item.key} className="flex items-center gap-3.5 border-t border-line px-2.5 py-3.5 first:border-t-0">
                  <IconTile icon={item.icon} tone={item.tone} size="lg" />
                  <span className="min-w-0 flex-1">
                    <b className="block text-[15px] font-semibold">{item.title}</b>
                    <span className="mt-0.5 block truncate text-[13.5px] text-muted">{item.detail}</span>
                  </span>
                  <Link href={item.href} className={buttonClass("secondary", "sm")}>
                    {item.action}
                  </Link>
                </li>
              ))}
            </ul>
          )}
        </Card>

        {summary ? <StaffStatus summary={summary} /> : <YourWork tasks={mine} hasPerson={Boolean(user.person_id)} />}
      </div>

      <div className="grid grid-cols-1 gap-5 lg:grid-cols-3">
        <Delivery tasks={open} projects={projects} />
        {summary ? <PendingApprovals summary={summary} /> : <TodayReportCard report={todayReport} hasPerson={Boolean(user.person_id)} />}
        <JumpTo />
      </div>

      {summary ? (
        <RecentActivity audit={summary.recent_audit} viewerId={user.id} />
      ) : (
        summaryError &&
        summaryError.status !== 403 && (
          <ProblemNotice error={summaryError} title="The executive summary could not be loaded" />
        )
      )}
    </div>
  );
}

function leadSentence(things: number, overdue: number, approvals: number, executive: boolean): ReactNode {
  const head =
    things === 0
      ? "Nothing needs your attention right now."
      : `${things === 1 ? "One thing needs" : `${things} things need`} your attention.`;
  const tail = [
    overdue === 0 ? "No work is overdue" : `${plural(overdue, "task is", "tasks are")} overdue`,
    executive ? (approvals === 0 ? "no approvals are waiting" : `${plural(approvals, "approval is", "approvals are")} waiting`) : null,
  ]
    .filter(Boolean)
    .join(" and ");
  return (
    <>
      <b className="font-semibold text-white">{head}</b> {tail}.
    </>
  );
}

function buildAttention({
  tasks,
  overdue,
  blocked,
  projects,
  escalations,
  summary,
  todayReport,
  canReport,
}: {
  tasks: Task[];
  overdue: Task[];
  blocked: Task[];
  projects: Project[];
  escalations: Page<Escalation> | null;
  summary: ExecutiveSummary | null;
  todayReport: DailyReport | null;
  canReport: boolean;
}): Attention[] {
  const items: Attention[] = [];

  const escalationCount = summary?.work.open_escalations ?? escalations?.items.length ?? 0;
  if (escalationCount > 0) {
    const newest = escalations?.items[0];
    items.push({
      key: "escalations",
      icon: "alert",
      tone: "warning",
      title: `${plural(escalationCount, "escalation is", "escalations are")} open`,
      detail: newest?.reason ? `Newest: ${newest.reason} · ${timeAgo(newest.created_at)}` : "Raised by the Atlas sweeps.",
      href: "/work/escalations",
      action: "Review",
    });
  }

  if (overdue.length > 0) {
    items.push({
      key: "overdue",
      icon: "clock",
      tone: "danger",
      title: `${plural(overdue.length, "task is", "tasks are")} past the deadline`,
      detail: listNames(overdue.map((t) => t.title)),
      href: "/work/tasks?overdue=true",
      action: "Open",
    });
  }

  for (const project of projects) {
    if (project.status !== "planning" || !project.due_at) continue;
    const days = daysFromToday(project.due_at);
    if (days === null || days < 0 || days > 7) continue;
    items.push({
      key: `planning-${project.id}`,
      icon: "clock",
      tone: "danger",
      title: `${project.name} is due ${days === 0 ? "today" : `in ${plural(days, "day", "days")}`} and still in Planning`,
      detail: project.owner_person_id ? "Target date is close" : "Target date is close · no owner set",
      href: `/work/projects/${project.id}`,
      action: "Open project",
    });
  }

  if (canReport && !todayReport?.submitted_at && todayReport?.status !== "excused") {
    items.push({
      key: "report",
      icon: "clipboard",
      tone: "violet",
      title: "Your daily report for today isn’t filed",
      detail: "What moved, what’s blocked, what’s next — a few lines is enough.",
      href: "/daily-reports",
      action: "File it",
    });
  }

  const unowned = tasks.filter((task) => !task.assignee_person_id);
  if (unowned.length > 0) {
    items.push({
      key: "unowned-tasks",
      icon: "user",
      tone: "orange",
      title: `${plural(unowned.length, "task has", "tasks have")} no owner`,
      detail: listNames(unowned.map((t) => t.title)),
      href: "/work/tasks?assignee_person_id=unassigned",
      action: "Assign",
    });
  }

  const ownerless = projects.filter((p) => OPEN_PROJECT.has(p.status) && !p.owner_person_id);
  if (ownerless.length > 0) {
    items.push({
      key: "unowned-projects",
      icon: "folder",
      tone: "info",
      title: `${plural(ownerless.length, "project has", "projects have")} no owner`,
      detail: listNames(ownerless.map((p) => p.name)),
      href: ownerless.length === 1 ? `/work/projects/${ownerless[0].id}#details` : "/work",
      action: "Set owners",
    });
  }

  if (blocked.length > 0) {
    items.push({
      key: "blocked",
      icon: "alert",
      tone: "danger",
      title: `${plural(blocked.length, "task is", "tasks are")} blocked`,
      detail: listNames(blocked.map((t) => (t.blocked_reason ? `${t.title}: ${t.blocked_reason}` : t.title)), 1),
      href: "/work/workload",
      action: "See",
    });
  }

  const inReview = tasks.filter((task) => task.status === "review");
  if (inReview.length > 0) {
    items.push({
      key: "review",
      icon: "eye",
      tone: "violet",
      title: `${plural(inReview.length, "task is", "tasks are")} waiting for review`,
      detail: listNames(inReview.map((t) => t.title)),
      href: "/work/tasks",
      action: "Review",
    });
  }

  return items;
}

function OverdueStat({ count }: { count: number }) {
  return (
    <Stat
      label="Overdue work"
      value={count}
      tone={count > 0 ? "danger" : "neutral"}
      hint="Open tasks past their deadline"
      icon="clock"
      iconTone={count > 0 ? "danger" : "success"}
      foot={
        count > 0 ? (
          <ArrowLink href="/work/tasks?overdue=true">See them</ArrowLink>
        ) : (
          <Badge tone="success">
            <Icon name="check" className="h-3.5 w-3.5" />
            All on schedule
          </Badge>
        )
      }
    />
  );
}

function EscalationStat({ count, capped = false }: { count: number; capped?: boolean }) {
  return (
    <Stat
      label="Open escalations"
      value={capped ? `${count}+` : count}
      tone={count > 0 ? "warning" : "neutral"}
      hint="Raised by the Atlas sweeps"
      icon="alert"
      iconTone={count > 0 ? "warning" : "success"}
      foot={
        count > 0 ? (
          <>
            <Badge tone="warning" dot>Needs review</Badge>
            <ArrowLink href="/work/escalations">Open</ArrowLink>
          </>
        ) : (
          <Badge tone="success" dot>None open</Badge>
        )
      }
    />
  );
}

const PEOPLE_STATUSES = [
  { key: "active", label: "Active", color: "#22A55A" },
  { key: "on_leave", label: "On leave", color: "#1A6CF5" },
  { key: "onboarding", label: "Onboarding", color: "#F5A524" },
  { key: "exited", label: "Exited", color: "#A3AEC2" },
] as const;

function StaffStatus({ summary }: { summary: ExecutiveSummary }) {
  const by = summary.people.by_status ?? {};
  return (
    <Card
      title="Staff status"
      description="Everyone on record today."
      actions={<span className="tabular text-[30px] font-semibold leading-none tracking-[-0.02em]">{summary.people.total}</span>}
    >
      <Meter
        className="mb-3.5 mt-1.5"
        label={PEOPLE_STATUSES.map((s) => `${by[s.key] ?? 0} ${s.label.toLowerCase()}`).join(", ")}
        segments={PEOPLE_STATUSES.map((s) => ({ value: by[s.key] ?? 0, color: s.color }))}
      />
      <Legend items={PEOPLE_STATUSES.map((s) => ({ label: s.label, value: by[s.key] ?? 0, color: s.color }))} />
      <div className="pt-1">
        <ArrowLink href="/people">Availability &amp; leave</ArrowLink>
      </div>
    </Card>
  );
}

function YourWork({ tasks, hasPerson }: { tasks: Task[]; hasPerson: boolean }) {
  const sorted = [...tasks].sort((a, b) => (a.due_at ?? "9999") < (b.due_at ?? "9999") ? -1 : 1).slice(0, 5);
  return (
    <Card title="Your work" description="Open tasks assigned to you, soonest first.">
      {sorted.length === 0 ? (
        <EmptyState icon="user" iconTone="info" title={hasPerson ? "Nothing assigned to you" : "No person record linked"}>
          {hasPerson ? "Tasks you own show up here." : "Ask an administrator to link your sign-in to your person record."}
        </EmptyState>
      ) : (
        <ul className="flex flex-col gap-2">
          {sorted.map((task) => (
            <li key={task.id}>
              <Link href={`/work/tasks/${task.id}`} className="flex items-center gap-3 rounded-[12px] border border-line px-3 py-2.5 transition hover:border-[#B9CEFA]">
                <span className="min-w-0 flex-1">
                  <b className="block truncate text-[14.5px] font-semibold">{task.title}</b>
                  <span className={isOverdue(task.due_at, task.status) ? "text-[13px] font-medium text-[#C0352B]" : "text-[13px] text-muted"}>
                    {task.due_at ? formatDay(task.due_at) : "No deadline"}
                  </span>
                </span>
                <Badge tone={statusTone(task.status)} dot>{columnLabel(task.status)}</Badge>
              </Link>
            </li>
          ))}
        </ul>
      )}
    </Card>
  );
}

function Delivery({ tasks, projects }: { tasks: Task[]; projects: Project[] }) {
  const live = projects.filter((p) => OPEN_PROJECT.has(p.status));
  const estimate = Math.round(tasks.reduce((sum, t) => sum + (t.estimate_hours ?? 0), 0));
  const count = (status: string) => tasks.filter((t) => t.status === status).length;
  const blocked = tasks.filter((t) => t.blocked).length;
  return (
    <Card title="Delivery" description="Projects and tasks in flight." actions={<ArrowLink href="/work/tasks">Task board</ArrowLink>}>
      <div className="grid grid-cols-2 gap-3">
        <DeliveryStat label="Projects" value={live.length}>
          {(["active", "planning", "paused"] as const)
            .filter((s) => live.some((p) => p.status === s))
            .map((s) => (
              <Badge key={s} tone={statusTone(s)} dot>
                {live.filter((p) => p.status === s).length} {s}
              </Badge>
            ))}
        </DeliveryStat>
        <DeliveryStat label="Open tasks" value={tasks.length}>
          {(["todo", "in_progress", "review"] as const)
            .filter((s) => count(s) > 0)
            .map((s) => (
              <Badge key={s} tone={statusTone(s)} dot>
                {count(s)} {columnLabel(s).toLowerCase()}
              </Badge>
            ))}
        </DeliveryStat>
        <DeliveryStat label="Estimated load" value={`${estimate}h`}>
          <span className="text-[13px] text-muted">Sum of open estimates</span>
        </DeliveryStat>
        <DeliveryStat label="Blocked" value={blocked}>
          <span className="text-[13px] text-muted">{blocked === 0 ? "Nothing stuck" : "Waiting on blockers"}</span>
        </DeliveryStat>
      </div>
    </Card>
  );
}

function DeliveryStat({ label, value, children }: { label: string; value: ReactNode; children?: ReactNode }) {
  return (
    <div className="rounded-[14px] border border-line px-4 py-3.5">
      <Label>{label}</Label>
      <span className="tabular my-2 block text-[28px] font-semibold leading-none tracking-[-0.02em]">{value}</span>
      <span className="flex flex-wrap gap-1.5">{children}</span>
    </div>
  );
}

function PendingApprovals({ summary }: { summary: ExecutiveSummary }) {
  const byRole = Object.entries(summary.approvals.pending_by_role ?? {});
  return (
    <Card title="Pending approvals" description="Where the queue is sitting, by role.">
      {byRole.length === 0 ? (
        <EmptyState icon="check" title="Nothing waiting on anyone" className="min-h-[196px]">
          New requests appear here, grouped by the role that must decide.
        </EmptyState>
      ) : (
        <Legend items={byRole.map(([role, count]) => ({ label: humanise(role), value: count, color: "#7A5AF8" }))} />
      )}
    </Card>
  );
}

function TodayReportCard({ report, hasPerson }: { report: DailyReport | null; hasPerson: boolean }) {
  const filed = Boolean(report?.submitted_at);
  return (
    <Card title="Today’s report" description="Your daily report, due by the next working morning.">
      <EmptyState
        icon={filed ? "check" : "clipboard"}
        iconTone={filed ? "success" : "violet"}
        title={filed ? "Filed — thank you" : hasPerson ? "Not filed yet" : "No person record linked"}
        className="min-h-[196px]"
        action={
          hasPerson && (
            <ButtonLink href="/daily-reports" size="sm" variant={filed ? "secondary" : "primary"}>
              {filed ? "View or edit" : "File today’s report"}
            </ButtonLink>
          )
        }
      >
        {filed ? `Filed ${timeAgo(report!.submitted_at)}.` : "What moved, what’s blocked and what’s next."}
      </EmptyState>
    </Card>
  );
}

const JUMPS: Array<{ href: string; label: string; icon: IconName; tone: Tone }> = [
  { href: "/work", label: "Projects & tasks", icon: "folder", tone: "info" },
  { href: "/work/escalations", label: "Open escalations", icon: "alert", tone: "warning" },
  { href: "/daily-reports", label: "Daily reports", icon: "clipboard", tone: "violet" },
  { href: "/people", label: "Availability & leave", icon: "calendar", tone: "success" },
];

function JumpTo() {
  return (
    <Card title="Jump to" description="The places you visit most." padded={false}>
      <ul className="px-3 pb-3">
        {JUMPS.map((jump) => (
          <li key={jump.href}>
            <Link href={jump.href} className="flex min-h-14 items-center gap-3 rounded-control px-2.5 py-2 font-medium transition hover:bg-[#F3F6FC]">
              <IconTile icon={jump.icon} tone={jump.tone} className="h-[38px] w-[38px] rounded-[11px]" />
              {jump.label}
              <Icon name="chevronRight" className="ml-auto text-[#9AA6BA]" />
            </Link>
          </li>
        ))}
      </ul>
    </Card>
  );
}

/** `work.escalation.raised` -> "Escalation raised", with a matching marker. */
function describeAction(action: string): { title: string; icon: IconName; tone: Tone } {
  const parts = action.split(".");
  const verb = parts[parts.length - 1] ?? action;
  const entity = parts.length > 1 ? parts[parts.length - 2] : "";
  const title = `${humanise(entity)} ${verb.replace(/_/g, " ")}`.trim();
  if (/(raised|missed|declined|blocker_raised|overdue|revoked|offboarded)$/.test(verb)) {
    return { title, icon: "alert", tone: "warning" };
  }
  if (/(resolved|approved|submitted|verified|completed|accepted|created|added)$/.test(verb)) {
    return { title, icon: "check", tone: "success" };
  }
  return { title, icon: "pulse", tone: "info" };
}

function RecentActivity({ audit, viewerId }: { audit: AuditHighlight[]; viewerId: string }) {
  return (
    <Card title="Recent activity" description="Highlights from the audit trail." padded={false}>
      {audit.length === 0 ? (
        <div className="px-[22px] pb-[22px]">
          <EmptyState icon="pulse" iconTone="info" title="No audit events yet" />
        </div>
      ) : (
        <Table>
          <thead>
            <tr>
              <Th>Event</Th>
              <Th>Entity</Th>
              <Th>By</Th>
              <Th>When</Th>
            </tr>
          </thead>
          <tbody>
            {audit.map((entry) => {
              const described = describeAction(entry.action);
              const isSystem = entry.actor_type !== "user";
              const isViewer = entry.actor_id === viewerId;
              return (
                <tr key={entry.id}>
                  <Td>
                    <div className="flex items-center gap-3">
                      <IconTile icon={described.icon} tone={described.tone} size="sm" round />
                      <span>
                        <b className="block text-[14.5px] font-semibold">{described.title}</b>
                        <span className="font-mono text-xs text-muted">{entry.action}</span>
                      </span>
                    </div>
                  </Td>
                  <Td>{humanise(entry.entity_type.replace(/\./g, " "))}</Td>
                  <Td>
                    <span className="inline-flex items-center gap-2 whitespace-nowrap">
                      <Avatar
                        size="sm"
                        variant={isSystem ? "system" : isViewer ? "me" : "default"}
                        initials={isSystem ? "S" : isViewer ? "You" : "U"}
                        className={isViewer ? "text-[9px]" : undefined}
                      />
                      {isSystem ? (
                        "System"
                      ) : isViewer ? (
                        "You"
                      ) : (
                        <span className="font-mono text-xs" title={entry.actor_id}>
                          user {entry.actor_id.slice(0, 8)}
                        </span>
                      )}
                    </span>
                  </Td>
                  <Td className="whitespace-nowrap text-muted">{formatDateTime(entry.occurred_at)}</Td>
                </tr>
              );
            })}
          </tbody>
        </Table>
      )}
    </Card>
  );
}
