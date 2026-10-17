"use client";

import { useRouter } from "next/navigation";
import { useState, type FormEvent } from "react";

import { Button, Field, Select, TextArea, TextInput } from "@/components/form";
import { ProblemNotice } from "@/components/ui";
import { createMilestone, MILESTONE_STATUSES } from "@/lib/api/atlas";
import { humanise, toApiTime } from "@/lib/format";
import { useMutation } from "@/lib/use-mutation";

/** Add a milestone to a project (`work.project.manage`). */
export function MilestoneForm({ projectId }: { projectId: string }) {
  const router = useRouter();
  const { busy, problem, run } = useMutation();
  const [open, setOpen] = useState(false);

  async function onSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = event.currentTarget;
    const data = new FormData(form);

    const created = await run(() =>
      createMilestone(projectId, {
        name: String(data.get("name") ?? "").trim(),
        description: String(data.get("description") ?? ""),
        status: String(data.get("status") ?? "not_started"),
        due_at: toApiTime(String(data.get("due_at") ?? "")),
        weight: Number(data.get("weight") ?? 0) || 0,
      }),
    );

    if (created) {
      form.reset();
      setOpen(false);
      router.refresh();
    }
  }

  if (!open) {
    return <Button onClick={() => setOpen(true)}>Add milestone</Button>;
  }

  return (
    <form onSubmit={onSubmit} className="space-y-3">
      {problem && <ProblemNotice error={problem} title="Could not add the milestone" />}

      <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-4">
        <Field label="Name" htmlFor="milestone-name">
          <TextInput id="milestone-name" name="name" required maxLength={200} />
        </Field>
        <Field label="Status" htmlFor="milestone-status">
          <Select id="milestone-status" name="status" defaultValue="not_started">
            {MILESTONE_STATUSES.map((status) => (
              <option key={status} value={status}>
                {humanise(status)}
              </option>
            ))}
          </Select>
        </Field>
        <Field label="Due date" hint="optional" htmlFor="milestone-due">
          <TextInput id="milestone-due" name="due_at" type="date" />
        </Field>
        <Field label="Weight" hint="0-100" htmlFor="milestone-weight">
          <TextInput id="milestone-weight" name="weight" type="number" min="0" max="100" defaultValue={0} />
        </Field>
      </div>

      <Field label="Description" hint="optional" htmlFor="milestone-description">
        <TextArea id="milestone-description" name="description" rows={2} />
      </Field>

      <div className="flex items-center gap-2">
        <Button type="submit" disabled={busy}>
          {busy ? "Adding…" : "Add milestone"}
        </Button>
        <Button type="button" variant="secondary" onClick={() => setOpen(false)} disabled={busy}>
          Cancel
        </Button>
      </div>
    </form>
  );
}
