"use client";

import { useRouter } from "next/navigation";
import { useState, type FormEvent } from "react";

import { Button, Field, Select, TextArea, TextInput } from "@/components/form";
import { ProblemNotice } from "@/components/ui";
import { createTask, PRIORITIES, TASK_STATUSES } from "@/lib/api/atlas";
import { humanise, toApiTime } from "@/lib/format";
import { useMutation } from "@/lib/use-mutation";

function optionalNumber(raw: FormDataEntryValue | null): number | null {
  const value = String(raw ?? "").trim();
  return value === "" ? null : Number(value);
}

/**
 * Create a task in a project (`POST /api/v1/work/projects/{id}/tasks`, gated by
 * `work.task.write`). Shared by the task board and the project detail page.
 */
export function NewTaskForm({
  projects,
  defaultProjectId,
  lockProject = false,
}: {
  projects: { id: string; name: string }[];
  defaultProjectId: string;
  /** When true the project is fixed (used on a project's own page). */
  lockProject?: boolean;
}) {
  const router = useRouter();
  const { busy, problem, run } = useMutation();
  const [open, setOpen] = useState(false);

  async function onSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = event.currentTarget;
    const data = new FormData(form);
    const projectId = String(data.get("project_id") ?? "");

    const created = await run(() =>
      createTask(projectId, {
        title: String(data.get("title") ?? "").trim(),
        description: String(data.get("description") ?? ""),
        assignee_person_id:
          String(data.get("assignee_person_id") ?? "").trim() || null,
        status: String(data.get("status") ?? "todo"),
        priority: String(data.get("priority") ?? "medium"),
        due_at: toApiTime(String(data.get("due_at") ?? "")),
        estimate_hours: optionalNumber(data.get("estimate_hours")),
      }),
    );

    if (created) {
      form.reset();
      setOpen(false);
      router.refresh();
      if (!lockProject) router.push(`/work/tasks/${created.id}`);
    }
  }

  if (projects.length === 0) {
    return (
      <p className="text-sm text-slate-500">
        Create a project first — every task belongs to one.
      </p>
    );
  }

  if (!open) {
    return <Button onClick={() => setOpen(true)}>New task</Button>;
  }

  return (
    <form onSubmit={onSubmit} className="space-y-4">
      {problem && <ProblemNotice error={problem} title="Could not create the task" />}

      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
        <Field label="Project" htmlFor="task-project">
          <Select
            id="task-project"
            name="project_id"
            required
            disabled={lockProject}
            defaultValue={defaultProjectId || projects[0]?.id}
          >
            {projects.map((project) => (
              <option key={project.id} value={project.id}>
                {project.name}
              </option>
            ))}
          </Select>
        </Field>
        <Field label="Title" htmlFor="task-title">
          <TextInput id="task-title" name="title" required maxLength={300} />
        </Field>
        <Field label="Status" htmlFor="task-status">
          <Select id="task-status" name="status" defaultValue="todo">
            {TASK_STATUSES.map((status) => (
              <option key={status} value={status}>
                {humanise(status)}
              </option>
            ))}
          </Select>
        </Field>
        <Field label="Priority" htmlFor="task-priority">
          <Select id="task-priority" name="priority" defaultValue="medium">
            {PRIORITIES.map((priority) => (
              <option key={priority} value={priority}>
                {humanise(priority)}
              </option>
            ))}
          </Select>
        </Field>
        <Field label="Assignee person id" hint="optional" htmlFor="task-assignee">
          <TextInput id="task-assignee" name="assignee_person_id" placeholder="UUID" />
        </Field>
        <Field label="Due date" hint="optional" htmlFor="task-due">
          <TextInput id="task-due" name="due_at" type="date" />
        </Field>
        <Field label="Estimate hours" hint="optional" htmlFor="task-estimate">
          <TextInput id="task-estimate" name="estimate_hours" type="number" min="0" step="0.5" />
        </Field>
      </div>

      <Field label="Description" hint="optional" htmlFor="task-description">
        <TextArea id="task-description" name="description" rows={3} />
      </Field>

      <div className="flex items-center gap-2">
        <Button type="submit" disabled={busy}>
          {busy ? "Creating…" : "Create task"}
        </Button>
        <Button
          type="button"
          variant="secondary"
          onClick={() => setOpen(false)}
          disabled={busy}
        >
          Cancel
        </Button>
      </div>
    </form>
  );
}
