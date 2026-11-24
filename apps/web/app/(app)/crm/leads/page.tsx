import Link from "next/link";

import { ButtonLink } from "@/components/form";
import { Icon } from "@/components/icons";
import { SignalRefresh } from "@/components/signal-refresh";
import {
  Badge,
  Card,
  ChipLink,
  Chips,
  EmptyState,
  PageHeader,
  ProblemNotice,
  Table,
  Tag,
  Td,
  Th,
  priorityTone,
  type Tone,
} from "@/components/ui";
import {
  LEAD_STATUSES,
  listLeadSources,
  listLeads,
  type Lead,
} from "@/lib/api/atlas";
import { ApiError } from "@/lib/api/client";
import { personDirectory, personName } from "@/lib/directory";
import { dueLabel, formatDay, formatMinor, humanise } from "@/lib/format";
import { SALES_TOPIC } from "@/lib/realtime/topics";

/**
 * Leads index (`GET /api/v1/crm/leads`, gated by `crm.lead.read`; §8.1).
 * The full register of captured demand, filterable by status, each row
 * linking to its detail page. Go owns every rule; this page only reads.
 */
export const dynamic = "force-dynamic";

/** Lead badge tones; the shared work/people map does not know these states. */
function leadTone(status: string): Tone {
  if (status === "converted") return "success";
  if (status === "disqualified") return "danger";
  if (status === "qualified" || status === "contacted") return "info";
  if (status === "nurture") return "warning";
  return "neutral";
}

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

export default async function LeadsPage({
  searchParams,
}: {
  searchParams: Promise<{ status?: string }>;
}) {
  const { status } = await searchParams;
  const filter = LEAD_STATUSES.find((s) => s === status);

  const [leadsPage, sources, names] = await Promise.all([
    load(() => listLeads({ limit: 200 })),
    load(() => listLeadSources()),
    personDirectory(),
  ]);

  if (leadsPage.error) {
    return (
      <div className="space-y-6">
        <PageHeader title="Leads" description="Captured demand and its owners." />
        <ProblemNotice
          error={leadsPage.error}
          title={
            leadsPage.error.status === 403
              ? "You do not have permission to view leads"
              : "Leads could not be loaded"
          }
        />
      </div>
    );
  }

  const leads = leadsPage.value?.items ?? [];
  const sourceLabel = (key: string) =>
    sources.value?.find((s) => s.key === key)?.label ?? humanise(key);
  const shown = filter ? leads.filter((l) => l.status === filter) : leads;

  return (
    <div className="space-y-6">
      <PageHeader
        title="Leads"
        description="Every captured lead, who owns it, and what happens next."
        actions={
          <>
            <SignalRefresh topic={SALES_TOPIC} quiet />
            <ButtonLink href="/crm/leads/new" variant="primary">
              <Icon name="plus" className="h-4 w-4" />
              New lead
            </ButtonLink>
          </>
        }
      />

      <Chips label="Filter leads by status">
        <ChipLink
          href="/crm/leads"
          pressed={!filter}
          count={leads.length}
        >
          All
        </ChipLink>
        {LEAD_STATUSES.map((s) => (
          <ChipLink
            key={s}
            href={`/crm/leads?status=${s}`}
            pressed={filter === s}
            count={leads.filter((l) => l.status === s).length}
          >
            {humanise(s)}
          </ChipLink>
        ))}
      </Chips>

      <Card padded={false}>
        {shown.length === 0 ? (
          <div className="p-[22px]">
            <EmptyState
              icon="check"
              className="min-h-[220px]"
              title={
                filter ? `No ${humanise(filter).toLowerCase()} leads` : "No leads yet"
              }
            >
              Website-form submissions and manually captured leads land here.
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
              {shown.map((lead) => (
                <tr key={lead.id}>
                  <Td>
                    <div className="flex flex-col items-start gap-1">
                      <Link
                        href={`/crm/leads/${lead.id}`}
                        className="font-semibold text-night hover:underline"
                      >
                        <Tag>{lead.ref}</Tag>
                      </Link>
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
                  <Td className="text-muted">{sourceLabel(lead.source_key)}</Td>
                  <Td>
                    <div className="flex flex-wrap items-center gap-1.5">
                      <Badge tone={leadTone(lead.status)} dot>
                        {humanise(lead.status)}
                      </Badge>
                      {lead.priority && lead.priority !== "normal" && (
                        <Badge tone={priorityTone(lead.priority)}>
                          {humanise(lead.priority)}
                        </Badge>
                      )}
                    </div>
                  </Td>
                  <Td className="tabular text-right">
                    {lead.estimated_value_minor !== undefined
                      ? formatMinor(lead.estimated_value_minor, lead.currency)
                      : "—"}
                  </Td>
                  <Td>
                    {lead.next_action ? (
                      <div className="flex flex-col gap-0.5">
                        <span className="line-clamp-1 max-w-[24ch]">
                          {lead.next_action}
                        </span>
                        {lead.next_action_due_at && (
                          <span className="text-[13px] text-muted">
                            {dueLabel(lead.next_action_due_at)}
                          </span>
                        )}
                      </div>
                    ) : (
                      <span className="text-muted">—</span>
                    )}
                  </Td>
                  <Td className="text-muted">
                    {lead.owner_person_id
                      ? (personName(names, lead.owner_person_id) ?? "—")
                      : "Unassigned"}
                  </Td>
                </tr>
              ))}
            </tbody>
          </Table>
        )}
      </Card>

      <p className="text-xs text-[#7A879C]">
        Showing {shown.length} of {leads.length} leads · updated{" "}
        {formatDay(new Date().toISOString())}
      </p>
    </div>
  );
}
