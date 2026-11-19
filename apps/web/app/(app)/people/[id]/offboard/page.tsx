"use client";

import { useParams, useRouter } from "next/navigation";
import { useState } from "react";

import { Button, Field, TextInput } from "@/components/form";
import { ProblemNotice } from "@/components/ui";
import { useToast } from "@/components/toast";
import { offboardPerson } from "@/lib/api/atlas";
import { useMutation } from "@/lib/use-mutation";

/**
 * Offboard a person (`POST /api/v1/people/{id}/offboard`,
 * gated by `people.person.manage`). Off-boarding marks the person as exited,
 * deactivates their login, and revokes their sessions and role grants — all
 * in one transaction. A destructive action, so it requires explicit
 * confirmation.
 */
export default function OffboardPage() {
  // A client component cannot receive `params` as a promise and await it, so
  // the route param is read on the client instead.
  const { id } = useParams<{ id: string }>();
  const router = useRouter();
  const notify = useToast();
  const { busy, problem, run, reset } = useMutation();
  const [acknowledged, setAcknowledged] = useState(false);
  const [summary, setSummary] = useState<string | null>(null);

  async function onSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const data = new FormData(event.currentTarget);

    const result = await run(() =>
      offboardPerson(id, {
        reason: String(data.get("reason") ?? "").trim(),
        last_working_day:
          String(data.get("last_working_day") ?? "").trim() || undefined,
      }),
    );

    if (result) {
      setSummary(
        `${result.status}${
          result.account_status ? ` · login ${result.account_status}` : ""
        } · ${result.sessions_revoked} session(s) and ${result.roles_revoked} role grant(s) revoked.`,
      );
      notify("Person off-boarded");
      setAcknowledged(false);
      router.refresh();
    }
  }

  return (
    <form onSubmit={onSubmit} className="flex flex-col gap-4 px-6 py-[22px]">
      {problem && (
        <ProblemNotice error={problem} title="Off-boarding failed" />
      )}
      {summary && (
        <p className="rounded-control bg-[#E3F4EA] px-3 py-2.5 text-sm font-medium text-[#17663A]">
          {summary}
        </p>
      )}

      <Field label="Reason" htmlFor="offboard-reason">
        <TextInput
          id="offboard-reason"
          name="reason"
          placeholder="Reason for off-boarding (optional)"
        />
      </Field>

      <Field label="Last working day" htmlFor="offboard-last-day">
        <TextInput id="offboard-last-day" name="last_working_day" type="date" />
      </Field>

      <label className="flex min-h-11 cursor-pointer items-center gap-2.5 text-[14.5px] font-medium text-night">
        <input
          type="checkbox"
          checked={acknowledged}
          onChange={(event) => setAcknowledged(event.target.checked)}
          className="h-4 w-4 flex-none rounded border-line-strong accent-atlas"
        />
        <span>
          I understand this deactivates the person&apos;s login and revokes their
          sessions and role grants immediately.
        </span>
      </label>

      <div className="flex justify-end gap-2 pt-4">
        <Button type="button" variant="secondary" onClick={() => router.back()}>
          Cancel
        </Button>
        <Button
          type="submit"
          variant="danger"
          disabled={busy || !acknowledged}
        >
          {busy ? "Off-boarding…" : "Off-board person"}
        </Button>
      </div>
    </form>
  );
}
