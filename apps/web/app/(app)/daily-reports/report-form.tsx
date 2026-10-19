"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useState, type FormEvent } from "react";

import { Button, ButtonLink, Field, TextArea, TextInput } from "@/components/form";
import { Icon } from "@/components/icons";
import { useToast } from "@/components/toast";
import { Badge, Card, EmptyState, ProblemNotice, statusTone, type Tone } from "@/components/ui";
import { columnLabel } from "@/components/work/board";
import { submitDailyReport, type DailyReport, type Task } from "@/lib/api/atlas";
import { formatLongDay, formatTime } from "@/lib/format";
import { useMutation } from "@/lib/use-mutation";

/** How Go's report lifecycle (§8.5, ADR-017) reads on the card. */
function reportStatus(report: DailyReport | null, editing: boolean): { label: string; tone: Tone } {
  if (editing) return { label: "Editing", tone: "info" };
  if (!report || !report.submitted_at) {
    return report?.status === "missed"
      ? { label: "Missed", tone: "danger" }
      : report?.status === "excused"
        ? { label: "Excused", tone: "neutral" }
        : { label: "Not filed yet", tone: "warning" };
  }
  if (report.status === "late") return { label: "Filed late", tone: "warning" };
  if (report.status === "reviewed") return { label: "Reviewed", tone: "success" };
  return { label: "Filed", tone: statusTone(report.status) };
}

interface Draft {
  day: string;
  hours: string;
  moved: string;
  blockers: string;
  risks: string;
  next: string;
}

function draftOf(report: DailyReport | null, day: string): Draft {
  return {
    day: report?.report_date ?? day,
    hours: report?.hours_worked != null ? String(report.hours_worked) : "",
    moved: report?.work_done ?? "",
    blockers: report?.blockers ?? "",
    risks: report?.risks ?? "",
    next: report?.next_day ?? "",
  };
}

/** Append a bullet to a free-text field without duplicating it. */
function appendLine(text: string, line: string): string {
  if (text.split("\n").some((existing) => existing.trim() === line.trim())) return text;
  return text.trim() ? `${text.replace(/\s+$/, "")}\n${line}` : line;
}

/**
 * Today's report and the work to report against (prototype 08).
 *
 * Filing is `POST /api/v1/daily-reports` (`people.daily_report.submit`); the
 * same call updates the day's report until it is reviewed, which Go enforces.
 * Whether a filing is late is decided by the daily-report sweep, never here.
 *
 * "Your work" lists the viewer's open tasks so each one can be dropped into
 * the report as a line rather than written from memory. It only edits the
 * text being typed; nothing is sent until the report is filed.
 */
export function DailyReportWorkspace({
  existing,
  today,
  myTasks,
  unowned,
  hasPerson,
}: {
  existing: DailyReport | null;
  today: string;
  myTasks: Task[];
  unowned: number | null;
  hasPerson: boolean;
}) {
  const router = useRouter();
  const notify = useToast();
  const { busy, problem, run, reset } = useMutation();
  const [report, setReport] = useState<DailyReport | null>(existing);
  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState<Draft>(() => draftOf(existing, today));

  const filed = Boolean(report?.submitted_at) && report?.report_date === draft.day;
  const showForm = !filed || editing;
  const status = reportStatus(report, editing);

  function set<K extends keyof Draft>(key: K, value: Draft[K]) {
    setDraft((current) => ({ ...current, [key]: value }));
  }

  async function onSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const saved = await run(() =>
      submitDailyReport({
        report_date: draft.day || undefined,
        work_done: draft.moved.trim(),
        blockers: draft.blockers.trim(),
        risks: draft.risks.trim(),
        next_day: draft.next.trim(),
        hours_worked: draft.hours.trim() === "" ? null : Number(draft.hours),
      }),
    );
    if (saved) {
      const wasEditing = editing;
      setReport(saved);
      setEditing(false);
      notify(wasEditing ? "Report updated" : "Daily report filed");
      router.refresh();
    }
  }

  return (
    <div className="grid grid-cols-1 items-start gap-5 xl:grid-cols-[minmax(0,1.8fr)_minmax(0,1fr)]">
      <Card
        title={draft.day === today ? "Today’s report" : "Report"}
        description={formatLongDay(draft.day)}
        actions={<Badge tone={status.tone} dot>{status.label}</Badge>}
        padded={false}
      >
        {showForm ? (
          <form onSubmit={onSubmit} className="flex flex-col gap-4 px-6 pb-6 pt-1">
            {problem && <ProblemNotice error={problem} title="The report wasn’t filed" />}

            <div className="grid grid-cols-1 gap-3.5 sm:grid-cols-2">
              <Field label="Day" htmlFor="report-date">
                <TextInput
                  id="report-date"
                  type="date"
                  value={draft.day}
                  max={today}
                  onChange={(event) => event.target.value && set("day", event.target.value)}
                />
              </Field>
              <Field label="Hours worked" hint="optional" htmlFor="report-hours">
                <span className="relative block">
                  <TextInput
                    id="report-hours"
                    type="number"
                    min="0"
                    max="24"
                    step="0.5"
                    inputMode="decimal"
                    placeholder="e.g. 7.5"
                    value={draft.hours}
                    onChange={(event) => set("hours", event.target.value)}
                    className="pr-8"
                  />
                  <span aria-hidden="true" className="pointer-events-none absolute right-3 top-1/2 -translate-y-1/2 text-sm text-muted">
                    h
                  </span>
                </span>
              </Field>
            </div>

            <Field label="What moved" hint="required" htmlFor="report-work">
              <TextArea
                id="report-work"
                required
                rows={4}
                value={draft.moved}
                onChange={(event) => set("moved", event.target.value)}
                placeholder="Shipped the invoice export; reviewed two PRs…"
              />
            </Field>

            <div className="grid grid-cols-1 gap-3.5 sm:grid-cols-2">
              <Field label="Blockers" hint="optional" htmlFor="report-blockers">
                <TextArea
                  id="report-blockers"
                  rows={3}
                  value={draft.blockers}
                  onChange={(event) => set("blockers", event.target.value)}
                  placeholder="What is stopping you, and who can unblock it?"
                />
              </Field>
              <Field label="Risks" hint="optional" htmlFor="report-risks">
                <TextArea
                  id="report-risks"
                  rows={3}
                  value={draft.risks}
                  onChange={(event) => set("risks", event.target.value)}
                  placeholder="Anything that could slip a deadline or a commitment"
                />
              </Field>
            </div>

            <Field label="Next" hint="optional" htmlFor="report-next">
              <TextArea
                id="report-next"
                rows={3}
                value={draft.next}
                onChange={(event) => set("next", event.target.value)}
                placeholder="What you’ll pick up tomorrow"
              />
            </Field>

            <div className="flex flex-wrap items-center gap-2.5">
              <Button type="submit" disabled={busy || !hasPerson}>
                <Icon name="check" />
                {busy ? "Filing…" : editing ? "Save changes" : "File report"}
              </Button>
              {editing && (
                <Button
                  type="button"
                  variant="secondary"
                  onClick={() => {
                    setEditing(false);
                    setDraft(draftOf(report, today));
                    reset();
                  }}
                >
                  Cancel
                </Button>
              )}
              <span className="text-[13.5px] text-muted">
                A blocker that holds up a task belongs on the task too, so its owner sees it.
              </span>
            </div>
            {!hasPerson && (
              <p className="text-[13.5px] text-[#B42318]">
                This sign-in isn’t linked to a person record, so it can’t file reports.
              </p>
            )}
          </form>
        ) : (
          <div className="flex flex-col gap-[18px] px-[22px] pb-6 pt-2">
            <div className="flex items-center gap-3.5">
              <span className="grid h-[52px] w-[52px] flex-none place-items-center rounded-full bg-[#E3F4EA] text-[#17803D]">
                <Icon name="check" className="h-6 w-6" />
              </span>
              <span>
                <b className="block text-[19px] font-semibold">Report filed</b>
                <small className="block text-sm text-muted">
                  {formatLongDay(report!.report_date)} · filed at {formatTime(report!.submitted_at)}
                </small>
              </span>
            </div>
            <dl className="grid grid-cols-1 overflow-hidden rounded-[14px] border border-line sm:grid-cols-[140px_minmax(0,1fr)]">
              {[
                ["What moved", report!.work_done || "—"],
                ["Hours", report!.hours_worked != null ? `${report!.hours_worked} hours` : "Not given"],
                ["Blockers", report!.blockers || "None"],
                ["Risks", report!.risks || "None"],
                ["Next", report!.next_day || "Not given"],
              ].map(([term, value], index) => (
                <div key={term} className="contents">
                  <dt className={`bg-[#FAFBFC] px-3.5 py-3 text-[14.5px] font-medium text-muted ${index ? "border-t border-line" : ""}`}>
                    {term}
                  </dt>
                  <dd className={`whitespace-pre-line px-3.5 py-3 text-[14.5px] ${index ? "sm:border-t sm:border-line" : ""}`}>
                    {value}
                  </dd>
                </div>
              ))}
            </dl>
            <div>
              <Button
                variant="secondary"
                onClick={() => {
                  setDraft(draftOf(report, today));
                  setEditing(true);
                }}
              >
                Edit report
              </Button>
            </div>
          </div>
        )}
      </Card>

      <Card title="Your work" description="Tasks you own, to report against.">
        {myTasks.length === 0 ? (
          <div className="flex flex-col gap-3">
            <EmptyState
              icon="user"
              iconTone="orange"
              title="No tasks are assigned to you"
              action={
                <ButtonLink href="/work/tasks?assignee_person_id=unassigned" size="sm">
                  Open the task board
                </ButtonLink>
              }
            >
              {unowned
                ? `${unowned} open task${unowned === 1 ? " has" : "s have"} no owner yet.`
                : "Work assigned to you shows up here."}
            </EmptyState>
            <p className="text-sm text-muted">
              Once work is assigned to you, it appears here so you can add each task to your report.
            </p>
          </div>
        ) : (
          <ul className="flex flex-col gap-2.5">
            {myTasks.map((task) => {
              const line = `• ${task.title} (${columnLabel(task.status).toLowerCase()})`;
              return (
                <li key={task.id} className="rounded-[14px] border border-line p-3">
                  <Link href={`/work/tasks/${task.id}`} className="block font-semibold hover:text-atlas-ink">
                    {task.title}
                  </Link>
                  <span className="mt-1.5 flex flex-wrap items-center gap-1.5">
                    <Badge tone={statusTone(task.status)} dot>
                      {columnLabel(task.status)}
                    </Badge>
                    {task.blocked && <Badge tone="danger">Blocked</Badge>}
                  </span>
                  {showForm && (
                    <span className="mt-2.5 flex flex-wrap gap-1.5">
                      <Button size="sm" variant="secondary" onClick={() => set("moved", appendLine(draft.moved, line))}>
                        <Icon name="plus" className="h-4 w-4" />
                        What moved
                      </Button>
                      <Button size="sm" variant="secondary" onClick={() => set("next", appendLine(draft.next, line))}>
                        <Icon name="plus" className="h-4 w-4" />
                        Next
                      </Button>
                      {task.blocked && task.blocked_reason && (
                        <Button
                          size="sm"
                          variant="secondary"
                          onClick={() =>
                            set("blockers", appendLine(draft.blockers, `• ${task.title}: ${task.blocked_reason}`))
                          }
                        >
                          <Icon name="plus" className="h-4 w-4" />
                          Blocker
                        </Button>
                      )}
                    </span>
                  )}
                </li>
              );
            })}
          </ul>
        )}
      </Card>
    </div>
  );
}
