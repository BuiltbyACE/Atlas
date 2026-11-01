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
  createFollowup,
  getFollowup,
  listFollowups,
  type Followup,
} from "@/lib/api/atlas";
import { ApiError } from "@/lib/api/client";
import { personDirectory, personName } from "@/lib/directory";
import { formatDay, formatDateTime, humanise } from "@/lib/format";
import { useMutation } from "@/lib/use-mutation";

/**
 * CRM Follow-ups (§8.1). Scheduled next touches across leads and opportunities.
 */
export const dynamic = "force-dynamic";

export default async function FollowupsPage() {
  const router = useRouter();
  const [followups, setFollowups] = useState<Followup[]>([]);
  const [error, setError] = useState<ApiError | null>(null);

  useEffect(() => {
    listFollowups({ limit: 200 }).then(
      (page) => setFollowups(page.items),
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
        title="Follow-ups"
        description="Scheduled next touches across leads and opportunities."
        actions={
          <>
            <ButtonLink href="/crm/followups/new" variant="primary">
              <Icon name="plus" className="h-4 w-4" />
              Schedule follow-up
            </ButtonLink>
          </>
        }
      />

      {error && (
        <ProblemNotice
          error={error}
          title={
            error.status === 403
              ? "You do not have permission to view follow-ups"
              : "Follow-ups could not be loaded"
          }
        />
      )}

      <Card padded={false}>
        {followups.length === 0 ? (
          <div className="p-[22px]">
            <EmptyState
              icon="clock"
              iconTone="info"
              className="min-h-[220px]"
              title="No follow-ups yet"
            >
              Schedule the first next touch.
            </EmptyState>
          </div>
        ) : (
          <Table>
            <thead>
              <tr>
                <Th>Related to</Th>
                <Th>Owner</Th>
                <Th>Due</Th>
                <Th>Channel</Th>
                <Th>Status</Th>
                <Th>Reason</Th>
              </tr>
            </thead>
            <tbody>
              {followups.map((f) => (
                <tr key={f.id}>
                  <Td className="tabular">
                    {f.lead_id ? "Lead" : f.opportunity_id ? "Opportunity" : "—"}
                    {`#${(f.lead_id ?? f.opportunity_id ?? "").slice(0, 8)}`}
                  </Td>
                  <Td className="tabular text-muted">
                    {personName(names, f.owner_person_id) ?? "—"}
                  </Td>
                  <Td className="tabular text-muted">
                    {formatDay(f.due_at)}
                  </Td>
                  <Td className="tabular text-muted">{humanise(f.channel)}</Td>
                  <Td>
                    <Badge
                      tone={
                        {
                          scheduled: "neutral",
                          done: "success",
                          missed: "danger",
                          cancelled: "neutral",
                        }[f.status] as import("@/components/ui").Tone
                      }
                    >
                      {humanise(f.status)}
                    </Badge>
                  </Td>
                  <Td className="tabular text-muted">
                    {f.reason ?? "—"}
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
