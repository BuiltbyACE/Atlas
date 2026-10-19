"use client";

import { useRouter } from "next/navigation";
import { useState, type FormEvent } from "react";

import { Button, Field, TextArea } from "@/components/form";
import { Icon } from "@/components/icons";
import { useToast } from "@/components/toast";
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
  const notify = useToast();
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
      notify("Blocker raised");
      router.refresh();
    }
  }

  async function onResolve(id: string) {
    const resolved = await run(() => resolveBlocker(id));
    if (resolved) {
      notify("Blocker resolved");
      router.refresh();
    }
  }

  const open = blockers.filter((blocker) => !blocker.resolved_at);
  const resolved = blockers.filter((blocker) => blocker.resolved_at);

  return (
    <Card
      title="Blockers"
      description="A blocker holds the task; resolving it releases it."
      actions={
        !openForm && (
          <Button size="sm" variant="secondary" onClick={() => setOpenForm(true)}>
            Raise blocker
          </Button>
        )
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
          <div className="flex flex-wrap gap-2">
            <Button type="submit" size="sm" disabled={busy}>
              {busy ? "Raising…" : "Raise blocker"}
            </Button>
            <Button type="button" size="sm" variant="secondary" onClick={() => setOpenForm(false)}>
              Cancel
            </Button>
          </div>
        </form>
      )}

      {blockers.length === 0 ? (
        !openForm && (
          <EmptyState icon="check" title="Nothing blocking this task">
            Raise a blocker when something outside the task is holding it up.
          </EmptyState>
        )
      ) : (
        <ul className="space-y-2">
          {open.map((blocker) => (
            <li
              key={blocker.id}
              className="flex items-start gap-2.5 rounded-control bg-[#FDECEC] px-3 py-2.5 text-[#B42318]"
            >
              <Icon name="alert" className="mt-0.5 h-4 w-4" />
              <div className="min-w-0 flex-1">
                <p className="text-sm font-medium">{blocker.reason}</p>
                <p className="mt-0.5 text-xs text-[#C0352B]/80">
                  Raised {formatDateTime(blocker.created_at)}
                </p>
              </div>
              <Button size="sm" variant="secondary" disabled={busy} onClick={() => onResolve(blocker.id)}>
                Resolve
              </Button>
            </li>
          ))}

          {resolved.map((blocker) => (
            <li
              key={blocker.id}
              className="flex items-start gap-2.5 rounded-control border border-line bg-[#FAFBFC] px-3 py-2.5"
            >
              <Icon name="check" className="mt-0.5 h-4 w-4 text-[#17803D]" />
              <div className="min-w-0">
                <p className="text-sm text-muted line-through">{blocker.reason}</p>
                <p className="mt-0.5 text-xs text-[#7A879C]">
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
