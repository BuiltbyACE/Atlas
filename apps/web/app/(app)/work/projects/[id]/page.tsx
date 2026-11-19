import Link from "next/link";

import { ButtonLink } from "@/components/form";
import { Icon } from "@/components/icons";
import { SignalRefresh } from "@/components/signal-refresh";
import {
  Avatar,
  Badge,
  Card,
  EmptyState,
  Meter,
  PageHeader,
  ProblemNotice,
  Stat,
  StatGrid,
  Table,
  Td,
  Th,
  priorityTone,
  statusTone,
} from "@/components/ui";
import { BOARD_COLUMNS, columnLabel } from "@/components/work/board";
import { EvidencePanel } from "@/components/work/evidence-panel";
import { MilestoneForm } from "@/components/work/milestone-form";
import { NewTaskButton } from "@/components/work/new-task-form";
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
import { requireUser } from "@/lib/auth/session";
import { personDirectory, personName, rosterOf } from "@/lib/directory";
import { dueLabel, formatDay, humanise, initials, isOverdue } from "@/lib/format";
import { projectTopic } from "@/lib/realtime/topics";

/**
 * Project overview (blueprint §8.2 key screen). One page per project: health at
 * a glance, its tasks, milestones, members and evidence. Every read is gated by
 * its own permission; a refused read degrades to an empty section rather than
 * failing the page. Changes re-fetch this route, and signals on
 * `work:project:{id}` re-fetch it live.
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
  const user = await requireUser();

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
  const people = rosterOf(names);

  const live = tasks.filter((task) => task.status !== "cancelled");
  const openTasks = live.filter((task) => task.status !== "done");
  const overdue = openTasks.filter((task) => isOverdue(task.due_at, task.status)).length;
  const blocked = openTasks.filter((task) => task.blocked).length;
  const done = live.filter((task) => task.status === "done").length;
  const progress = live.length === 0 ? 0 : Math.round((done / live.length) * 100);
  const due = dueLabel(project.due_at);

  return (
    <div className="space-y-6">
      <PageHeader
        title={project.name}
        eyebrow={
          <span className="flex flex-wrap items-center gap-2 text-sm text-muted">
            <Badge tone={statusTone(project.status)} dot>
              {humanise(project.status)}
            </Badge>
            {project.ref && <span className="font-mono text-xs">{project.ref}</span>}
            {project.project_type && <span>· {humanise(project.project_type)}</span>}
          </span>
        }
        description={
          <span className="flex flex-wrap items-center gap-x-2 gap-y-1">
            <span>
              Owner{" "}
              <b className="font-semibold text-night">
                {project.owner_person_id ? personName(names, project.owner_person_id) : "not set"}
              </b>
            </span>
            {project.due_at && (
              <>
                <span aria-hidden="true">·</span>
                <span>
                  Target {formatDay(project.due_at)}
                  {due ? ` (${due.replace("due ", "")})` : ""}
                </span>
              </>
            )}
            <SignalRefresh topic={projectTopic(project.id)} quiet />
          </span>
        }
        actions={
          <>
            <ButtonLink href={`/work/tasks?project_id=${project.id}`} variant="secondary">
              <Icon name="board" />
              Task board
            </ButtonLink>
            <NewTaskButton
              projects={[{ id: project.id, name: project.name }]}
              people={people}
              me={user.person_id}
              defaultProjectId={project.id}
              lockProject
            />
          </>
        }
      />

      {project.description && (
        <p className="max-w-3xl whitespace-pre-wrap text-[15px] text-muted">{project.description}</p>
      )}

      <StatGrid label="Project health">
        <Stat label="Open tasks" value={openTasks.length} hint={`${done} done · ${live.length} in total`} icon="board" iconTone="info" />
        <Stat
          label="Progress"
          value={`${progress}%`}
          hint="Tasks completed"
          icon="check"
          iconTone="success"
          foot={
            <Meter
              label={`${progress}% of tasks done`}
              className="w-full"
              segments={BOARD_COLUMNS.map((column) => ({
                value: live.filter((t) => t.status === column.status).length,
                color: column.dot,
              }))}
            />
          }
        />
        <Stat
          label="Overdue"
          value={overdue}
          tone={overdue > 0 ? "danger" : "neutral"}
          hint="Open tasks past due"
          icon="clock"
          iconTone={overdue > 0 ? "danger" : "success"}
        />
        <Stat
          label="Blocked"
          value={blocked}
          tone={blocked > 0 ? "warning" : "neutral"}
          hint="Tasks with an open blocker"
          icon="alert"
          iconTone={blocked > 0 ? "warning" : "success"}
        />
      </StatGrid>

      <Card title="Tasks" description={`${live.length} in this project`} padded={false}>
        {live.length === 0 ? (
          <div className="px-[22px] pb-[22px]">
            <EmptyState icon="board" iconTone="info" title="No tasks yet">
              Add the first one with “New task” above.
            </EmptyState>
          </div>
        ) : (
          <Table>
            <thead>
              <tr>
                <Th>Task</Th>
                <Th>Status</Th>
                <Th>Priority</Th>
                <Th>Owner</Th>
                <Th>Due</Th>
              </tr>
            </thead>
            <tbody>
              {live.map((task) => (
                <tr key={task.id} className="hover:bg-[#FAFBFD]">
                  <Td>
                    <Link href={`/work/tasks/${task.id}`} className="font-semibold text-night hover:text-atlas-ink">
                      {task.title}
                    </Link>
                    {task.blocked && (
                      <span className="ml-2 align-middle">
                        <Badge tone="danger">blocked</Badge>
                      </span>
                    )}
                  </Td>
                  <Td>
                    <Badge tone={statusTone(task.status)} dot>
                      {columnLabel(task.status)}
                    </Badge>
                  </Td>
                  <Td>
                    <Badge tone={priorityTone(task.priority)}>{humanise(task.priority)}</Badge>
                  </Td>
                  <Td className="text-muted">
                    {task.assignee_person_id ? personName(names, task.assignee_person_id) : "—"}
                  </Td>
                  <Td
                    className={
                      isOverdue(task.due_at, task.status)
                        ? "whitespace-nowrap font-medium text-[#C0352B]"
                        : "whitespace-nowrap text-muted"
                    }
                  >
                    {formatDay(task.due_at)}
                  </Td>
                </tr>
              ))}
            </tbody>
          </Table>
        )}
      </Card>

      <div className="grid grid-cols-1 items-start gap-5 xl:grid-cols-[minmax(0,1.6fr)_minmax(0,1fr)]">
        <Card
          title="Milestones"
          description="Weighted markers on the path to done."
          actions={<MilestoneForm projectId={project.id} />}
        >
          {milestones.length === 0 ? (
            <EmptyState icon="flag" iconTone="info" title="No milestones yet">
              Milestones group tasks into the steps a client or a CEO cares about.
            </EmptyState>
          ) : (
            <ul className="flex flex-col gap-2.5">
              {milestones.map((milestone) => (
                <li
                  key={milestone.id}
                  className="flex flex-wrap items-center justify-between gap-2 rounded-[14px] border border-line px-3.5 py-3"
                >
                  <div className="min-w-0">
                    <p className="font-semibold text-night">{milestone.name}</p>
                    {milestone.description && (
                      <p className="text-[13px] text-muted">{milestone.description}</p>
                    )}
                  </div>
                  <div className="flex items-center gap-3 text-[13px] text-muted">
                    <span>weight {milestone.weight}</span>
                    <span>{formatDay(milestone.due_at)}</span>
                    <Badge tone={statusTone(milestone.status)} dot>
                      {humanise(milestone.status)}
                    </Badge>
                  </div>
                </li>
              ))}
            </ul>
          )}
        </Card>

        <Card title="Members" description={`${members.length} on the project`}>
          {members.length === 0 ? (
            <EmptyState icon="people" iconTone="info" title="No members recorded" />
          ) : (
            <ul className="flex flex-col gap-2">
              {members.map((member) => {
                const name = personName(names, member.person_id);
                const isMe = member.person_id === user.person_id;
                return (
                  <li key={member.person_id} className="flex items-center gap-3">
                    <Avatar initials={initials(name)} variant={isMe ? "me" : "default"} />
                    <span className="min-w-0 flex-1 truncate font-medium">
                      {name}
                      {isMe && <span className="font-normal text-muted"> (you)</span>}
                    </span>
                    <Badge tone="neutral">{humanise(member.role)}</Badge>
                  </li>
                );
              })}
            </ul>
          )}
        </Card>
      </div>

      <EvidencePanel projectId={project.id} evidence={evidence} title="Project evidence" />

      <ProjectEditor project={project} people={people} me={user.person_id} />
    </div>
  );
}
