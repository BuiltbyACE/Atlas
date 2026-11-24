"use client";

import { usePathname, useRouter } from "next/navigation";
import { useState, type FormEvent } from "react";

import { Drawer } from "@/components/drawer";
import { Button, Field, Select, TextArea, TextInput } from "@/components/form";
import { Icon } from "@/components/icons";
import { PersonSelect } from "@/components/person-select";
import { useToast } from "@/components/toast";
import { ProblemNotice } from "@/components/ui";
import { createProject, PROJECT_STATUSES } from "@/lib/api/atlas";
import type { PersonOption } from "@/lib/directory";
import { humanise, toApiTime } from "@/lib/format";
import { useMutation } from "@/lib/use-mutation";

/** Project types already configured for timesheets (D7) plus common ones. */
const SUGGESTED_TYPES = ["client_delivery", "internal_product", "operations", "research"];

/**
 * "New project" (`POST /api/v1/work/projects`, `work.project.manage`): a button
 * that opens the form in a drawer. `autoOpen` lets the shell's New menu land
 * here with the drawer already open (`/work?new=project`).
 */
export function NewProjectButton({
  people,
  me,
  autoOpen = false,
  variant = "primary",
}: {
  people: PersonOption[];
  me?: string;
  autoOpen?: boolean;
  variant?: "primary" | "secondary";
}) {
  const router = useRouter();
  const pathname = usePathname();
  const notify = useToast();
  const { busy, problem, run, reset } = useMutation();
  const [open, setOpen] = useState(autoOpen);

  function close() {
    setOpen(false);
    reset();
    if (autoOpen) router.replace(pathname);
  }

  async function onSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const data = new FormData(event.currentTarget);

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
      setOpen(false);
      notify(`Started “${created.name}”`);
      router.push(`/work/projects/${created.id}`);
    }
  }

  return (
    <>
      <Button variant={variant} onClick={() => setOpen(true)}>
        <Icon name="plus" />
        New project
      </Button>
      <Drawer
        open={open}
        onClose={close}
        title="New project"
        description="Give it an owner, a target date and a type. It’s audited the moment it exists."
      >
        <form onSubmit={onSubmit} className="flex flex-col gap-4 px-6 py-[22px]">
          {problem && <ProblemNotice error={problem} title="Could not create the project" />}

          <Field label="Name" htmlFor="project-name">
            <TextInput
              id="project-name"
              name="name"
              required
              maxLength={200}
              placeholder="e.g. Client portal v2"
              data-autofocus
            />
          </Field>
          <Field label="Owner" hint="recommended" htmlFor="project-owner">
            <PersonSelect
              id="project-owner"
              name="owner_person_id"
              people={people}
              me={me}
              emptyLabel="No owner yet"
            />
          </Field>
          <div className="grid grid-cols-1 gap-3.5 sm:grid-cols-2">
            <Field label="Target date" hint="optional" htmlFor="project-due">
              <TextInput id="project-due" name="due_at" type="date" />
            </Field>
            <Field label="Status" htmlFor="project-status">
              <Select id="project-status" name="status" defaultValue="planning">
                {PROJECT_STATUSES.filter((s) => s === "planning" || s === "active").map((status) => (
                  <option key={status} value={status}>
                    {humanise(status)}
                  </option>
                ))}
              </Select>
            </Field>
          </div>
          <Field label="Type" hint="optional" htmlFor="project-type">
            <TextInput
              id="project-type"
              name="project_type"
              list="project-type-options"
              placeholder="client_delivery, internal_product…"
            />
            <datalist id="project-type-options">
              {SUGGESTED_TYPES.map((type) => (
                <option key={type} value={type}>
                  {humanise(type)}
                </option>
              ))}
            </datalist>
          </Field>
          <Field label="Description" hint="optional" htmlFor="project-description">
            <TextArea id="project-description" name="description" rows={4} />
          </Field>

          <div className="flex flex-wrap gap-2.5">
            <Button type="submit" disabled={busy}>
              {busy ? "Creating…" : "Create project"}
            </Button>
            <Button type="button" variant="secondary" onClick={close} disabled={busy}>
              Cancel
            </Button>
          </div>
        </form>
      </Drawer>
    </>
  );
}
