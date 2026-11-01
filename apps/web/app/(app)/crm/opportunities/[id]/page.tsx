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
  createProposal,
  getOpportunity,
  listLeads,
  listOpportunities,
  moveOpportunityStage,
  type CrmOrganization,
  type Lead,
  type Opportunity,
} from "@/lib/api/atlas";
import { ApiError } from "@/lib/api/client";
import { personDirectory, personName } from "@/lib/directory";
import { formatDay, formatMinor, humanise } from "@/lib/format";
import { useMutation } from "@/lib/use-mutation";

/**
 * Opportunity detail. Reads the opportunity and its related lead from Go, then
 * offers stage movement and win/lose actions.
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

export default async function OpportunityDetailPage({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = await params;

  let opportunity: Opportunity;
  try {
    opportunity = await getOpportunity(id);
  } catch (error) {
    if (!(error instanceof ApiError)) throw error;
    return (
      <div className="space-y-6">
        <PageHeader title="Opportunity" />
        <ProblemNotice
          error={error}
          title={
            error.status === 404
              ? "That opportunity does not exist"
              : "The opportunity could not be loaded"
          }
        />
      </div>
    );
  }

  const [leadsResult, names] = await Promise.all([
    safe<{ items: Lead[] }>(listLeads(), { items: [] }),
    personDirectory(),
  ]);

  const relatedLead = leadsResult.items.find((l) => l.converted_opportunity_id === id);

  const isOpen =
    opportunity.stage_key !== "won" &&
    opportunity.stage_key !== "lost" &&
    opportunity.stage_key !== "rejected";

  return (
    <div className="space-y-6">
      <PageHeader
        title={opportunity.title}
        eyebrow={
          <span className="flex flex-wrap items-center gap-2">
            <Badge tone="info">{opportunity.ref}</Badge>
            <Badge tone="neutral">{humanise(opportunity.stage_key)}</Badge>
            {isOpen ? <Badge tone="warning">Open</Badge> : <Badge tone="neutral">Closed</Badge>}
          </span>
        }
        description={
          <span className="flex flex-wrap items-center gap-x-2 gap-y-1">
            <span>
              {opportunity.owner_person_id
                ? personName(names, opportunity.owner_person_id)
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

      <StatGrid label="Opportunity at a glance">
        <Stat
          label="Value"
          value={formatMinor(opportunity.value_minor, opportunity.currency)}
          icon="card"
          iconTone="info"
        />
        <Stat
          label="Probability"
          value={
            opportunity.probability !== undefined
              ? `${opportunity.probability}%`
              : "—"
          }
          icon="pie"
          iconTone="info"
          hint="Weighted pipeline value"
        />
        <Stat
          label="Expected close"
          value={formatDay(opportunity.expected_close_at ?? "—")}
          icon="clock"
          iconTone="neutral"
        />
      </StatGrid>

      <div className="grid grid-cols-1 gap-5 xl:grid-cols-[minmax(0,1.7fr)_minmax(0,1fr)]">
        <div className="flex flex-col gap-5">
          <Card title="Details">
            <Table>
              <tbody>
                <tr>
                  <Th className="text-muted w-[30%]">Stage</Th>
                  <Td className="tabular">
                    <Badge tone="neutral">{humanise(opportunity.stage_key)}</Badge>
                  </Td>
                </tr>
                <tr>
                  <Th className="text-muted">Currency</Th>
                  <Td className="tabular text-muted">
                    {opportunity.currency}
                  </Td>
                </tr>
                {relatedLead && (
                  <tr>
                    <Th className="text-muted">Source lead</Th>
                    <Td className="tabular">
                      <Link
                        href={`/crm/leads/${relatedLead.id}`}
                        className="font-semibold text-night hover:underline"
                      >
                        {relatedLead.ref}
                      </Link>
                    </Td>
                  </tr>
                )}
                {opportunity.lost_reason && opportunity.stage_key === "lost" && (
                  <tr>
                    <Th className="text-muted">Lost reason</Th>
                    <Td className="text-[#C0352B] whitespace-pre-wrap text-[14px]">
                      {opportunity.lost_reason}
                    </Td>
                  </tr>
                )}
              </tbody>
            </Table>
          </Card>
        </div>

        <OpportunityEditor opportunity={opportunity} />
      </div>

      <p className="text-xs text-[#7A879C]">
        Created {formatDay(opportunity.created_at)} · updated {formatDay(opportunity.updated_at)} ·
        version {opportunity.version}
      </p>
    </div>
  );
}

function OpportunityEditor({
  opportunity,
}: {
  opportunity: Opportunity;
}) {
  const { busy, problem, run, reset } = useMutation();
  const [stageKey, setStageKey] = useState(opportunity.stage_key);
  const [isOpen, setIsOpen] = useState(
    opportunity.stage_key !== "won" &&
      opportunity.stage_key !== "lost" &&
      opportunity.stage_key !== "rejected",
  );

  const isWonOrLost = opportunity.stage_key === "won" || opportunity.stage_key === "lost";

  async function onSave(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const updated = await run(async () => {
      return moveOpportunityStage(opportunity.id, opportunity.version, {
        stage_key: stageKey as any,
      });
    });
    if (updated) {
      reset();
    }
  }

  async function onWin(event: React.MouseEvent<HTMLButtonElement>) {
    event.preventDefault();
    const updated = await run(async () => {
      return moveOpportunityStage(opportunity.id, opportunity.version, {
        stage_key: "won" as any,
      });
    });
    if (updated) {
      reset();
    }
  }

  async function onLose(event: React.MouseEvent<HTMLButtonElement>) {
    event.preventDefault();
    const updated = await run(async () => {
      return moveOpportunityStage(opportunity.id, opportunity.version, {
        stage_key: "lost" as any,
      });
    });
    if (updated) {
      reset();
    }
  }

  async function onCreateProposal(event: React.MouseEvent<HTMLButtonElement>) {
    event.preventDefault();
    const created = await run(async () => {
      return createProposal(opportunity.id, {
        scope_summary: "",
        price_minor: 0,
        currency: opportunity.currency,
      });
    });
    if (created) {
      reset();
    }
  }

  return (
    <Card title="Manage opportunity">
      {problem && <ProblemNotice error={problem} title="The opportunity wasn't updated" />}

      <form onSubmit={onSave} className="flex flex-col gap-4 px-6 py-[22px]">
        <Field label="Stage">
          <Select
            name="stage_key"
            value={stageKey}
            onChange={(e) => setStageKey(e.target.value)}
          >
            <option value="new">New</option>
            <option value="qualified">Qualified</option>
            <option value="proposal">Proposal</option>
            <option value="negotiation">Negotiation</option>
            <option value="closed_won">Closed won</option>
            <option value="closed_lost">Closed lost</option>
          </Select>
        </Field>

        {isOpen && (
          <div className="flex items-end gap-2 pt-4">
            <Button type="submit" variant="primary" disabled={busy}>
              {busy ? "Saving…" : "Save stage"}
            </Button>
            <Button
              type="button"
              variant="primary"
              onClick={onWin}
              disabled={busy}
            >
              Mark as won
            </Button>
            <Button
              type="button"
              variant="danger"
              onClick={onLose}
              disabled={busy}
            >
              Mark as lost
            </Button>
            <Button
              type="button"
              variant="secondary"
              onClick={onCreateProposal}
              disabled={busy}
            >
              {busy ? "Creating…" : "Create proposal"}
            </Button>
          </div>
        )}

        {!isOpen && (
          <div className="flex items-end gap-2 pt-4">
            <Button
              type="button"
              variant="ghost"
              onClick={() => setIsOpen(true)}
            >
              Reopen
            </Button>
          </div>
        )}
      </form>
    </Card>
  );
}
