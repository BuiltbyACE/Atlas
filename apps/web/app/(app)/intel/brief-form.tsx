"use client";

import { useRouter } from "next/navigation";
import { useState, type FormEvent } from "react";

import { Drawer } from "@/components/drawer";
import {
  Button,
  Field,
  TextArea,
  TextInput,
} from "@/components/form";
import { Icon } from "@/components/icons";
import { useToast } from "@/components/toast";
import { cx, ProblemNotice } from "@/components/ui";
import {
  createBrief,
  deliverBrief,
  type Brief,
  type BriefBody,
  updateBrief,
} from "@/lib/api/atlas";
import { formatDay, humanise } from "@/lib/format";
import { useMutation } from "@/lib/use-mutation";

import type { PersonOption } from "@/lib/directory";

/**
 * Intel briefs (§8.8). Creating takes `intel.brief.manage`; editing and
 * delivering take `intel.brief.manage`. Every write is audited with the
 * actor's name.
 */

/** A text field, trimmed; `""` when the field is absent. */
function field(data: FormData, key: string): string {
  return String(data.get(key) ?? "").trim();
}

/** Build the create/update body from the drawer's form. */
function briefBody(data: FormData): BriefBody {
  let sections: Record<string, any> = {};
  const sectionsRaw = field(data, "sections");
  if (sectionsRaw) {
    try {
      sections = JSON.parse(sectionsRaw);
    } catch {
      sections = {};
    }
  }
  return {
    ...(field(data, "for_date") ? { for_date: field(data, "for_date") } : {}),
    summary: field(data, "summary"),
    sections,
  };
}

/** The fields shared by "new brief" and "edit brief". */
function BriefFields({
  brief,
}: {
  brief?: Brief;
}) {
  return (
    <>
      <Field label="For date" htmlFor="brief-for-date" hint="YYYY-MM-DD.">
        <TextInput
          id="brief-for-date"
          name="for_date"
          type="date"
          defaultValue={brief?.for_date ?? ""}
          data-autofocus
        />
      </Field>

      <Field label="Summary" htmlFor="brief-summary">
        <TextArea
          id="brief-summary"
          name="summary"
          defaultValue={brief?.summary ?? ""}
          placeholder="Yesterday: won/lost deals, payments received, tasks completed, incidents."
        />
      </Field>

      <Field label="Sections" htmlFor="brief-sections" hint="JSON object of brief sections, e.g. needs_attention: [...], today: [].">
        <TextArea
          id="brief-sections"
          name="sections"
          defaultValue={brief?.sections ? JSON.stringify(brief.sections, null, 2) : ""}
          placeholder='{"needs_attention": [], "today": []}'
        />
      </Field>
    </>
  );
}

/**
 * "New brief" (`POST /api/v1/intel/briefs`, `intel.brief.manage`): a button
 * that opens the brief form in a drawer.
 */
export function NewBriefButton({
  people,
  me,
  capabilities,
  variant = "primary",
}: {
  people: PersonOption[];
  me?: string;
  capabilities: string[];
  variant?: "primary" | "secondary";
}) {
  const router = useRouter();
  const notify = useToast();
  const { busy, problem, run, reset } = useMutation();
  const [open, setOpen] = useState(false);

  if (!capabilities.includes("intel.brief.manage")) return null;

  function close() {
    setOpen(false);
    reset();
  }

  async function onSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const created = await run(() => createBrief(briefBody(new FormData(event.currentTarget))));
    if (created) {
      setOpen(false);
      notify(`Created brief ${created.ref}`);
      router.refresh();
    }
  }

  return (
    <>
      <Button
        variant={variant}
        size={variant === "secondary" ? "sm" : "md"}
        onClick={() => setOpen(true)}
      >
        <Icon name="calendar" className={variant === "secondary" ? "h-4 w-4" : undefined} />
        New brief
      </Button>
      <Drawer
        open={open}
        onClose={close}
        title="New brief"
        description="Daily executive brief — summary and structured sections."
      >
        <form onSubmit={onSubmit} className="flex flex-col gap-4 px-6 py-[22px]">
          {problem && <ProblemNotice error={problem} title="The brief wasn’t created" />}
          <BriefFields />
          <div className="flex flex-wrap gap-2.5">
            <Button type="submit" disabled={busy}>
              {busy ? "Creating…" : "Create brief"}
            </Button>
            <Button type="button" variant="secondary" onClick={close} disabled={busy}>
              Cancel
            </Button>
          </div>
        </form>
      </Drawer>
    </>
  );
}

/**
 * The actions for one brief row: edit it or mark it as delivered.
 * `intel.brief.manage`, guarded by `If-Match`.
 */
export function BriefActions({
  brief,
  people,
  capabilities,
}: {
  brief: Brief;
  people: PersonOption[];
  capabilities: string[];
}) {
  const router = useRouter();
  const notify = useToast();
  const { busy, problem, run, reset } = useMutation();
  const [open, setOpen] = useState(false);
  const [delivering, setDelivering] = useState(false);

  if (!capabilities.includes("intel.brief.manage")) return null;

  function close() {
    setOpen(false);
    reset();
  }

  async function onSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const updated = await run(() =>
      updateBrief(brief.id, brief.version, briefBody(new FormData(event.currentTarget))),
    );
    if (updated) {
      close();
      notify(`Saved brief ${brief.ref}`);
      router.refresh();
    }
  }

  async function onDeliver() {
    setDelivering(true);
    try {
      const updated = await run(() => deliverBrief(brief.id, brief.version));
      if (updated) {
        close();
        notify(`Brief ${brief.ref} delivered`);
        router.refresh();
      }
    } finally {
      setDelivering(false);
    }
  }

  const canDeliver = !brief.delivered;

  return (
    <div className="flex flex-col items-end gap-2">
      <div className="flex flex-wrap items-center justify-end gap-1.5">
        <Button size="sm" variant="secondary" disabled={busy} onClick={() => setOpen(true)}>
          Edit
        </Button>
        {canDeliver && (
          <Button
            size="sm"
            variant="secondary"
            disabled={busy || delivering}
            onClick={() => {
              setDelivering(true);
              void onDeliver();
            }}
          >
            Deliver
          </Button>
        )}
      </div>

      {problem && (
        <span className={cx("max-w-[240px] text-[12.5px] font-medium text-[#B42318]")}>
          {problem.message}
        </span>
      )}

      <Drawer open={open} onClose={close} title="Edit brief" description={brief.ref}>
        <form onSubmit={onSubmit} className="flex flex-col gap-4 px-6 py-[22px]">
          {problem && <ProblemNotice error={problem} title="The change wasn’t saved" />}
          <BriefFields brief={brief} />
          <div className="flex flex-wrap gap-2.5">
            <Button type="submit" disabled={busy}>
              {busy ? "Saving…" : "Save brief"}
            </Button>
            <Button type="button" variant="secondary" onClick={close} disabled={busy}>
              Cancel
            </Button>
          </div>
        </form>
      </Drawer>

      {canDeliver && (
        <Drawer
          open={delivering}
          onClose={() => setDelivering(false)}
          title={`Deliver brief: ${brief.ref}`}
          description="Mark this brief as delivered to executives."
        >
          <form
            onSubmit={(e) => {
              e.preventDefault();
              void onDeliver();
            }}
            className="flex flex-col gap-4 px-6 py-[22px]"
          >
            <p className="text-[13.5px] text-muted">
              Marking as delivered records that the executive brief has been
              sent to the CEOs. This cannot be undone.
            </p>
            <div className="flex flex-wrap gap-2.5">
              <Button
                type="submit"
                variant="primary"
                disabled={busy || delivering}
              >
                {busy || delivering ? "Delivering…" : "Mark delivered"}
              </Button>
              <Button
                type="button"
                variant="secondary"
                onClick={() => setDelivering(false)}
                disabled={busy || delivering}
              >
                Cancel
              </Button>
            </div>
          </form>
        </Drawer>
      )}
    </div>
  );
}
