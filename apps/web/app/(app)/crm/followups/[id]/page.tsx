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
  getFollowup,
  type Followup,
} from "@/lib/api/atlas";
import { ApiError } from "@/lib/api/client";
import { personDirectory, personName } from "@/lib/directory";
import { formatDay, formatDateTime, humanise } from "@/lib/format";

import { FollowupActions } from "./followup-actions";

/**
 * Follow-up detail. Read one scheduled follow-up and complete or cancel it.
 */
export const dynamic = "force-dynamic";

export default async function FollowupDetailPage({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = await params;

  let followup: Followup;
  try {
    followup = await getFollowup(id);
  } catch (error) {
    if (!(error instanceof ApiError)) throw error;
    return (
      <div className="space-y-6">
        <PageHeader title="Follow-up" />
        <ProblemNotice
          error={error}
          title={
            error.status === 404
              ? "That follow-up does not exist"
              : "The follow-up could not be loaded"
          }
        />
      </div>
    );
  }

  const names = await personDirectory();

  const isOpen =
    followup.status !== "done" &&
    followup.status !== "missed" &&
    followup.status !== "cancelled";

  return (
    <div className="space-y-6">
      <PageHeader
        title={humanise(followup.channel)}
        eyebrow={
          <span className="flex flex-wrap items-center gap-2">
            <Badge
              tone={
                {
                  scheduled: "neutral",
                  done: "success",
                  missed: "danger",
                  cancelled: "neutral",
                }[followup.status] as import("@/components/ui").Tone
              }
            >
              {humanise(followup.status)}
            </Badge>
          </span>
        }
        description={
          <span className="flex flex-wrap items-center gap-x-2 gap-y-1">
            <span>
              Owner:{" "}
              <b className="font-semibold text-night">
                {personName(names, followup.owner_person_id) ?? "—"}
              </b>
            </span>
            <span aria-hidden="true">·</span>
            <span>Due {formatDay(followup.due_at)}</span>
            <SignalRefresh topic="crm" quiet />
          </span>
        }
        actions={
          <>
            <ButtonLink href="/crm/followups" variant="secondary">
              <Icon name="chevronLeft" />
              Back to follow-ups
            </ButtonLink>
          </>
        }
      />

      <Card title="Details">
        <Table>
          <tbody>
            <tr>
              <Th className="text-muted w-[30%]">Channel</Th>
              <Td className="tabular">{humanise(followup.channel)}</Td>
            </tr>
            <tr>
              <Th className="text-muted">Status</Th>
              <Td className="tabular">
                <Badge tone="neutral">{humanise(followup.status)}</Badge>
              </Td>
            </tr>
            <tr>
              <Th className="text-muted">Reason</Th>
              <Td className="tabular text-muted">
                {followup.reason ?? "—"}
              </Td>
            </tr>
            <tr>
              <Th className="text-muted">Outcome</Th>
              <Td className="tabular text-muted">
                {followup.outcome ?? "—"}
              </Td>
            </tr>
            <tr>
              <Th className="text-muted">Completed at</Th>
              <Td className="tabular text-muted">
                {followup.completed_at
                  ? formatDateTime(followup.completed_at)
                  : "—"}
              </Td>
            </tr>
          </tbody>
        </Table>
      </Card>

      {isOpen ? (
        <FollowupActions
          followupId={followup.id}
          version={followup.version}
        />
      ) : null}

      <Card title="Timestamps">
        <Table>
          <tbody>
            <tr>
              <Th className="text-muted">Due at</Th>
              <Td className="tabular text-muted">
                {formatDateTime(followup.due_at)}
              </Td>
            </tr>
            <tr>
              <Th className="text-muted">Created</Th>
              <Td className="tabular text-muted">
                {formatDateTime(followup.created_at)}
              </Td>
            </tr>
            <tr>
              <Th className="text-muted">Version</Th>
              <Td className="tabular text-muted">{followup.version}</Td>
            </tr>
          </tbody>
        </Table>
      </Card>
    </div>
  );
}
