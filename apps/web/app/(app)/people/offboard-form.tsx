"use client";

import { useRouter } from "next/navigation";
import { useState, type FormEvent } from "react";

import { Button, Checkbox, Field, TextInput } from "@/components/form";
import { PersonSelect } from "@/components/person-select";
import { useToast } from "@/components/toast";
import { ProblemNotice } from "@/components/ui";
import { offboardPerson } from "@/lib/api/atlas";
import type { PersonOption } from "@/lib/directory";
import { useMutation } from "@/lib/use-mutation";

/**
 * Off-board a person (`POST /api/v1/people/{id}/offboard`, gated by
 * `people.person.manage`). One transaction in Go: the person becomes `exited`,
 * the linked login is deactivated, and its sessions and role grants are revoked.
 * A destructive action, so it needs an explicit acknowledgement first.
 */
export function OffboardForm({ people, me }: { people: PersonOption[]; me?: string }) {
  const router = useRouter();
  const notify = useToast();
  const { busy, problem, run } = useMutation();
  const [acknowledged, setAcknowledged] = useState(false);
  const [person, setPerson] = useState("");
  const [summary, setSummary] = useState<string | null>(null);

  async function onSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const data = new FormData(event.currentTarget);

    const result = await run(() =>
      offboardPerson(person, {
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
      notify("Person off-boarded");
      setAcknowledged(false);
      setPerson("");
      router.refresh();
    }
  }

  return (
    <form onSubmit={onSubmit} className="flex flex-col gap-4">
      {problem && <ProblemNotice error={problem} title="Off-boarding failed" />}
      {summary && (
        <p className="rounded-control bg-[#E3F4EA] px-3 py-2.5 text-sm font-medium text-[#17663A]">
          {summary}
        </p>
      )}

      <div className="grid grid-cols-1 gap-3.5 md:grid-cols-3">
        <Field label="Person" htmlFor="offboard-person">
          <PersonSelect
            id="offboard-person"
            people={people.filter((p) => p.id !== me)}
            value={person}
            onChange={setPerson}
            emptyLabel="Choose a person"
          />
        </Field>
        <Field label="Last working day" hint="defaults to today" htmlFor="offboard-day">
          <TextInput id="offboard-day" name="last_working_day" type="date" />
        </Field>
        <Field label="Reason" hint="optional" htmlFor="offboard-reason">
          <TextInput id="offboard-reason" name="reason" />
        </Field>
      </div>

      <Checkbox
        checked={acknowledged}
        onChange={(event) => setAcknowledged(event.target.checked)}
        label="I understand this deactivates the person’s login and revokes their sessions and roles immediately."
      />

      <div>
        <Button type="submit" variant="danger" disabled={busy || !acknowledged || !person}>
          {busy ? "Off-boarding…" : "Off-board person"}
        </Button>
      </div>
    </form>
  );
}
