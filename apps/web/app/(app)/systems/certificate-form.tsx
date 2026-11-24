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
  createCertificate,
  type SystemsCertificate,
  type SystemsCertificateBody,
  updateCertificate,
} from "@/lib/api/atlas";
import { useMutation } from "@/lib/use-mutation";

import type { PersonOption } from "@/lib/directory";

/**
 * Certificates (`/api/v1/systems/certificates`, §8.6). Creating takes
 * `systems.domain.manage`; editing takes `systems.domain.manage`. Only the
 * location pointer is stored — never the private key or secret.
 */

/** A text field, trimmed; `""` when the field is absent. */
function field(data: FormData, key: string): string {
  return String(data.get(key) ?? "").trim();
}

/** Build the create/update body from the drawer's form. */
function certificateBody(data: FormData): SystemsCertificateBody {
  const owner = field(data, "owner_person_id");
  return {
    hostname: field(data, "hostname"),
    ...(field(data, "system_id") ? { system_id: field(data, "system_id") } : {}),
    issuer: field(data, "issuer"),
    issued_on: field(data, "issued_on") || undefined,
    expires_on: field(data, "expires_on") || undefined,
    ...(owner ? { owner_person_id: owner } : {}),
    auto_renew: data.get("auto_renew") !== null,
    notes: field(data, "notes"),
  };
}

/** The fields shared by "new certificate" and "edit certificate". */
function CertificateFields({
  certificate,
  systems,
  people,
}: {
  certificate?: SystemsCertificate;
  systems: { id: string; name: string }[];
  people: PersonOption[];
}) {
  return (
    <>
      <Field label="Hostname" htmlFor="cert-hostname">
        <TextInput
          id="cert-hostname"
          name="hostname"
          required
          placeholder="e.g. api.safaristack.co.ke"
          data-autofocus
        />
      </Field>

      <div className="grid grid-cols-1 gap-3.5 sm:grid-cols-2">
        <Field label="System" htmlFor="cert-system">
          <Select id="cert-system" name="system_id">
            <option value="" selected={!certificate}>
              Unassigned
            </option>
            {systems.map((s) => (
              <option key={s.id} value={s.id} selected={certificate?.system_id === s.id}>
                {s.name}
              </option>
            ))}
          </Select>
        </Field>
        <Field label="Issuer" htmlFor="cert-issuer" hint="Who issued the certificate.">
          <TextInput
            id="cert-issuer"
            name="issuer"
            defaultValue={certificate ? "" : ""}
            placeholder="e.g. Let's Encrypt"
          />
        </Field>
      </div>

      <div className="grid grid-cols-1 gap-3.5 sm:grid-cols-2">
        <Field label="Issued on" htmlFor="cert-issued" hint="YYYY-MM-DD.">
          <TextInput id="cert-issued" name="issued_on" type="date" defaultValue="" />
        </Field>
        <Field label="Expires on" htmlFor="cert-expires" hint="YYYY-MM-DD; Go warns at 30/14/7 days.">
          <TextInput id="cert-expires" name="expires_on" type="date" defaultValue="" />
        </Field>
      </div>

      <Field label="Owner" htmlFor="cert-owner" hint="Person who owns the certificate.">
        <PersonSelect
          id="cert-owner"
          name="owner_person_id"
          people={people}
          defaultValue={certificate?.owner?.id}
          emptyLabel="Unassigned"
        />
      </Field>

      <Switch name="auto_renew" defaultChecked={certificate?.auto_renew ?? true} label="Auto-renew" />
      <Field label="Notes" htmlFor="cert-notes">
        <TextInput id="cert-notes" name="notes" defaultValue={certificate ? "" : ""} placeholder="Free-form notes." />
      </Field>
    </>
  );
}

/**
 * "New certificate" (`POST /api/v1/systems/certificates`, `systems.domain.manage`):
 * a button that opens the certificate form in a drawer.
 */
export function NewCertificateButton({
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
    const created = await run(() =>
      createCertificate(certificateBody(new FormData(event.currentTarget))),
    );
    if (created) {
      setOpen(false);
      notify(`Registered certificate ${created.hostname}`);
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
        <Icon name="shield" className={variant === "secondary" ? "h-4 w-4" : undefined} />
        New certificate
      </Button>
      <Drawer
        open={open}
        onClose={close}
        title="New certificate"
        description="The certificate pointer — Go warns when it lapses at 30/14/7 days."
      >
        <form onSubmit={onSubmit} className="flex flex-col gap-4 px-6 py-[22px]">
          {problem && <ProblemNotice error={problem} title="The certificate wasn’t saved" />}
          <CertificateFields systems={systems} people={people} />
          <div className="flex flex-wrap gap-2.5">
            <Button type="submit" disabled={busy}>
              {busy ? "Saving…" : "Register certificate"}
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
 * The actions for one certificate row: edit its record. `systems.domain.manage`,
 * guarded by `If-Match`.
 */
export function CertificateActions({
  certificate,
  systems,
  people,
  me,
  capabilities,
}: {
  certificate: SystemsCertificate;
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
      updateCertificate(certificate.id, certificate.version, certificateBody(new FormData(event.currentTarget))),
    );
    if (updated) {
      close();
      notify(`Saved ${updated.hostname}`);
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

      <Drawer open={open} onClose={close} title="Edit certificate" description={certificate.hostname}>
        <form onSubmit={onSubmit} className="flex flex-col gap-4 px-6 py-[22px]">
          {problem && <ProblemNotice error={problem} title="The change wasn’t saved" />}
          <CertificateFields certificate={certificate} systems={systems} people={people} />
          <div className="flex flex-wrap gap-2.5">
            <Button type="submit" disabled={busy}>
              {busy ? "Saving…" : "Save certificate"}
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
