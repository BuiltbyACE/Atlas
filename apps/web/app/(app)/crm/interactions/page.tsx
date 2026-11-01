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
  createInteraction,
  getInteraction,
  listInteractions,
  type CrmInteraction,
} from "@/lib/api/atlas";
import { ApiError } from "@/lib/api/client";
import { personDirectory, personName } from "@/lib/directory";
import { formatDay, formatDateTime, humanise } from "@/lib/format";
import { useMutation } from "@/lib/use-mutation";

/**
 * CRM Interactions (§8.1). Logged calls, emails, WhatsApp messages, meetings
 * and notes across leads, contacts and opportunities.
 */
export const dynamic = "force-dynamic";

export default async function InteractionsPage() {
  const router = useRouter();
  const [interactions, setInteractions] = useState<CrmInteraction[]>([]);
  const [error, setError] = useState<ApiError | null>(null);

  useEffect(() => {
    listInteractions({ limit: 200 }).then(
      (page) => setInteractions(page.items),
      (err) => {
        if (err instanceof ApiError) setError(err);
        else throw err;
      },
    );
  }, []);

  const names = await personDirectory();

  return (
    <div className="space-y-6">
      <PageHeader
        title="Interactions"
        description="Logged calls, emails, WhatsApp messages, meetings and notes."
        actions={
          <>
            <ButtonLink href="/crm/interactions/new" variant="primary">
              <Icon name="plus" className="h-4 w-4" />
              Log interaction
            </ButtonLink>
          </>
        }
      />

      {error && (
        <ProblemNotice
          error={error}
          title={
            error.status === 403
              ? "You do not have permission to view interactions"
              : "Interactions could not be loaded"
          }
        />
      )}

      <Card padded={false}>
        {interactions.length === 0 ? (
          <div className="p-[22px]">
            <EmptyState
              icon="clipboard"
              iconTone="info"
              className="min-h-[220px]"
              title="No interactions yet"
            >
              Log the first call, email, meeting or note.
            </EmptyState>
          </div>
        ) : (
          <Table>
            <thead>
              <tr>
                <Th>Type</Th>
                <Th>Direction</Th>
                <Th>Related to</Th>
                <Th>Occurred</Th>
                <th>Outcome</th>
                <Th>Recorded by</Th>
              </tr>
            </thead>
            <tbody>
              {interactions.map((i) => (
                <tr key={i.id}>
                  <Td>
                    <Badge tone="neutral">{humanise(i.interaction_type)}</Badge>
                  </Td>
                  <Td className="tabular text-muted">{humanise(i.direction)}</Td>
                  <Td className="tabular">
                    {i.contact_id || i.lead_id || i.opportunity_id ? (
                      <span className="text-muted">
                        {i.contact_id ? "Contact" : i.lead_id ? "Lead" : "Opportunity"}{" "}
                        {`#${(i.contact_id ?? i.lead_id ?? i.opportunity_id!).slice(0, 8)}`}
                      </span>
                    ) : (
                      <span className="text-muted">—</span>
                    )}
                  </Td>
                  <Td className="tabular text-muted">
                    {formatDay(i.occurred_at)}
                  </Td>
                  <Td className="tabular text-muted">
                    {i.outcome ?? "—"}
                  </Td>
                  <Td className="tabular text-muted">
                    {personName(names, i.recorded_by) ?? i.recorded_by.slice(0, 8)}
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
