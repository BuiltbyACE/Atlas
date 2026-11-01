"use client";

import { useRouter } from "next/navigation";
import { useState } from "react";

import { Button, Field, Select, TextInput } from "@/components/form";
import { ProblemNotice } from "@/components/ui";
import { useToast } from "@/components/toast";
import { createInvite } from "@/lib/api/atlas";
import { ApiError } from "@/lib/api/client";
import { useMutation } from "@/lib/use-mutation";

/**
 * Invite a new person to join Atlas (`POST /api/v1/auth/invites`,
 * gated by `core.user.manage`). Opens in a drawer over the people page.
 * Only name and email are required; role, employment type and reference
 * are optional context.
 */
export default async function NewPersonPage() {
  const router = useRouter();
  const notify = useToast();
  const { busy, problem, run, reset } = useMutation();
  const [open, setOpen] = useState(true);

  async function onSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const data = new FormData(event.currentTarget);
    const roleGrants = String(data.get("role_grants") ?? "")
      .split(",")
      .map((r) => r.trim())
      .filter(Boolean);

    const created = await run(async () => {
      return createInvite({
        email: String(data.get("email") ?? "").trim(),
        full_name: String(data.get("full_name") ?? "").trim(),
        employment_type:
          String(data.get("employment_type") ?? "").trim() || undefined,
        role_grants: roleGrants.length > 0 ? roleGrants : undefined,
        ttl_hours:
          Number(String(data.get("ttl_hours") ?? "").trim() || "72") ||
          undefined,
      });
    });

    if (created) {
      setOpen(false);
      reset();
      notify(`Invited ${created.email}`);
      router.refresh();
    }
  }

  function close() {
    setOpen(false);
  }

  return (
    <form onSubmit={onSubmit} className="flex flex-col gap-4 px-6 py-[22px]">
      {problem && (
        <ProblemNotice error={problem} title="The invitation couldn't be sent" />
      )}

      <Field label="Email" htmlFor="invite-email">
        <TextInput
          id="invite-email"
          name="email"
          required
          data-autofocus
          placeholder="name@company.com"
        />
      </Field>

      <Field label="Full name" htmlFor="invite-full-name">
        <TextInput
          id="invite-full-name"
          name="full_name"
          required
          placeholder="Jane Wanjiku"
        />
      </Field>

      <div className="grid grid-cols-1 gap-3.5 md:grid-cols-3">
        <Field label="Employment type" htmlFor="invite-employment-type">
          <Select id="invite-employment-type" name="employment_type">
            <option value="">Not specified</option>
            <option value="full_time">Full-time</option>
            <option value="part_time">Part-time</option>
            <option value="contractor">Contractor</option>
            <option value="intern">Intern</option>
            <option value="external">External</option>
          </Select>
        </Field>
        <Field label="Role grants" htmlFor="invite-role-grants">
          <TextInput
            id="invite-role-grants"
            name="role_grants"
            placeholder="e.g. ceo_internal, finance_officer"
          />
          <p className="mt-1 text-[11.5px] text-muted">
            Comma-separated role keys. Leave empty for no role grant.
          </p>
        </Field>
        <Field label="Expires in (hours)" htmlFor="invite-ttl-hours">
          <TextInput
            id="invite-ttl-hours"
            name="ttl_hours"
            type="number"
            defaultValue="72"
          />
          <p className="mt-1 text-[11.5px] text-muted">
            Invitation link expiry in hours.
          </p>
        </Field>
      </div>

      <div className="flex justify-end gap-2 pt-4">
        <Button type="button" variant="secondary" onClick={close}>
          Cancel
        </Button>
        <Button type="submit" variant="primary" disabled={busy}>
          {busy ? "Inviting…" : "Invite person"}
        </Button>
      </div>
    </form>
  );
}
