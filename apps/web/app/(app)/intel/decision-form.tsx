"use client";

import { useRouter } from "next/navigation";
import { useState, type FormEvent } from "react";

import { Drawer } from "@/components/drawer";
import {
  Button,
  Field,
  Select,
  TextArea,
  TextInput,
} from "@/components/form";
import { Icon } from "@/components/icons";
import { PersonSelect } from "@/components/person-select";
import { useToast } from "@/components/toast";
import { cx, ProblemNotice } from "@/components/ui";
import {
  createDecision,
  DECISION_OUTCOMES,
  decideDecision,
  type Decision,
  type DecisionBody,
  updateDecision,
} from "@/lib/api/atlas";
import { humanise } from "@/lib/format";
import { useMutation } from "@/lib/use-mutation";

import type { PersonOption } from "@/lib/directory";

/**
 * Intel decisions (§8.8). Creating takes `intel.decision.manage`; editing and
 * deciding take `intel.decision.manage`. Every write is audited with the
 * actor's name.
 */

/** A text field, trimmed; `""` when the field is absent. */
function field(data: FormData, key: string): string {
  return String(data.get(key) ?? "").trim();
}

/** Build the create/update body from the drawer's form. */
function decisionBody(data: FormData): DecisionBody {
  const decidersRaw = data.getAll("deciders").map(String).filter((v) => v.trim());
  const contextLinksRaw = data.getAll("context_links").map(String).filter((v) => v.trim());
  const optionsRaw = data.getAll("options").map(String).filter((v) => v.trim());
  return {
    question: field(data, "question"),
    ...(contextLinksRaw.length ? { context_links: contextLinksRaw } : {}),
    ...(optionsRaw.length ? { options: optionsRaw } : {}),
    recommendation: field(data, "recommendation"),
    ...(decidersRaw.length ? { deciders: decidersRaw } : {}),
    ...(field(data, "requested_by") ? { requested_by: field(data, "requested_by") } : {}),
    requested_by_name: field(data, "requested_by_name"),
    ...(field(data, "due_on") ? { due_on: field(data, "due_on") } : {}),
    outcome: field(data, "outcome") || "open",
    rationale: field(data, "rationale"),
  };
}

/** The fields shared by "new decision" and "edit decision". */
function DecisionFields({
  decision,
}: {
  decision?: Decision;
}) {
  return (
    <>
      <Field label="Question" htmlFor="decision-question">
        <TextInput
          id="decision-question"
          name="question"
          required
          defaultValue={decision?.question ?? ""}
          placeholder="e.g. Should we renew the school MIS contract?"
          data-autofocus
        />
      </Field>

      <Field label="Recommendation" htmlFor="decision-recommendation">
        <TextArea
          id="decision-recommendation"
          name="recommendation"
          defaultValue={decision?.recommendation ?? ""}
          placeholder="e.g. Approve renewal at current pricing"
        />
      </Field>

      <Field label="Rationale" htmlFor="decision-rationale">
        <TextArea
          id="decision-rationale"
          name="rationale"
          defaultValue={decision?.rationale ?? ""}
          placeholder="Why this decision matters and what was considered."
        />
      </Field>

      <div className="grid grid-cols-1 gap-3.5 sm:grid-cols-2">
        <Field label="Outcome" htmlFor="decision-outcome">
          <Select id="decision-outcome" name="outcome" required>
            {DECISION_OUTCOMES.map((outcome) => (
              <option
                key={outcome}
                value={outcome}
                selected={decision?.outcome === outcome}
              >
                {humanise(outcome)}
              </option>
            ))}
          </Select>
        </Field>
        <Field label="Due on" htmlFor="decision-due" hint="YYYY-MM-DD.">
          <TextInput
            id="decision-due"
            name="due_on"
            type="date"
            defaultValue={decision?.due_on ?? ""}
          />
        </Field>
      </div>

      <Field label="Requested by" htmlFor="decision-requested-by">
        <PersonSelect
          id="decision-requested-by"
          name="requested_by"
          people={[]}
          defaultValue={decision?.requested_by}
          emptyLabel="Unassigned"
        />
      </Field>

      <Field label="Context links" htmlFor="decision-context-links" hint="One or more URLs, comma-separated.">
        <TextInput
          id="decision-context-links"
          name="context_links"
          defaultValue={decision?.context_links.join(", ") ?? ""}
          placeholder="e.g. https://contracts.example.com/123"
        />
      </Field>

      <Field label="Options" htmlFor="decision-options" hint="One or more options, comma-separated.">
        <TextInput
          id="decision-options"
          name="options"
          defaultValue={decision?.options.join(", ") ?? ""}
          placeholder="e.g. Renew, Re-tender, Bring in-house"
        />
      </Field>

      <Field label="Deciders" htmlFor="decision-deciders" hint="One or more person IDs, comma-separated.">
        <TextInput
          id="decision-deciders"
          name="deciders"
          defaultValue={decision?.deciders.join(", ") ?? ""}
          placeholder="e.g. person-id-1, person-id-2"
        />
      </Field>
    </>
  );
}

/**
 * "New decision" (`POST /api/v1/intel/decisions`, `intel.decision.manage`):
 * a button that opens the decision form in a drawer.
 */
export function NewDecisionButton({
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

  if (!capabilities.includes("intel.decision.manage")) return null;

  function close() {
    setOpen(false);
    reset();
  }

  async function onSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const created = await run(() =>
      createDecision(decisionBody(new FormData(event.currentTarget))),
    );
    if (created) {
      setOpen(false);
      notify(`Created decision ${created.ref}`);
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
        <Icon name="sliders" className={variant === "secondary" ? "h-4 w-4" : undefined} />
        New decision
      </Button>
      <Drawer
        open={open}
        onClose={close}
        title="New decision"
        description="Record the question, options, recommendation and who decides."
      >
        <form onSubmit={onSubmit} className="flex flex-col gap-4 px-6 py-[22px]">
          {problem && <ProblemNotice error={problem} title="The decision wasn’t created" />}
          <DecisionFields />
          <div className="flex flex-wrap gap-2.5">
            <Button type="submit" disabled={busy}>
              {busy ? "Creating…" : "Create decision"}
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
 * The actions for one decision row: edit it or record its outcome.
 * `intel.decision.manage`, guarded by `If-Match`.
 */
export function DecisionActions({
  decision,
  people,
  capabilities,
}: {
  decision: Decision;
  people: PersonOption[];
  capabilities: string[];
}) {
  const router = useRouter();
  const notify = useToast();
  const { busy, problem, run, reset } = useMutation();
  const [open, setOpen] = useState(false);
  const [deciding, setDeciding] = useState(false);

  if (!capabilities.includes("intel.decision.manage")) return null;

  function close() {
    setOpen(false);
    reset();
  }

  async function onSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const updated = await run(() =>
      updateDecision(decision.id, decision.version, decisionBody(new FormData(event.currentTarget))),
    );
    if (updated) {
      close();
      notify(`Saved decision ${decision.ref}`);
      router.refresh();
    }
  }

  async function onDecide(outcome: string, rationale: string) {
    setDeciding(true);
    try {
      const updated = await run(() =>
        decideDecision(decision.id, decision.version, { outcome, rationale }),
      );
      if (updated) {
        close();
        notify(`Decision ${updated.ref} ${updated.outcome}`);
        router.refresh();
      }
    } finally {
      setDeciding(false);
    }
  }

  const canDecide = decision.outcome === "open";

  return (
    <div className="flex flex-col items-end gap-2">
      <div className="flex flex-wrap items-center justify-end gap-1.5">
        <Button size="sm" variant="secondary" disabled={busy} onClick={() => setOpen(true)}>
          Edit
        </Button>
        {canDecide && (
          <Button
            size="sm"
            variant="secondary"
            disabled={busy || deciding}
            onClick={() => setOpen(false)}
          >
            Decide
          </Button>
        )}
      </div>

      {problem && (
        <span className={cx("max-w-[240px] text-[12.5px] font-medium text-[#B42318]")}>
          {problem.message}
        </span>
      )}

      <Drawer open={open} onClose={close} title="Edit decision" description={decision.question}>
        <form onSubmit={onSubmit} className="flex flex-col gap-4 px-6 py-[22px]">
          {problem && <ProblemNotice error={problem} title="The change wasn’t saved" />}
          <DecisionFields decision={decision} />
          <div className="flex flex-wrap gap-2.5">
            <Button type="submit" disabled={busy}>
              {busy ? "Saving…" : "Save decision"}
            </Button>
            <Button type="button" variant="secondary" onClick={close} disabled={busy}>
              Cancel
            </Button>
          </div>
        </form>
      </Drawer>

      {canDecide && (
        <Drawer
          open={deciding}
          onClose={() => setDeciding(false)}
          title={`Decide: ${decision.question}`}
          description={`Record the outcome for ${decision.ref}.`}
        >
          <form onSubmit={(e) => { e.preventDefault(); }} className="flex flex-col gap-4 px-6 py-[22px]">
            {problem && <ProblemNotice error={problem} title="The decision wasn’t updated" />}
            <Field label="Outcome" htmlFor="decide-outcome">
              <Select
                id="decide-outcome"
                name="outcome"
                required
                defaultValue="open"
              >
                {DECISION_OUTCOMES.map((outcome) => (
                  <option key={outcome} value={outcome}>
                    {humanise(outcome)}
                  </option>
                ))}
              </Select>
            </Field>
            <Field label="Rationale" htmlFor="decide-rationale">
              <TextArea
                id="decide-rationale"
                name="rationale"
                placeholder="Why this outcome was reached."
              />
            </Field>
            <div className="flex flex-wrap gap-2.5">
              <Button
                type="submit"
                variant="primary"
                disabled={busy || deciding}
                onClick={(e) => {
                  e.preventDefault();
                  const form = (e.currentTarget as HTMLButtonElement).form;
                  if (!form) return;
                  const formData = new FormData(form);
                  void onDecide(field(formData, "outcome"), field(formData, "rationale"));
                }}
              >
                {busy || deciding ? "Deciding…" : "Record decision"}
              </Button>
              <Button
                type="button"
                variant="secondary"
                onClick={() => setDeciding(false)}
                disabled={busy || deciding}
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
