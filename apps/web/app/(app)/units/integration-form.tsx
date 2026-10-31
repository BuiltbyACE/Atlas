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
  createIntegration,
  INTEGRATION_KINDS,
  INTEGRATION_STATUSES,
  type UnitIntegration,
  type IntegrationBody,
  updateIntegration,
} from "@/lib/api/atlas";
import { humanise } from "@/lib/format";
import { useMutation } from "@/lib/use-mutation";

import type { PersonOption } from "@/lib/directory";

/**
 * Unit integrations (§8.7). Creating takes `units.unit.manage`; editing takes
 * `units.unit.manage`. Only the credential reference is stored — never the
 * secret itself.
 */

/** A text field, trimmed; `""` when the field is absent. */
function field(data: FormData, key: string): string {
  return String(data.get(key) ?? "").trim();
}

/** Build the create/update body from the drawer's form. */
function integrationBody(data: FormData): IntegrationBody {
  return {
    unit_id: field(data, "unit_id"),
    name: field(data, "name"),
    kind: field(data, "kind"),
    endpoint: field(data, "endpoint"),
    credential_ref: field(data, "credential_ref"),
    schedule: field(data, "schedule"),
    active: data.get("active") !== null,
  };
}

/** The fields shared by "new integration" and "edit integration". */
function IntegrationFields({
  integration,
  units,
}: {
  integration?: UnitIntegration;
  units: { id: string; name: string }[];
}) {
  return (
    <>
      <Field label="Unit" htmlFor="integration-unit">
        <Select id="integration-unit" name="unit_id" required>
          <option value="">Pick a unit</option>
          {units.map((u) => (
            <option key={u.id} value={u.id} selected={integration?.unit_id === u.id}>
              {u.name}
            </option>
          ))}
        </Select>
      </Field>

      <Field label="Name" htmlFor="integration-name">
        <TextInput
          id="integration-name"
          name="name"
          required
          defaultValue={integration?.name ?? ""}
          placeholder="e.g. School MIS API"
          data-autofocus
        />
      </Field>

      <div className="grid grid-cols-1 gap-3.5 sm:grid-cols-2">
        <Field label="Kind" htmlFor="integration-kind">
          <Select id="integration-kind" name="kind" required>
            {INTEGRATION_KINDS.map((kind) => (
              <option key={kind} value={kind} selected={integration?.kind === kind}>
                {humanise(kind)}
              </option>
            ))}
          </Select>
        </Field>
        <Field label="Status" htmlFor="integration-status">
          <Select id="integration-status" name="active">
            <option value="true" selected={integration?.active !== false}>
              Active
            </option>
            <option value="false" selected={integration?.active === false}>
              Inactive
            </option>
          </Select>
        </Field>
      </div>

      <Field label="Endpoint" htmlFor="integration-endpoint" hint="HTTPS URL of the unit's system of record.">
        <TextInput
          id="integration-endpoint"
          name="endpoint"
          required
          defaultValue={integration?.endpoint ?? ""}
          placeholder="https://mis.school.example.com/api"
        />
      </Field>

      <Field label="Credential ref" htmlFor="integration-credential" hint="Secrets-manager path, never the secret itself.">
        <TextInput
          id="integration-credential"
          name="credential_ref"
          defaultValue={integration?.credential_ref ?? ""}
          placeholder="sm://atlas/units/school-mis-key"
        />
      </Field>

      <Field label="Schedule" htmlFor="integration-schedule" hint="Sync cadence, e.g. daily 02:00 EAT.">
        <TextInput
          id="integration-schedule"
          name="schedule"
          defaultValue={integration?.schedule ?? ""}
          placeholder="daily 02:00 EAT"
        />
      </Field>
    </>
  );
}

/**
 * "New integration" (`POST /api/v1/units/integrations`, `units.unit.manage`):
 * a button that opens the integration form in a drawer.
 */
export function NewIntegrationButton({
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
      createIntegration(integrationBody(new FormData(event.currentTarget))),
    );
    if (created) {
      setOpen(false);
      notify(`Created integration ${created.name}`);
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
        <Icon name="signal" className={variant === "secondary" ? "h-4 w-4" : undefined} />
        New integration
      </Button>
      <Drawer
        open={open}
        onClose={close}
        title="New integration"
        description="Endpoint and credential reference only — never the secret itself."
      >
        <form onSubmit={onSubmit} className="flex flex-col gap-4 px-6 py-[22px]">
          {problem && <ProblemNotice error={problem} title="The integration wasn’t created" />}
          <IntegrationFields units={units} />
          <div className="flex flex-wrap gap-2.5">
            <Button type="submit" disabled={busy}>
              {busy ? "Creating…" : "Create integration"}
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
 * The actions for one integration row: edit its record. `units.unit.manage`,
 * guarded by `If-Match`.
 */
export function IntegrationActions({
  integration,
  units,
  people,
  me,
  capabilities,
}: {
  integration: UnitIntegration;
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
      updateIntegration(
        integration.id,
        integration.version,
        integrationBody(new FormData(event.currentTarget)),
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
        title="Edit integration"
        description={integration.name}
      >
        <form onSubmit={onSubmit} className="flex flex-col gap-4 px-6 py-[22px]">
          {problem && <ProblemNotice error={problem} title="The change wasn’t saved" />}
          <IntegrationFields integration={integration} units={units} />
          <div className="flex flex-wrap gap-2.5">
            <Button type="submit" disabled={busy}>
              {busy ? "Saving…" : "Save integration"}
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
