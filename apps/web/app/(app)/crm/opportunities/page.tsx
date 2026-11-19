import Link from "next/link";

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
  Tag,
  Td,
  Th,
} from "@/components/ui";
import {
  listOpportunities,
  listPipelineStages,
  type Opportunity,
} from "@/lib/api/atlas";
import { ApiError } from "@/lib/api/client";
import { personDirectory, personName } from "@/lib/directory";
import { formatDay, formatMinor, formatMinorCompact, humanise } from "@/lib/format";
import { SALES_TOPIC } from "@/lib/realtime/topics";

/**
 * Opportunities index (`GET /api/v1/crm/opportunities`, gated by
 * `crm.opportunity.read`; §8.1). Every qualified deal on the board, with its
 * stage, value and owner, linking to its detail page for stage moves.
 */
export const dynamic = "force-dynamic";

/** A settled load: either a value or the `ApiError` Go returned. */
async function load<T>(
  fn: () => Promise<T>,
): Promise<{ value: T | null; error: ApiError | null }> {
  try {
    return { value: await fn(), error: null };
  } catch (caught) {
    if (!(caught instanceof ApiError)) throw caught;
    return { value: null, error: caught };
  }
}

/** Stage badge tones: won is green, lost red, everything in play is info. */
function stageTone(stageKey: string): "success" | "danger" | "info" | "neutral" {
  if (stageKey === "won" || stageKey === "closed_won") return "success";
  if (stageKey === "lost" || stageKey === "rejected" || stageKey === "closed_lost")
    return "danger";
  if (stageKey === "new") return "neutral";
  return "info";
}

export default async function OpportunitiesPage() {
  const [opportunitiesPage, stages, names] = await Promise.all([
    load(() => listOpportunities({ limit: 200 })),
    load(() => listPipelineStages()),
    personDirectory(),
  ]);

  if (opportunitiesPage.error) {
    return (
      <div className="space-y-6">
        <PageHeader
          title="Opportunities"
          description="Qualified deals on the pipeline."
        />
        <ProblemNotice
          error={opportunitiesPage.error}
          title={
            opportunitiesPage.error.status === 403
              ? "You do not have permission to view opportunities"
              : "Opportunities could not be loaded"
          }
        />
      </div>
    );
  }

  const opportunities = opportunitiesPage.value?.items ?? [];
  const stageLabel = (key: string) =>
    stages.value?.find((s) => s.key === key)?.label ?? humanise(key);

  const open = opportunities.filter(
    (o) => stageTone(o.stage_key) === "info" || o.stage_key === "new",
  );
  const won = opportunities.filter((o) => stageTone(o.stage_key) === "success");
  const currency =
    opportunities.find((o) => o.currency)?.currency ?? "KES";
  const openValue = open.reduce((sum, o) => sum + (o.value_minor ?? 0), 0);

  return (
    <div className="space-y-6">
      <PageHeader
        title="Opportunities"
        description="Every qualified deal, where it sits, and who owns it."
        actions={<SignalRefresh topic={SALES_TOPIC} quiet />}
      />

      <StatGrid label="Pipeline at a glance">
        <Stat
          label="Open deals"
          value={open.length}
          icon="pie"
          iconTone="info"
          hint="Currently in play"
        />
        <Stat
          label="Open value"
          value={formatMinorCompact(openValue, currency)}
          icon="card"
          iconTone="violet"
          hint="Sum of open deal values"
        />
        <Stat
          label="Won"
          value={won.length}
          tone="success"
          icon="check"
          iconTone="success"
          hint="Closed won all time"
        />
        <Stat
          label="Total on board"
          value={opportunities.length}
          icon="people"
          iconTone="accent"
          hint="Including lost and rejected"
        />
      </StatGrid>

      <Card padded={false}>
        {opportunities.length === 0 ? (
          <div className="p-[22px]">
            <EmptyState
              icon="card"
              iconTone="info"
              className="min-h-[220px]"
              title="No opportunities yet"
            >
              Convert a qualified lead and it becomes an opportunity here.
            </EmptyState>
          </div>
        ) : (
          <Table>
            <thead>
              <tr>
                <Th>Deal</Th>
                <Th>Stage</Th>
                <Th className="text-right">Value</Th>
                <Th className="text-right">Probability</Th>
                <Th>Expected close</Th>
                <Th>Owner</Th>
              </tr>
            </thead>
            <tbody>
              {opportunities.map((opportunity) => (
                <tr key={opportunity.id}>
                  <Td>
                    <div className="flex flex-col items-start gap-1">
                      <Link
                        href={`/crm/opportunities/${opportunity.id}`}
                        className="font-semibold text-night hover:underline"
                      >
                        {opportunity.title}
                      </Link>
                      <Tag>{opportunity.ref}</Tag>
                    </div>
                  </Td>
                  <Td>
                    <Badge tone={stageTone(opportunity.stage_key)}>
                      {stageLabel(opportunity.stage_key)}
                    </Badge>
                  </Td>
                  <Td className="tabular text-right">
                    {opportunity.value_minor !== undefined
                      ? formatMinor(opportunity.value_minor, opportunity.currency)
                      : "—"}
                  </Td>
                  <Td className="tabular text-right text-muted">
                    {opportunity.probability !== undefined
                      ? `${opportunity.probability}%`
                      : "—"}
                  </Td>
                  <Td className="text-muted">
                    {opportunity.expected_close_at
                      ? formatDay(opportunity.expected_close_at)
                      : "—"}
                  </Td>
                  <Td className="text-muted">
                    {opportunity.owner_person_id
                      ? (personName(names, opportunity.owner_person_id) ?? "—")
                      : "—"}
                  </Td>
                </tr>
              ))}
            </tbody>
          </Table>
        )}
      </Card>
    </div>
  );
}
