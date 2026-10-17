"use client";

import { useRouter } from "next/navigation";
import { useState, type FormEvent } from "react";

import { Button, Field, TextArea } from "@/components/form";
import { Card, EmptyState, ProblemNotice } from "@/components/ui";
import { raiseBlocker, resolveBlocker, type Blocker } from "@/lib/api/atlas";
import { formatDateTime } from "@/lib/format";
import { useMutation } from "@/lib/use-mutation";

/**
 * A task's blockers (`/api/v1/work/tasks/{id}/blockers`). Raising needs
 * `work.blocker.manage` and is recorded against the caller; a task with an open
 * blocker is marked blocked by Go.
 */
export function BlockerPanel({
  taskId,
  blockers,
}: {
  taskId: string;
  blockers: Blocker[];
}) {
  const router = useRouter();
  const { busy, problem, run } = useMutation();
  const [openForm, setOpenForm] = useState(false);

  async function onSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = event.currentTarget;
    const data = new FormData(form);
    const created = await run(() =>
      raiseBlocker(taskId, String(data.get("reason") ?? "").trim()),
    );
    if (created) {
      form.reset();
      setOpenForm(false);
      router.refresh();
    }
  }

  async function onResolve(id: string) {
    const resolved = await run(() => resolveBlocker(id));
    if (resolved) router.refresh();
  }

  const open = blockers.filter((blocker) => !blocker.resolved_at);
  const resolved = blockers.filter((blocker) => blocker.resolved_at);

  return (
    <Card
      title="Blockers"
      description="A blocker holds the task; resolving it releases it."
      actions={
        <Button
          variant={openForm ? "secondary" : "primary"}
          onClick={() => setOpenForm((value) => !value)}
        >
          {openForm ? "Cancel" : "Raise blocker"}
        </Button>
      }
    >
      {problem && (
        <div className="mb-3">
          <ProblemNotice error={problem} title="Blocker change failed" />
        </div>
      )}

      {openForm && (
        <form onSubmit={onSubmit} className="mb-4 space-y-3">
          <Field label="What is blocking it?" htmlFor="blocker-reason">
            <TextArea
              id="blocker-reason"
              name="reason"
              required
              rows={2}
              placeholder="Waiting on the client's API credentials…"
            />
          </Field>
          <Button type="submit" disabled={busy}>
            {busy ? "Raising…" : "Raise blocker"}
          </Button>
        </form>
      )}

      {blockers.length === 0 ? (
        <EmptyState>No blockers have been raised.</EmptyState>
      ) : (
        <ul className="space-y-2">
          {open.map((blocker) => (
            <li
              key={blocker.id}
              className="flex items-start justify-between gap-3 rounded-lg border border-amber-200 bg-amber-50 px-3 py-2"
            >
              <div>
                <p className="text-sm text-amber-900">{blocker.reason}</p>
                <p className="mt-0.5 text-xs text-amber-700">
                  Raised {formatDateTime(blocker.created_at)}
                </p>
              </div>
              <Button variant="secondary" disabled={busy} onClick={() => onResolve(blocker.id)}>
                Resolve
              </Button>
            </li>
          ))}

          {resolved.map((blocker) => (
            <li
              key={blocker.id}
              className="flex items-start justify-between gap-3 rounded-lg border border-slate-200 bg-slate-50 px-3 py-2"
            >
              <div>
                <p className="text-sm text-slate-600 line-through">{blocker.reason}</p>
                <p className="mt-0.5 text-xs text-slate-500">
                  Resolved {formatDateTime(blocker.resolved_at)}
                </p>
              </div>
            </li>
          ))}
        </ul>
      )}
    </Card>
  );
}
