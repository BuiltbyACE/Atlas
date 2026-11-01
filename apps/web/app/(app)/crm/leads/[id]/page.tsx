"use client";

import Link from "next/link";
import { useState } from "react";

import { Button, ButtonLink, Field, Select } from "@/components/form";
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
  convertLead,
  getLead,
  listOpportunities,
  setLeadStatus,
  type Lead,
  type Opportunity,
} from "@/lib/api/atlas";
import { ApiError } from "@/lib/api/client";
import { personDirectory, personName } from "@/lib/directory";
import { formatDay, formatMinor, humanise } from "@/lib/format";
import { useMutation } from "@/lib/use-mutation";
import { SALES_TOPIC } from "@/lib/realtime/topics";

/**
 * Lead detail. Reads the lead and its related opportunities from Go, then
 * offers status change and conversion actions. Writes re-fetch this route.
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

export default async function LeadDetailPage({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = await params;

  let lead: Lead;
  try {
    lead = await getLead(id);
  } catch (error) {
    if (!(error instanceof ApiError)) throw error;
    return (
      <div className="space-y-6">
        <PageHeader title="Lead" />
        <ProblemNotice
          error={error}
          title={
            error.status === 404
              ? "That lead does not exist"
              : "The lead could not be loaded"
          }
        />
      </div>
    );
  }

  const [opportunitiesResult, names] = await Promise.all([
    safe<{ items: Opportunity[] }>(listOpportunities(), { items: [] }),
    personDirectory(),
  ]);

  const opportunities = opportunitiesResult.items.filter(
    (o) =>
      o.organization_id === lead.organization_id ||
      o.id === lead.converted_opportunity_id,
  );

  return (
    <div className="space-y-6">
      <PageHeader
        title={lead.ref}
        eyebrow={
          <span className="flex flex-wrap items-center gap-2">
            <Badge
              tone={
                {
                  draft: "neutral",
                  submitted: "info",
                  qualified: "success",
                  converted: "success",
                  lost: "danger",
                  disqualified: "danger",
                }[lead.status] as import("@/components/ui").Tone
              }
            >
              {humanise(lead.status)}
            </Badge>
            {lead.priority && lead.priority !== "normal" && (
              <Badge tone="neutral">{humanise(lead.priority)} priority</Badge>
            )}
          </span>
        }
        description={
          <span className="flex flex-wrap items-center gap-x-2 gap-y-1">
            <span>
              {lead.owner_person_id ? personName(names, lead.owner_person_id) : "Unassigned"}
            </span>
            <SignalRefresh topic={SALES_TOPIC} quiet />
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

      <StatGrid label="Lead at a glance">
        <Stat
          label="Estimated value"
          value={formatMinor(lead.estimated_value_minor, lead.currency)}
          icon="card"
          iconTone="info"
          hint={`Source: ${humanise(lead.source_key)}`}
        />
        <Stat
          label="Next action"
          value={lead.next_action ?? "—"}
          icon="clock"
          iconTone={lead.next_action_due_at ? "info" : "neutral"}
          hint={lead.next_action_due_at ? `Due ${formatDay(lead.next_action_due_at)}` : "No next action set"}
        />
      </StatGrid>

      <div className="grid grid-cols-1 gap-5 xl:grid-cols-[minmax(0,1.7fr)_minmax(0,1fr)]">
        <div className="flex flex-col gap-5">
          <Card title="Details">
            <Table>
              <tbody>
                <tr>
                  <Th className="text-muted w-[30%]">Contact</Th>
                  <Td className="tabular">
                    {lead.contact_name ?? (
                      <span className="text-muted">—</span>
                    )}
                  </Td>
                </tr>
                <tr>
                  <Th className="text-muted">Phone</Th>
                  <Td className="tabular text-muted">
                    {lead.contact_phone ?? "—"}
                  </Td>
                </tr>
                <tr>
                  <Th className="text-muted">Email</Th>
                  <Td className="tabular text-muted">
                    {lead.contact_email ?? "—"}
                  </Td>
                </tr>
                <tr>
                  <Th className="text-muted">Organization</Th>
                  <Td className="tabular text-muted">
                    {lead.organization_id ?? "—"}
                  </Td>
                </tr>
                {lead.need_summary && (
                  <tr>
                    <Th className="text-muted">Need summary</Th>
                    <Td className="whitespace-pre-wrap text-[14px]">
                      {lead.need_summary}
                    </Td>
                  </tr>
                )}
                {lead.qualification_notes && (
                  <tr>
                    <Th className="text-muted">Qualification notes</Th>
                    <Td className="whitespace-pre-wrap text-[14px]">
                      {lead.qualification_notes}
                    </Td>
                  </tr>
                )}
                {lead.lost_reason && lead.status === "lost" && (
                  <tr>
                    <Th className="text-muted">Lost reason</Th>
                    <Td className="text-[#C0352B] whitespace-pre-wrap text-[14px]">
                      {lead.lost_reason}
                    </Td>
                  </tr>
                )}
              </tbody>
            </Table>
          </Card>

          {lead.converted_opportunity_id && opportunities.length > 0 && (
            <Card title="Converted opportunity">
              <Table>
                <thead>
                  <tr>
                    <Th>Opportunity</Th>
                    <Th className="text-right">Value</Th>
                    <Th>Stage</Th>
                  </tr>
                </thead>
                <tbody>
                  {opportunities
                    .filter((o) => o.id === lead.converted_opportunity_id)
                    .map((o) => (
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

        <LeadEditor lead={lead} names={names} />
      </div>

      <p className="text-xs text-[#7A879C]">
        Created {formatDay(lead.created_at)} · updated {formatDay(lead.updated_at)} ·
        version {lead.version}
      </p>
    </div>
  );
}

function LeadEditor({
  lead,
  names,
}: {
  lead: Lead;
  names: Awaited<ReturnType<typeof personDirectory>>;
}) {
  const { busy, problem, run, reset } = useMutation();
  const [status, setStatus] = useState(lead.status);
  const [converting, setConverting] = useState(false);

  async function onSave(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const updated = await run(async () => {
      return setLeadStatus(lead.id, lead.version, { status: status as any });
    });
    if (updated) {
      reset();
    }
  }

  async function onConvert(event: React.MouseEvent<HTMLButtonElement>) {
    event.preventDefault();
    setConverting(true);
    const converted = await run(async () => {
      return convertLead(lead.id, lead.version, {
        title: lead.need_summary ?? `Opportunity from ${lead.ref}`,
        organization_id: lead.organization_id,
      });
    });
    if (converted) {
      setConverting(false);
      reset();
    }
  }

  return (
    <Card title="Manage lead">
      {problem && <ProblemNotice error={problem} title="The lead wasn't updated" />}

      <form onSubmit={onSave} className="flex flex-col gap-4 px-6 py-[22px]">
        <Field label="Status">
          <Select
            name="status"
            value={status}
            onChange={(e) => setStatus(e.target.value as any)}
          >
            <option value="draft">Draft</option>
            <option value="submitted">Submitted</option>
            <option value="assigned">Assigned</option>
            <option value="contacted">Contacted</option>
            <option value="qualified">Qualified</option>
            <option value="nurture">Nurture</option>
            <option value="lost">Lost</option>
            <option value="disqualified">Disqualified</option>
          </Select>
        </Field>
        <div className="flex justify-end gap-2 pt-4">
          <Button type="submit" variant="primary" disabled={busy}>
            {busy ? "Saving…" : "Save status"}
          </Button>
          {lead.status !== "converted" && lead.status !== "lost" && (
            <Button
              type="button"
              variant="secondary"
              onClick={onConvert}
              disabled={converting || busy}
            >
              {converting ? "Converting…" : "Convert to opportunity"}
            </Button>
          )}
        </div>
      </form>
    </Card>
  );
}
