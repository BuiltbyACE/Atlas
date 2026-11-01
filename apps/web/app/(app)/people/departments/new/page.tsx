"use client";

import { useRouter } from "next/navigation";
import { useState } from "react";

import { Button, Field, TextInput } from "@/components/form";
import { ProblemNotice } from "@/components/ui";
import { useToast } from "@/components/toast";
import {
  createDepartment,
  type Department,
} from "@/lib/api/atlas";
import { ApiError } from "@/lib/api/client";
import { useMutation } from "@/lib/use-mutation";

/**
 * Create a new department (`POST /api/v1/people/departments`,
 * gated by `people.person.manage`). Name is the only required field.
 */
export default function NewDepartmentPage() {
  const router = useRouter();
  const notify = useToast();
  const { busy, problem, run, reset } = useMutation();
  const [name, setName] = useState("");

  async function onSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const data = new FormData(event.currentTarget);
    const created = await run(async () => {
      return createDepartment({
        name: String(data.get("name") ?? "").trim(),
      });
    });

    if (created) {
      setName("");
      reset();
      notify(`Department "${created.name}" created`);
      router.push(`/people/departments`);
    }
  }

  return (
    <form onSubmit={onSubmit} className="flex flex-col gap-4 px-6 py-[22px]">
      {problem && (
        <ProblemNotice error={problem} title="The department wasn't created" />
      )}

      <Field label="Department name" htmlFor="department-name">
        <TextInput
          id="department-name"
          name="name"
          required
          data-autofocus
          placeholder="e.g. Engineering"
        />
        <p className="mt-1 text-[11.5px] text-muted">
          A stable name for the department. Cannot be changed later.
        </p>
      </Field>

      <div className="flex justify-end gap-2 pt-4">
        <Button type="button" variant="secondary" onClick={() => router.back()}>
          Cancel
        </Button>
        <Button type="submit" variant="primary" disabled={busy}>
          {busy ? "Creating…" : "Create department"}
        </Button>
      </div>
    </form>
  );
}
