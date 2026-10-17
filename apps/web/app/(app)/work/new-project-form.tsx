"use client";

import { useRouter } from "next/navigation";
import { useState, type FormEvent } from "react";

import { Button, Field, Select, TextArea, TextInput } from "@/components/form";
import { ProblemNotice } from "@/components/ui";
import { createProject, PROJECT_STATUSES } from "@/lib/api/atlas";
import { humanise, toApiTime } from "@/lib/format";
import { useMutation } from "@/lib/use-mutation";

/** Start a project (`POST /api/v1/work/projects`, `work.project.manage`). */
export function NewProjectForm() {
  const router = useRouter();
  const { busy, problem, run } = useMutation();
  const [open, setOpen] = useState(false);

  async function onSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = event.currentTarget;
    const data = new FormData(form);

    const created = await run(() =>
      createProject({
        name: String(data.get("name") ?? "").trim(),
        description: String(data.get("description") ?? ""),
        project_type: String(data.get("project_type") ?? "").trim(),
        status: String(data.get("status") ?? "planning"),
        owner_person_id: String(data.get("owner_person_id") ?? "").trim() || null,
        due_at: toApiTime(String(data.get("due_at") ?? "")),
      }),
    );

    if (created) {
      form.reset();
      setOpen(false);
      router.refresh();
      router.push(`/work/projects/${created.id}`);
    }
  }

  if (!open) {
    return (
      <Button onClick={() => setOpen(true)}>New project</Button>
    );
  }

  return (
    <form onSubmit={onSubmit} className="space-y-4">
      {problem && <ProblemNotice error={problem} title="Could not create the project" />}

      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
        <Field label="Name" htmlFor="project-name">
          <TextInput id="project-name" name="name" required maxLength={200} placeholder="e.g. Client portal v2" />
        </Field>
        <Field label="Type" hint="optional" htmlFor="project-type">
          <TextInput id="project-type" name="project_type" placeholder="client_delivery, internal_product…" />
        </Field>
        <Field label="Status" htmlFor="project-status">
          <Select id="project-status" name="status" defaultValue="planning">
            {PROJECT_STATUSES.map((status) => (
              <option key={status} value={status}>
                {humanise(status)}
              </option>
            ))}
          </Select>
        </Field>
        <Field label="Target date" hint="optional" htmlFor="project-due">
          <TextInput id="project-due" name="due_at" type="date" />
        </Field>
        <Field label="Owner person id" hint="optional, UUID" htmlFor="project-owner">
          <TextInput id="project-owner" name="owner_person_id" placeholder="00000000-0000-0000-0000-000000000000" />
        </Field>
      </div>

      <Field label="Description" hint="optional" htmlFor="project-description">
        <TextArea id="project-description" name="description" rows={3} />
      </Field>

      <div className="flex items-center gap-2">
        <Button type="submit" disabled={busy}>
          {busy ? "Creating…" : "Create project"}
        </Button>
        <Button type="button" variant="secondary" onClick={() => setOpen(false)} disabled={busy}>
          Cancel
        </Button>
      </div>
    </form>
  );
}
