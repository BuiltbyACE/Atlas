import Link from "next/link";

import { ButtonLink } from "@/components/form";
import { Icon } from "@/components/icons";
import { SignalRefresh } from "@/components/signal-refresh";
import {
  Badge,
  Card,
  PageHeader,
  ProblemNotice,
  Tag,
  priorityTone,
  statusTone,
} from "@/components/ui";
import { columnLabel } from "@/components/work/board";
import { EvidencePanel } from "@/components/work/evidence-panel";
import {
  getProject,
  getTask,
  listBlockers,
  listTaskEvidence,
  type Blocker,
  type Evidence,
  type Project,
  type Task,
} from "@/lib/api/atlas";
import { ApiError } from "@/lib/api/client";
import { requireUser } from "@/lib/auth/session";
import { personDirectory, personName, rosterOf } from "@/lib/directory";
import { dueLabel, formatDateTime, formatDay, humanise, isOverdue } from "@/lib/format";
import { projectTopic } from "@/lib/realtime/topics";

import { BlockerPanel } from "./blocker-panel";
import { TaskEditor } from "./task-editor";

/**
 * Task detail. Reads the task, its project, blockers and evidence from Go (each
 * gated by its own permission); writes go through the client panels, which
 * re-fetch this route after every change.
 *
 * Live updates: signals on `work:project:{id}` are identifiers only, so
 * `SignalRefresh` calls `router.refresh()` and Go authorises the refetch.
 */

export const dynamic = "force-dynamic";

/** Resolve a read to a fallback when Go refuses (403) or it is missing (404). */
async function safe<T>(promise: Promise<T>, fallback: T): Promise<T> {
  try {
    return await promise;
  } catch (error) {
    if (error instanceof ApiError) return fallback;
    throw error;
  }
}

export default async function TaskDetailPage({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = await params;
  const user = await requireUser();

  let task: Task;
  try {
    task = await getTask(id);
  } catch (error) {
    if (!(error instanceof ApiError)) throw error;
    return (
      <div className="space-y-6">
        <PageHeader title="Task" />
        <ProblemNotice
          error={error}
          title={
            error.status === 404
              ? "That task does not exist"
              : "The task could not be loaded"
          }
        />
      </div>
    );
  }

  const [project, blockers, evidence, names] = await Promise.all([
    safe<Project | null>(getProject(task.project_id), null),
    safe<Blocker[]>(listBlockers(id, true), []),
    safe<Evidence[]>(listTaskEvidence(id), []),
    personDirectory(),
  ]);

  const due = dueLabel(task.due_at);
  const overdue = isOverdue(task.due_at, task.status);

  return (
    <div className="space-y-6">
      <PageHeader
        title={task.title}
        eyebrow={
          <span className="flex flex-wrap items-center gap-2">
            <Badge tone={statusTone(task.status)} dot>
              {columnLabel(task.status)}
            </Badge>
            <Badge tone={priorityTone(task.priority)}>{humanise(task.priority)} priority</Badge>
            {task.blocked && <Badge tone="danger">Blocked</Badge>}
            {task.ref && <Tag>{task.ref}</Tag>}
          </span>
        }
        description={
          <span className="flex flex-wrap items-center gap-x-2 gap-y-1">
            <Link
              href={`/work/projects/${task.project_id}`}
              className="font-semibold text-atlas-ink hover:underline"
            >
              {project ? project.name : "Project"}
            </Link>
            <span aria-hidden="true">·</span>
            <span>{task.assignee_person_id ? personName(names, task.assignee_person_id) : "Unassigned"}</span>
            <span aria-hidden="true">·</span>
            <span className={overdue ? "font-semibold text-[#C0352B]" : undefined}>
              {task.due_at ? `${formatDay(task.due_at)}${due ? ` (${due})` : ""}` : "No deadline"}
            </span>
            <SignalRefresh topic={projectTopic(task.project_id)} quiet />
          </span>
        }
        actions={
          <ButtonLink href={`/work/tasks?project_id=${task.project_id}`} variant="secondary">
            <Icon name="board" />
            Back to the board
          </ButtonLink>
        }
      />

      <div className="grid grid-cols-1 items-start gap-5 xl:grid-cols-[minmax(0,1.5fr)_minmax(0,1fr)]">
        <div className="flex flex-col gap-5">
          <Card title="What needs doing">
            {task.description ? (
              <p className="whitespace-pre-wrap text-[15px] leading-relaxed text-night">
                {task.description}
              </p>
            ) : (
              <p className="text-sm text-muted">No description yet. Add one in “Edit task”.</p>
            )}
          </Card>
          <BlockerPanel taskId={task.id} blockers={blockers} />
          <EvidencePanel taskId={task.id} evidence={evidence} />
        </div>
        <TaskEditor task={task} people={rosterOf(names)} me={user.person_id} />
      </div>

      <p className="text-xs text-[#7A879C]">
        Created {formatDateTime(task.created_at)} · updated {formatDateTime(task.updated_at)} ·
        version {task.version}
      </p>
    </div>
  );
}
