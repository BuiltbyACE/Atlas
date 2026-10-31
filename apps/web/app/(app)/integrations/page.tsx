import type { ReactNode } from "react";

import {
  Badge,
  Card,
  ChipLink,
  Chips,
  EmptyState,
  PageHeader,
  ProblemNotice,
  Stat,
  StatGrid,
  Table,
  Td,
  Th,
  type Tone,
} from "@/components/ui";
import {
  createIntegrationRegistration,
  INTEGRATION_DIRECTION,
  INTEGRATION_KINDS,
  INTEGRATION_STATUSES,
  type Integration,
  type IntegrationBody,
  type IntegrationDirection,
  type IntegrationKind,
  type IntegrationListQuery,
  type IntegrationInput,
  type IntegrationStatus,
  listIntegrationRegistrations,
  listOutboundLog,
  listWebhookInbox,
  type OutboundLogEntry,
  type OutboundLogQuery,
  type WebhookInboxEntry,
  type WebhookInboxQuery,
  type WebhookProcessingStatus,
  updateIntegration,
} from "@/lib/api/atlas";
import { ApiError } from "@/lib/api/client";
import { requireUser } from "@/lib/auth/session";
import { formatDay, humanise } from "@/lib/format";
import { NewIntegrationButton, IntegrationActions } from "./integration-form";

/**
 * Integrations (`GET /api/v1/integrations/*`, gated by `integrations.*`; blueprint §8.9).
 * The page lists integration registrations, the webhook inbox for inbound
 * provider payloads, and the outbound log for outbound call attempts — the
 * management view into how Atlas connects to external systems.
 *
 * Go owns every rule and every authorisation. The page only presents what Go
 * answered.
 */

export const dynamic = "force-dynamic";

/** Integration kind filters. */
const KIND_FILTERS = ["api", "webhook", "file", "sftp"] as const;

/** Integration direction filters. */
const DIRECTION_FILTERS = ["in", "out", "both"] as const;

/** Integration status filters. */
const STATUS_FILTERS = ["active", "paused", "error"] as const;

/** Integration kind tones. */
function kindTone(kind: string): Tone {
  switch (kind) {
    case "api":
      return "info";
    case "webhook":
      return "warning";
    case "file":
      return "neutral";
    case "sftp":
      return "neutral";
    default:
      return "neutral";
  }
}

/** Integration status tones. */
function statusTone(status: string): Tone {
  switch (status) {
    case "active":
      return "success";
    case "paused":
      return "neutral";
    case "error":
      return "danger";
    default:
      return "neutral";
  }
}

/** Webhook processing status tones. */
function processingStatusTone(status: string): Tone {
  switch (status) {
    case "processed":
      return "success";
    case "pending":
      return "neutral";
    case "failed":
      return "danger";
    default:
      return "neutral";
  }
}

/** Circuit state tones. */
function circuitStateTone(state: string): Tone {
  switch (state) {
    case "closed":
      return "success";
    case "open":
      return "danger";
    case "half_open":
      return "warning";
    default:
      return "neutral";
  }
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

/** A small "showing the first page" footnote under a paginated table. */
function PageNote({ count, hasMore }: { count: number; hasMore?: boolean }) {
  if (count === 0) return null;
  return (
    <p className="px-[22px] pb-4 pt-3 text-[13px] text-muted">
      {count === 1 ? "1 record" : `${count} records`}
      {hasMore ? " · more available" : ""}
    </p>
  );
}

/** Integrations table: one row per registered integration. */
function IntegrationsTable({
  integrations,
  capabilities,
}: {
  integrations: Integration[];
  capabilities: string[];
}) {
  return (
    <Table>
      <thead>
        <tr>
          <Th>Integration</Th>
          <Th>Kind</Th>
          <Th>Direction</Th>
          <Th>Status</Th>
          <Th className="text-right">Actions</Th>
        </tr>
      </thead>
      <tbody>
        {integrations.map((integration) => (
          <tr key={integration.id}>
            <Td className="font-medium text-night">{integration.name}</Td>
            <Td className="text-muted">{integration.ref}</Td>
            <Td>
              <Badge tone={kindTone(integration.kind)}>{humanise(integration.kind)}</Badge>
            </Td>
            <Td>
              <Badge tone={directionTone(integration.direction)}>{humanise(integration.direction)}</Badge>
            </Td>
            <Td>
              <Badge tone={statusTone(integration.status)}>{humanise(integration.status)}</Badge>
            </Td>
            <Td className="text-right">
              <IntegrationActions integration={integration} capabilities={capabilities} />
            </Td>
          </tr>
        ))}
      </tbody>
    </Table>
  );
}

/** Webhook inbox table: raw inbound provider payloads. */
function WebhookInboxTable({
  inbox,
}: {
  inbox: WebhookInboxEntry[];
}) {
  return (
    <Table>
      <thead>
        <tr>
          <Th>Provider</Th>
          <Th>Event</Th>
          <Th>Delivery</Th>
          <Th>Received</Th>
          <Th>Status</Th>
        </tr>
      </thead>
      <tbody>
        {inbox.map((entry) => (
          <tr key={entry.id}>
            <Td className="font-medium text-night">{entry.provider}</Td>
            <Td className="text-muted">{entry.event_type}</Td>
            <Td className="text-muted truncate max-w-[200px]"><span title={entry.delivery_id}>{entry.delivery_id}</span></Td>
            <Td className="text-muted">{entry.received_at}</Td>
            <Td>
              <Badge tone={processingStatusTone(entry.processing_status)}>{humanise(entry.processing_status)}</Badge>
            </Td>
          </tr>
        ))}
      </tbody>
    </Table>
  );
}

/** Outbound log table: outbound call attempts. */
function OutboundLogTable({
  logs,
}: {
  logs: OutboundLogEntry[];
}) {
  return (
    <Table>
      <thead>
        <tr>
          <Th>Provider</Th>
          <Th>Endpoint</Th>
          <Th>Method</Th>
          <Th>Status</Th>
          <Th>Attempts</Th>
          <Th>Circuit</Th>
          <Th>Last attempt</Th>
        </tr>
      </thead>
      <tbody>
        {logs.map((log) => (
          <tr key={log.id}>
            <Td className="font-medium text-night">{log.provider}</Td>
            <Td className="text-muted truncate max-w-[200px]"><span title={log.endpoint}>{log.endpoint}</span></Td>
            <Td className="text-muted">{log.method}</Td>
            <Td className="text-muted">{log.status_code ?? "—"}</Td>
            <Td className="tabular text-right">{log.attempt}/{log.max_attempts}</Td>
            <Td>
              <Badge tone={circuitStateTone(log.circuit_state)}>{humanise(log.circuit_state)}</Badge>
            </Td>
            <Td className="text-muted">{log.last_attempt_at}</Td>
          </tr>
        ))}
      </tbody>
    </Table>
  );
}

/** Direction tones. */
function directionTone(direction: string): Tone {
  switch (direction) {
    case "in":
      return "info";
    case "out":
      return "warning";
    case "both":
      return "accent";
    default:
      return "neutral";
  }
}

/** An error card for one panel, so one refusal never blanks the page. */
function PanelError({ title, error }: { title: string; error: ApiError }): ReactNode {
  return (
    <div className="p-[22px]">
      <ProblemNotice error={error} title={title} />
    </div>
  );
}

export default async function IntegrationsPage({
  searchParams,
}: {
  searchParams: Promise<{
    kind?: string;
    direction?: string;
    status?: string;
  }>;
}) {
  const params = await searchParams;
  const kind = KIND_FILTERS.find((k) => k === params.kind);
  const direction = DIRECTION_FILTERS.find((d) => d === params.direction);
  const status = STATUS_FILTERS.find((s) => s === params.status);

  // The shell already guaranteed a session; this is the same read the API makes.
  await requireUser();

  // Who is looking decides which write controls appear. Go still authorises.
  const user = await requireUser();
  const held = new Set(user.roles);
  const isTeam = held.has("ceo_commercial") || held.has("ceo_internal");
  const capabilities: string[] = isTeam
    ? ["integrations.registry.manage", "integrations.webhook.manage"]
    : [];

  const [
    integrationsPage,
    inboxPage,
    outboundPage,
  ] = await Promise.all([
    load(() => listIntegrationRegistrations({ kind, direction, status, limit: 50 })),
    load(() => listWebhookInbox({ limit: 50 })),
    load(() => listOutboundLog({ limit: 50 })),
  ]);

  return (
    <div className="space-y-6">
      <PageHeader
        title="Integrations"
        description="Integration registry, webhook inbox, and outbound log — how Atlas connects to external systems."
        actions={
          <>
            <NewIntegrationButton
              people={[]}
              me={user.person_id}
              capabilities={capabilities}
              variant="secondary"
            />
          </>
        }
      />

      <StatGrid label="Integrations overview">
        <Stat
          label="Integrations"
          value={integrationsPage.value?.items.length ?? "—"}
          icon="signal"
          iconTone="info"
          hint={
            integrationsPage.value
              ? `${integrationsPage.value.items.filter((i) => i.status === "active" as IntegrationStatus).length} active`
              : "Integrations could not be read"
          }
        />
        <Stat
          label="Webhook inbox"
          value={inboxPage.value?.items.length ?? "—"}
          icon="shield"
          iconTone="neutral"
          hint={
            inboxPage.value
              ? `${inboxPage.value.items.filter((i) => i.processing_status === "pending").length} pending`
              : "Inbox could not be read"
          }
        />
        <Stat
          label="Outbound log"
          value={outboundPage.value?.items.length ?? "—"}
          icon="signal"
          iconTone="neutral"
          hint={
            outboundPage.value
              ? `${outboundPage.value.items.filter((o) => o.circuit_state === "open").length} open circuits`
              : "Outbound log could not be read"
          }
        />
      </StatGrid>

      <Card
        title="Integration registry"
        description="Every connection to an external system — endpoint, credential reference (never the secret), scopes, and status."
        padded={false}
      >
        {integrationsPage.error ? (
          <PanelError title="We could not load integrations" error={integrationsPage.error} />
        ) : !integrationsPage.value || integrationsPage.value.items.length === 0 ? (
          <div className="p-[22px]">
            <EmptyState
              icon="signal"
              iconTone="info"
              title="No integrations yet"
            >
              Integrations appear here once they are registered. Each integration records how
              Atlas connects to an external system.
            </EmptyState>
          </div>
        ) : (
          <>
            <div className="px-[22px] pb-1 pt-4">
              <Chips label="Filter integrations by kind">
                <ChipLink href="#" pressed={kind === undefined}>
                  Every kind
                </ChipLink>
                {KIND_FILTERS.map((k) => (
                  <ChipLink key={k} href="#" pressed={kind === k}>
                    {humanise(k)}
                  </ChipLink>
                ))}
              </Chips>
            </div>
            <IntegrationsTable
              integrations={integrationsPage.value.items}
              capabilities={capabilities}
            />
            <PageNote count={integrationsPage.value.items.length} hasMore={integrationsPage.value.has_more} />
          </>
        )}
      </Card>

      <Card
        title="Webhook inbox"
        description="Raw inbound provider payloads awaiting processing (§8.9, §11)."
        padded={false}
      >
        {inboxPage.error ? (
          <PanelError title="We could not load webhook inbox" error={inboxPage.error} />
        ) : !inboxPage.value || inboxPage.value.items.length === 0 ? (
          <div className="p-[22px]">
            <EmptyState
              icon="shield"
              iconTone="info"
              title="No inbound webhooks yet"
            >
              Inbound webhooks appear here once they are received. Each payload is stored raw
              and processed asynchronously.
            </EmptyState>
          </div>
        ) : (
          <>
            <WebhookInboxTable inbox={inboxPage.value.items} />
            <PageNote count={inboxPage.value.items.length} hasMore={inboxPage.value.has_more} />
          </>
        )}
      </Card>

      <Card
        title="Outbound log"
        description="Outbound call attempts with retry state and circuit breaker status (§8.9)."
        padded={false}
      >
        {outboundPage.error ? (
          <PanelError title="We could not load outbound log" error={outboundPage.error} />
        ) : !outboundPage.value || outboundPage.value.items.length === 0 ? (
          <div className="p-[22px]">
            <EmptyState
              icon="signal"
              iconTone="info"
              title="No outbound log entries yet"
            >
              Outbound call attempts appear here once they are recorded.
            </EmptyState>
          </div>
        ) : (
          <>
            <OutboundLogTable logs={outboundPage.value.items} />
            <PageNote count={outboundPage.value.items.length} hasMore={outboundPage.value.has_more} />
          </>
        )}
      </Card>
    </div>
  );
}
