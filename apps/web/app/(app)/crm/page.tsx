import { ButtonLink } from "@/components/form";
import { Icon } from "@/components/icons";
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
  priorityTone,
  type Tone,
} from "@/components/ui";
import {
  LEAD_STATUSES,
  getPipeline,
  listFollowups,
  listLeadSources,
  listLeads,
  listOpportunities,
  listPipelineStages,
  type Lead,
  type LeadSource,
  type Opportunity,
  type PipelineStage,
  type PipelineSummary,
} from "@/lib/api/atlas";
import { ApiError } from "@/lib/api/client";
import { personDirectory, personName } from "@/lib/directory";
import { SALES_TOPIC } from "@/lib/realtime/topics";
import {
  dueLabel,
  formatDay,
  formatMinor,
  formatMinorCompact,
  humanise,
} from "@/lib/format";

/**
 * CRM (`GET /api/v1/crm/*`, gated by `crm.*`; blueprint §8.1). The page reads
 * the pipeline roll-up, one page of leads and opportunities, and the scheduled
 * follow-ups, then composes the sales picture: value in the pipeline, where
 * deals sit, and which leads need a next action. Go owns every rule and amount;
 * nothing here writes.
 */

export const dynamic = "force-dynamic";

/** The lead states that still need an owner and a dated next action (§8.1). */
const OPEN_LEAD_STATUSES = new Set(["assigned", "contacted", "qualified", "nurture"]);

/** Stage-bar colours, in pipeline order (won/lost are not part of the bar). */
const STAGE_COLORS = [
  SWATCH.info,
  SWATCH.violet,
  SWATCH.warning,
  SWATCH.orange,
  SWATCH.accent,
  SWATCH.success,
];

/** Lead badge tones; the shared work/people map does not know these states. */
function leadTone(status: string): Tone {
  if (status === "converted") return "success";
  if (status === "disqualified") return "danger";
  if (status === "qualified" || status === "contacted") return "info";
  if (status === "nurture") return "warning";
  return "neutral";
}

/** A settled load: either a value or the `ApiError` Go returned. */
async function load<T>(fn: () => Promise<T>): Promise<{ value: T | null; error: ApiError | null }> {
  try {
    return { value: await fn(), error: null };
  } catch (caught) {
    if (!(caught instanceof ApiError)) throw caught;
    return { value: null, error: caught };
  }
}

export default async function CrmPage({
  searchParams,
}: {
  searchParams: Promise<{ status?: string }>;
}) {
  const { status } = await searchParams;
  const filter = LEAD_STATUSES.find((s) => s === status);

  const [pipeline, stages, leadsPage, opportunitiesPage, followupsPage, sources] =
    await Promise.all([
      load(() => getPipeline()),
      load(() => listPipelineStages()),
      load(() => listLeads({ limit: 200 })),
      load(() => listOpportunities({ limit: 200 })),
      load(() => listFollowups({ status: "scheduled", limit: 100 })),
      load(() => listLeadSources()),
    ]);

  // Leads are the page: if they cannot be read, say why and stop.
  if (leadsPage.error) {
    return (
      <div className="space-y-6">
        <PageHeader title="CRM" description="Leads, deals and the pipeline." />
        <ProblemNotice
          error={leadsPage.error}
          title={
            leadsPage.error.status === 403
              ? "You do not have permission to view the CRM"
              : "The CRM could not be loaded"
          }
        />
      </div>
    );
  }

  const leads = leadsPage.value?.items ?? [];
  const opportunities = opportunitiesPage.value?.items ?? [];
  const followups = followupsPage.value?.items ?? [];
  const summary: PipelineSummary | null = pipeline.value;
  const stageList: PipelineStage[] = stages.value ?? [];
  const sourceList: LeadSource[] = sources.value ?? [];
  const names = await personDirectory();

  const currency =
    opportunities.find((o) => o.currency)?.currency ??
    leads.find((l) => l.currency)?.currency ??
    "KES";

  const sourceLabel = (key: string) =>
    sourceList.find((s) => s.key === key)?.label ?? humanise(key);
  const stageLabel = (key: string) =>
    stageList.find((s) => s.key === key)?.label ?? humanise(key);

  const openLeads = leads.filter((l) => OPEN_LEAD_STATUSES.has(l.status));
  const dueFollowups = followups.filter((f) => {
    if (!f.due_at) return false;
    return new Date(f.due_at).getTime() <= Date.now() + 86_400_000;
  });
  const shownLeads = filter ? leads.filter((l) => l.status === filter) : leads;
  const openStages = (summary?.stages ?? []).filter((s) => s.count > 0);

  return (
    <div className="space-y-6">
      <PageHeader
        title="CRM"
        description="Where every lead came from, who owns it, and what happens next."
        actions={
          <>
            <SignalRefresh topic={SALES_TOPIC} quiet />
            <ButtonLink href="/contracts" variant="secondary">
              <Icon name="document" />
              Contracts
            </ButtonLink>
          </>
        }
      />

      <StatGrid label="Sales overview">
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
          label="Follow-ups due"
          value={dueFollowups.length}
          tone={dueFollowups.length > 0 ? "warning" : "neutral"}
          icon="clock"
          iconTone="warning"
          hint="Scheduled for today or overdue"
        />
        <Stat
          label="Deals in play"
          value={opportunities.length}
          icon="card"
          iconTone="violet"
          hint="Opportunities on the board"
        />
      </StatGrid>

      <Card
        title="Pipeline"
        description="Open deals by stage, weighted by value."
      >
        {openStages.length === 0 ? (
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
              segments={openStages.map((s, i) => ({
                value: s.value_minor,
                color: STAGE_COLORS[i % STAGE_COLORS.length],
              }))}
            />
            <Legend
              items={openStages.map((s, i) => ({
                color: STAGE_COLORS[i % STAGE_COLORS.length],
                label: s.label,
                value: `${s.count} · ${formatMinor(s.value_minor, currency)}`,
              }))}
            />
          </div>
        )}
      </Card>

      <div className="flex flex-wrap items-center justify-between gap-x-5 gap-y-3">
        <Chips label="Filter leads by status">
          <ChipLink
            href="/crm"
            pressed={!filter}
            count={leads.length}
          >
            All
          </ChipLink>
          {LEAD_STATUSES.map((s) => (
            <ChipLink
              key={s}
              href={`/crm?status=${s}`}
              pressed={filter === s}
              count={leads.filter((l) => l.status === s).length}
            >
              {humanise(s)}
            </ChipLink>
          ))}
        </Chips>
        <span className="inline-flex items-center gap-2 text-sm text-muted">
          <Icon name="people" className="h-4 w-4" />
          Newest first
        </span>
      </div>

      <Card padded={false}>
        {shownLeads.length === 0 ? (
          <div className="p-[22px]">
            <EmptyState
              icon="check"
              className="min-h-[220px]"
              title={filter ? `No ${humanise(filter).toLowerCase()} leads` : "No leads yet"}
            >
              {filter
                ? "Leads move here as their status changes."
                : "Website-form submissions and manually captured leads land here."}
            </EmptyState>
          </div>
        ) : (
          <Table>
            <thead>
              <tr>
                <Th>Lead</Th>
                <Th>Contact</Th>
                <Th>Source</Th>
                <Th>Status</Th>
                <Th className="text-right">Value</Th>
                <Th>Next action</Th>
                <Th>Owner</Th>
              </tr>
            </thead>
            <tbody>
              {shownLeads.map((lead) => (
                <LeadRow
                  key={lead.id}
                  lead={lead}
                  currency={currency}
                  source={sourceLabel(lead.source_key)}
                  owner={lead.owner_person_id ? personName(names, lead.owner_person_id) : "Unassigned"}
                />
              ))}
            </tbody>
          </Table>
        )}
      </Card>

      <Card title="Opportunities" description="Qualified deals on the pipeline." padded={false}>
        {opportunities.length === 0 ? (
          <div className="p-[22px]">
            <EmptyState
              icon="card"
              iconTone="info"
              className="min-h-[200px]"
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
                <Th>Expected close</Th>
                <Th>Owner</Th>
              </tr>
            </thead>
            <tbody>
              {opportunities.map((opportunity) => (
                <OpportunityRow
                  key={opportunity.id}
                  opportunity={opportunity}
                  currency={currency}
                  stage={stageLabel(opportunity.stage_key)}
                  owner={personName(names, opportunity.owner_person_id)}
                />
              ))}
            </tbody>
          </Table>
        )}
      </Card>
    </div>
  );
}

function LeadRow({
  lead,
  currency,
  source,
  owner,
}: {
  lead: Lead;
  currency: string;
  source: string;
  owner: string;
}) {
  const followUp = dueLabel(lead.next_action_due_at);
  return (
    <tr>
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
      <Td className="text-muted">{source}</Td>
      <Td>
        <div className="flex flex-wrap items-center gap-1.5">
          <Badge tone={leadTone(lead.status)} dot>
            {humanise(lead.status)}
          </Badge>
          {lead.priority && lead.priority !== "normal" && (
            <Badge tone={priorityTone(lead.priority)}>{humanise(lead.priority)}</Badge>
          )}
        </div>
      </Td>
      <Td className="tabular text-right">
        {formatMinor(lead.estimated_value_minor, lead.currency || currency)}
      </Td>
      <Td>
        {lead.next_action ? (
          <div className="flex flex-col gap-0.5">
            <span className="line-clamp-1 max-w-[24ch]">{lead.next_action}</span>
            {followUp && <span className="text-[13px] text-muted">{followUp}</span>}
          </div>
        ) : (
          <span className="text-muted">—</span>
        )}
      </Td>
      <Td className="text-muted">{owner}</Td>
    </tr>
  );
}

function OpportunityRow({
  opportunity,
  currency,
  stage,
  owner,
}: {
  opportunity: Opportunity;
  currency: string;
  stage: string;
  owner: string;
}) {
  return (
    <tr>
      <Td>
        <div className="flex flex-col items-start gap-1">
          <span className="font-medium">{opportunity.title}</span>
          <Tag>{opportunity.ref}</Tag>
        </div>
      </Td>
      <Td>
        <Badge tone="info">{stage}</Badge>
      </Td>
      <Td className="tabular text-right">
        {formatMinor(opportunity.value_minor, opportunity.currency || currency)}
      </Td>
      <Td className="text-muted">{formatDay(opportunity.expected_close_at)}</Td>
      <Td className="text-muted">{owner}</Td>
    </tr>
  );
}
