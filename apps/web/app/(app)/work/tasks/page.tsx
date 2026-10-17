import Link from "next/link";

import { Badge, Card, EmptyState, PageHeader, ProblemNotice, priorityTone } from "@/components/ui";
import { listProjects, listTasks, type Task } from "@/lib/api/atlas";
import { ApiError } from "@/lib/api/client";
import { personDirectory } from "@/lib/directory";
import { dueLabel, formatDay, shortId } from "@/lib/format";

import { NewTaskForm } from "@/components/work/new-task-form";

import { TaskFilters } from "./task-filters";

/**
 * Task board (blueprint §8.2 key screen: "project board (Kanban + list)").
 *
 * A Kanban of the task lifecycle, built from one keyset page of
 * `GET /api/v1/work/tasks` (gated by `work.task.read`). Columns are the status
 * enum; the filters are query params so the board stays a server component.
 * Open a card to change status, raise a blocker or attach evidence.
 */

export const dynamic = "force-dynamic";

const BOARD_STATUSES = ["todo", "in_progress", "blocked", "review", "done"] as const;

export default async function TaskBoardPage({
  searchParams,
}: {
  searchParams: Promise<{
    project_id?: string;
    assignee_person_id?: string;
    overdue?: string;
  }>;
}) {
  const {
    project_id: projectId,
    assignee_person_id: assigneeId,
    overdue,
  } = await searchParams;

  let tasks: Task[] = [];
  let error: ApiError | null = null;
  try {
    const page = await listTasks({
      project_id: projectId || undefined,
      assignee_person_id: assigneeId || undefined,
      overdue: overdue === "true" ? "true" : undefined,
      limit: 200,
    });
    tasks = page.items;
  } catch (caught) {
    if (!(caught instanceof ApiError)) throw caught;
    error = caught;
  }

  const projects = error
    ? []
    : await listProjects({ limit: 200 })
        .then((page) => page.items.map((p) => ({ id: p.id, name: p.name })))
        .catch(() => []);

  const names = error ? new Map<string, string>() : await personDirectory();

  const columns = BOARD_STATUSES.map((status) => ({
    status,
    tasks: tasks.filter((task) => task.status === status),
  }));
  const cancelled = tasks.filter((task) => task.status === "cancelled").length;

  return (
    <div className="space-y-6">
      <PageHeader
        title="Task board"
        description={
          <>
            {tasks.length} task{tasks.length === 1 ? "" : "s"}
            {overdue === "true" ? " · overdue only" : ""}
            {cancelled > 0 ? ` · ${cancelled} cancelled hidden` : ""}. Open a card
            to change status, raise a blocker or attach evidence.
          </>
        }
        actions={
          <Link
            href="/work"
            className="rounded-md border border-slate-300 bg-white px-3 py-2 text-sm font-medium text-slate-700 transition hover:bg-slate-50"
          >
            Projects
          </Link>
        }
      />

      {error ? (
        <ProblemNotice error={error} title="The task board could not be loaded" />
      ) : (
        <>
          <TaskFilters
            projects={projects}
            projectId={projectId ?? ""}
            assigneeId={assigneeId ?? ""}
            overdue={overdue === "true"}
          />

          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-5">
            {columns.map((column) => (
              <section key={column.status} className="flex flex-col gap-2">
                <h2 className="flex items-center justify-between text-xs font-medium uppercase tracking-wide text-slate-500">
                  <span>{column.status.replace(/_/g, " ")}</span>
                  <span className="tabular-nums">{column.tasks.length}</span>
                </h2>
                {column.tasks.length === 0 ? (
                  <p className="rounded-lg border border-dashed border-slate-200 bg-slate-50 px-3 py-6 text-center text-xs text-slate-400">
                    Nothing here
                  </p>
                ) : (
                  <ul className="space-y-2">
                    {column.tasks.map((task) => (
                      <li key={task.id}>
                        <TaskCard
                          task={task}
                          assignee={
                            task.assignee_person_id
                              ? names.get(task.assignee_person_id)
                              : undefined
                          }
                        />
                      </li>
                    ))}
                  </ul>
                )}
              </section>
            ))}
          </div>

          <Card
            title="New task"
            description="A task belongs to a project and carries a deadline and owner."
          >
            <NewTaskForm projects={projects} defaultProjectId={projectId ?? ""} />
          </Card>
        </>
      )}
    </div>
  );
}

function TaskCard({ task, assignee }: { task: Task; assignee?: string }) {
  const overdue =
    task.due_at != null &&
    new Date(task.due_at).getTime() < Date.now() &&
    task.status !== "done" &&
    task.status !== "cancelled";

  return (
    <Link
      href={`/work/tasks/${task.id}`}
      className="block rounded-lg border border-slate-200 bg-white p-3 shadow-sm transition hover:border-atlas/40 hover:shadow"
    >
      <div className="flex items-start justify-between gap-2">
        <span className="text-sm font-medium text-slate-900">{task.title}</span>
        <Badge tone={priorityTone(task.priority)}>{task.priority}</Badge>
      </div>
      <div className="mt-2 flex flex-wrap items-center gap-2 text-xs text-slate-500">
        <span>
          {assignee ?? (task.assignee_person_id ? shortId(task.assignee_person_id) : "Unassigned")}
        </span>
        {task.blocked && <Badge tone="warning">blocked</Badge>}
        {task.due_at && (
          <span className={overdue ? "font-medium text-red-600" : ""}>
            {overdue ? dueLabel(task.due_at) : formatDay(task.due_at)}
          </span>
        )}
      </div>
    </Link>
  );
}
