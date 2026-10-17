import Link from "next/link";

import { SignalRefresh } from "@/components/signal-refresh";
import { Badge, Card, EmptyState, PageHeader, ProblemNotice, priorityTone, statusTone } from "@/components/ui";
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
import { personDirectory } from "@/lib/directory";
import { dueLabel, formatDateTime, humanise, shortId } from "@/lib/format";
import { projectTopic } from "@/lib/realtime/topics";

import { BlockerPanel } from "./blocker-panel";
import { EvidencePanel } from "@/components/work/evidence-panel";
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

  const assignee = task.assignee_person_id
    ? (names.get(task.assignee_person_id) ?? shortId(task.assignee_person_id))
    : "Unassigned";
  const due = dueLabel(task.due_at);

  return (
    <div className="space-y-6">
      <PageHeader
        title={task.title}
        description={
          <span className="flex flex-wrap items-center gap-2">
            <Badge tone={statusTone(task.status)}>{humanise(task.status)}</Badge>
            <Badge tone={priorityTone(task.priority)}>{task.priority} priority</Badge>
            {task.blocked && <Badge tone="warning">blocked</Badge>}
            <Link
              href={`/work/projects/${task.project_id}`}
              className="text-atlas hover:underline"
            >
              {project ? project.name : shortId(task.project_id)}
            </Link>
            <span className="text-slate-400">·</span>
            <span>{assignee}</span>
            {due && (
              <>
                <span className="text-slate-400">·</span>
                <span className={due.includes("overdue") ? "text-red-600" : ""}>
                  {due}
                </span>
              </>
            )}
          </span>
        }
        actions={<SignalRefresh topic={projectTopic(task.project_id)} />}
      />

      {task.description && (
        <Card title="Description">
          <p className="whitespace-pre-wrap text-sm text-slate-700">
            {task.description}
          </p>
        </Card>
      )}

      <TaskEditor task={task} />

      <BlockerPanel taskId={task.id} blockers={blockers} />

      <EvidencePanel taskId={task.id} evidence={evidence} />

      <p className="text-xs text-slate-400">
        Created {formatDateTime(task.created_at)} · updated{" "}
        {formatDateTime(task.updated_at)} · version {task.version}
      </p>
    </div>
  );
}
