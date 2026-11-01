"use client";

import Link from "next/link";
import { useState } from "react";

import { Button, ButtonLink, Field, TextInput } from "@/components/form";
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
  getOrganization,
  listContacts,
  listLeads,
  listOpportunities,
  type CrmContact,
  type CrmOrganization,
  type Lead,
  type Opportunity,
  updateOrganization,
} from "@/lib/api/atlas";
import { ApiError } from "@/lib/api/client";
import { personDirectory, personName } from "@/lib/directory";
import { formatDay, formatMinor, humanise } from "@/lib/format";
import { useMutation } from "@/lib/use-mutation";

/**
 * Organization detail. Reads the organization and its related contacts, leads
 * and opportunities from Go, then offers inline editing of organization fields.
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

export default async function OrganizationDetailPage({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = await params;

  let organization: CrmOrganization;
  try {
    organization = await getOrganization(id);
  } catch (error) {
    if (!(error instanceof ApiError)) throw error;
    return (
      <div className="space-y-6">
        <PageHeader title="Organization" />
        <ProblemNotice
          error={error}
          title={
            error.status === 404
              ? "That organization does not exist"
              : "The organization could not be loaded"
          }
        />
      </div>
    );
  }

  const [contactsResult, leadsResult, opportunitiesResult, names] =
    await Promise.all([
      safe<{ items: CrmContact[] }>(listContacts(), { items: [] }),
      safe<{ items: Lead[] }>(listLeads(), { items: [] }),
      safe<{ items: Opportunity[] }>(listOpportunities(), { items: [] }),
      personDirectory(),
    ]);

  const relatedContacts = contactsResult.items.filter(
    (c) => c.organization_id === id,
  );
  const relatedLeads = leadsResult.items.filter((l) => l.organization_id === id);
  const relatedOpportunities = opportunitiesResult.items.filter(
    (o) => o.organization_id === id,
  );

  return (
    <div className="space-y-6">
      <PageHeader
        title={organization.name}
        eyebrow={
          <span className="flex flex-wrap items-center gap-2">
            <Badge tone="neutral">{humanise(organization.status)}</Badge>
            {organization.industry && <Badge tone="info">{organization.industry}</Badge>}
          </span>
        }
        description={
          <span className="flex flex-wrap items-center gap-x-2 gap-y-1">
            <span>
              {organization.owner_person_id
                ? personName(names, organization.owner_person_id)
                : "Unassigned"}
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

      <StatGrid label="Organization at a glance">
        <Stat
          label="Industry"
          value={organization.industry ?? "—"}
          icon="building"
          iconTone="info"
        />
        <Stat
          label="Size"
          value={organization.size ?? "—"}
          icon="people"
          iconTone="info"
        />
        <Stat
          label="Location"
          value={organization.location ?? "—"}
          icon="document"
          iconTone="info"
        />
      </StatGrid>

      <div className="grid grid-cols-1 gap-5 xl:grid-cols-[minmax(0,1.7fr)_minmax(0,1fr)]">
        <div className="flex flex-col gap-5">
          <Card title="Details">
            <Table>
              <tbody>
                <tr>
                  <Th className="text-muted w-[30%]">KRA PIN</Th>
                  <Td className="tabular text-muted">
                    {organization.kra_pin ?? "—"}
                  </Td>
                </tr>
                <tr>
                  <Th className="text-muted">Industry</Th>
                  <Td className="tabular">
                    {organization.industry ?? <span className="text-muted">—</span>}
                  </Td>
                </tr>
                <tr>
                  <Th className="text-muted">Size</Th>
                  <Td className="tabular">
                    {organization.size ?? <span className="text-muted">—</span>}
                  </Td>
                </tr>
                <tr>
                  <Th className="text-muted">Location</Th>
                  <Td className="tabular">
                    {organization.location ?? <span className="text-muted">—</span>}
                  </Td>
                </tr>
              </tbody>
            </Table>
          </Card>

          {relatedContacts.length > 0 && (
            <Card title="Contacts">
              <Table>
                <thead>
                  <tr>
                    <Th>Contact</Th>
                    <Th>Role</Th>
                    <Th>Phones</Th>
                    <Th>Emails</Th>
                  </tr>
                </thead>
                <tbody>
                  {relatedContacts.map((c) => (
                    <tr key={c.id}>
                      <Td>
                        <Link
                          href={`/crm/contacts/${c.id}`}
                          className="font-semibold text-night hover:underline"
                        >
                          {c.name}
                        </Link>
                      </Td>
                      <Td className="tabular text-muted">
                        {c.role ?? "—"}
                      </Td>
                      <Td className="tabular text-muted">
                        {c.phones.join(", ") || "—"}
                      </Td>
                      <Td className="tabular text-muted">
                        {c.emails.join(", ") || "—"}
                      </Td>
                    </tr>
                  ))}
                </tbody>
              </Table>
            </Card>
          )}

          {relatedLeads.length > 0 && (
            <Card title="Leads">
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
            <Card title="Opportunities">
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

        <OrganizationEditor organization={organization} />
      </div>

      <p className="text-xs text-[#7A879C]">
        Created {formatDay(organization.created_at)} · updated {formatDay(organization.updated_at)} ·
        version {organization.version}
      </p>
    </div>
  );
}

function OrganizationEditor({
  organization,
}: {
  organization: CrmOrganization;
}) {
  const { busy, problem, run, reset } = useMutation();
  const [name, setName] = useState(organization.name);
  const [industry, setIndustry] = useState(organization.industry ?? "");
  const [size, setSize] = useState(organization.size ?? "");
  const [location, setLocation] = useState(organization.location ?? "");
  const [kraPin, setKraPin] = useState(organization.kra_pin ?? "");

  async function onSave(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const updated = await run(async () => {
      return updateOrganization(organization.id, organization.version, {
        name,
        industry: industry || undefined,
        size: size || undefined,
        location: location || undefined,
        kra_pin: kraPin || undefined,
      });
    });
    if (updated) {
      reset();
    }
  }

  return (
    <Card title="Manage organization">
      {problem && <ProblemNotice error={problem} title="The organization wasn't updated" />}

      <form onSubmit={onSave} className="flex flex-col gap-4 px-6 py-[22px]">
        <Field label="Name">
          <TextInput
            name="name"
            defaultValue={organization.name}
            onChange={(e) => setName(e.target.value)}
            required
          />
        </Field>
        <div className="grid grid-cols-2 gap-4">
          <Field label="Industry">
            <TextInput
              name="industry"
              defaultValue={organization.industry ?? ""}
              onChange={(e) => setIndustry(e.target.value)}
              placeholder="e.g. Agriculture"
            />
          </Field>
          <Field label="Size">
            <TextInput
              name="size"
              defaultValue={organization.size ?? ""}
              onChange={(e) => setSize(e.target.value)}
              placeholder="e.g. 50-200 employees"
            />
          </Field>
        </div>
        <Field label="Location">
          <TextInput
            name="location"
            defaultValue={organization.location ?? ""}
            onChange={(e) => setLocation(e.target.value)}
            placeholder="e.g. Nairobi, Kenya"
          />
        </Field>
        <Field label="KRA PIN">
          <TextInput
            name="kra_pin"
            defaultValue={organization.kra_pin ?? ""}
            onChange={(e) => setKraPin(e.target.value)}
            placeholder="e.g. P051234567A"
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
