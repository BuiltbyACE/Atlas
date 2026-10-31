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
  createDomain,
  type SystemsDomain,
  type SystemsDomainBody,
  updateDomain,
} from "@/lib/api/atlas";
import { useMutation } from "@/lib/use-mutation";

import type { PersonOption } from "@/lib/directory";

/**
 * Domains (`/api/v1/systems/domains`, §8.6). Creating takes
 * `systems.domain.manage`; editing takes `systems.domain.manage`. Secrets are
 * never stored here — only the registration pointer.
 */

/** A text field, trimmed; `""` when the field is absent. */
function field(data: FormData, key: string): string {
  return String(data.get(key) ?? "").trim();
}

/** Build the create/update body from the drawer's form. */
function domainBody(data: FormData): SystemsDomainBody {
  const owner = field(data, "owner_person_id");
  return {
    name: field(data, "name"),
    ...(field(data, "system_id") ? { system_id: field(data, "system_id") } : {}),
    registrar: field(data, "registrar"),
    registered_on: field(data, "registered_on") || undefined,
    expires_on: field(data, "expires_on") || undefined,
    ...(owner ? { owner_person_id: owner } : {}),
    auto_renew: data.get("auto_renew") !== null,
    notes: field(data, "notes"),
  };
}

/** The fields shared by "new domain" and "edit domain". */
function DomainFields({
  domain,
  systems,
  people,
}: {
  domain?: SystemsDomain;
  systems: { id: string; name: string }[];
  people: PersonOption[];
}) {
  return (
    <>
      <Field label="Name" htmlFor="domain-name">
        <TextInput
          id="domain-name"
          name="name"
          required
          placeholder="e.g. checkout.safaristack.co.ke"
          data-autofocus
        />
      </Field>

      <div className="grid grid-cols-1 gap-3.5 sm:grid-cols-2">
        <Field label="System" htmlFor="domain-system">
          <Select id="domain-system" name="system_id">
            <option value="" selected={!domain}>
              Unassigned
            </option>
            {systems.map((s) => (
              <option key={s.id} value={s.id} selected={domain?.system_id === s.id}>
                {s.name}
              </option>
            ))}
          </Select>
        </Field>
        <Field label="Registrar" htmlFor="domain-registrar" hint="Who the domain is registered with.">
          <TextInput
            id="domain-registrar"
            name="registrar"
            defaultValue={domain ? "" : ""}
            placeholder="e.g. Namecheap"
          />
        </Field>
      </div>

      <div className="grid grid-cols-1 gap-3.5 sm:grid-cols-2">
        <Field label="Registered on" htmlFor="domain-registered" hint="YYYY-MM-DD.">
          <TextInput id="domain-registered" name="registered_on" type="date" defaultValue="" />
        </Field>
        <Field label="Expires on" htmlFor="domain-expires" hint="YYYY-MM-DD; Go warns at 30/14/7 days.">
          <TextInput id="domain-expires" name="expires_on" type="date" defaultValue="" />
        </Field>
      </div>

      <Field label="Owner" htmlFor="domain-owner" hint="Person who owns the registration.">
        <PersonSelect
          id="domain-owner"
          name="owner_person_id"
          people={people}
          defaultValue={domain?.owner?.id}
          emptyLabel="Unassigned"
        />
      </Field>

      <Switch name="auto_renew" defaultChecked={domain?.auto_renew ?? true} label="Auto-renew" />
      <Field label="Notes" htmlFor="domain-notes">
        <TextInput id="domain-notes" name="notes" defaultValue={domain ? "" : ""} placeholder="Free-form notes." />
      </Field>
    </>
  );
}

/**
 * "New domain" (`POST /api/v1/systems/domains`, `systems.domain.manage`): a
 * button that opens the domain form in a drawer.
 */
export function NewDomainButton({
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

  if (!capabilities.includes("systems.domain.manage")) return null;

  function close() {
    setOpen(false);
    reset();
  }

  async function onSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const created = await run(() => createDomain(domainBody(new FormData(event.currentTarget))));
    if (created) {
      setOpen(false);
      notify(`Registered domain ${created.name}`);
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
        <Icon name="server" className={variant === "secondary" ? "h-4 w-4" : undefined} />
        New domain
      </Button>
      <Drawer
        open={open}
        onClose={close}
        title="New domain"
        description="The registration pointer — Go warns when it lapses at 30/14/7 days."
      >
        <form onSubmit={onSubmit} className="flex flex-col gap-4 px-6 py-[22px]">
          {problem && <ProblemNotice error={problem} title="The domain wasn’t saved" />}
          <DomainFields systems={systems} people={people} />
          <div className="flex flex-wrap gap-2.5">
            <Button type="submit" disabled={busy}>
              {busy ? "Saving…" : "Register domain"}
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
 * The actions for one domain row: edit its registration. `systems.domain.manage`,
 * guarded by `If-Match`.
 */
export function DomainActions({
  domain,
  systems,
  people,
  me,
  capabilities,
}: {
  domain: SystemsDomain;
  systems: { id: string; name: string }[];
  people: PersonOption[];
  me?: string;
  capabilities: string[];
}) {
  const router = useRouter();
  const notify = useToast();
  const { busy, problem, run, reset } = useMutation();
  const [open, setOpen] = useState(false);

  if (!capabilities.includes("systems.domain.manage")) return null;

  function close() {
    setOpen(false);
    reset();
  }

  async function onSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const updated = await run(() =>
      updateDomain(domain.id, domain.version, domainBody(new FormData(event.currentTarget))),
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

      <Drawer open={open} onClose={close} title="Edit domain" description={domain.name}>
        <form onSubmit={onSubmit} className="flex flex-col gap-4 px-6 py-[22px]">
          {problem && <ProblemNotice error={problem} title="The change wasn’t saved" />}
          <DomainFields domain={domain} systems={systems} people={people} />
          <div className="flex flex-wrap gap-2.5">
            <Button type="submit" disabled={busy}>
              {busy ? "Saving…" : "Save domain"}
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
