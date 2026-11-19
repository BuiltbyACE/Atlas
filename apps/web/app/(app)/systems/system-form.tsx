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
  createSystem,
  retireSystem,
  SYSTEM_CRITICALITIES,
  SYSTEM_ENVIRONMENTS,
  SYSTEM_TYPES,
  type AtlasSystem,
  type SystemsSystemBody,
  updateSystem,
} from "@/lib/api/atlas";
import { humanise } from "@/lib/format";
import { useMutation } from "@/lib/use-mutation";

import type { PersonOption } from "@/lib/directory";

/**
 * Systems (`/api/v1/systems`, §8.6). Registering a system takes
 * `systems.system.create`; editing or retiring takes `systems.system.manage`.
 * Every write is audited with the actor's name, so the footprint is the control.
 */

/** A text field, trimmed; `""` when the field is absent. */
function field(data: FormData, key: string): string {
  return String(data.get(key) ?? "").trim();
}

/** Build the create/update body from the drawer's form. */
function systemBody(data: FormData): SystemsSystemBody {
  const owner = field(data, "owner_person_id");
  const organization = field(data, "organization_id");
  const contract = field(data, "contract_id");
  return {
    name: field(data, "name"),
    system_type: field(data, "system_type"),
    environment: field(data, "environment") || "prod",
    criticality: field(data, "criticality") || "tier3",
    ...(owner ? { owner_person_id: owner } : {}),
    ...(organization ? { organization_id: organization } : {}),
    ...(contract ? { contract_id: contract } : {}),
    runbook_url: field(data, "runbook_url"),
    repository_url: field(data, "repository_url"),
    description: field(data, "description"),
  };
}

/** The fields shared by "new system" and "edit system". */
function SystemFields({
  system,
}: {
  system?: AtlasSystem;
}) {
  return (
    <>
      <Field label="Name" htmlFor="system-name">
        <TextInput
          id="system-name"
          name="name"
          required
          defaultValue={system?.name ?? ""}
          placeholder="e.g. Checkout API"
          data-autofocus
        />
      </Field>

      <div className="grid grid-cols-1 gap-3.5 sm:grid-cols-3">
        <Field label="Type" htmlFor="system-type">
          <Select id="system-type" name="system_type" required>
            {SYSTEM_TYPES.map((type) => (
              <option key={type} value={type} selected={system?.system_type === type}>
                {humanise(type)}
              </option>
            ))}
          </Select>
        </Field>
        <Field label="Environment" htmlFor="system-environment">
          <Select id="system-environment" name="environment">
            {SYSTEM_ENVIRONMENTS.map((env) => (
              <option key={env} value={env} selected={system?.environment === env}>
                {humanise(env)}
              </option>
            ))}
          </Select>
        </Field>
        <Field label="Criticality" htmlFor="system-criticality">
          <Select id="system-criticality" name="criticality">
            {SYSTEM_CRITICALITIES.map((crit) => (
              <option key={crit} value={crit} selected={system?.criticality === crit}>
                {humanise(crit)}
              </option>
            ))}
          </Select>
        </Field>
      </div>

      <Field label="Owner" htmlFor="system-owner" hint="Person who runs this system.">
        <PersonSelect
          id="system-owner"
          name="owner_person_id"
          people={[]}
          defaultValue={system?.owner?.id}
          emptyLabel="Unassigned"
        />
      </Field>

      <div className="grid grid-cols-1 gap-3.5 sm:grid-cols-2">
        <Field label="Organization" htmlFor="system-organization">
          <TextInput
            id="system-organization"
            name="organization_id"
            defaultValue=""
            placeholder="Organization id (UUID)"
          />
        </Field>
        <Field label="Contract" htmlFor="system-contract">
          <TextInput
            id="system-contract"
            name="contract_id"
            defaultValue=""
            placeholder="Contract id (UUID)"
          />
        </Field>
      </div>

      <Field label="Runbook URL" htmlFor="system-runbook" hint="Link to the runbook for this system.">
        <TextInput
          id="system-runbook"
          name="runbook_url"
          defaultValue=""
          placeholder="https://runbook.example.com/checkout"
        />
      </Field>
      <Field label="Repository URL" htmlFor="system-repository" hint="Link to the source repository.">
        <TextInput
          id="system-repository"
          name="repository_url"
          defaultValue=""
          placeholder="https://github.com/safaristack/checkout"
        />
      </Field>
      <Field label="Description" htmlFor="system-description">
        <TextArea
          id="system-description"
          name="description"
          defaultValue=""
          placeholder="What this system does and who to page."
        />
      </Field>
    </>
  );
}

/**
 * "New system" (`POST /api/v1/systems`, `systems.system.create`): a button
 * that opens the system form in a drawer.
 */
export function NewSystemButton({
  systems,
  people,
  me,
  capabilities,
  variant = "primary",
}: {
  systems: { id: string; name: string }[];
  people: PersonOption[];
  me?: string;
  capabilities: string[];
  variant?: "primary" | "secondary";
}) {
  const router = useRouter();
  const notify = useToast();
  const { busy, problem, run, reset } = useMutation();
  const [open, setOpen] = useState(false);

  if (!capabilities.includes("systems.system.create")) return null;

  function close() {
    setOpen(false);
    reset();
  }

  async function onSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const created = await run(() =>
      createSystem(systemBody(new FormData(event.currentTarget))),
    );
    if (created) {
      setOpen(false);
      notify(`Registered ${created.name}`);
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
        New system
      </Button>
      <Drawer
        open={open}
        onClose={close}
        title="New system"
        description="What it is, who owns it, and where its runbook and repository live."
      >
        <form onSubmit={onSubmit} className="flex flex-col gap-4 px-6 py-[22px]">
          {problem && <ProblemNotice error={problem} title="The system wasn’t saved" />}
          <SystemFields />
          <div className="flex flex-wrap gap-2.5">
            <Button type="submit" disabled={busy}>
              {busy ? "Saving…" : "Register system"}
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
 * The actions for one system row: edit its metadata, or retire it. All of it
 * is `systems.system.manage` via `PUT /api/v1/systems/{id}` or
 * `POST /api/v1/systems/{id}/retire`, guarded by `If-Match`.
 */
export function SystemActions({
  system,
  people,
  systems,
  me,
  capabilities,
}: {
  system: AtlasSystem;
  people: PersonOption[];
  systems: { id: string; name: string }[];
  me?: string;
  capabilities: string[];
}) {
  const router = useRouter();
  const notify = useToast();
  const { busy, problem, run, reset } = useMutation();
  const [open, setOpen] = useState(false);
  const [retiring, setRetiring] = useState(false);

  if (!capabilities.includes("systems.system.manage")) return null;

  function close() {
    setOpen(false);
    reset();
  }

  async function onSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const updated = await run(() =>
      updateSystem(system.id, system.version, systemBody(new FormData(event.currentTarget))),
    );
    if (updated) {
      close();
      notify(`Saved ${updated.name}`);
      router.refresh();
    }
  }

  async function onRetire() {
    setRetiring(true);
    try {
      const updated = await run(() => retireSystem(system.id, system.version));
      if (updated) {
        close();
        notify(`Retired ${updated.name}`);
        router.refresh();
      }
    } finally {
      setRetiring(false);
    }
  }

  return (
    <div className="flex flex-col items-end gap-2">
      <div className="flex flex-wrap items-center justify-end gap-1.5">
        <Button size="sm" variant="secondary" disabled={busy} onClick={() => setOpen(true)}>
          Edit
        </Button>
        <Button
          size="sm"
          variant="danger"
          disabled={busy || retiring}
          onClick={() => {
            setRetiring(true);
            void onRetire();
          }}
        >
          Retire
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
        title="Edit system"
        description={system.name}
      >
        <form onSubmit={onSubmit} className="flex flex-col gap-4 px-6 py-[22px]">
          {problem && <ProblemNotice error={problem} title="The change wasn’t saved" />}
          <SystemFields system={system} />
          <div className="flex flex-wrap gap-2.5">
            <Button type="submit" disabled={busy}>
              {busy ? "Saving…" : "Save system"}
            </Button>
            <Button type="button" variant="secondary" onClick={close} disabled={busy}>
              Cancel
            </Button>
          </div>
        </form>
      </Drawer>

      {retiring && (
        <Drawer
          open={retiring}
          onClose={() => setRetiring(false)}
          title="Retire system"
          description={`Retiring ${system.name} is final. It is refused while an incident is still open.`}
        >
          <form onSubmit={(e) => { e.preventDefault(); void onRetire(); }} className="flex flex-col gap-4 px-6 py-[22px]">
            <p className="text-[13.5px] text-muted">
              A retired system is frozen: its metadata can no longer be edited and its checks stop
              being probed. Go refuses the retire while any incident on this system is still open.
            </p>
            <div className="flex flex-wrap gap-2.5">
              <Button type="submit" variant="danger" disabled={busy || retiring}>
                {busy || retiring ? "Retiring…" : "Retire system"}
              </Button>
              <Button type="button" variant="secondary" onClick={() => setRetiring(false)} disabled={busy || retiring}>
                Cancel
              </Button>
            </div>
          </form>
        </Drawer>
      )}
    </div>
  );
}
