import { ButtonLink } from "@/components/form";
import { Icon } from "@/components/icons";
import { PageHeader, ProblemNotice } from "@/components/ui";
import { listProjects, listTasks, type Task } from "@/lib/api/atlas";
import { ApiError } from "@/lib/api/client";
import { requireUser } from "@/lib/auth/session";
import { personDirectory, rosterOf } from "@/lib/directory";

import { TaskBoard } from "./task-board";

/**
 * Task board (blueprint §8.2 key screen: "project board (Kanban + list)").
 *
 * A Kanban of the task lifecycle, built from one keyset page of
 * `GET /api/v1/work/tasks` (gated by `work.task.read`). The project, owner and
 * overdue filters are query params Go applies; "unassigned" has no Go filter,
 * so the board narrows its page to tasks without an owner. Moves, blockers and
 * evidence are written from `TaskBoard`.
 */

export const dynamic = "force-dynamic";

export default async function TaskBoardPage({
  searchParams,
}: {
  searchParams: Promise<{
    project_id?: string;
    assignee_person_id?: string;
    overdue?: string;
    new?: string;
  }>;
}) {
  const {
    project_id: projectId = "",
    assignee_person_id: assigneeId = "",
    overdue,
    new: create,
  } = await searchParams;
  const user = await requireUser();

  let tasks: Task[] = [];
  let truncated = false;
  let error: ApiError | null = null;
  try {
    const page = await listTasks({
      project_id: projectId || undefined,
      assignee_person_id:
        assigneeId && assigneeId !== "unassigned" ? assigneeId : undefined,
      overdue: overdue === "true" ? "true" : undefined,
      limit: 200,
    });
    tasks = page.items;
    truncated = page.has_more;
  } catch (caught) {
    if (!(caught instanceof ApiError)) throw caught;
    error = caught;
  }

  const [projects, directory] = error
    ? [[], new Map<string, string>()]
    : await Promise.all([
        listProjects({ limit: 200 })
          .then((page) => page.items.map((p) => ({ id: p.id, name: p.name })))
          .catch(() => []),
        personDirectory(),
      ]);

  const onBoard = tasks.filter((task) => task.status !== "cancelled");
  const cancelled = tasks.length - onBoard.length;

  return (
    <div className="space-y-6">
      <PageHeader
        title="Task board"
        description={
          error ? undefined : (
            <>
              {onBoard.length} task{onBoard.length === 1 ? "" : "s"}
              {truncated ? " (the first 200 — narrow the filters to see the rest)" : ""}.
              Drag a card between columns, or open it to change status, raise a blocker or
              attach evidence.
            </>
          )
        }
        actions={
          <ButtonLink href="/work" variant="secondary">
            <Icon name="folder" />
            Projects
          </ButtonLink>
        }
      />

      {error ? (
        <ProblemNotice
          error={error}
          title={
            error.status === 403
              ? "You do not have permission to view tasks"
              : "The task board could not be loaded"
          }
        />
      ) : (
        <div>
          <TaskBoard
            initialTasks={onBoard}
            projects={projects}
            people={rosterOf(directory)}
            me={user.person_id}
            filters={{
              projectId,
              assigneeId,
              overdue: overdue === "true",
            }}
            cancelled={cancelled}
            openNew={create === "task"}
          />
        </div>
      )}
    </div>
  );
}
