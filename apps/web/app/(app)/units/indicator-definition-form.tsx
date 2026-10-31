"use client";

import { useRouter } from "next/navigation";
import { useState, type FormEvent } from "react";

import { Drawer } from "@/components/drawer";
import {
  Button,
  Field,
  Select,
  Switch,
  TextArea,
  TextInput,
} from "@/components/form";
import { Icon } from "@/components/icons";
import { useToast } from "@/components/toast";
import { cx, ProblemNotice } from "@/components/ui";
import {
  createIndicatorDefinition,
  INDICATOR_KINDS,
  type UnitIndicatorDefinition,
  type IndicatorDefinitionBody,
  updateIndicatorDefinition,
} from "@/lib/api/atlas";
import { humanise } from "@/lib/format";
import { useMutation } from "@/lib/use-mutation";

import type { PersonOption } from "@/lib/directory";

/**
 * Unit indicator definitions (§8.7). Creating takes `units.unit.manage`;
 * editing takes `units.unit.manage`.
 */

/** A text field, trimmed; `""` when the field is absent. */
function field(data: FormData, key: string): string {
  return String(data.get(key) ?? "").trim();
}

/** Build the create/update body from the drawer's form. */
function indicatorDefinitionBody(data: FormData): IndicatorDefinitionBody {
  return {
    unit_id: field(data, "unit_id"),
    name: field(data, "name"),
    kind: field(data, "kind"),
    source_field: field(data, "source_field"),
    formula: field(data, "formula"),
    target: field(data, "target"),
    warning_threshold: field(data, "warning_threshold"),
    critical_threshold: field(data, "critical_threshold"),
    active: data.get("active") !== null,
  };
}

/** The fields shared by "new indicator definition" and "edit indicator definition". */
function IndicatorDefinitionFields({
  definition,
  units,
}: {
  definition?: UnitIndicatorDefinition;
  units: { id: string; name: string }[];
}) {
  return (
    <>
      <Field label="Unit" htmlFor="indicator-unit">
        <Select id="indicator-unit" name="unit_id" required>
          <option value="">Pick a unit</option>
          {units.map((u) => (
            <option key={u.id} value={u.id} selected={definition?.unit_id === u.id}>
              {u.name}
            </option>
          ))}
        </Select>
      </Field>

      <Field label="Name" htmlFor="indicator-name">
        <TextInput
          id="indicator-name"
          name="name"
          required
          defaultValue={definition?.name ?? ""}
          placeholder="e.g. Fee collection rate"
          data-autofocus
        />
      </Field>

      <div className="grid grid-cols-1 gap-3.5 sm:grid-cols-2">
        <Field label="Kind" htmlFor="indicator-kind">
          <Select id="indicator-kind" name="kind" required>
            {INDICATOR_KINDS.map((kind) => (
              <option key={kind} value={kind} selected={definition?.kind === kind}>
                {humanise(kind)}
              </option>
            ))}
          </Select>
        </Field>
        <Field label="Active" htmlFor="indicator-active">
          <Switch name="active" defaultChecked={definition?.active ?? true} label="Active" />
        </Field>
      </div>

      <Field label="Source field" htmlFor="indicator-source" hint="The source field or formula input in the unit's system of record.">
        <TextInput
          id="indicator-source"
          name="source_field"
          required
          defaultValue={definition?.source_field ?? ""}
          placeholder="e.g. fees.collected"
        />
      </Field>

      <Field label="Formula" htmlFor="indicator-formula" hint="Optional formula used to derive the value.">
        <TextInput
          id="indicator-formula"
          name="formula"
          defaultValue={definition?.formula ?? ""}
          placeholder="collected / billed * 100"
        />
      </Field>

      <Field label="Target" htmlFor="indicator-target" hint="Target value for the indicator.">
        <TextInput
          id="indicator-target"
          name="target"
          defaultValue={definition?.target ?? ""}
          placeholder="e.g. 95%"
        />
      </Field>

      <div className="grid grid-cols-1 gap-3.5 sm:grid-cols-2">
        <Field label="Warning threshold" htmlFor="indicator-warning" hint="Threshold at which a warning is raised.">
          <TextInput
            id="indicator-warning"
            name="warning_threshold"
            defaultValue={definition?.warning_threshold ?? ""}
            placeholder="e.g. 85%"
          />
        </Field>
        <Field label="Critical threshold" htmlFor="indicator-critical" hint="Threshold at which a critical alert is raised.">
          <TextInput
            id="indicator-critical"
            name="critical_threshold"
            defaultValue={definition?.critical_threshold ?? ""}
            placeholder="e.g. 70%"
          />
        </Field>
      </div>
    </>
  );
}

/**
 * "New indicator definition" (`POST /api/v1/units/indicator-definitions`,
 * `units.unit.manage`): a button that opens the indicator definition form in a
 * drawer.
 */
export function NewIndicatorDefinitionButton({
  units,
  people,
  me,
  capabilities,
  variant = "primary",
}: {
  units: { id: string; name: string }[];
  people: PersonOption[];
  me?: string;
  capabilities: string[];
  variant?: "primary" | "secondary";
}) {
  const router = useRouter();
  const notify = useToast();
  const { busy, problem, run, reset } = useMutation();
  const [open, setOpen] = useState(false);

  if (!capabilities.includes("units.unit.manage")) return null;

  function close() {
    setOpen(false);
    reset();
  }

  async function onSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const created = await run(() =>
      createIndicatorDefinition(
        indicatorDefinitionBody(new FormData(event.currentTarget)),
      ),
    );
    if (created) {
      setOpen(false);
      notify(`Created indicator ${created.name}`);
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
        New indicator
      </Button>
      <Drawer
        open={open}
        onClose={close}
        title="New indicator definition"
        description="What we measure for each unit — name, formula, target, thresholds."
      >
        <form onSubmit={onSubmit} className="flex flex-col gap-4 px-6 py-[22px]">
          {problem && <ProblemNotice error={problem} title="The indicator definition wasn’t created" />}
          <IndicatorDefinitionFields units={units} />
          <div className="flex flex-wrap gap-2.5">
            <Button type="submit" disabled={busy}>
              {busy ? "Creating…" : "Create indicator"}
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
 * The actions for one indicator definition row: edit its definition.
 * `units.unit.manage`, guarded by `If-Match`.
 */
export function IndicatorDefinitionActions({
  definition,
  units,
  people,
  me,
  capabilities,
}: {
  definition: UnitIndicatorDefinition;
  units: { id: string; name: string }[];
  people: PersonOption[];
  me?: string;
  capabilities: string[];
}) {
  const router = useRouter();
  const notify = useToast();
  const { busy, problem, run, reset } = useMutation();
  const [open, setOpen] = useState(false);

  if (!capabilities.includes("units.unit.manage")) return null;

  function close() {
    setOpen(false);
    reset();
  }

  async function onSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const updated = await run(() =>
      updateIndicatorDefinition(
        definition.id,
        definition.version,
        indicatorDefinitionBody(new FormData(event.currentTarget)),
      ),
    );
    if (updated) {
      close();
      notify(`Saved ${updated.name}`);
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

      <Drawer
        open={open}
        onClose={close}
        title="Edit indicator definition"
        description={definition.name}
      >
        <form onSubmit={onSubmit} className="flex flex-col gap-4 px-6 py-[22px]">
          {problem && <ProblemNotice error={problem} title="The change wasn’t saved" />}
          <IndicatorDefinitionFields definition={definition} units={units} />
          <div className="flex flex-wrap gap-2.5">
            <Button type="submit" disabled={busy}>
              {busy ? "Saving…" : "Save indicator"}
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
