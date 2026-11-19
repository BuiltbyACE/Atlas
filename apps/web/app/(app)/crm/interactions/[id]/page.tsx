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
  getInteraction,
  type CrmInteraction,
} from "@/lib/api/atlas";
import { ApiError } from "@/lib/api/client";
import { personDirectory, personName } from "@/lib/directory";
import { formatDay, formatDateTime, humanise } from "@/lib/format";

/**
 * Interaction detail. Reads one logged interaction from Go.
 */
export const dynamic = "force-dynamic";

export default async function InteractionDetailPage({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = await params;

  let interaction: CrmInteraction;
  try {
    interaction = await getInteraction(id);
  } catch (error) {
    if (!(error instanceof ApiError)) throw error;
    return (
      <div className="space-y-6">
        <PageHeader title="Interaction" />
        <ProblemNotice
          error={error}
          title={
            error.status === 404
              ? "That interaction does not exist"
              : "The interaction could not be loaded"
          }
        />
      </div>
    );
  }

  const names = await personDirectory();

  return (
    <div className="space-y-6">
      <PageHeader
        title={humanise(interaction.interaction_type)}
        eyebrow={
          <span className="flex flex-wrap items-center gap-2">
            <Badge tone="neutral">{humanise(interaction.direction)}</Badge>
            {interaction.duration_minutes && (
              <Badge tone="info">{interaction.duration_minutes} min</Badge>
            )}
          </span>
        }
        description={
          <span className="flex flex-wrap items-center gap-x-2 gap-y-1">
            <span>
              Recorded by{" "}
              <b className="font-semibold text-night">
                {personName(names, interaction.recorded_by) ?? "—"}
              </b>
            </span>
            <span aria-hidden="true">·</span>
            <span>{formatDay(interaction.occurred_at)}</span>
            <SignalRefresh topic="crm" quiet />
          </span>
        }
        actions={
          <>
            <ButtonLink href="/crm/interactions" variant="secondary">
              <Icon name="chevronLeft" />
              Back to interactions
            </ButtonLink>
          </>
        }
      />

      <Card title="Details">
        <Table>
          <tbody>
            <tr>
              <Th className="text-muted w-[30%]">Type</Th>
              <Td className="tabular">{humanise(interaction.interaction_type)}</Td>
            </tr>
            <tr>
              <Th className="text-muted">Direction</Th>
              <Td className="tabular">{humanise(interaction.direction)}</Td>
            </tr>
            <tr>
              <Th className="text-muted">Outcome</Th>
              <Td className="tabular text-muted">
                {interaction.outcome ?? "—"}
              </Td>
            </tr>
            <tr>
              <Th className="text-muted">Next action</Th>
              <Td className="tabular text-muted">
                {interaction.next_action ?? "—"}
              </Td>
            </tr>
            {interaction.next_action_due_at && (
              <tr>
                <Th className="text-muted">Next action due</Th>
                <Td className="tabular text-muted">
                  {formatDay(interaction.next_action_due_at)}
                </Td>
              </tr>
            )}
            {interaction.summary && (
              <tr>
                <Th className="text-muted">Summary</Th>
                <Td className="tabular whitespace-pre-wrap text-[14px]">
                  {interaction.summary}
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
              <Th className="text-muted">Occurred at</Th>
              <Td className="tabular text-muted">
                {formatDateTime(interaction.occurred_at)}
              </Td>
            </tr>
            <tr>
              <Th className="text-muted">Occurred at</Th>
              <Td className="tabular text-muted">
                {formatDateTime(interaction.occurred_at)}
              </Td>
            </tr>
            <tr>
              <Th className="text-muted">Created</Th>
              <Td className="tabular text-muted">
                {formatDateTime(interaction.created_at)}
              </Td>
            </tr>
            <tr>
              <Th className="text-muted">Version</Th>
              <Td className="tabular text-muted">{interaction.version}</Td>
            </tr>
          </tbody>
        </Table>
      </Card>
    </div>
  );
}
