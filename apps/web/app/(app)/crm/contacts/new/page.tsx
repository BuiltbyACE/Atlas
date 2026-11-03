"use client";

import { useRouter } from "next/navigation";
import { useEffect, useState } from "react";

import { Drawer } from "@/components/drawer";
import { Button, Field, Select, TextInput } from "@/components/form";
import { ProblemNotice } from "@/components/ui";
import { useToast } from "@/components/toast";
import {
  createContact,
  listOrganizations,
  type CrmContact,
  type CrmOrganization,
} from "@/lib/api/atlas";
import { useMutation } from "@/lib/use-mutation";

/**
 * New contact (`POST /api/v1/crm/contacts`, `crm.registry.manage`).
 * Opens in a drawer over the CRM page. Name is required; organization links
 * the contact to a company record.
 */
export default function NewContactPage() {
  const router = useRouter();
  const notify = useToast();
  const { busy, problem, run, reset } = useMutation();
  const [open, setOpen] = useState(true);
  const [organizations, setOrganizations] = useState<CrmOrganization[]>([]);

  useEffect(() => {
    listOrganizations({ limit: 200 }).then(
      (page) => setOrganizations(page.items),
      () => setOrganizations([]),
    );
  }, []);

  async function onSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const data = new FormData(event.currentTarget);
    const created = await run(async () => {
      return createContact({
        name: String(data.get("name") ?? "").trim(),
        role: String(data.get("role") ?? "").trim() || undefined,
        organization_id:
          String(data.get("organization_id") ?? "").trim() || undefined,
        phones: String(data.get("phones") ?? "")
          .split(",")
          .map((p) => p.trim())
          .filter(Boolean),
        emails: String(data.get("emails") ?? "")
          .split(",")
          .map((e) => e.trim())
          .filter(Boolean),
        whatsapp_number:
          String(data.get("whatsapp_number") ?? "").trim() || undefined,
        consent_marketing: data.get("consent_marketing") === "on",
        consent_whatsapp: data.get("consent_whatsapp") === "on",
        consent_call_recording: data.get("consent_call_recording") === "on",
        consent_source: String(data.get("consent_source") ?? "").trim() || undefined,
      });
    });
    if (created) {
      setOpen(false);
      reset();
      notify(`Recorded ${created.name}`);
      router.refresh();
    }
  }

  function close() {
    setOpen(false);
  }

  return (
    <>
      <Drawer
        open={open}
        onClose={close}
        title="New contact"
        description="A person at an organization. Only the name is required; consents default to unset."
      >
        {problem && (
          <ProblemNotice error={problem} title="The contact wasn't created" />
        )}
        <form onSubmit={onSubmit} className="flex flex-col gap-4 px-6 py-[22px]">
          <Field label="Name" htmlFor="contact-name">
            <TextInput
              id="contact-name"
              name="name"
              required
              data-autofocus
              placeholder="Jane Wanjiku"
            />
          </Field>

          <Field label="Role" htmlFor="contact-role">
            <TextInput id="contact-role" name="role" placeholder="e.g. Procurement Manager" />
          </Field>

          <Field label="Organization" htmlFor="contact-org">
            <Select id="contact-org" name="organization_id">
              <option value="">Unassigned</option>
              {organizations.map((o) => (
                <option key={o.id} value={o.id}>
                  {o.name}
                </option>
              ))}
            </Select>
          </Field>

          <div className="grid grid-cols-1 gap-3.5 sm:grid-cols-2">
            <Field label="Phones" htmlFor="contact-phones">
              <TextInput
                id="contact-phones"
                name="phones"
                placeholder="Comma-separated: +254 700 000 000, +254 701 000 001"
              />
              <p className="mt-1 text-[11.5px] text-muted">
                Separate multiple numbers with commas.
              </p>
            </Field>
            <Field label="Emails" htmlFor="contact-emails">
              <TextInput
                id="contact-emails"
                name="emails"
                placeholder="jane@acme.com, j.wanjiku@acme.com"
              />
              <p className="mt-1 text-[11.5px] text-muted">
                Separate multiple addresses with commas.
              </p>
            </Field>
          </div>

          <Field label="WhatsApp number" htmlFor="contact-whatsapp">
            <TextInput
              id="contact-whatsapp"
              name="whatsapp_number"
              placeholder="+254 700 000 000"
            />
          </Field>

          <div className="grid grid-cols-1 gap-3.5 sm:grid-cols-3">
            <Field label="Marketing consent" htmlFor="contact-consent-marketing">
              <label className="flex items-center gap-2 text-sm">
                <input
                  id="contact-consent-marketing"
                  name="consent_marketing"
                  type="checkbox"
                  className="h-4 w-4 flex-none rounded border-line-strong accent-atlas"
                />
                Consent to marketing
              </label>
            </Field>
            <Field label="WhatsApp consent" htmlFor="contact-consent-whatsapp">
              <label className="flex items-center gap-2 text-sm">
                <input
                  id="contact-consent-whatsapp"
                  name="consent_whatsapp"
                  type="checkbox"
                  className="h-4 w-4 flex-none rounded border-line-strong accent-atlas"
                />
                Consent to WhatsApp
              </label>
            </Field>
            <Field label="Call recording consent" htmlFor="contact-consent-recording">
              <label className="flex items-center gap-2 text-sm">
                <input
                  id="contact-consent-recording"
                  name="consent_call_recording"
                  type="checkbox"
                  className="h-4 w-4 flex-none rounded border-line-strong accent-atlas"
                />
                Consent to call recording
              </label>
            </Field>
          </div>

          <Field label="Consent source" htmlFor="contact-consent-source">
            <TextInput
              id="contact-consent-source"
              name="consent_source"
              placeholder="e.g. website form, event signup"
            />
          </Field>

          <div className="flex justify-end gap-2 pt-4">
            <Button type="button" variant="secondary" onClick={close}>
              Cancel
            </Button>
            <Button type="submit" variant="primary" disabled={busy}>
              {busy ? "Saving…" : "Create contact"}
            </Button>
          </div>
        </form>
      </Drawer>
    </>
  );
}
