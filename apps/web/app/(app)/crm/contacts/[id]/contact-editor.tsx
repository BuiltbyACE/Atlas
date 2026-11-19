"use client";

import { useState } from "react";

import { Button, Field, TextInput } from "@/components/form";
import { Card, ProblemNotice } from "@/components/ui";
import { updateContact, type CrmContact } from "@/lib/api/atlas";
import { useMutation } from "@/lib/use-mutation";

/** Inline edit of a contact's role and WhatsApp number (§8.1). */
export function ContactEditor({ contact }: { contact: CrmContact }) {
  const { busy, problem, run, reset } = useMutation();
  const [role, setRole] = useState(contact.role ?? "");
  const [whatsapp, setWhatsapp] = useState(contact.whatsapp_number ?? "");

  async function onSave(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const updated = await run(async () => {
      return updateContact(contact.id, contact.version, {
        name: contact.name,
        role: role || undefined,
        whatsapp_number: whatsapp || undefined,
        phones: contact.phones,
        emails: contact.emails,
        consent_marketing: contact.consent_marketing,
        consent_whatsapp: contact.consent_whatsapp,
        consent_call_recording: contact.consent_call_recording,
      });
    });
    if (updated) {
      reset();
    }
  }

  return (
    <Card title="Manage contact">
      {problem && (
        <ProblemNotice error={problem} title="The contact wasn't updated" />
      )}

      <form onSubmit={onSave} className="flex flex-col gap-4 px-6 py-[22px]">
        <Field label="Role">
          <TextInput
            name="role"
            defaultValue={contact.role ?? ""}
            onChange={(e) => setRole(e.target.value)}
            placeholder="e.g. Procurement Manager"
          />
        </Field>
        <Field label="WhatsApp number">
          <TextInput
            name="whatsapp"
            defaultValue={contact.whatsapp_number ?? ""}
            onChange={(e) => setWhatsapp(e.target.value)}
            placeholder="+254 7xx xxx xxx"
          />
        </Field>
        <div className="flex items-end gap-2 pt-4">
          <Button type="submit" variant="primary" disabled={busy}>
            {busy ? "Saving…" : "Save"}
          </Button>
        </div>
      </form>
    </Card>
  );
}
