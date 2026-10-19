"use client";

import Link from "next/link";
import { useCallback, useEffect, useRef, useState, type FormEvent } from "react";

import { Drawer, DrawerSection } from "@/components/drawer";
import { Button, Select, TextArea, TextInput } from "@/components/form";
import { Icon } from "@/components/icons";
import { PersonSelect } from "@/components/person-select";
import { useToast } from "@/components/toast";
import {
  Badge,
  InlineError,
  ProblemNotice,
  cx,
  priorityTone,
} from "@/components/ui";
import {
  getTask,
  listBlockers,
  listTaskEvidence,
  PRIORITIES,
  raiseBlocker,
  resolveBlocker,
  taskBody,
  updateTask,
  type Blocker,
  type Evidence,
  type Task,
  type TaskBody,
  type TaskStatus,
} from "@/lib/api/atlas";
import { ApiError } from "@/lib/api/client";
import type { PersonOption } from "@/lib/directory";
import {
  dateInput,
  dueLabel,
  formatDateTime,
  formatShortDay,
  humanise,
  isOverdue,
  toApiTime,
} from "@/lib/format";
import { useMutation } from "@/lib/use-mutation";

import { BOARD_COLUMNS, EVIDENCE_LABELS, columnLabel } from "./board";
import { EvidenceForm } from "./evidence-form";

export type DrawerFocus = "blocker" | "evidence" | null;

/**
 * A task opened from the board (prototype `.tb-drawer`): what needs doing, the
 * status, quick edits, the blocker and the evidence. Every change is a call to
 * Go; the board is handed the task Go returns so the card updates in place.
 *
 * `focus` opens straight onto the blocker form (after a drop into Blocked) or
 * the evidence form (after Go refused Done for want of evidence), and `notice`
 * carries Go's words for why.
 */
export function TaskDrawer({
  task,
  projectName,
  people,
  me,
  focus,
  notice,
  onClose,
  onMove,
  onChanged,
}: {
  task: Task | null;
  projectName?: string;
  people: PersonOption[];
  me?: string;
  focus: DrawerFocus;
  notice?: string | null;
  onClose: () => void;
  onMove: (task: Task, status: TaskStatus) => void;
  onChanged: (task: Task) => void;
}) {
  const notify = useToast();
  const { busy, problem, run, reset } = useMutation();
  const [blockers, setBlockers] = useState<Blocker[] | null>(null);
  const [evidence, setEvidence] = useState<Evidence[] | null>(null);
  const [readProblem, setReadProblem] = useState<string | null>(null);
  const [blockerOpen, setBlockerOpen] = useState(false);
  const [evidenceOpen, setEvidenceOpen] = useState(false);
  const evidenceRef = useRef<HTMLDivElement>(null);
  const taskId = task?.id;

  const load = useCallback(async (id: string) => {
    setReadProblem(null);
    const [b, e] = await Promise.allSettled([listBlockers(id, true), listTaskEvidence(id)]);
    setBlockers(b.status === "fulfilled" ? b.value : []);
    setEvidence(e.status === "fulfilled" ? e.value : []);
    const refused = [b, e].find((r) => r.status === "rejected") as PromiseRejectedResult | undefined;
    if (refused) {
      setReadProblem(
        refused.reason instanceof ApiError ? refused.reason.message : "Part of this task could not be loaded.",
      );
    }
  }, []);

  useEffect(() => {
    if (!taskId) return;
    setBlockers(null);
    setEvidence(null);
    reset();
    void load(taskId);
  }, [taskId, load, reset]);

  useEffect(() => {
    setBlockerOpen(focus === "blocker");
    setEvidenceOpen(focus === "evidence");
    if (focus === "evidence") {
      window.setTimeout(() => evidenceRef.current?.scrollIntoView({ behavior: "smooth", block: "start" }), 120);
    }
  }, [focus, taskId]);

  if (!task) return <Drawer open={false} onClose={onClose} title="">{null}</Drawer>;
  const current = task;

  async function save(patch: Partial<TaskBody>, message: string) {
    const updated = await run(() => updateTask(current.id, taskBody(current, patch), current.version));
    if (updated) {
      onChanged(updated);
      notify(message);
    }
  }

  /** Re-read the task after a blocker change: Go owns the `blocked` flag. */
  async function refreshTask() {
    try {
      onChanged(await getTask(current.id));
    } catch {
      // The board keeps its copy; the next navigation re-reads it.
    }
  }

  async function onRaise(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = event.currentTarget;
    const reason = String(new FormData(form).get("reason") ?? "").trim();
    const created = await run(() => raiseBlocker(current.id, reason));
    if (created) {
      form.reset();
      setBlockerOpen(false);
      notify("Blocker saved");
      await Promise.all([load(current.id), refreshTask()]);
    }
  }

  async function onResolve(id: string) {
    const resolved = await run(() => resolveBlocker(id));
    if (resolved) {
      notify("Blocker resolved");
      await Promise.all([load(current.id), refreshTask()]);
    }
  }

  const openBlockers = blockers?.filter((b) => !b.resolved_at) ?? [];
  const resolvedBlockers = blockers?.filter((b) => b.resolved_at) ?? [];
  const due = dueLabel(task.due_at);
  const overdue = isOverdue(task.due_at, task.status);
  const needsReason = task.status === "blocked" && !task.blocked;

  return (
    <Drawer
      open
      onClose={onClose}
      title={task.title}
      eyebrow={
        <span className="flex flex-wrap items-center gap-2">
          <Badge tone={priorityTone(task.priority)}>{humanise(task.priority)} priority</Badge>
          {task.ref && (
            <span className="inline-flex h-[22px] items-center rounded-md bg-white/15 px-2 font-mono text-[11px] text-white">
              {task.ref}
            </span>
          )}
        </span>
      }
      description={
        <>
          {columnLabel(task.status)} ·{" "}
          <span className={overdue ? "font-semibold text-[#FFB3A5]" : undefined}>
            {task.due_at ? `${formatShortDay(task.due_at)}${due ? ` (${due})` : ""}` : "No deadline"}
          </span>
          {projectName && <> · {projectName}</>}
        </>
      }
    >
      {problem && (
        <div className="px-6 pt-5">
          <ProblemNotice error={problem} title="Atlas didn’t accept that" />
        </div>
      )}
      {readProblem && (
        <div className="px-6 pt-5">
          <ProblemNotice error={{ message: readProblem }} title="Some details are hidden" />
        </div>
      )}

      {task.description && (
        <DrawerSection title="What needs doing">
          <p className="whitespace-pre-wrap text-[15px] leading-relaxed text-night">{task.description}</p>
        </DrawerSection>
      )}

      <DrawerSection title="Status">
        <div role="group" aria-label="Task status" className="flex flex-wrap gap-1.5">
          {BOARD_COLUMNS.map((column) => {
            const pressed = task.status === column.status;
            return (
              <button
                key={column.status}
                type="button"
                aria-pressed={pressed}
                disabled={busy}
                onClick={() => onMove(task, column.status)}
                className={cx(
                  "inline-flex h-9 items-center gap-2 rounded-full border px-[13px] text-[13.5px] font-medium transition",
                  pressed
                    ? "border-navy bg-navy text-white"
                    : "border-line-strong bg-white text-night hover:border-[#B8C3D6]",
                )}
              >
                <i aria-hidden="true" className="h-2 w-2 rounded-full" style={{ background: column.dot }} />
                {column.label}
              </button>
            );
          })}
        </div>
        <p className="mt-2.5 text-[13px] text-muted">
          Atlas checks each move: some steps can’t be skipped, and Done needs evidence.
        </p>
      </DrawerSection>

      <DrawerSection title="Details">
        <dl className="grid grid-cols-[96px_minmax(0,1fr)] items-center gap-x-3.5 gap-y-3 text-[14.5px]">
          <dt className="text-muted">
            <label htmlFor="drawer-owner">Owner</label>
          </dt>
          <dd>
            <PersonSelect
              id="drawer-owner"
              people={people}
              me={me}
              value={task.assignee_person_id ?? ""}
              disabled={busy}
              onChange={(personId) =>
                void save(
                  { assignee_person_id: personId || null },
                  personId ? "Owner updated" : "Owner cleared",
                )
              }
            />
          </dd>
          <dt className="text-muted">
            <label htmlFor="drawer-priority">Priority</label>
          </dt>
          <dd>
            <Select
              id="drawer-priority"
              value={task.priority}
              disabled={busy}
              onChange={(event) => void save({ priority: event.target.value }, "Priority updated")}
            >
              {PRIORITIES.map((priority) => (
                <option key={priority} value={priority}>
                  {humanise(priority)}
                </option>
              ))}
            </Select>
          </dd>
          <dt className="text-muted">
            <label htmlFor="drawer-due">Due</label>
          </dt>
          <dd>
            <TextInput
              id="drawer-due"
              type="date"
              defaultValue={dateInput(task.due_at)}
              key={task.due_at ?? "none"}
              disabled={busy}
              onBlur={(event) => {
                const next = toApiTime(event.target.value);
                if ((next ?? "").slice(0, 10) === (task.due_at ?? "").slice(0, 10)) return;
                void save({ due_at: next }, next ? "Deadline updated" : "Deadline cleared");
              }}
            />
          </dd>
        </dl>
      </DrawerSection>

      <DrawerSection title="Blocker" id="drawer-blocker">
        {needsReason && !blockerOpen && (
          <p className="mb-3 flex items-center gap-2 rounded-[10px] border border-dashed border-[#F3A39C] px-2.5 py-2 text-[13px] font-semibold text-[#B42318]">
            <Icon name="alert" className="h-4 w-4" />
            Blocked — add the reason so someone can unblock it
          </p>
        )}
        {blockers === null ? (
          <p className="text-sm text-muted">Loading…</p>
        ) : (
          <>
            {openBlockers.map((blocker) => (
              <div
                key={blocker.id}
                className="mb-2.5 flex items-start gap-2.5 rounded-[10px] bg-[#FDECEC] px-3 py-2.5 text-[13.5px] text-[#B42318]"
              >
                <Icon name="alert" className="mt-0.5 h-4 w-4" />
                <div className="min-w-0 flex-1">
                  <p className="font-medium">{blocker.reason}</p>
                  <p className="mt-0.5 text-xs text-[#C0352B]/80">
                    Raised {formatDateTime(blocker.created_at)}
                  </p>
                </div>
                <Button size="sm" variant="secondary" disabled={busy} onClick={() => onResolve(blocker.id)}>
                  Resolve
                </Button>
              </div>
            ))}
            {blockerOpen ? (
              <form onSubmit={onRaise} className="flex flex-col gap-2.5">
                <label className="sr-only" htmlFor="drawer-blocker-reason">
                  What is blocking this task?
                </label>
                <TextArea
                  id="drawer-blocker-reason"
                  name="reason"
                  required
                  rows={3}
                  data-autofocus={focus === "blocker" ? true : undefined}
                  placeholder="What is blocking this task, and who can unblock it?"
                />
                <div className="flex flex-wrap gap-2">
                  <Button type="submit" size="sm" disabled={busy}>
                    {busy ? "Saving…" : "Save blocker"}
                  </Button>
                  <Button type="button" size="sm" variant="secondary" onClick={() => setBlockerOpen(false)}>
                    Cancel
                  </Button>
                </div>
              </form>
            ) : (
              <Button size="sm" variant="secondary" onClick={() => setBlockerOpen(true)}>
                {openBlockers.length > 0 ? "Raise another blocker" : "Raise a blocker"}
              </Button>
            )}
            {resolvedBlockers.length > 0 && (
              <p className="mt-3 text-[13px] text-muted">
                {resolvedBlockers.length} resolved blocker{resolvedBlockers.length === 1 ? "" : "s"} on record.
              </p>
            )}
          </>
        )}
      </DrawerSection>

      <DrawerSection title="Evidence" last>
        <div ref={evidenceRef} className="scroll-mt-4">
          {notice && (
            <div className="mb-3">
              <InlineError>{notice}</InlineError>
            </div>
          )}
          {evidence === null ? (
            <p className="text-sm text-muted">Loading…</p>
          ) : (
            <>
              {evidence.length === 0 ? (
                <p className="mb-3 text-[13.5px] text-muted">
                  No evidence yet. A task moves to Done once evidence is attached — a merged
                  pull request, a document, a test result or a client approval.
                </p>
              ) : (
                <ul className="mb-3 flex flex-col gap-2">
                  {evidence.map((item) => (
                    <li
                      key={item.id}
                      className="flex items-center gap-2.5 rounded-control border border-line px-3 py-2.5 text-sm"
                    >
                      <Icon name="check" className="text-[#17803D]" />
                      <span className="min-w-0 flex-1">
                        <span className="block font-medium text-night">
                          {EVIDENCE_LABELS[(item.evidence_type ?? "file") as keyof typeof EVIDENCE_LABELS] ??
                            humanise(item.evidence_type ?? "evidence")}
                        </span>
                        <span className="block truncate text-[13px] text-muted">
                          {item.reference ?? item.description ?? "Uploaded file"}
                        </span>
                      </span>
                      {item.verified_at && <Badge tone="success">verified</Badge>}
                    </li>
                  ))}
                </ul>
              )}
              {evidenceOpen ? (
                <EvidenceForm
                  taskId={task.id}
                  onCancel={() => setEvidenceOpen(false)}
                  onAttached={(attached) => {
                    setEvidence((list) => [attached, ...(list ?? [])]);
                    setEvidenceOpen(false);
                    notify("Evidence attached");
                  }}
                />
              ) : (
                <div className="flex flex-wrap gap-2">
                  <Button size="sm" variant="secondary" onClick={() => setEvidenceOpen(true)}>
                    <Icon name="paperclip" className="h-4 w-4" />
                    Attach evidence
                  </Button>
                  {notice && evidence.length > 0 && task.status !== "done" && (
                    <Button size="sm" onClick={() => onMove(task, "done")}>
                      <Icon name="check" className="h-4 w-4" />
                      Move to Done now
                    </Button>
                  )}
                </div>
              )}
            </>
          )}
        </div>

        <div className="mt-6 border-t border-line pt-4">
          <Link
            href={`/work/tasks/${task.id}`}
            className="inline-flex items-center gap-1.5 text-sm font-semibold text-atlas-ink hover:underline"
          >
            Open the full task page
            <Icon name="arrowRight" className="h-4 w-4" />
          </Link>
        </div>
      </DrawerSection>
    </Drawer>
  );
}
