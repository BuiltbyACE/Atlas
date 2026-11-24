"use client";

import { useRouter } from "next/navigation";
import { useState, type FormEvent } from "react";

import { Button, Field, Select, TextArea, TextInput } from "@/components/form";
import { PersonSelect } from "@/components/person-select";
import { useToast } from "@/components/toast";
import { Card, ProblemNotice } from "@/components/ui";
import { PRIORITIES, TASK_STATUSES, updateTask, type Task } from "@/lib/api/atlas";
import type { PersonOption } from "@/lib/directory";
import { dateInput, humanise, statusLabel, toApiTime } from "@/lib/format";
import { useMutation } from "@/lib/use-mutation";

function optionalNumber(raw: FormDataEntryValue | null): number | null {
  const value = String(raw ?? "").trim();
  return value === "" ? null : Number(value);
}

/**
 * Edit a task. `PUT /api/v1/work/tasks/{id}` replaces the whole record, so the
 * form sends every editable field and carries the version it read in `If-Match`
 * (§6.6). `milestone_id`/`parent_id` are not editable here but are preserved so
 * the replacement does not clear them.
 */
export function TaskEditor({
  task,
  people,
  me,
}: {
  task: Task;
  people: PersonOption[];
  me?: string;
}) {
  const router = useRouter();
  const notify = useToast();
  const { busy, problem, run } = useMutation();
  const [saved, setSaved] = useState(false);

  async function onSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setSaved(false);
    const data = new FormData(event.currentTarget);

    const updated = await run(() =>
      updateTask(
        task.id,
        {
          title: String(data.get("title") ?? "").trim(),
          description: String(data.get("description") ?? ""),
          milestone_id: task.milestone_id ?? null,
          parent_id: task.parent_id ?? null,
          assignee_person_id:
            String(data.get("assignee_person_id") ?? "").trim() || null,
          status: String(data.get("status") ?? task.status),
          priority: String(data.get("priority") ?? task.priority),
          due_at: toApiTime(String(data.get("due_at") ?? "")),
          estimate_hours: optionalNumber(data.get("estimate_hours")),
          actual_hours: optionalNumber(data.get("actual_hours")),
        },
        task.version,
      ),
    );

    if (updated) {
      setSaved(true);
      notify("Task saved");
      router.refresh();
    }
  }

  return (
    <Card
      title="Edit task"
      description="Atlas checks every move: some steps can’t be skipped, Done needs evidence, and a change someone else saved first is refused rather than overwritten."
    >
      <form onSubmit={onSubmit} className="space-y-4">
        {problem && <ProblemNotice error={problem} title="Could not save the task" />}

        <Field label="Title" htmlFor="edit-title">
          <TextInput id="edit-title" name="title" required maxLength={300} defaultValue={task.title} />
        </Field>

        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <Field label="Status" htmlFor="edit-status">
            <Select id="edit-status" name="status" defaultValue={task.status}>
              {TASK_STATUSES.map((status) => (
                <option key={status} value={status}>
                  {statusLabel(status)}
                </option>
              ))}
            </Select>
          </Field>
          <Field label="Priority" htmlFor="edit-priority">
            <Select id="edit-priority" name="priority" defaultValue={task.priority}>
              {PRIORITIES.map((priority) => (
                <option key={priority} value={priority}>
                  {humanise(priority)}
                </option>
              ))}
            </Select>
          </Field>
          <Field label="Owner" htmlFor="edit-assignee">
            <PersonSelect
              id="edit-assignee"
              name="assignee_person_id"
              people={people}
              me={me}
              defaultValue={task.assignee_person_id ?? ""}
            />
          </Field>
          <Field label="Due date" hint="optional" htmlFor="edit-due">
            <TextInput id="edit-due" name="due_at" type="date" defaultValue={dateInput(task.due_at)} />
          </Field>
          <Field label="Estimate hours" hint="optional" htmlFor="edit-estimate">
            <TextInput
              id="edit-estimate"
              name="estimate_hours"
              type="number"
              min="0"
              step="0.5"
              defaultValue={task.estimate_hours ?? ""}
            />
          </Field>
          <Field label="Actual hours" hint="optional" htmlFor="edit-actual">
            <TextInput
              id="edit-actual"
              name="actual_hours"
              type="number"
              min="0"
              step="0.5"
              defaultValue={task.actual_hours ?? ""}
            />
          </Field>
        </div>

        <Field label="Description" hint="optional" htmlFor="edit-description">
          <TextArea id="edit-description" name="description" rows={4} defaultValue={task.description ?? ""} />
        </Field>

        <div className="flex items-center gap-3">
          <Button type="submit" disabled={busy}>
            {busy ? "Saving…" : "Save changes"}
          </Button>
          {saved && !busy && <span className="text-sm font-medium text-[#17803D]">Saved.</span>}
        </div>
      </form>
    </Card>
  );
}
