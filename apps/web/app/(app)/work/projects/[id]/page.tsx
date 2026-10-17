import Link from "next/link";

import { SignalRefresh } from "@/components/signal-refresh";
import { Badge, Card, EmptyState, PageHeader, ProblemNotice, Stat, Table, Td, Th, priorityTone, statusTone } from "@/components/ui";
import { EvidencePanel } from "@/components/work/evidence-panel";
import { MilestoneForm } from "@/components/work/milestone-form";
import { NewTaskForm } from "@/components/work/new-task-form";
import { ProjectEditor } from "@/components/work/project-editor";
import {
  getProject,
  listMembers,
  listMilestones,
  listProjectEvidence,
  listTasks,
  type Evidence,
  type Milestone,
  type Page,
  type ProjectMember,
  type Task,
} from "@/lib/api/atlas";
import { ApiError } from "@/lib/api/client";
import { personDirectory } from "@/lib/directory";
import { dueLabel, formatDay, humanise, shortId } from "@/lib/format";
import { projectTopic } from "@/lib/realtime/topics";

/**
 * Project overview (blueprint §8.2 key screen). One page per project: health at
 * a glance, its milestones, its tasks, its members and its evidence. Every read
 * is gated by its own permission; a refused read degrades to an empty section
 * rather than failing the page. Changes re-fetch this route.
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

export default async function ProjectDetailPage({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = await params;

  let project;
  try {
    project = await getProject(id);
  } catch (error) {
    if (!(error instanceof ApiError)) throw error;
    return (
      <div className="space-y-6">
        <PageHeader title="Project" />
        <ProblemNotice
          error={error}
          title={
            error.status === 404
              ? "That project does not exist"
              : "The project could not be loaded"
          }
        />
      </div>
    );
  }

  const [milestones, tasks, members, evidence, names] = await Promise.all([
    safe<Milestone[]>(listMilestones(id), []),
    safe<Page<Task>>(listTasks({ project_id: id, limit: 200 }), {
      items: [],
      has_more: false,
      limit: 0,
    }).then((page) => page.items),
    safe<ProjectMember[]>(listMembers(id), []),
    safe<Evidence[]>(listProjectEvidence(id), []),
    personDirectory(),
  ]);

  const openTasks = tasks.filter(
    (task) => task.status !== "done" && task.status !== "cancelled",
  );
  const overdue = openTasks.filter(
    (task) => task.due_at != null && new Date(task.due_at).getTime() < Date.now(),
  ).length;
  const blocked = tasks.filter((task) => task.blocked).length;
  const done = tasks.filter((task) => task.status === "done").length;
  const progress = tasks.length === 0 ? 0 : Math.round((done / tasks.length) * 100);

  const owner = project.owner_person_id
    ? (names.get(project.owner_person_id) ?? shortId(project.owner_person_id))
    : "Unassigned";

  return (
    <div className="space-y-6">
      <PageHeader
        title={project.name}
        description={
          <span className="flex flex-wrap items-center gap-2">
            <Badge tone={statusTone(project.status)}>{humanise(project.status)}</Badge>
            {project.project_type && (
              <span className="text-slate-500">{humanise(project.project_type)}</span>
            )}
            <span className="text-slate-400">·</span>
            <span>Owner {owner}</span>
            {project.due_at && (
              <>
                <span className="text-slate-400">·</span>
                <span>{dueLabel(project.due_at) ?? formatDay(project.due_at)}</span>
              </>
            )}
          </span>
        }
        actions={<SignalRefresh topic={projectTopic(project.id)} />}
      />

      {project.description && (
        <p className="max-w-3xl text-sm text-slate-600">{project.description}</p>
      )}

      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
        <Stat label="Open tasks" value={openTasks.length} hint={`${done} done`} />
        <Stat label="Progress" value={`${progress}%`} hint="Tasks completed" tone="success" />
        <Stat
          label="Overdue"
          value={overdue}
          tone={overdue > 0 ? "danger" : "neutral"}
          hint="Open tasks past due"
        />
        <Stat
          label="Blocked"
          value={blocked}
          tone={blocked > 0 ? "warning" : "neutral"}
          hint="Tasks with an open blocker"
        />
      </div>

      <Card title="Tasks" description={`${tasks.length} in this project`} padded={false}>
        {tasks.length === 0 ? (
          <div className="px-5 py-6">
            <EmptyState>No tasks yet. Add the first one below.</EmptyState>
          </div>
        ) : (
          <Table>
            <thead>
              <tr>
                <Th>Task</Th>
                <Th>Status</Th>
                <Th>Priority</Th>
                <Th>Assignee</Th>
                <Th>Due</Th>
              </tr>
            </thead>
            <tbody>
              {tasks.map((task) => (
                <tr key={task.id}>
                  <Td>
                    <Link href={`/work/tasks/${task.id}`} className="text-atlas hover:underline">
                      {task.title}
                    </Link>
                  </Td>
                  <Td>
                    <Badge tone={statusTone(task.status)}>{humanise(task.status)}</Badge>
                  </Td>
                  <Td>
                    <Badge tone={priorityTone(task.priority)}>{task.priority}</Badge>
                  </Td>
                  <Td className="text-slate-600">
                    {task.assignee_person_id
                      ? (names.get(task.assignee_person_id) ?? shortId(task.assignee_person_id))
                      : "—"}
                  </Td>
                  <Td className="whitespace-nowrap text-slate-500">{formatDay(task.due_at)}</Td>
                </tr>
              ))}
            </tbody>
          </Table>
        )}
      </Card>

      <Card
        title="Milestones"
        description="Weighted markers on the path to done."
        actions={<MilestoneForm projectId={project.id} />}
      >
        {milestones.length === 0 ? (
          <EmptyState>No milestones yet.</EmptyState>
        ) : (
          <ul className="space-y-2">
            {milestones.map((milestone) => (
              <li
                key={milestone.id}
                className="flex flex-wrap items-center justify-between gap-2 rounded-lg border border-slate-200 px-3 py-2"
              >
                <div>
                  <p className="text-sm font-medium text-slate-800">{milestone.name}</p>
                  {milestone.description && (
                    <p className="text-xs text-slate-500">{milestone.description}</p>
                  )}
                </div>
                <div className="flex items-center gap-3 text-xs text-slate-500">
                  <span>weight {milestone.weight}</span>
                  <span>{formatDay(milestone.due_at)}</span>
                  <Badge tone={statusTone(milestone.status)}>{humanise(milestone.status)}</Badge>
                </div>
              </li>
            ))}
          </ul>
        )}
      </Card>

      <Card title="Add a task" description="Assigned, dated and tracked like any other.">
        <NewTaskForm
          projects={[{ id: project.id, name: project.name }]}
          defaultProjectId={project.id}
          lockProject
        />
      </Card>

      <Card title="Members" description={`${members.length} on the project`}>
        {members.length === 0 ? (
          <EmptyState>No members recorded.</EmptyState>
        ) : (
          <ul className="flex flex-wrap gap-2">
            {members.map((member) => (
              <li
                key={member.person_id}
                className="rounded-full bg-slate-100 px-3 py-1 text-xs text-slate-700"
              >
                {names.get(member.person_id) ?? shortId(member.person_id)}
                <span className="text-slate-400"> · {member.role}</span>
              </li>
            ))}
          </ul>
        )}
      </Card>

      <EvidencePanel projectId={project.id} evidence={evidence} title="Project evidence" />

      <ProjectEditor project={project} />
    </div>
  );
}
