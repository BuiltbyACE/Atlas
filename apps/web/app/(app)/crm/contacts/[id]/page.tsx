"use client";

import Link from "next/link";
import { useState } from "react";

import { Button, ButtonLink, Field, Select, TextInput } from "@/components/form";
import { Icon } from "@/components/icons";
import { SignalRefresh } from "@/components/signal-refresh";
import {
  Badge,
  Card,
  EmptyState,
  PageHeader,
  ProblemNotice,
  Stat,
  StatGrid,
  Table,
  Td,
  Th,
} from "@/components/ui";
import {
  getContact,
  listLeads,
  listOpportunities,
  updateContact,
  type CrmContact,
  type Lead,
  type Opportunity,
} from "@/lib/api/atlas";
import { ApiError } from "@/lib/api/client";
import { personDirectory, personName } from "@/lib/directory";
import { formatDay, formatMinor, humanise } from "@/lib/format";
import { useMutation } from "@/lib/use-mutation";

/**
 * Contact detail. Reads the contact and its related leads and opportunities
 * from Go, then offers inline editing of consent and role fields.
 */
export const dynamic = "force-dynamic";

async function safe<T>(promise: Promise<T>, fallback: T): Promise<T> {
  try {
    return await promise;
  } catch (error) {
    if (error instanceof ApiError) return fallback;
    throw error;
  }
}

export default async function ContactDetailPage({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = await params;

  let contact: CrmContact;
  try {
    contact = await getContact(id);
  } catch (error) {
    if (!(error instanceof ApiError)) throw error;
    return (
      <div className="space-y-6">
        <PageHeader title="Contact" />
        <ProblemNotice
          error={error}
          title={
            error.status === 404
              ? "That contact does not exist"
              : "The contact could not be loaded"
          }
        />
      </div>
    );
  }

  const [leadsResult, opportunitiesResult, names] = await Promise.all([
    safe<{ items: Lead[] }>(listLeads(), { items: [] }),
    safe<{ items: Opportunity[] }>(listOpportunities(), { items: [] }),
    personDirectory(),
  ]);

  const relatedLeads = leadsResult.items.filter((l) => l.contact_id === id);
  const relatedOpportunities = opportunitiesResult.items.filter(
    (o) => o.organization_id === contact.organization_id,
  );

  return (
    <div className="space-y-6">
      <PageHeader
        title={contact.name}
        eyebrow={
          <span className="flex flex-wrap items-center gap-2">
            {contact.role && <Badge tone="neutral">{humanise(contact.role)}</Badge>}
            {contact.consent_marketing && (
              <Badge tone="info">Marketing consent</Badge>
            )}
            {contact.consent_whatsapp && <Badge tone="info">WhatsApp consent</Badge>}
          </span>
        }
        description={
          <span className="flex flex-wrap items-center gap-x-2 gap-y-1">
            <span>
              {contact.organization_id ?? "No organization"}
            </span>
            <SignalRefresh topic="crm" quiet />
          </span>
        }
        actions={
          <>
            <ButtonLink href="/crm" variant="secondary">
              <Icon name="chevronLeft" />
              Back to CRM
            </ButtonLink>
          </>
        }
      />

      <StatGrid label="Contact at a glance">
        <Stat
          label="Phones"
          value={contact.phones.length.toString()}
          icon="people"
          iconTone="info"
          hint={contact.phones.join(", ") || "None recorded"}
        />
        <Stat
          label="Emails"
          value={contact.emails.length.toString()}
          icon="card"
          iconTone="info"
          hint={contact.emails.join(", ") || "None recorded"}
        />
      </StatGrid>

      <div className="grid grid-cols-1 gap-5 xl:grid-cols-[minmax(0,1.7fr)_minmax(0,1fr)]">
        <div className="flex flex-col gap-5">
          <Card title="Details">
            <Table>
              <tbody>
                <tr>
                  <Th className="text-muted w-[30%]">Role</Th>
                  <Td className="tabular">
                    {contact.role ?? <span className="text-muted">—</span>}
                  </Td>
                </tr>
                <tr>
                  <Th className="text-muted">Organization</Th>
                  <Td className="tabular">
                    {contact.organization_id ?? (
                      <span className="text-muted">—</span>
                    )}
                  </Td>
                </tr>
                <tr>
                  <Th className="text-muted">WhatsApp</Th>
                  <Td className="tabular text-muted">
                    {contact.whatsapp_number ?? "—"}
                  </Td>
                </tr>
                {contact.phones.length > 0 && (
                  <tr>
                    <Th className="text-muted">Phone numbers</Th>
                    <Td className="tabular text-muted">
                      {contact.phones.join(", ")}
                    </Td>
                  </tr>
                )}
                {contact.emails.length > 0 && (
                  <tr>
                    <Th className="text-muted">Email addresses</Th>
                    <Td className="tabular text-muted">
                      {contact.emails.join(", ")}
                    </Td>
                  </tr>
                )}
              </tbody>
            </Table>
          </Card>

          {relatedLeads.length > 0 && (
            <Card title="Related leads">
              <Table>
                <thead>
                  <tr>
                    <Th>Lead</Th>
                    <Th className="text-right">Value</Th>
                    <Th>Status</Th>
                  </tr>
                </thead>
                <tbody>
                  {relatedLeads.map((l) => (
                    <tr key={l.id}>
                      <Td>
                        <Link
                          href={`/crm/leads/${l.id}`}
                          className="font-semibold text-night hover:underline"
                        >
                          {l.ref}
                        </Link>
                      </Td>
                      <Td className="tabular text-right">
                        {formatMinor(l.estimated_value_minor, l.currency)}
                      </Td>
                      <Td>
                        <Badge tone="neutral">{humanise(l.status)}</Badge>
                      </Td>
                    </tr>
                  ))}
                </tbody>
              </Table>
            </Card>
          )}

          {relatedOpportunities.length > 0 && (
            <Card title="Related opportunities">
              <Table>
                <thead>
                  <tr>
                    <Th>Opportunity</Th>
                    <Th className="text-right">Value</Th>
                    <Th>Stage</Th>
                  </tr>
                </thead>
                <tbody>
                  {relatedOpportunities.map((o) => (
                    <tr key={o.id}>
                      <Td>
                        <Link
                          href={`/crm/opportunities/${o.id}`}
                          className="font-semibold text-night hover:underline"
                        >
                          {o.title}
                        </Link>
                      </Td>
                      <Td className="tabular text-right">
                        {formatMinor(o.value_minor, o.currency)}
                      </Td>
                      <Td className="text-muted">{humanise(o.stage_key)}</Td>
                    </tr>
                  ))}
                </tbody>
              </Table>
            </Card>
          )}
        </div>

        <ContactEditor contact={contact} />
      </div>

      <p className="text-xs text-[#7A879C]">
        Created {formatDay(contact.created_at)} · updated {formatDay(contact.updated_at)} ·
        version {contact.version}
      </p>
    </div>
  );
}

function ContactEditor({
  contact,
}: {
  contact: CrmContact;
}) {
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
      {problem && <ProblemNotice error={problem} title="The contact wasn't updated" />}

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
