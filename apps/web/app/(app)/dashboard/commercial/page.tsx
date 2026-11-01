"use client";

import { SignalRefresh } from "@/components/signal-refresh";
import {
  Badge,
  Card,
  ChipLink,
  Chips,
  EmptyState,
  Legend,
  Meter,
  PageHeader,
  ProblemNotice,
  Stat,
  StatGrid,
  SWATCH,
  Table,
  Tag,
  Td,
  Th,
  statusTone,
  type Tone,
} from "@/components/ui";
import {
  getFinanceOverview,
  getPipeline,
  getReceivables,
  listLeads,
  listOpportunities,
  type FinanceOverview,
  type Lead,
  type Opportunity,
  type PipelineSummary,
  type ReceivablesAgeing,
} from "@/lib/api/atlas";
import { ApiError } from "@/lib/api/client";
import { SALES_TOPIC } from "@/lib/realtime/topics";
import { formatDay, formatMinor, formatMinorCompact, humanise } from "@/lib/format";

/**
 * External CEO commercial dashboard (`GET /api/v1/finance/overview`,
 * `GET /api/v1/crm/pipeline`, `GET /api/v1/finance/receivables`; blueprint
 * §8.1, §8.4). Commercial-first view: pipeline value, win/loss, receivables
 * ageing, cash position, open leads and follow-ups due.
 */
export const dynamic = "force-dynamic";

/** A settled load: either a value or the `ApiError` Go returned. */
async function load<T>(fn: () => Promise<T>): Promise<{ value: T | null; error: ApiError | null }> {
  try {
    return { value: await fn(), error: null };
  } catch (caught) {
    if (!(caught instanceof ApiError)) throw caught;
    return { value: null, error: caught };
  }
}

/** Lead badge tones. */
function leadBadgeTone(status: string): Tone {
  if (status === "converted") return "success";
  if (status === "disqualified") return "danger";
  if (status === "qualified" || status === "contacted") return "info";
  if (status === "nurture") return "warning";
  return "neutral";
}

export default async function CommercialDashboardPage({
  searchParams,
}: {
  searchParams: Promise<{ status?: string }>;
}) {
  const { status } = await searchParams;
  const filter = ["new", "assigned", "contacted", "qualified", "nurture"].includes(
    status ?? "",
  )
    ? (status as "new" | "assigned" | "contacted" | "qualified" | "nurture")
    : undefined;

  const [
    finance,
    pipeline,
    receivables,
    leadsPage,
    opportunitiesPage,
  ] = await Promise.all([
    load(() => getFinanceOverview()),
    load(() => getPipeline()),
    load(() => getReceivables()),
    load(() => listLeads({ limit: 200 })),
    load(() => listOpportunities({ limit: 200 })),
  ]);

  // Leads and opportunities are the core commercial data; if they can't be
  // read, show why and stop.
  if (leadsPage.error) {
    return (
      <div className="space-y-6">
        <PageHeader
          title="Commercial dashboard"
          description="Pipeline, receivables, cash and leads at a glance."
        />
        <ProblemNotice
          error={leadsPage.error}
          title={
            leadsPage.error.status === 403
              ? "You do not have permission to view commercial data"
              : "Commercial dashboard could not be loaded"
          }
        />
      </div>
    );
  }

  const leads = leadsPage.value?.items ?? [];
  const opportunities = opportunitiesPage.value?.items ?? [];
  const summary: PipelineSummary | null = pipeline.value;
  const financeData: FinanceOverview | null = finance.value;
  const receivablesData: ReceivablesAgeing | null = receivables.value;

  const openLeads = leads.filter(
    (l) =>
      l.status === "assigned" ||
      l.status === "contacted" ||
      l.status === "qualified" ||
      l.status === "nurture",
  );
  const openOpportunities = opportunities.filter(
    (o) =>
      o.stage_key !== "won" &&
      o.stage_key !== "lost" &&
      o.stage_key !== "rejected",
  );

  const currency =
    financeData?.currency ??
    opportunities.find((o) => o.currency)?.currency ??
    leads.find((l) => l.currency)?.currency ??
    "KES";

  const stages = (summary?.stages ?? []).filter((s) => s.count > 0);
  const buckets = receivablesData?.buckets ?? [];

  return (
    <div className="space-y-6">
      <PageHeader
        title="Commercial dashboard"
        description="Pipeline, receivables, cash and leads — the External CEO's view."
        actions={<SignalRefresh topic={SALES_TOPIC} quiet />}
      />

      <StatGrid label="Commercial overview">
        <Stat
          label="Open pipeline"
          value={formatMinorCompact(summary?.open_value_minor, currency)}
          icon="pie"
          iconTone="info"
          hint={`${summary?.open_opportunities ?? 0} open deal${summary?.open_opportunities === 1 ? "" : "s"}`}
        />
        <Stat
          label="Open leads"
          value={openLeads.length}
          icon="people"
          iconTone="accent"
          hint="Assigned, contacted, qualified or nurturing"
        />
        <Stat
          label="Cash position"
          value={formatMinorCompact(financeData?.cash_minor, currency)}
          icon="card"
          iconTone="success"
          hint="Across all accounts"
        />
        <Stat
          label="Receivables outstanding"
          value={formatMinorCompact(receivablesData?.total_outstanding_minor, currency)}
          tone={(receivablesData?.total_outstanding_minor ?? 0) > 0 ? "warning" : "neutral"}
          icon="clock"
          iconTone="warning"
          hint="Money owed by clients"
        />
      </StatGrid>

      <div className="grid grid-cols-1 gap-5 lg:grid-cols-2">
        <Card
          title="Pipeline by stage"
          description="Open deals by stage, weighted by value."
        >
          {stages.length === 0 ? (
            <EmptyState
              icon="pie"
              iconTone="info"
              className="min-h-[200px]"
              title="No open deals"
            >
              Opportunities appear here once a lead is qualified and converted.
            </EmptyState>
          ) : (
            <div className="space-y-4">
              <Meter
                label="Open pipeline value by stage"
                segments={stages.map((s, i) => ({
                  value: s.value_minor,
                  color: Object.values(SWATCH)[i % Object.values(SWATCH).length] as string,
                }))}
              />
              <Legend
                items={stages.map((s, i) => ({
                  color: Object.values(SWATCH)[i % Object.values(SWATCH).length] as string,
                  label: s.label,
                  value: `${s.count} · ${formatMinor(s.value_minor, currency)}`,
                }))}
              />
            </div>
          )}
        </Card>

        <Card
          title="Receivables ageing"
          description="Money owed by clients, by how long it's been outstanding."
        >
          {buckets.length === 0 ? (
            <EmptyState
              icon="check"
              className="min-h-[200px]"
              title="No receivables"
            >
              Invoices appear here once issued and unpaid.
            </EmptyState>
          ) : (
            <Table>
              <thead>
                <tr>
                  <Th>Ageing bucket</Th>
                  <Th className="text-right">Count</Th>
                  <Th className="text-right">Amount</Th>
                </tr>
              </thead>
              <tbody>
                {buckets.map((b) => (
                  <tr key={b.key}>
                    <Td>{humanise(b.key)}</Td>
                    <Td className="tabular text-right">{b.count}</Td>
                    <Td className="tabular text-right">
                      {formatMinor(b.amount_minor, currency)}
                    </Td>
                  </tr>
                ))}
              </tbody>
            </Table>
          )}
        </Card>
      </div>

      <div className="flex flex-wrap items-center justify-between gap-x-5 gap-y-3">
        <Chips label="Filter leads by status">
          <ChipLink href="/dashboard/commercial" pressed={!filter}>
            All
          </ChipLink>
          {["new", "assigned", "contacted", "qualified", "nurture"].map((s) => (
            <ChipLink
              key={s}
              href={`/dashboard/commercial?status=${s}`}
              pressed={filter === s}
            >
              {humanise(s)}
            </ChipLink>
          ))}
        </Chips>
      </div>

      <Card padded={false}>
        {openLeads.length === 0 ? (
          <div className="p-[22px]">
            <EmptyState
              icon="check"
              className="min-h-[220px]"
              title="No open leads"
            >
              Leads move here as their status changes.
            </EmptyState>
          </div>
        ) : (
          <Table>
            <thead>
              <tr>
                <Th>Lead</Th>
                <Th>Contact</Th>
                <Th>Status</Th>
                <Th className="text-right">Value</Th>
                <Th>Next action</Th>
                <Th>Owner</Th>
              </tr>
            </thead>
            <tbody>
              {leads.map((lead) => (
                <tr key={lead.id}>
                  <Td>
                    <div className="flex flex-col items-start gap-1">
                      <Tag>{lead.ref}</Tag>
                      {lead.need_summary && (
                        <span className="line-clamp-1 max-w-[26ch] text-[13px] text-muted">
                          {lead.need_summary}
                        </span>
                      )}
                    </div>
                  </Td>
                  <Td>
                    <div className="flex flex-col gap-0.5">
                      <span className="font-medium">{lead.contact_name ?? "—"}</span>
                      {(lead.contact_email || lead.contact_phone) && (
                        <span className="text-[13px] text-muted">
                          {lead.contact_email ?? lead.contact_phone}
                        </span>
                      )}
                    </div>
                  </Td>
                  <Td>
                    <Badge tone={leadBadgeTone(lead.status)} dot>
                      {humanise(lead.status)}
                    </Badge>
                  </Td>
                  <Td className="tabular text-right">
                    {formatMinor(lead.estimated_value_minor, lead.currency || currency)}
                  </Td>
                  <Td>
                    {lead.next_action ? (
                      <div className="flex flex-col gap-0.5">
                        <span className="line-clamp-1 max-w-[24ch]">{lead.next_action}</span>
                      </div>
                    ) : (
                      <span className="text-muted">—</span>
                    )}
                  </Td>
                  <Td className="text-muted">{lead.owner_person_id ?? "Unassigned"}</Td>
                </tr>
              ))}
            </tbody>
          </Table>
        )}
      </Card>
    </div>
  );
}
