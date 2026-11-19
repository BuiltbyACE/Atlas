"use client";

import { useRouter } from "next/navigation";
import { useEffect, useRef, useState } from "react";

import { Button } from "@/components/form";
import { PersonSelect } from "@/components/person-select";
import { useToast } from "@/components/toast";
import { taskBody, updateTask, type Task } from "@/lib/api/atlas";
import type { PersonOption } from "@/lib/directory";
import { useMutation } from "@/lib/use-mutation";

/**
 * Give an unowned task an owner from the workload view: a small popover with
 * the people picker. It is the same `PUT /api/v1/work/tasks/{id}` the board
 * uses, carrying the version it read.
 */
export function AssignButton({
  task,
  people,
  me,
}: {
  task: Task;
  people: PersonOption[];
  me?: string;
}) {
  const router = useRouter();
  const notify = useToast();
  const { busy, problem, run } = useMutation();
  const [open, setOpen] = useState(false);
  const [person, setPerson] = useState(me && people.some((p) => p.id === me) ? me : "");
  const root = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!open) return;
    const onPointer = (event: PointerEvent) => {
      if (!root.current?.contains(event.target as Node)) setOpen(false);
    };
    const onKey = (event: KeyboardEvent) => {
      if (event.key === "Escape") setOpen(false);
    };
    document.addEventListener("pointerdown", onPointer);
    document.addEventListener("keydown", onKey);
    return () => {
      document.removeEventListener("pointerdown", onPointer);
      document.removeEventListener("keydown", onKey);
    };
  }, [open]);

  async function assign() {
    if (!person) return;
    const updated = await run(() =>
      updateTask(task.id, taskBody(task, { assignee_person_id: person }), task.version),
    );
    if (updated) {
      setOpen(false);
      notify(`Assigned “${task.title}”`);
      router.refresh();
    }
  }

  return (
    <div ref={root} className="relative">
      <Button size="sm" variant="secondary" onClick={() => setOpen((value) => !value)} aria-expanded={open}>
        Assign
      </Button>
      {open && (
        <div
          role="dialog"
          aria-label={`Assign ${task.title}`}
          className="absolute right-0 top-[calc(100%+6px)] z-20 flex w-[280px] animate-fade-in flex-col gap-2.5 rounded-[14px] border border-line bg-white p-3 shadow-pop"
        >
          <label htmlFor={`assign-${task.id}`} className="text-sm font-semibold text-night">
            Owner for “{task.title}”
          </label>
          <PersonSelect
            id={`assign-${task.id}`}
            people={people}
            me={me}
            value={person}
            onChange={setPerson}
            emptyLabel="Choose a person"
          />
          {problem && <p className="text-[13px] font-medium text-[#B42318]">{problem.message}</p>}
          <div className="flex gap-2">
            <Button size="sm" onClick={assign} disabled={busy || !person}>
              {busy ? "Assigning…" : "Assign"}
            </Button>
            <Button size="sm" variant="secondary" onClick={() => setOpen(false)}>
              Cancel
            </Button>
          </div>
        </div>
      )}
    </div>
  );
}
