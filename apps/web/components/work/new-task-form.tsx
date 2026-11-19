"use client";

import { useRouter } from "next/navigation";
import { useState, type FormEvent } from "react";

import { Drawer } from "@/components/drawer";
import { Button, Field, Select, TextArea, TextInput } from "@/components/form";
import { Icon } from "@/components/icons";
import { PersonSelect } from "@/components/person-select";
import { useToast } from "@/components/toast";
import { ProblemNotice } from "@/components/ui";
import { createTask, PRIORITIES, type Task } from "@/lib/api/atlas";
import type { PersonOption } from "@/lib/directory";
import { humanise, toApiTime } from "@/lib/format";
import { useMutation } from "@/lib/use-mutation";

function optionalNumber(raw: FormDataEntryValue | null): number | null {
  const value = String(raw ?? "").trim();
  return value === "" ? null : Number(value);
}

/**
 * Create a task in a project (`POST /api/v1/work/projects/{id}/tasks`, gated by
 * `work.task.write`). A new task starts in To do; it moves along the board from
 * there. Shared by the task board and the project page, inside a drawer.
 */
export function NewTaskForm({
  projects,
  people,
  me,
  defaultProjectId,
  lockProject = false,
  onCreated,
  onCancel,
}: {
  projects: { id: string; name: string }[];
  people: PersonOption[];
  me?: string;
  defaultProjectId: string;
  /** When true the project is fixed (used on a project's own page). */
  lockProject?: boolean;
  onCreated: (task: Task) => void;
  onCancel: () => void;
}) {
  const { busy, problem, run } = useMutation();

  async function onSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const data = new FormData(event.currentTarget);
    const projectId = lockProject ? defaultProjectId : String(data.get("project_id") ?? "");

    const created = await run(() =>
      createTask(projectId, {
        title: String(data.get("title") ?? "").trim(),
        description: String(data.get("description") ?? ""),
        assignee_person_id:
          String(data.get("assignee_person_id") ?? "").trim() || null,
        status: "todo",
        priority: String(data.get("priority") ?? "medium"),
        due_at: toApiTime(String(data.get("due_at") ?? "")),
        estimate_hours: optionalNumber(data.get("estimate_hours")),
      }),
    );

    if (created) onCreated(created);
  }

  if (projects.length === 0) {
    return (
      <p className="px-6 py-6 text-sm text-muted">
        Create a project first — every task belongs to one.
      </p>
    );
  }

  return (
    <form onSubmit={onSubmit} className="flex flex-col gap-4 px-6 py-[22px]">
      {problem && <ProblemNotice error={problem} title="Could not create the task" />}

      <Field label="Title" htmlFor="task-title">
        <TextInput
          id="task-title"
          name="title"
          required
          maxLength={300}
          placeholder="What needs doing?"
          data-autofocus
        />
      </Field>

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

      <div className="grid grid-cols-1 gap-3.5 sm:grid-cols-2">
        <Field label="Owner" hint="optional" htmlFor="task-assignee">
          <PersonSelect id="task-assignee" name="assignee_person_id" people={people} me={me} />
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
        <Field label="Due date" hint="optional" htmlFor="task-due">
          <TextInput id="task-due" name="due_at" type="date" />
        </Field>
        <Field label="Estimate" hint="hours, optional" htmlFor="task-estimate">
          <TextInput id="task-estimate" name="estimate_hours" type="number" min="0" step="0.5" inputMode="decimal" />
        </Field>
      </div>

      <Field label="What needs doing" hint="optional" htmlFor="task-description">
        <TextArea
          id="task-description"
          name="description"
          rows={4}
          placeholder="Context, acceptance criteria, links…"
        />
      </Field>

      <p className="text-[13.5px] text-muted">
        New tasks start in To do. Moving one to Done needs evidence attached.
      </p>

      <div className="flex flex-wrap gap-2.5">
        <Button type="submit" disabled={busy}>
          {busy ? "Creating…" : "Create task"}
        </Button>
        <Button type="button" variant="secondary" onClick={onCancel} disabled={busy}>
          Cancel
        </Button>
      </div>
    </form>
  );
}

/**
 * A "New task" button that opens the form in a drawer and refreshes the route
 * once Go has created the task (used where the page is a server component).
 */
export function NewTaskButton({
  projects,
  people,
  me,
  defaultProjectId = "",
  lockProject = false,
  variant = "primary",
  label = "New task",
}: {
  projects: { id: string; name: string }[];
  people: PersonOption[];
  me?: string;
  defaultProjectId?: string;
  lockProject?: boolean;
  variant?: "primary" | "secondary";
  label?: string;
}) {
  const router = useRouter();
  const notify = useToast();
  const [open, setOpen] = useState(false);

  return (
    <>
      <Button variant={variant} onClick={() => setOpen(true)}>
        <Icon name="plus" />
        {label}
      </Button>
      <Drawer
        open={open}
        onClose={() => setOpen(false)}
        title="New task"
        description="Owned, dated and tracked from the moment it exists."
      >
        <NewTaskForm
          projects={projects}
          people={people}
          me={me}
          defaultProjectId={defaultProjectId}
          lockProject={lockProject}
          onCancel={() => setOpen(false)}
          onCreated={(task) => {
            setOpen(false);
            notify(`Created “${task.title}”`);
            router.refresh();
          }}
        />
      </Drawer>
    </>
  );
}
