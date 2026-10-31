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
import { PersonSelect } from "@/components/person-select";
import { Icon } from "@/components/icons";
import { useToast } from "@/components/toast";
import { cx, ProblemNotice } from "@/components/ui";
import {
  createUnit,
  UNIT_KINDS,
  UNIT_STATUSES,
  type Unit,
  type UnitBody,
  updateUnit,
} from "@/lib/api/atlas";
import { humanise } from "@/lib/format";
import { useMutation } from "@/lib/use-mutation";

import type { PersonOption } from "@/lib/directory";

/**
 * Units (§8.7). Creating a unit takes `units.unit.manage`; editing takes
 * `units.unit.manage`. Every write is audited with the actor's name.
 */

/** A text field, trimmed; `""` when the field is absent. */
function field(data: FormData, key: string): string {
  return String(data.get(key) ?? "").trim();
}

/** Build the create/update body from the drawer's form. */
function unitBody(data: FormData): UnitBody {
  const owner = field(data, "owner_person_id");
  return {
    name: field(data, "name"),
    code: field(data, "code"),
    kind: field(data, "kind"),
    ...(owner ? { owner_person_id: owner } : {}),
    description: field(data, "description"),
    status: field(data, "status") || "active",
  };
}

/** The fields shared by "new unit" and "edit unit". */
function UnitFields({
  unit,
}: {
  unit?: Unit;
}) {
  return (
    <>
      <Field label="Name" htmlFor="unit-name">
        <TextInput
          id="unit-name"
          name="name"
          required
          defaultValue={unit?.name ?? ""}
          placeholder="e.g. School of the Future"
          data-autofocus
        />
      </Field>

      <Field label="Code" htmlFor="unit-code" hint="Stable handle, lowercase letters, digits and dashes.">
        <TextInput
          id="unit-code"
          name="code"
          required
          defaultValue={unit?.code ?? ""}
          placeholder="e.g. school-primary"
        />
      </Field>

      <div className="grid grid-cols-1 gap-3.5 sm:grid-cols-2">
        <Field label="Kind" htmlFor="unit-kind">
          <Select id="unit-kind" name="kind" required>
            {UNIT_KINDS.map((kind) => (
              <option key={kind} value={kind} selected={unit?.kind === kind}>
                {humanise(kind)}
              </option>
            ))}
          </Select>
        </Field>
        <Field label="Status" htmlFor="unit-status">
          <Select id="unit-status" name="status">
            {UNIT_STATUSES.map((status) => (
              <option key={status} value={status} selected={unit?.status === status}>
                {humanise(status)}
              </option>
            ))}
          </Select>
        </Field>
      </div>

      <Field label="Owner" htmlFor="unit-owner" hint="Person who manages this unit.">
        <PersonSelect
          id="unit-owner"
          name="owner_person_id"
          people={[]}
          defaultValue={unit?.owner?.id}
          emptyLabel="Unassigned"
        />
      </Field>

      <Field label="Description" htmlFor="unit-description">
        <TextArea
          id="unit-description"
          name="description"
          defaultValue={unit?.description ?? ""}
          placeholder="What this unit does and who to contact."
        />
      </Field>
    </>
  );
}

/**
 * "New unit" (`POST /api/v1/units`, `units.unit.manage`): a button that opens
 * the unit form in a drawer.
 */
export function NewUnitButton({
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

  if (!capabilities.includes("units.unit.manage")) return null;

  function close() {
    setOpen(false);
    reset();
  }

  async function onSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const created = await run(() => createUnit(unitBody(new FormData(event.currentTarget))));
    if (created) {
      setOpen(false);
      notify(`Created unit ${created.name}`);
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
        <Icon name="plus" className={variant === "secondary" ? "h-4 w-4" : undefined} />
        New unit
      </Button>
      <Drawer
        open={open}
        onClose={close}
        title="New unit"
        description="A business unit is a scope — unit managers see theirs, CEOs see all."
      >
        <form onSubmit={onSubmit} className="flex flex-col gap-4 px-6 py-[22px]">
          {problem && <ProblemNotice error={problem} title="The unit wasn’t created" />}
          <UnitFields />
          <div className="flex flex-wrap gap-2.5">
            <Button type="submit" disabled={busy}>
              {busy ? "Creating…" : "Create unit"}
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
 * The actions for one unit row: edit its metadata. `units.unit.manage`, guarded
 * by `If-Match`.
 */
export function UnitActions({
  unit,
  people,
  capabilities,
}: {
  unit: Unit;
  people: PersonOption[];
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
      updateUnit(unit.id, unit.version, unitBody(new FormData(event.currentTarget))),
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

      <Drawer open={open} onClose={close} title="Edit unit" description={unit.name}>
        <form onSubmit={onSubmit} className="flex flex-col gap-4 px-6 py-[22px]">
          {problem && <ProblemNotice error={problem} title="The change wasn’t saved" />}
          <UnitFields unit={unit} />
          <div className="flex flex-wrap gap-2.5">
            <Button type="submit" disabled={busy}>
              {busy ? "Saving…" : "Save unit"}
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
