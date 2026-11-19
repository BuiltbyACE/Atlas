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
import { PersonSelect } from "@/components/person-select";
import { Icon } from "@/components/icons";
import { useToast } from "@/components/toast";
import { cx, ProblemNotice } from "@/components/ui";
import {
  createRisk,
  RISK_IMPACTS,
  RISK_KINDS,
  RISK_LIKELIHOODS,
  RISK_STATUSES,
  type Risk,
  type RiskBody,
  updateRisk,
} from "@/lib/api/atlas";
import { humanise } from "@/lib/format";
import { useMutation } from "@/lib/use-mutation";

import type { PersonOption } from "@/lib/directory";

/**
 * Intel risks (§8.8). Creating takes `intel.risk.manage`; editing takes
 * `intel.risk.manage`. Every write is audited with the actor's name.
 */

/** A text field, trimmed; `""` when the field is absent. */
function field(data: FormData, key: string): string {
  return String(data.get(key) ?? "").trim();
}

/** Build the create/update body from the drawer's form. */
function riskBody(data: FormData): RiskBody {
  const owner = field(data, "owner_person_id");
  return {
    title: field(data, "title"),
    kind: field(data, "kind"),
    ...(owner ? { owner_person_id: owner } : {}),
    likelihood: field(data, "likelihood"),
    impact: field(data, "impact"),
    mitigation: field(data, "mitigation"),
    status: field(data, "status") || "open",
  };
}

/** The fields shared by "new risk" and "edit risk". */
function RiskFields({
  risk,
}: {
  risk?: Risk;
}) {
  return (
    <>
      <Field label="Title" htmlFor="risk-title">
        <TextInput
          id="risk-title"
          name="title"
          required
          defaultValue={risk?.title ?? ""}
          placeholder="e.g. School MIS vendor may not renew contract"
          data-autofocus
        />
      </Field>

      <div className="grid grid-cols-1 gap-3.5 sm:grid-cols-3">
        <Field label="Kind" htmlFor="risk-kind">
          <Select id="risk-kind" name="kind" required>
            {RISK_KINDS.map((kind) => (
              <option key={kind} value={kind} selected={risk?.kind === kind}>
                {humanise(kind)}
              </option>
            ))}
          </Select>
        </Field>
        <Field label="Likelihood" htmlFor="risk-likelihood">
          <Select id="risk-likelihood" name="likelihood" required>
            {RISK_LIKELIHOODS.map((likelihood) => (
              <option
                key={likelihood}
                value={likelihood}
                selected={risk?.likelihood === likelihood}
              >
                {humanise(likelihood)}
              </option>
            ))}
          </Select>
        </Field>
        <Field label="Impact" htmlFor="risk-impact">
          <Select id="risk-impact" name="impact" required>
            {RISK_IMPACTS.map((impact) => (
              <option key={impact} value={impact} selected={risk?.impact === impact}>
                {humanise(impact)}
              </option>
            ))}
          </Select>
        </Field>
        <Field label="Status" htmlFor="risk-status">
          <Select id="risk-status" name="status">
            {RISK_STATUSES.map((status) => (
              <option key={status} value={status} selected={risk?.status === status}>
                {humanise(status)}
              </option>
            ))}
          </Select>
        </Field>
      </div>

      <Field label="Owner" htmlFor="risk-owner" hint="Person responsible for managing this risk.">
        <PersonSelect
          id="risk-owner"
          name="owner_person_id"
          people={[]}
          defaultValue={risk?.owner?.id}
          emptyLabel="Unassigned"
        />
      </Field>

      <Field label="Mitigation" htmlFor="risk-mitigation">
        <TextArea
          id="risk-mitigation"
          name="mitigation"
          defaultValue={risk?.mitigation ?? ""}
          placeholder="What is being done to reduce likelihood or impact."
        />
      </Field>
    </>
  );
}

/**
 * "New risk" (`POST /api/v1/intel/risks`, `intel.risk.manage`): a button that
 * opens the risk form in a drawer.
 */
export function NewRiskButton({
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

  if (!capabilities.includes("intel.risk.manage")) return null;

  function close() {
    setOpen(false);
    reset();
  }

  async function onSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const created = await run(() => createRisk(riskBody(new FormData(event.currentTarget))));
    if (created) {
      setOpen(false);
      notify(`Created risk ${created.ref}`);
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
        <Icon name="alert" className={variant === "secondary" ? "h-4 w-4" : undefined} />
        New risk
      </Button>
      <Drawer
        open={open}
        onClose={close}
        title="New risk"
        description="Client, delivery, financial, people or systems risk with owner, likelihood and impact."
      >
        <form onSubmit={onSubmit} className="flex flex-col gap-4 px-6 py-[22px]">
          {problem && <ProblemNotice error={problem} title="The risk wasn’t created" />}
          <RiskFields />
          <div className="flex flex-wrap gap-2.5">
            <Button type="submit" disabled={busy}>
              {busy ? "Creating…" : "Create risk"}
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
 * The actions for one risk row: edit its record. `intel.risk.manage`, guarded
 * by `If-Match`.
 */
export function RiskActions({
  risk,
  people,
  capabilities,
}: {
  risk: Risk;
  people: PersonOption[];
  capabilities: string[];
}) {
  const router = useRouter();
  const notify = useToast();
  const { busy, problem, run, reset } = useMutation();
  const [open, setOpen] = useState(false);

  if (!capabilities.includes("intel.risk.manage")) return null;

  function close() {
    setOpen(false);
    reset();
  }

  async function onSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const updated = await run(() =>
      updateRisk(risk.id, risk.version, riskBody(new FormData(event.currentTarget))),
    );
    if (updated) {
      close();
      notify(`Saved risk ${updated.ref}`);
      router.refresh();
    }
  }

  return (
    <div className="flex flex-col items-end gap-2">
      <div className="flex flex-wrap items-center justify-end gap-1.5">
        <Button size="sm" variant="secondary" disabled={busy} onClick={() => setOpen(true)}>
          Edit
        </Button>
      </div>

      {problem && (
        <span className={cx("max-w-[240px] text-[12.5px] font-medium text-[#B42318]")}>
          {problem.message}
        </span>
      )}

      <Drawer open={open} onClose={close} title="Edit risk" description={risk.title}>
        <form onSubmit={onSubmit} className="flex flex-col gap-4 px-6 py-[22px]">
          {problem && <ProblemNotice error={problem} title="The change wasn’t saved" />}
          <RiskFields risk={risk} />
          <div className="flex flex-wrap gap-2.5">
            <Button type="submit" disabled={busy}>
              {busy ? "Saving…" : "Save risk"}
            </Button>
            <Button type="button" variant="secondary" onClick={close} disabled={busy}>
              Cancel
            </Button>
          </div>
        </form>
      </Drawer>
    </div>
  );
}
