"use client";

import { useRouter } from "next/navigation";
import { useState, type FormEvent } from "react";

import { Button, Field, TextArea, TextInput } from "@/components/form";
import { ProblemNotice } from "@/components/ui";
import { submitDailyReport, type DailyReport } from "@/lib/api/atlas";
import { today } from "@/lib/format";
import { useMutation } from "@/lib/use-mutation";

function optionalNumber(raw: FormDataEntryValue | null): number | null {
  const value = String(raw ?? "").trim();
  return value === "" ? null : Number(value);
}

/**
 * File or update today's daily report (`POST /api/v1/daily-reports`, gated by
 * `people.daily_report.submit`). When a report already exists its values seed
 * the form, so submitting again updates the same day's record.
 */
export function ReportForm({ existing }: { existing: DailyReport | null }) {
  const router = useRouter();
  const { busy, problem, run } = useMutation();
  const [saved, setSaved] = useState(false);

  async function onSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setSaved(false);
    const data = new FormData(event.currentTarget);

    const report = await run(() =>
      submitDailyReport({
        report_date: String(data.get("report_date") ?? "").trim() || undefined,
        work_done: String(data.get("work_done") ?? ""),
        blockers: String(data.get("blockers") ?? ""),
        risks: String(data.get("risks") ?? ""),
        next_day: String(data.get("next_day") ?? ""),
        hours_worked: optionalNumber(data.get("hours_worked")),
      }),
    );

    if (report) {
      setSaved(true);
      router.refresh();
    }
  }

  return (
    <form onSubmit={onSubmit} className="space-y-4">
      {problem && <ProblemNotice error={problem} title="Could not file the report" />}

      <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
        <Field label="Day" htmlFor="report-date">
          <TextInput
            id="report-date"
            name="report_date"
            type="date"
            defaultValue={existing?.report_date ?? today()}
          />
        </Field>
        <Field label="Hours worked" hint="optional" htmlFor="report-hours">
          <TextInput
            id="report-hours"
            name="hours_worked"
            type="number"
            min="0"
            step="0.5"
            defaultValue={existing?.hours_worked ?? ""}
          />
        </Field>
      </div>

      <Field label="What moved" htmlFor="report-work">
        <TextArea
          id="report-work"
          name="work_done"
          rows={3}
          defaultValue={existing?.work_done ?? ""}
          placeholder="Shipped the invoice export; reviewed two PRs…"
        />
      </Field>

      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
        <Field label="Blockers" hint="optional" htmlFor="report-blockers">
          <TextArea
            id="report-blockers"
            name="blockers"
            rows={2}
            defaultValue={existing?.blockers ?? ""}
          />
        </Field>
        <Field label="Risks" hint="optional" htmlFor="report-risks">
          <TextArea
            id="report-risks"
            name="risks"
            rows={2}
            defaultValue={existing?.risks ?? ""}
          />
        </Field>
      </div>

      <Field label="Next" hint="optional" htmlFor="report-next">
        <TextArea
          id="report-next"
          name="next_day"
          rows={2}
          defaultValue={existing?.next_day ?? ""}
        />
      </Field>

      <div className="flex items-center gap-3">
        <Button type="submit" disabled={busy}>
          {busy ? "Filing…" : existing ? "Update report" : "File report"}
        </Button>
        {saved && !busy && <span className="text-sm text-emerald-600">Filed.</span>}
      </div>
    </form>
  );
}
