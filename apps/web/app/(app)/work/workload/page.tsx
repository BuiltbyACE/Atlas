import Link from "next/link";

import { ButtonLink } from "@/components/form";
import { Icon } from "@/components/icons";
import {
  Avatar,
  Badge,
  Card,
  DateBlock,
  EmptyState,
  PageHeader,
  ProblemNotice,
  Stat,
  StatGrid,
  cx,
  priorityTone,
  statusTone,
} from "@/components/ui";
import { columnLabel } from "@/components/work/board";
import { listProjects, listTasks, type Project, type Task } from "@/lib/api/atlas";
import { ApiError } from "@/lib/api/client";
import { requireUser } from "@/lib/auth/session";
import { personDirectory, personName, rosterOf } from "@/lib/directory";
import {
  addDays,
  dayParts,
  dueLabel,
  formatShortDay,
  humanise,
  initials,
  isOverdue,
  today,
} from "@/lib/format";

import { AssignButton } from "./assign-button";

/**
 * Workload view (blueprint §8.2 key screen: "workload view (who has what, by due
 * date)"; prototype 04). Derived from the task listing - there is no dedicated
 * endpoint - so it groups open tasks by owner and sorts each person's load by
 * due date, and lays the next fortnight's deadlines (tasks and project targets)
 * on a strip. Overdue and blocked work is called out, because that is what
 * needs action.
 */

export const dynamic = "force-dynamic";

const TERMINAL = new Set(["done", "cancelled"]);
const OPEN_PROJECT = new Set(["planning", "active", "paused"]);
const HORIZON_DAYS = 14;

export default async function WorkloadPage() {
  const user = await requireUser();

  let tasks: Task[] = [];
  let error: ApiError | null = null;
  try {
    tasks = (await listTasks({ limit: 200 })).items;
  } catch (caught) {
    if (!(caught instanceof ApiError)) throw caught;
    error = caught;
  }

  const [projects, names] = error
    ? [[] as Project[], new Map<string, string>()]
    : await Promise.all([
        listProjects({ limit: 200 })
          .then((page) => page.items)
          .catch(() => [] as Project[]),
        personDirectory(),
      ]);
  const people = rosterOf(names);

  const open = tasks.filter((task) => !TERMINAL.has(task.status));
  const overdue = open.filter((task) => isOverdue(task.due_at, task.status));
  const blocked = open.filter((task) => task.blocked);
  const estimate = open.reduce((sum, task) => sum + (task.estimate_hours ?? 0), 0);
  const unassigned = open.filter((task) => !task.assignee_person_id);
  const unassignedHours = unassigned.reduce((sum, task) => sum + (task.estimate_hours ?? 0), 0);
  const byStatus = (status: string) => open.filter((task) => task.status === status).length;

  return (
    <div className="space-y-6">
      <PageHeader
        title="Workload"
        description="Who is carrying what, by due date. Open tasks across every project."
        actions={
          <ButtonLink href="/work/tasks" variant="secondary">
            <Icon name="board" />
            Task board
          </ButtonLink>
        }
      />

      {error ? (
        <ProblemNotice error={error} title="Workload could not be loaded" />
      ) : (
        <>
          <StatGrid label="Workload totals">
            <Stat
              label="Open tasks"
              value={open.length}
              hint={`${tasks.length} total on the board`}
              icon="board"
              iconTone="info"
              foot={
                <span className="text-muted">
                  {byStatus("todo")} to do · {byStatus("in_progress")} in progress · {byStatus("review")} in review
                </span>
              }
            />
            <Stat
              label="Overdue"
              value={overdue.length}
              tone={overdue.length > 0 ? "danger" : "neutral"}
              hint="Past their deadline"
              icon="clock"
              iconTone={overdue.length > 0 ? "danger" : "success"}
              foot={
                overdue.length > 0 ? (
                  <Link href="/work/tasks?overdue=true" className="font-semibold text-atlas-ink hover:underline">
                    See overdue tasks
                  </Link>
                ) : (
                  <Badge tone="success" dot>Nothing late</Badge>
                )
              }
            />
            <Stat
              label="Blocked"
              value={blocked.length}
              tone={blocked.length > 0 ? "warning" : "neutral"}
              hint="Waiting on a blocker"
              icon="alert"
              iconTone={blocked.length > 0 ? "warning" : "success"}
              foot={
                blocked.length > 0 ? (
                  <span className="text-muted">Open each to see who can unblock it</span>
                ) : (
                  <Badge tone="success" dot>Nothing stuck</Badge>
                )
              }
            />
            <Stat
              label="Estimated load"
              value={`${Math.round(estimate)}h`}
              hint="Sum of open estimates"
              icon="bars"
              iconTone="warning"
              foot={
                unassignedHours > 0 ? (
                  <Badge tone="warning" dot>
                    {Math.round(unassignedHours) === Math.round(estimate)
                      ? "All of it unassigned"
                      : `${Math.round(unassignedHours)}h unassigned`}
                  </Badge>
                ) : (
                  <span className="text-muted">Every estimate has an owner</span>
                )
              }
            />
          </StatGrid>

          <div className="grid grid-cols-1 items-start gap-5 xl:grid-cols-[minmax(0,1.65fr)_minmax(0,1fr)]">
            <Card
              title="By person"
              description="Open work grouped by owner, heaviest load first."
              padded={false}
            >
              {open.length === 0 ? (
                <div className="px-[22px] pb-[22px]">
                  <EmptyState icon="check" title="No open work">
                    Everything is done or cancelled.
                  </EmptyState>
                </div>
              ) : (
                groupByOwner(open).map(([personId, personTasks]) => (
                  <PersonLoad
                    key={personId}
                    personId={personId}
                    name={personId === "unassigned" ? "Unassigned" : personName(names, personId)}
                    isMe={personId === user.person_id}
                    tasks={personTasks}
                    people={people}
                    me={user.person_id}
                  />
                ))
              )}
            </Card>

            <NextFortnight tasks={open} projects={projects} names={names} />
          </div>
        </>
      )}
    </div>
  );
}

function groupByOwner(tasks: Task[]): Array<[string, Task[]]> {
  const groups = new Map<string, Task[]>();
  for (const task of tasks) {
    const key = task.assignee_person_id ?? "unassigned";
    const list = groups.get(key) ?? [];
    list.push(task);
    groups.set(key, list);
  }
  for (const list of groups.values()) {
    list.sort((a, b) => {
      const at = a.due_at ? new Date(a.due_at).getTime() : Number.MAX_SAFE_INTEGER;
      const bt = b.due_at ? new Date(b.due_at).getTime() : Number.MAX_SAFE_INTEGER;
      return at - bt;
    });
  }
  return [...groups.entries()].sort((a, b) => b[1].length - a[1].length);
}

function PersonLoad({
  personId,
  name,
  isMe,
  tasks,
  people,
  me,
}: {
  personId: string;
  name: string;
  isMe: boolean;
  tasks: Task[];
  people: ReturnType<typeof rosterOf>;
  me?: string;
}) {
  const unassigned = personId === "unassigned";
  const overdueCount = tasks.filter((task) => isOverdue(task.due_at, task.status)).length;
  const hours = Math.round(tasks.reduce((sum, task) => sum + (task.estimate_hours ?? 0), 0));

  return (
    <div className="border-t border-line">
      <div className="flex flex-wrap items-center gap-3.5 bg-[#FAFBFC] px-[22px] py-4">
        {unassigned ? (
          <Avatar initials={<Icon name="user" className="h-4 w-4" />} variant="empty" size="lg" />
        ) : (
          <Avatar initials={initials(name)} variant={isMe ? "me" : "default"} size="lg" />
        )}
        <span>
          <b className="block text-base font-semibold">
            {name}
            {isMe && <span className="font-normal text-muted"> (you)</span>}
          </b>
          <span className="block text-[13.5px] text-muted">
            {tasks.length} open · {hours}h estimated
          </span>
        </span>
        <span className="ml-auto flex items-center gap-2.5">
          {unassigned ? (
            <Badge tone="warning" dot>Needs owners</Badge>
          ) : overdueCount > 0 ? (
            <Badge tone="danger" dot>{overdueCount} overdue</Badge>
          ) : null}
        </span>
      </div>
      <ul>
        {tasks.map((task) => {
          const due = dueLabel(task.due_at);
          const late = isOverdue(task.due_at, task.status);
          return (
            <li
              key={task.id}
              className="grid grid-cols-1 items-center gap-x-[18px] gap-y-2 border-t border-line px-[22px] py-4 sm:grid-cols-[minmax(0,1fr)_auto_auto] sm:pl-20"
            >
              <div className="min-w-0">
                <Link href={`/work/tasks/${task.id}`} className="mb-2 block text-[15.5px] font-semibold hover:text-atlas-ink">
                  {task.title}
                </Link>
                <span className="flex flex-wrap gap-1.5">
                  <Badge tone={statusTone(task.status)} dot>{columnLabel(task.status)}</Badge>
                  <Badge tone={priorityTone(task.priority)}>{humanise(task.priority)}</Badge>
                  {task.blocked && <Badge tone="danger">Blocked</Badge>}
                </span>
              </div>
              <div className="whitespace-nowrap text-[13.5px] text-muted sm:text-right">
                <b className={cx("block text-[14.5px] font-semibold", late ? "text-[#C0352B]" : "text-night")}>
                  {task.due_at ? formatShortDay(task.due_at) : "No deadline"}
                </b>
                {task.due_at ? due : "set one to track it"}
              </div>
              {unassigned ? <AssignButton task={task} people={people} me={me} /> : <span className="hidden sm:block" />}
            </li>
          );
        })}
      </ul>
    </div>
  );
}

interface AgendaItem {
  key: string;
  day: string | null;
  title: string;
  href: string;
  detail: string;
  kind: "task" | "project";
}

function NextFortnight({
  tasks,
  projects,
  names,
}: {
  tasks: Task[];
  projects: Project[];
  names: Map<string, string>;
}) {
  const start = today();
  const end = addDays(start, HORIZON_DAYS - 1);
  const days = Array.from({ length: HORIZON_DAYS }, (_, i) => addDays(start, i));
  const openProjects = projects.filter((project) => OPEN_PROJECT.has(project.status));

  const dated: AgendaItem[] = [
    ...tasks
      .filter((task) => task.due_at && task.due_at.slice(0, 10) <= end)
      .map<AgendaItem>((task) => ({
        key: `t-${task.id}`,
        day: task.due_at!.slice(0, 10),
        title: task.title,
        href: `/work/tasks/${task.id}`,
        kind: "task",
        detail: `Task ${isOverdue(task.due_at, task.status) ? "overdue" : "due"} · ${humanise(task.priority)} · ${
          task.assignee_person_id ? personName(names, task.assignee_person_id) : "unassigned"
        }`,
      })),
    ...openProjects
      .filter((project) => project.due_at && project.due_at.slice(0, 10) <= end)
      .map<AgendaItem>((project) => ({
        key: `p-${project.id}`,
        day: project.due_at!.slice(0, 10),
        title: project.name,
        href: `/work/projects/${project.id}`,
        kind: "project",
        detail: `Project target · ${humanise(project.status)} · ${
          project.owner_person_id ? personName(names, project.owner_person_id) : "no owner"
        }`,
      })),
  ].sort((a, b) => (a.day! < b.day! ? -1 : 1));

  const undated: AgendaItem[] = [
    ...tasks
      .filter((task) => !task.due_at)
      .map<AgendaItem>((task) => ({
        key: `t-${task.id}`,
        day: null,
        title: task.title,
        href: `/work/tasks/${task.id}`,
        kind: "task",
        detail: `Task · ${columnLabel(task.status).toLowerCase()} · no deadline`,
      })),
    ...openProjects
      .filter((project) => !project.due_at)
      .map<AgendaItem>((project) => ({
        key: `p-${project.id}`,
        day: null,
        title: project.name,
        href: `/work/projects/${project.id}`,
        kind: "project",
        detail: `Project · ${humanise(project.status).toLowerCase()} · no target date`,
      })),
  ];

  const taskDays = new Set(dated.filter((i) => i.kind === "task").map((i) => i.day));
  const projectDays = new Set(dated.filter((i) => i.kind === "project").map((i) => i.day));
  const late = dated.filter((item) => item.day! < start);
  const upcoming = dated.filter((item) => item.day! >= start);

  return (
    <Card
      title="Next 14 days"
      description={`Deadlines from today, ${formatShortDay(start)}, to ${formatShortDay(end)}.`}
      padded={false}
    >
      <div className="grid grid-cols-7 gap-1.5 px-[22px] pb-2.5">
        {days.map((day) => {
          const { weekday, day: dom } = dayParts(day);
          const weekend = weekday === "Sat" || weekday === "Sun";
          const hasTask = taskDays.has(day);
          const hasProject = projectDays.has(day);
          return (
            <div
              key={day}
              title={[hasTask && "Task due", hasProject && "Project target"].filter(Boolean).join(" · ") || undefined}
              className={cx(
                "flex min-h-[68px] flex-col items-center gap-0.5 rounded-control border px-0.5 py-2 text-center",
                hasTask
                  ? "border-[#FFCFAA] bg-[#FFF1E6]"
                  : hasProject
                    ? "border-[#BFD3FF] bg-[#E8F0FF]"
                    : weekend
                      ? "border-line bg-[#F5F6F9]"
                      : "border-line bg-white",
                day === start && "border-navy shadow-[inset_0_0_0_1px_#00204D]",
              )}
            >
              <small className="text-[10.5px] uppercase tracking-[0.06em] text-muted">{weekday}</small>
              <b className="text-base font-semibold leading-tight">{dom}</b>
              <span className="mt-0.5 flex gap-0.5">
                {hasTask && <i aria-hidden="true" className="h-2 w-2 rounded-full bg-[#E8590C]" />}
                {hasProject && <i aria-hidden="true" className="h-2 w-2 rounded-full bg-atlas" />}
              </span>
            </div>
          );
        })}
      </div>
      <div className="flex flex-wrap gap-4 px-[22px] pb-4 pt-1 text-[13px] text-muted">
        <Key color="#E8590C">Task due</Key>
        <Key color="#1A6CF5">Project target</Key>
        <Key color="#00204D">Today</Key>
      </div>

      {dated.length + undated.length === 0 ? (
        <div className="px-[22px] pb-[22px]">
          <EmptyState icon="calendar" title="Nothing on the calendar">
            No open task or project has a deadline in the next fortnight.
          </EmptyState>
        </div>
      ) : (
        <div className="flex flex-col gap-4 px-[22px] pb-[22px]">
          {late.length > 0 && (
            <AgendaGroup title="Already past" tone="danger" items={late} />
          )}
          {upcoming.length > 0 && (
            <AgendaGroup title={late.length > 0 ? "Coming up" : undefined} items={upcoming} />
          )}
          {undated.length > 0 && (
            <AgendaGroup
              title="No date yet"
              items={undated.slice(0, 6)}
              more={undated.length > 6 ? undated.length - 6 : 0}
            />
          )}
        </div>
      )}
    </Card>
  );
}

function AgendaGroup({
  title,
  tone,
  items,
  more = 0,
}: {
  title?: string;
  tone?: "danger";
  items: AgendaItem[];
  more?: number;
}) {
  return (
    <section>
      {title && (
        <h3
          className={cx(
            "mb-2 text-xs font-semibold uppercase tracking-[0.09em]",
            tone === "danger" ? "text-[#C0352B]" : "text-muted",
          )}
        >
          {title}
        </h3>
      )}
      <ul className="flex flex-col gap-2.5">
        {items.map((item) => {
          const parts = item.day ? dayParts(item.day) : null;
          return (
            <li key={item.key}>
              <Link
                href={item.href}
                className="flex items-center gap-3 rounded-[14px] border border-line px-3 py-2.5 transition hover:border-[#B9CEFA]"
              >
                {parts ? (
                  <DateBlock top={parts.weekday} day={parts.day} />
                ) : (
                  <DateBlock top="No" day="—" muted />
                )}
                <span className="min-w-0">
                  <b className="block truncate text-[14.5px] font-semibold">{item.title}</b>
                  <span className="block truncate text-[13px] text-muted">{item.detail}</span>
                </span>
              </Link>
            </li>
          );
        })}
      </ul>
      {more > 0 && <p className="mt-2 text-[13px] text-muted">And {more} more without a date.</p>}
    </section>
  );
}

function Key({ color, children }: { color: string; children: string }) {
  return (
    <span className="inline-flex items-center gap-1.5">
      <i aria-hidden="true" className="h-2 w-2 rounded-full" style={{ background: color }} />
      {children}
    </span>
  );
}
