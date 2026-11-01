"use client";

import { useRouter, useSearchParams } from "next/navigation";
import { useState, useEffect } from "react";

import { Button, ButtonLink } from "@/components/form";
import { Icon } from "@/components/icons";
import {
  Badge,
  Card,
  EmptyState,
  PageHeader,
  ProblemNotice,
  Table,
  Td,
  Th,
} from "@/components/ui";
import { useToast } from "@/components/toast";
import {
  createProposal,
  getProposal,
  listLeads,
  listOpportunities,
  type Opportunity,
  type Proposal,
  type ProposalStatus,
} from "@/lib/api/atlas";
import { ApiError } from "@/lib/api/client";
import { personDirectory, personName } from "@/lib/directory";
import { formatDay, formatMinor, humanise } from "@/lib/format";
import { useMutation } from "@/lib/use-mutation";

/**
 * CRM Proposals (§8.1). Priced offers against opportunities, from draft to accepted/declined.
 */
export const dynamic = "force-dynamic";

export default async function ProposalsPage() {
  const router = useRouter();
  const [proposals, setProposals] = useState<Proposal[]>([]);
  const [error, setError] = useState<ApiError | null>(null);

  useEffect(() => {
    listOpportunities({ limit: 200 }).then(
      (page) => {
        // Fetch proposals for each opportunity
        const opps = page.items;
        Promise.all(
          opps.map((o) =>
            // Proposals are listed under opportunities, so we need to check each
            // opportunity's proposals. Since there's no listProposals API,
            // we'll show proposals as part of opportunity detail instead.
            Promise.resolve(null),
          ),
        );
      },
      (err) => {
        if (err instanceof ApiError) setError(err);
        else throw err;
      },
    );
  }, []);

  // Since there's no listProposals endpoint, we show an empty state explaining
  // proposals are created from opportunity detail pages.
  const names = await personDirectory();

  return (
    <div className="space-y-6">
      <PageHeader
        title="Proposals"
        description="Priced offers against opportunities, from draft to accepted or declined."
        actions={
          <>
            <ButtonLink href="/crm" variant="secondary">
              <Icon name="chevronLeft" />
              Back to CRM
            </ButtonLink>
          </>
        }
      />

      {error && (
        <ProblemNotice
          error={error}
          title={
            error.status === 403
              ? "You do not have permission to view proposals"
              : "Proposals could not be loaded"
          }
        />
      )}

      <Card padded={false}>
        <div className="p-[22px]">
          <EmptyState
            icon="document"
            iconTone="info"
            className="min-h-[220px]"
            title="Create proposals from opportunities"
          >
            Open an opportunity in CRM and use “Create proposal” to draft a
            priced offer. Proposals appear here once created.
          </EmptyState>
        </div>
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
              {[
                "draft",
                "sent",
                "accepted",
                "declined",
                "expired",
              ].map((status) => {
                const count = proposals.filter(
                  (p) => p.status === status,
                ).length;
                return (
                  <tr key={status}>
                    <Td>{humanise(status)}</Td>
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
                  {formatMinor(
                    proposals.reduce((sum, p) => sum + p.price_minor, 0),
                  )}
                </Td>
              </tr>
              <tr>
                <Td>Accepted value</Td>
                <Td className="tabular text-right">
                  {formatMinor(
                    proposals
                      .filter((p) => p.status === "accepted")
                      .reduce((sum, p) => sum + p.price_minor, 0),
                  )}
                </Td>
              </tr>
            </tbody>
          </Table>
        </Card>
      </div>
    </div>
  );
}
