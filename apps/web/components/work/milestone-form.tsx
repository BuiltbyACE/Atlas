"use client";

import { useRouter } from "next/navigation";
import { useState, type FormEvent } from "react";

import { Drawer } from "@/components/drawer";
import { Button, Field, Select, TextArea, TextInput } from "@/components/form";
import { Icon } from "@/components/icons";
import { useToast } from "@/components/toast";
import { ProblemNotice } from "@/components/ui";
import { createMilestone, MILESTONE_STATUSES } from "@/lib/api/atlas";
import { humanise, toApiTime } from "@/lib/format";
import { useMutation } from "@/lib/use-mutation";

/** Add a milestone to a project (`work.project.manage`), in a drawer. */
export function MilestoneForm({ projectId }: { projectId: string }) {
  const router = useRouter();
  const notify = useToast();
  const { busy, problem, run, reset } = useMutation();
  const [open, setOpen] = useState(false);

  function close() {
    setOpen(false);
    reset();
  }

  async function onSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const data = new FormData(event.currentTarget);

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
      setOpen(false);
      notify(`Added “${created.name}”`);
      router.refresh();
    }
  }

  return (
    <>
      <Button size="sm" variant="secondary" onClick={() => setOpen(true)}>
        <Icon name="plus" className="h-4 w-4" />
        Add milestone
      </Button>
      <Drawer open={open} onClose={close} title="Add a milestone" description="A weighted step on the path to done.">
        <form onSubmit={onSubmit} className="flex flex-col gap-4 px-6 py-[22px]">
          {problem && <ProblemNotice error={problem} title="Could not add the milestone" />}

          <Field label="Name" htmlFor="milestone-name">
            <TextInput id="milestone-name" name="name" required maxLength={200} data-autofocus />
          </Field>
          <div className="grid grid-cols-1 gap-3.5 sm:grid-cols-2">
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
            <Field label="Weight" hint="0–100" htmlFor="milestone-weight">
              <TextInput id="milestone-weight" name="weight" type="number" min="0" max="100" defaultValue={0} />
            </Field>
          </div>
          <Field label="Description" hint="optional" htmlFor="milestone-description">
            <TextArea id="milestone-description" name="description" rows={3} />
          </Field>

          <div className="flex flex-wrap gap-2.5">
            <Button type="submit" disabled={busy}>
              {busy ? "Adding…" : "Add milestone"}
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
