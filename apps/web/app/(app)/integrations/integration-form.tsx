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
  createIntegrationRegistration,
  INTEGRATION_DIRECTION,
  INTEGRATION_KINDS,
  INTEGRATION_STATUSES,
  type Integration,
  type IntegrationDirection,
  type IntegrationInput,
  type IntegrationKind,
  type IntegrationStatus,
  updateIntegrationRegistration,
} from "@/lib/api/atlas";
import { humanise } from "@/lib/format";
import { useMutation } from "@/lib/use-mutation";

import type { PersonOption } from "@/lib/directory";

/**
 * Integrations (§8.9). Creating takes `integrations.registry.manage`; editing
 * takes `integrations.registry.manage`. Only the credential reference is
 * stored — never the secret itself.
 */

/** A text field, trimmed; `""` when the field is absent. */
function field(data: FormData, key: string): string {
  return String(data.get(key) ?? "").trim();
}

/** Build the create/update body from the drawer's form. */
function integrationBody(data: FormData): IntegrationInput {
  const owner = field(data, "owner_person_id");
  return {
    name: field(data, "name"),
    kind: field(data, "kind") as IntegrationKind,
    direction: field(data, "direction") as IntegrationDirection,
    ...(owner ? { owner_person_id: owner } : {}),
    credential_ref: field(data, "credential_ref"),
    scopes: field(data, "scopes").split(",").map((s) => s.trim()).filter(Boolean),
    status: (field(data, "status") || "active") as IntegrationStatus,
  };
}

/** The fields shared by "new integration" and "edit integration". */
function IntegrationFields({
  integration,
}: {
  integration?: Integration;
}) {
  return (
    <>
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
        <Field label="Direction" htmlFor="integration-direction">
          <Select id="integration-direction" name="direction" required>
            {INTEGRATION_DIRECTION.map((dir) => (
              <option key={dir} value={dir} selected={integration?.direction === dir}>
                {humanise(dir)}
              </option>
            ))}
          </Select>
        </Field>
        <Field label="Status" htmlFor="integration-status">
          <Select id="integration-status" name="status">
            {INTEGRATION_STATUSES.map((status) => (
              <option key={status} value={status} selected={integration?.status === status}>
                {humanise(status)}
              </option>
            ))}
          </Select>
        </Field>
      </div>

      <Field label="Owner" htmlFor="integration-owner" hint="Person responsible for this integration.">
        <PersonSelect
          id="integration-owner"
          name="owner_person_id"
          people={[]}
          defaultValue={integration?.owner_person_id}
          emptyLabel="Unassigned"
        />
      </Field>

      <Field label="Credential ref" htmlFor="integration-credential-ref" hint="Secrets-manager path, never the secret itself.">
        <TextInput
          id="integration-credential-ref"
          name="credential_ref"
          required
          defaultValue={integration?.credential_ref ?? ""}
          placeholder="e.g. sm://atlas/integrations/school-mis-key"
        />
      </Field>

      <Field label="Scopes" htmlFor="integration-scopes" hint="Comma-separated list of OAuth scopes, e.g. calendar.read,contacts.write.">
        <TextInput
          id="integration-scopes"
          name="scopes"
          defaultValue={integration?.scopes.join(", ") ?? ""}
          placeholder="e.g. calendar.read, contacts.write"
        />
      </Field>
    </>
  );
}

/**
 * "New integration" (`POST /api/v1/integrations`, `integrations.registry.manage`):
 * a button that opens the integration form in a drawer.
 */
export function NewIntegrationButton({
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

  if (!capabilities.includes("integrations.registry.manage")) return null;

  function close() {
    setOpen(false);
    reset();
  }

  async function onSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const created = await run(() =>
      createIntegrationRegistration(integrationBody(new FormData(event.currentTarget))),
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
        description="Records how Atlas connects to an external system. Only the credential reference is stored — never the secret."
      >
        <form onSubmit={onSubmit} className="flex flex-col gap-4 px-6 py-[22px]">
          {problem && <ProblemNotice error={problem} title="The integration wasn’t created" />}
          <IntegrationFields />
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
 * The actions for one integration row: edit its record. `integrations.registry.manage`,
 * guarded by `If-Match`.
 */
export function IntegrationActions({
  integration,
  capabilities,
}: {
  integration: Integration;
  capabilities: string[];
}) {
  const router = useRouter();
  const notify = useToast();
  const { busy, problem, run, reset } = useMutation();
  const [open, setOpen] = useState(false);

  if (!capabilities.includes("integrations.registry.manage")) return null;

  function close() {
    setOpen(false);
    reset();
  }

  async function onSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const updated = await run(() =>
      updateIntegrationRegistration(integration.id, integration.version, integrationBody(new FormData(event.currentTarget))),
    );
    if (updated) {
      close();
      notify(`Saved integration ${updated.name}`);
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

      <Drawer open={open} onClose={close} title="Edit integration" description={integration.name}>
        <form onSubmit={onSubmit} className="flex flex-col gap-4 px-6 py-[22px]">
          {problem && <ProblemNotice error={problem} title="The change wasn’t saved" />}
          <IntegrationFields integration={integration} />
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
