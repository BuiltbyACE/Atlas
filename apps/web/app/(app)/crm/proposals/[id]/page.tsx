import { ButtonLink } from "@/components/form";
import { Icon } from "@/components/icons";
import { SignalRefresh } from "@/components/signal-refresh";
import {
  Badge,
  Card,
  PageHeader,
  ProblemNotice,
  Table,
  Td,
  Th,
} from "@/components/ui";
import {
  getProposal,
  type Proposal,
} from "@/lib/api/atlas";
import { ApiError } from "@/lib/api/client";
import { formatDay, formatMinor, humanise } from "@/lib/format";

/**
 * Proposal detail. Reads one priced offer from Go.
 */
export const dynamic = "force-dynamic";

export default async function ProposalDetailPage({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = await params;

  let proposal: Proposal;
  try {
    proposal = await getProposal(id);
  } catch (error) {
    if (!(error instanceof ApiError)) throw error;
    return (
      <div className="space-y-6">
        <PageHeader title="Proposal" />
        <ProblemNotice
          error={error}
          title={
            error.status === 404
              ? "That proposal does not exist"
              : "The proposal could not be loaded"
          }
        />
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <PageHeader
        title={proposal.ref}
        eyebrow={
          <span className="flex flex-wrap items-center gap-2">
            <Badge
              tone={
                {
                  draft: "neutral",
                  sent: "info",
                  accepted: "success",
                  declined: "danger",
                  expired: "neutral",
                }[proposal.status] as import("@/components/ui").Tone
              }
            >
              {humanise(proposal.status)}
            </Badge>
          </span>
        }
        description={
          <span className="flex flex-wrap items-center gap-x-2 gap-y-1">
            <span>
              Opportunity:{" "}
              <b className="font-semibold text-night">
                {proposal.opportunity_id.slice(0, 8)}
              </b>
            </span>
            <SignalRefresh topic="crm" quiet />
          </span>
        }
        actions={
          <>
            <ButtonLink href="/crm/proposals" variant="secondary">
              <Icon name="chevronLeft" />
              Back to proposals
            </ButtonLink>
          </>
        }
      />

      <Card title="Proposal details">
        <Table>
          <tbody>
            <tr>
              <Th className="text-muted w-[30%]">Opportunity</Th>
              <Td className="tabular text-muted">
                {proposal.opportunity_id.slice(0, 8)}
              </Td>
            </tr>
            <tr>
              <Th className="text-muted">Version</Th>
              <Td className="tabular">{proposal.version_no}</Td>
            </tr>
            <tr>
              <Th className="text-muted">Price</Th>
              <Td className="tabular text-right">
                {formatMinor(proposal.price_minor, proposal.currency)}
              </Td>
            </tr>
            {proposal.discount_minor && (
              <tr>
                <Th className="text-muted">Discount</Th>
                <Td className="tabular text-right">
                  {formatMinor(proposal.discount_minor, proposal.currency)}
                </Td>
              </tr>
            )}
            <tr>
              <Th className="text-muted">Valid until</Th>
              <Td className="tabular text-muted">
                {proposal.valid_until ? formatDay(proposal.valid_until) : "—"}
              </Td>
            </tr>
            {proposal.scope_summary && (
              <tr>
                <Th className="text-muted">Scope summary</Th>
                <Td className="tabular whitespace-pre-wrap text-[14px]">
                  {proposal.scope_summary}
                </Td>
              </tr>
            )}
          </tbody>
        </Table>
      </Card>

      <Card title="Timestamps">
        <Table>
          <tbody>
            <tr>
              <Th className="text-muted">Created</Th>
              <Td className="tabular text-muted">
                {proposal.created_at ? new Date(proposal.created_at).toLocaleDateString() : "—"}
              </Td>
            </tr>
            <tr>
              <Th className="text-muted">Updated</Th>
              <Td className="tabular text-muted">
                {proposal.updated_at ? new Date(proposal.updated_at).toLocaleDateString() : "—"}
              </Td>
            </tr>
            <tr>
              <Th className="text-muted">Version</Th>
              <Td className="tabular text-muted">{proposal.version}</Td>
            </tr>
          </tbody>
        </Table>
      </Card>
    </div>
  );
}
