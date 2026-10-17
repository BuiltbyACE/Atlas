"use client";

import { useRouter } from "next/navigation";
import { useState, type FormEvent } from "react";

import { Button, Field, Select, TextArea, TextInput } from "@/components/form";
import { Card, ProblemNotice } from "@/components/ui";
import { PROJECT_STATUSES, updateProject, type Project } from "@/lib/api/atlas";
import { dateInput, humanise, toApiTime } from "@/lib/format";
import { useMutation } from "@/lib/use-mutation";

/**
 * Edit a project. `PUT /api/v1/work/projects/{id}` replaces the record, so the
 * form sends every editable field and carries the version it read in `If-Match`
 * (§6.6). `started_at` is preserved (Go stamps it when a project goes active).
 * Status moves are validated by Go (a 409/412 comes back as a problem).
 */
export function ProjectEditor({ project }: { project: Project }) {
  const router = useRouter();
  const { busy, problem, run } = useMutation();
  const [saved, setSaved] = useState(false);

  async function onSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setSaved(false);
    const data = new FormData(event.currentTarget);

    const updated = await run(() =>
      updateProject(
        project.id,
        {
          name: String(data.get("name") ?? "").trim(),
          description: String(data.get("description") ?? ""),
          project_type: String(data.get("project_type") ?? "").trim(),
          status: String(data.get("status") ?? project.status),
          owner_person_id:
            String(data.get("owner_person_id") ?? "").trim() || null,
          department_id: project.department_id ?? null,
          started_at: project.started_at ?? null,
          due_at: toApiTime(String(data.get("due_at") ?? "")),
        },
        project.version,
      ),
    );

    if (updated) {
      setSaved(true);
      router.refresh();
    }
  }

  return (
    <Card title="Project details" description="Go validates status moves and audits the change.">
      <form onSubmit={onSubmit} className="space-y-4">
        {problem && <ProblemNotice error={problem} title="Could not save the project" />}

        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <Field label="Name" htmlFor="proj-name">
            <TextInput id="proj-name" name="name" required maxLength={200} defaultValue={project.name} />
          </Field>
          <Field label="Type" hint="optional" htmlFor="proj-type">
            <TextInput id="proj-type" name="project_type" defaultValue={project.project_type ?? ""} />
          </Field>
          <Field label="Status" htmlFor="proj-status">
            <Select id="proj-status" name="status" defaultValue={project.status}>
              {PROJECT_STATUSES.map((status) => (
                <option key={status} value={status}>
                  {humanise(status)}
                </option>
              ))}
            </Select>
          </Field>
          <Field label="Owner person id" hint="optional" htmlFor="proj-owner">
            <TextInput
              id="proj-owner"
              name="owner_person_id"
              defaultValue={project.owner_person_id ?? ""}
              placeholder="UUID"
            />
          </Field>
          <Field label="Target date" hint="optional" htmlFor="proj-due">
            <TextInput id="proj-due" name="due_at" type="date" defaultValue={dateInput(project.due_at)} />
          </Field>
        </div>

        <Field label="Description" hint="optional" htmlFor="proj-description">
          <TextArea id="proj-description" name="description" rows={3} defaultValue={project.description ?? ""} />
        </Field>

        <div className="flex items-center gap-3">
          <Button type="submit" disabled={busy}>
            {busy ? "Saving…" : "Save changes"}
          </Button>
          {saved && !busy && <span className="text-sm text-emerald-600">Saved.</span>}
        </div>
      </form>
    </Card>
  );
}
