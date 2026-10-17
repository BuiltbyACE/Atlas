"use client";

import { useRouter } from "next/navigation";
import { useState, type FormEvent } from "react";

import { Button, Field, TextInput } from "@/components/form";
import { ProblemNotice } from "@/components/ui";
import { offboardPerson } from "@/lib/api/atlas";
import { useMutation } from "@/lib/use-mutation";

/**
 * Off-board a person (`POST /api/v1/people/{id}/offboard`, gated by
 * `people.person.manage`). One transaction in Go: the person becomes `exited`,
 * the linked login is deactivated, and its sessions and role grants are revoked.
 * A destructive action, so it needs an explicit acknowledgement first.
 */
export function OffboardForm() {
  const router = useRouter();
  const { busy, problem, run } = useMutation();
  const [acknowledged, setAcknowledged] = useState(false);
  const [summary, setSummary] = useState<string | null>(null);

  async function onSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const data = new FormData(event.currentTarget);

    const result = await run(() =>
      offboardPerson(String(data.get("person_id") ?? "").trim(), {
        reason: String(data.get("reason") ?? ""),
        last_working_day:
          String(data.get("last_working_day") ?? "").trim() || undefined,
      }),
    );

    if (result) {
      setSummary(
        `${result.status}${result.account_status ? ` · login ${result.account_status}` : ""} · ` +
          `${result.sessions_revoked} session(s) and ${result.roles_revoked} role grant(s) revoked.`,
      );
      setAcknowledged(false);
      router.refresh();
    }
  }

  return (
    <form onSubmit={onSubmit} className="space-y-3">
      {problem && <ProblemNotice error={problem} title="Off-boarding failed" />}
      {summary && (
        <p className="rounded-md border border-emerald-200 bg-emerald-50 px-3 py-2 text-sm text-emerald-800">
          {summary}
        </p>
      )}

      <div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
        <Field label="Person id" htmlFor="offboard-person">
          <TextInput
            id="offboard-person"
            name="person_id"
            required
            placeholder="00000000-0000-0000-0000-000000000000"
          />
        </Field>
        <Field label="Last working day" hint="defaults to today" htmlFor="offboard-day">
          <TextInput id="offboard-day" name="last_working_day" type="date" />
        </Field>
        <Field label="Reason" hint="optional" htmlFor="offboard-reason">
          <TextInput id="offboard-reason" name="reason" />
        </Field>
      </div>

      <label className="flex items-center gap-2 text-sm text-slate-700">
        <input
          type="checkbox"
          checked={acknowledged}
          onChange={(event) => setAcknowledged(event.target.checked)}
          className="h-4 w-4 rounded border-slate-300 text-atlas focus:ring-atlas"
        />
        I understand this deactivates the person&apos;s login and revokes access.
      </label>

      <Button type="submit" variant="danger" disabled={busy || !acknowledged}>
        {busy ? "Off-boarding…" : "Off-board person"}
      </Button>
    </form>
  );
}
