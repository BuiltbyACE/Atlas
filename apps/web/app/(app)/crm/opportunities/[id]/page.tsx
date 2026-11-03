import Link from "next/link";

import { ButtonLink } from "@/components/form";
import { Icon } from "@/components/icons";
import { SignalRefresh } from "@/components/signal-refresh";
import {
  Badge,
  Card,
  PageHeader,
  ProblemNotice,
  Stat,
  StatGrid,
  Table,
  Td,
  Th,
} from "@/components/ui";
import {
  getOpportunity,
  listLeads,
  type Lead,
  type Opportunity,
} from "@/lib/api/atlas";
import { ApiError } from "@/lib/api/client";
import { personDirectory, personName } from "@/lib/directory";
import { formatDay, formatMinor, humanise } from "@/lib/format";

import { OpportunityEditor } from "./opportunity-editor";

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

