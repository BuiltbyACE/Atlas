import Link from "next/link";

import { ButtonLink } from "@/components/form";
import { Icon } from "@/components/icons";
import { SignalRefresh } from "@/components/signal-refresh";
import {
  Badge,
  Card,
  EmptyState,
  PageHeader,
  ProblemNotice,
  Table,
  Tag,
  Td,
  Th,
  type Tone,
} from "@/components/ui";
import {
  PROPOSAL_STATUSES,
  listOpportunities,
  listProposals,
} from "@/lib/api/atlas";
import { ApiError } from "@/lib/api/client";
import { formatDay, formatMinor, humanise } from "@/lib/format";
import { SALES_TOPIC } from "@/lib/realtime/topics";

/**
 * CRM Proposals (§8.1). Priced offers against opportunities, from draft to
 * accepted or declined.
 *
 * Go exposes proposals per opportunity (`GET /api/v1/crm/opportunities/{id}/
 * proposals`, `crm.proposal.read`), not as one global list, so this page
 * reads the opportunity board and fans out per deal. The board is bounded by
 * the limit below, which keeps the fan-out honest.
 */
export const dynamic = "force-dynamic";

/** How many opportunities' proposals the index will pull in one load. */
const OPPORTUNITY_LIMIT = 100;

/** Proposal badge tones, in lifecycle order. */
function proposalTone(status: string): Tone {
  if (status === "accepted") return "success";
  if (status === "declined") return "danger";
  if (status === "sent") return "info";
  if (status === "expired") return "warning";
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

export default async function ProposalsPage() {
  const opportunitiesPage = await load(() =>
    listOpportunities({ limit: OPPORTUNITY_LIMIT }),
  );

  // The board is what the fan-out walks; if it cannot be read, say why.
  if (opportunitiesPage.error) {
    return (
      <div className="space-y-6">
        <PageHeader
          title="Proposals"
          description="Priced offers against opportunities."
        />
        <ProblemNotice
          error={opportunitiesPage.error}
          title={
            opportunitiesPagePageTitle(opportunitiesPage.error.status)
          }
        />
      </div>
    );
  }

  const opportunities = opportunitiesPage.value?.items ?? [];
  const proposalLists = await Promise.all(
    opportunities.map((o) =>
      load(() => listProposals(o.id)).then((result) => ({
        opportunity: o,
        result,
      })),
    ),
  );

  const proposals = proposalLists.flatMap(({ opportunity, result }) =>
    (result.value?.items ?? []).map((proposal) => ({
      proposal,
      opportunity,
    })),
  );
  // Newest first, matching the per-opportunity ordering Go returns.
  proposals.sort(
    (a, b) =>
      new Date(b.proposal.created_at).getTime() -
      new Date(a.proposal.created_at).getTime(),
  );

  const totalProposed = proposals.reduce(
    (sum, { proposal }) => sum + proposal.price_minor,
    0,
  );
  const acceptedValue = proposals
    .filter(({ proposal }) => proposal.status === "accepted")
    .reduce((sum, { proposal }) => sum + proposal.price_minor, 0);
  const currency = proposals.find(({ proposal }) => proposal.currency)
    ?.proposal.currency;

  return (
    <div className="space-y-6">
      <PageHeader
        title="Proposals"
        description="Priced offers against opportunities, from draft to accepted or declined."
        actions={
          <>
            <SignalRefresh topic={SALES_TOPIC} quiet />
            <ButtonLink href="/crm" variant="secondary">
              <Icon name="chevronLeft" />
              Back to CRM
            </ButtonLink>
          </>
        }
      />

      <Card padded={false}>
        {proposals.length === 0 ? (
          <div className="p-[22px]">
            <EmptyState
              icon="document"
              iconTone="info"
              className="min-h-[220px]"
              title={
                opportunities.length === 0
                  ? "No proposals yet"
                  : "Create proposals from opportunities"
              }
            >
              Open an opportunity in CRM and use “Create proposal” to draft a
              priced offer. Proposals appear here once created.
            </EmptyState>
          </div>
        ) : (
          <Table>
            <thead>
              <tr>
                <Th>Proposal</Th>
                <Th>Opportunity</Th>
                <Th className="text-right">Version</Th>
                <Th className="text-right">Price</Th>
                <Th>Valid until</Th>
                <Th>Status</Th>
              </tr>
            </thead>
            <tbody>
              {proposals.map(({ proposal, opportunity }) => (
                <tr key={proposal.id}>
                  <Td>
                    <div className="flex flex-col items-start gap-1">
                      <Link
                        href={`/crm/proposals/${proposal.id}`}
                        className="font-semibold text-night hover:underline"
                      >
                        <Tag>{proposal.ref}</Tag>
                      </Link>
                      {proposal.scope_summary && (
                        <span className="line-clamp-1 max-w-[28ch] text-[13px] text-muted">
                          {proposal.scope_summary}
                        </span>
                      )}
                    </div>
                  </Td>
                  <Td>
                    <Link
                      href={`/crm/opportunities/${opportunity.id}`}
                      className="font-medium text-night hover:underline"
                    >
                      {opportunity.title}
                    </Link>
                  </Td>
                  <Td className="tabular text-right text-muted">
                    v{proposal.version_no}
                  </Td>
                  <Td className="tabular text-right">
                    {formatMinor(proposal.price_minor, proposal.currency)}
                    {proposal.discount_minor ? (
                      <span className="block text-[12px] text-muted">
                        −{formatMinor(proposal.discount_minor, proposal.currency)}{" "}
                        discount
                      </span>
                    ) : null}
                  </Td>
                  <Td className="text-muted">
                    {proposal.valid_until ? formatDay(proposal.valid_until) : "—"}
                  </Td>
                  <Td>
                    <Badge tone={proposalTone(proposal.status)} dot>
                      {humanise(proposal.status)}
                    </Badge>
                  </Td>
                </tr>
              ))}
            </tbody>
          </Table>
        )}
      </Card>

      <div className="grid grid-cols-1 gap-5 xl:grid-cols-2">
        <Card title="Proposal statuses">
          <Table>
            <thead>
              <tr>
                <Th>Status</Th>
                <Th className="text-right">Count</Th>
              </tr>
            </thead>
            <tbody>
              {PROPOSAL_STATUSES.map((status) => {
                const count = proposals.filter(
                  ({ proposal }) => proposal.status === status,
                ).length;
                return (
                  <tr key={status}>
                    <Td>
                      <Badge tone={proposalTone(status)}>{humanise(status)}</Badge>
                    </Td>
                    <Td className="tabular text-right">{count}</Td>
                  </tr>
                );
              })}
            </tbody>
          </Table>
        </Card>
        <Card title="Proposal value">
          <Table>
            <thead>
              <tr>
                <Th>Metric</Th>
                <Th className="text-right">Value</Th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <Td>Total proposed</Td>
                <Td className="tabular text-right">
                  {formatMinor(totalProposed, currency)}
                </Td>
              </tr>
              <tr>
                <Td>Accepted value</Td>
                <Td className="tabular text-right">
                  {formatMinor(acceptedValue, currency)}
                </Td>
              </tr>
            </tbody>
          </Table>
        </Card>
      </div>
    </div>
  );
}

/** The ProblemNotice title for a failed board load. */
function opportunitiesPagePageTitle(status: number): string {
  return status === 403
    ? "You do not have permission to view proposals"
    : "Proposals could not be loaded";
}
