"use client";

import { useRouter } from "next/navigation";
import { useState, type FormEvent } from "react";

import { Drawer } from "@/components/drawer";
import {
  Button,
  Field,
  Select,
  Switch,
  TextInput,
} from "@/components/form";
import { PersonSelect } from "@/components/person-select";
import { Icon } from "@/components/icons";
import { useToast } from "@/components/toast";
import { cx, ProblemNotice } from "@/components/ui";
import {
  createCredential,
  type SystemCredential,
  type SystemsCredentialBody,
  updateCredential,
} from "@/lib/api/atlas";
import { useMutation } from "@/lib/use-mutation";

import type { PersonOption } from "@/lib/directory";

/**
 * Credentials (`/api/v1/systems/credentials`, §8.6). Creating takes
 * `systems.system.manage`; editing takes `systems.system.manage`. Only the
 * storage location (`store_path`) is recorded — never the secret itself.
 */

/** A text field, trimmed; `""` when the field is absent. */
function field(data: FormData, key: string): string {
  return String(data.get(key) ?? "").trim();
}

/** Build the create/update body from the drawer's form. */
function credentialBody(data: FormData): SystemsCredentialBody {
  const owner = field(data, "owner_person_id");
  const rotated = field(data, "rotated_on");
  const next = field(data, "next_rotation_on");
  return {
    system_id: field(data, "system_id"),
    label: field(data, "label"),
    credential_kind: field(data, "credential_kind"),
    store_path: field(data, "store_path"),
    ...(owner ? { owner_person_id: owner } : {}),
    ...(rotated ? { rotated_on: rotated } : {}),
    ...(next ? { next_rotation_on: next } : {}),
    notes: field(data, "notes"),
    active: data.get("active") !== null,
  };
}

/** The fields shared by "new credential" and "edit credential". */
function CredentialFields({
  credential,
  systems,
  people,
}: {
  credential?: SystemCredential;
  systems: { id: string; name: string }[];
  people: PersonOption[];
}) {
  return (
    <>
      <Field label="System" htmlFor="cred-system">
        <Select id="cred-system" name="system_id" required>
          <option value="" selected={!credential}>
            Pick a system
          </option>
          {systems.map((s) => (
            <option key={s.id} value={s.id} selected={credential?.system_id === s.id}>
              {s.name}
            </option>
          ))}
        </Select>
      </Field>

      <Field label="Label" htmlFor="cred-label">
        <TextInput
          id="cred-label"
          name="label"
          required
          placeholder="e.g. Stripe API key"
          data-autofocus
        />
      </Field>

      <div className="grid grid-cols-1 gap-3.5 sm:grid-cols-2">
        <Field label="Kind" htmlFor="cred-kind" hint="API key, database password, signing key, …">
          <TextInput
            id="cred-kind"
            name="credential_kind"
            defaultValue={credential ? "" : ""}
            placeholder="e.g. api_key"
          />
        </Field>
        <Field label="Store path" htmlFor="cred-path" hint="Secrets-manager path, never the secret.">
          <TextInput
            id="cred-path"
            name="store_path"
            required
            defaultValue={credential ? "" : ""}
            placeholder="e.g. aws/secrets/prod/stripe-key"
          />
        </Field>
      </div>

      <Field label="Owner" htmlFor="cred-owner" hint="Person responsible for rotation.">
        <PersonSelect
          id="cred-owner"
          name="owner_person_id"
          people={people}
          defaultValue={credential?.owner?.id}
          emptyLabel="Unassigned"
        />
      </Field>

      <div className="grid grid-cols-1 gap-3.5 sm:grid-cols-2">
        <Field label="Rotated on" htmlFor="cred-rotated" hint="YYYY-MM-DD.">
          <TextInput id="cred-rotated" name="rotated_on" type="date" defaultValue="" />
        </Field>
        <Field label="Next rotation on" htmlFor="cred-next" hint="YYYY-MM-DD.">
          <TextInput id="cred-next" name="next_rotation_on" type="date" defaultValue="" />
        </Field>
      </div>

      <Switch name="active" defaultChecked={credential?.active ?? true} label="Active" />
      <Field label="Notes" htmlFor="cred-notes">
        <TextInput id="cred-notes" name="notes" defaultValue={credential ? "" : ""} placeholder="Free-form notes." />
      </Field>
    </>
  );
}

/**
 * "New credential" (`POST /api/v1/systems/credentials`, `systems.system.manage`):
 * a button that opens the credential form in a drawer.
 */
export function NewCredentialButton({
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

  if (!capabilities.includes("systems.system.manage")) return null;

  function close() {
    setOpen(false);
    reset();
  }

  async function onSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const created = await run(() =>
      createCredential(credentialBody(new FormData(event.currentTarget))),
    );
    if (created) {
      setOpen(false);
      notify(`Recorded credential ${created.label}`);
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
        <Icon name="card" className={variant === "secondary" ? "h-4 w-4" : undefined} />
        New credential
      </Button>
      <Drawer
        open={open}
        onClose={close}
        title="New credential"
        description="Only the storage location is recorded — never the secret itself."
      >
        <form onSubmit={onSubmit} className="flex flex-col gap-4 px-6 py-[22px]">
          {problem && <ProblemNotice error={problem} title="The credential wasn’t saved" />}
          <CredentialFields systems={systems} people={people} />
          <div className="flex flex-wrap gap-2.5">
            <Button type="submit" disabled={busy}>
              {busy ? "Saving…" : "Record credential"}
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
 * The actions for one credential row: edit its record. `systems.system.manage`,
 * guarded by `If-Match`.
 */
export function CredentialActions({
  credential,
  systems,
  people,
  me,
  capabilities,
}: {
  credential: SystemCredential;
  systems: { id: string; name: string }[];
  people: PersonOption[];
  me?: string;
  capabilities: string[];
}) {
  const router = useRouter();
  const notify = useToast();
  const { busy, problem, run, reset } = useMutation();
  const [open, setOpen] = useState(false);

  if (!capabilities.includes("systems.system.manage")) return null;

  function close() {
    setOpen(false);
    reset();
  }

  async function onSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const updated = await run(() =>
      updateCredential(credential.id, credential.version, credentialBody(new FormData(event.currentTarget))),
    );
    if (updated) {
      close();
      notify(`Saved ${updated.label}`);
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

      <Drawer open={open} onClose={close} title="Edit credential" description={credential.label}>
        <form onSubmit={onSubmit} className="flex flex-col gap-4 px-6 py-[22px]">
          {problem && <ProblemNotice error={problem} title="The change wasn’t saved" />}
          <CredentialFields credential={credential} systems={systems} people={people} />
          <div className="flex flex-wrap gap-2.5">
            <Button type="submit" disabled={busy}>
              {busy ? "Saving…" : "Save credential"}
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
