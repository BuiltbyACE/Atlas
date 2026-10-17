import Link from "next/link";

import { Badge, Card, EmptyState, PageHeader, ProblemNotice, Stat, priorityTone, statusTone } from "@/components/ui";
import { listTasks, type Task } from "@/lib/api/atlas";
import { ApiError } from "@/lib/api/client";
import { personDirectory } from "@/lib/directory";
import { dueLabel, formatDay, humanise, shortId } from "@/lib/format";

/**
 * Workload view (blueprint §8.2 key screen: "workload view (who has what, by due
 * date)"). Derived from the task listing - there is no dedicated endpoint - so
 * it groups open tasks by assignee and sorts each person's load by due date.
 * Overdue and blocked work is called out, because that is what needs action.
 */

export const dynamic = "force-dynamic";

const TERMINAL = new Set(["done", "cancelled"]);

export default async function WorkloadPage() {
  let tasks: Task[] = [];
  let error: ApiError | null = null;
  try {
    const page = await listTasks({ limit: 200 });
    tasks = page.items;
  } catch (caught) {
    if (!(caught instanceof ApiError)) throw caught;
    error = caught;
  }

  const names = error ? new Map<string, string>() : await personDirectory();

  const open = tasks.filter((task) => !TERMINAL.has(task.status));
  const overdue = open.filter(
    (task) => task.due_at != null && new Date(task.due_at).getTime() < Date.now(),
  );
  const blocked = open.filter((task) => task.blocked);
  const estimate = open.reduce((sum, task) => sum + (task.estimate_hours ?? 0), 0);

  // Open work is grouped per assignee by `groupByAssignee` below.

  return (
    <div className="space-y-6">
      <PageHeader
        title="Workload"
        description="Who is carrying what, by due date. Open tasks across every project."
        actions={
          <Link
            href="/work/tasks"
            className="rounded-md border border-slate-300 bg-white px-3 py-2 text-sm font-medium text-slate-700 transition hover:bg-slate-50"
          >
            Task board
          </Link>
        }
      />

      {error ? (
        <ProblemNotice error={error} title="Workload could not be loaded" />
      ) : (
        <>
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
            <Stat label="Open tasks" value={open.length} hint={`${tasks.length} total`} />
            <Stat
              label="Overdue"
              value={overdue.length}
              tone={overdue.length > 0 ? "danger" : "neutral"}
            />
            <Stat
              label="Blocked"
              value={blocked.length}
              tone={blocked.length > 0 ? "warning" : "neutral"}
            />
            <Stat
              label="Estimated load"
              value={`${Math.round(estimate)}h`}
              hint="Sum of open estimates"
            />
          </div>

          {open.length === 0 ? (
            <EmptyState>No open work. Everything is done or cancelled.</EmptyState>
          ) : (
            <div className="space-y-4">
              {groupByAssignee(open).map(([personId, personTasks]) => (
                <WorkloadCard
                  key={personId}
                  name={
                    personId === "unassigned"
                      ? "Unassigned"
                      : (names.get(personId) ?? shortId(personId))
                  }
                  tasks={personTasks}
                />
              ))}
            </div>
          )}
        </>
      )}
    </div>
  );
}

function groupByAssignee(tasks: Task[]): Array<[string, Task[]]> {
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

function WorkloadCard({ name, tasks }: { name: string; tasks: Task[] }) {
  const overdueCount = tasks.filter(
    (task) => task.due_at != null && new Date(task.due_at).getTime() < Date.now(),
  ).length;
  const hours = Math.round(
    tasks.reduce((sum, task) => sum + (task.estimate_hours ?? 0), 0),
  );

  return (
    <Card
      title={name}
      description={`${tasks.length} open · ${hours}h estimated${
        overdueCount > 0 ? ` · ${overdueCount} overdue` : ""
      }`}
      padded={false}
    >
      <ul className="divide-y divide-slate-100">
        {tasks.map((task) => {
          const due = dueLabel(task.due_at);
          const isOverdue = due?.includes("overdue") ?? false;
          return (
            <li key={task.id} className="flex flex-wrap items-center justify-between gap-2 px-5 py-2.5">
              <div className="min-w-0">
                <Link href={`/work/tasks/${task.id}`} className="text-sm text-slate-800 hover:text-atlas">
                  {task.title}
                </Link>
                <div className="mt-0.5 flex items-center gap-2 text-xs text-slate-500">
                  <Badge tone={statusTone(task.status)}>{humanise(task.status)}</Badge>
                  <Badge tone={priorityTone(task.priority)}>{task.priority}</Badge>
                  {task.blocked && <Badge tone="warning">blocked</Badge>}
                </div>
              </div>
              <span
                className={
                  "whitespace-nowrap text-xs " +
                  (isOverdue ? "font-medium text-red-600" : "text-slate-500")
                }
              >
                {due ?? formatDay(task.due_at)}
              </span>
            </li>
          );
        })}
      </ul>
    </Card>
  );
}
