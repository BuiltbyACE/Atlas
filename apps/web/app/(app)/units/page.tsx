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
  getUnit,
  getIntegration,
  getIndicatorDefinition,
  getSnapshot,
  getFinanceSummary,
  listFinanceSummaries,
  listIndicatorDefinitions,
  listIntegrations,
  listSnapshots,
  listUnits,
  type Unit,
  type UnitIntegration,
  type UnitFinanceSummary,
  type UnitIndicatorDefinition,
  type UnitSnapshot,
} from "@/lib/api/atlas";
import { ApiError } from "@/lib/api/client";
import { requireUser } from "@/lib/auth/session";
import { formatDay, humanise } from "@/lib/format";
import type { PersonOption } from "@/lib/directory";
import { NewUnitButton, UnitActions } from "./unit-form";
import { NewIntegrationButton, IntegrationActions } from "./integration-form";
import {
  NewIndicatorDefinitionButton,
  IndicatorDefinitionActions,
} from "./indicator-definition-form";

/**
 * Units (`GET /api/v1/units/*`, gated by `units.*`; blueprint §8.7). The page
 * lists business units, their integrations with systems of record, indicator
 * definitions and snapshots, and finance summaries — the management view into
 * each unit without over-collecting its data.
 *
 * Go owns every rule and every authorisation. The page only presents what Go
 * answered.
 */

export const dynamic = "force-dynamic";

/** Unit kind filters. */
const KIND_FILTERS = ["school", "department", "project", "region"] as const;

/** Unit kind tones. */
function kindTone(kind: string): Tone {
  switch (kind) {
    case "school":
      return "info";
    case "department":
      return "neutral";
    case "project":
      return "warning";
    default:
      return "neutral";
  }
}

/** Integration sync status tones. */
function syncStatusTone(status: string): Tone {
  switch (status) {
    case "ok":
      return "success";
    case "failed":
      return "danger";
    default:
      return "neutral";
  }
}

/** Indicator kind tones. */
function indicatorKindTone(kind: string): Tone {
  switch (kind) {
    case "count":
      return "info";
    case "ratio":
      return "neutral";
    case "amount":
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

/** The units register: one row per unit with its derived counts. */
function UnitsTable({
  units,
  people,
  me,
  capabilities,
}: {
  units: Unit[];
  people: PersonOption[];
  me?: string;
  capabilities: string[];
}) {
  return (
    <Table>
      <thead>
        <tr>
          <Th>Unit</Th>
          <Th>Code</Th>
          <Th>Kind</Th>
          <Th>Status</Th>
          <Th className="text-right">Integrations</Th>
          <Th className="text-right">Indicators</Th>
          <Th>Owner</Th>
          <Th className="text-right">Actions</Th>
        </tr>
      </thead>
      <tbody>
        {units.map((unit) => (
          <tr key={unit.id}>
            <Td>
              <span className="flex items-center gap-2">
                <span className="font-medium text-night">{unit.name}</span>
                <span className="mt-0.5 block font-mono text-[12px] text-muted">{unit.ref}</span>
              </span>
            </Td>
            <Td className="text-muted">{unit.code}</Td>
            <Td>
              <Badge tone={kindTone(unit.kind)}>
                {humanise(unit.kind)}
              </Badge>
            </Td>
            <Td>
              <Badge tone={unit.status === "active" ? "success" : "neutral"}>
                {humanise(unit.status)}
              </Badge>
            </Td>
            <Td className="tabular text-right">
              {unit.integration_count}
            </Td>
            <Td className="tabular text-right">
              {unit.indicator_count}
            </Td>
            <Td className="text-muted">{unit.owner?.name ?? "—"}</Td>
            <Td className="text-right">
              <UnitActions
                unit={unit}
                people={people}
                capabilities={capabilities}
              />
            </Td>
          </tr>
        ))}
      </tbody>
    </Table>
  );
}

/** Integrations: how Atlas connects to each unit's system of record. */
function IntegrationsTable({
  integrations,
  people,
  me,
  capabilities,
  unitOptions,
}: {
  integrations: UnitIntegration[];
  people: PersonOption[];
  me?: string;
  capabilities: string[];
  unitOptions: { id: string; name: string }[];
}) {
  return (
    <Table>
      <thead>
        <tr>
          <Th>Integration</Th>
          <Th>Unit</Th>
          <Th>Kind</Th>
          <Th>Endpoint</Th>
          <Th>Last sync</Th>
          <Th>Status</Th>
          <Th>Active</Th>
          <Th className="text-right">Actions</Th>
        </tr>
      </thead>
      <tbody>
        {integrations.map((integration) => (
          <tr key={integration.id}>
            <Td className="font-medium text-night">{integration.name}</Td>
            <Td className="text-muted">{integration.unit_name}</Td>
            <Td>
              <Badge tone={indicatorKindTone(integration.kind)}>
                {humanise(integration.kind)}
              </Badge>
            </Td>
            <Td className="max-w-[280px] truncate font-mono text-[12px] text-muted">
              <span title={integration.endpoint}>{integration.endpoint}</span>
            </Td>
            <Td className="text-muted">
              {integration.last_sync_at ? formatDay(integration.last_sync_at) : "—"}
            </Td>
            <Td>
              <Badge tone={syncStatusTone(integration.last_sync_status)}>
                {humanise(integration.last_sync_status)}
              </Badge>
            </Td>
            <Td>
              <Badge tone={integration.active ? "success" : "neutral"}>
                {integration.active ? "Active" : "Inactive"}
              </Badge>
            </Td>
            <Td className="text-right">
              <IntegrationActions
                integration={integration}
                units={unitOptions}
                people={people}
                me={me}
                capabilities={capabilities}
              />
            </Td>
          </tr>
        ))}
      </tbody>
    </Table>
  );
}

/** Indicator definitions: what we measure for each unit. */
function IndicatorDefinitionsTable({
  definitions,
  people,
  me,
  capabilities,
  unitOptions,
}: {
  definitions: UnitIndicatorDefinition[];
  people: PersonOption[];
  me?: string;
  capabilities: string[];
  unitOptions: { id: string; name: string }[];
}) {
  return (
    <Table>
      <thead>
        <tr>
          <Th>Indicator</Th>
          <Th>Unit</Th>
          <Th>Kind</Th>
          <Th>Source field</Th>
          <Th>Target</Th>
          <Th>Active</Th>
          <Th className="text-right">Actions</Th>
        </tr>
      </thead>
      <tbody>
        {definitions.map((definition) => (
          <tr key={definition.id}>
            <Td className="font-medium text-night">{definition.name}</Td>
            <Td className="text-muted">{definition.unit_name}</Td>
            <Td>
              <Badge tone={indicatorKindTone(definition.kind)}>
                {humanise(definition.kind)}
              </Badge>
            </Td>
            <Td className="text-muted">{definition.source_field}</Td>
            <Td className="text-muted">{definition.target ?? "—"}</Td>
            <Td>
              <Badge tone={definition.active ? "success" : "neutral"}>
                {definition.active ? "Active" : "Inactive"}
              </Badge>
            </Td>
            <Td className="text-right">
              <IndicatorDefinitionActions
                definition={definition}
                units={unitOptions}
                people={people}
                me={me}
                capabilities={capabilities}
              />
            </Td>
          </tr>
        ))}
      </tbody>
    </Table>
  );
}

/** Snapshots: a dated indicator set for a unit. */
function SnapshotsTable({
  snapshots,
}: {
  snapshots: UnitSnapshot[];
}) {
  return (
    <Table>
      <thead>
        <tr>
          <Th>Indicator</Th>
          <Th>Unit</Th>
          <Th>Date</Th>
          <Th>Value</Th>
          <Th>Met target</Th>
        </tr>
      </thead>
      <tbody>
        {snapshots.map((snapshot) => (
          <tr key={snapshot.id}>
            <Td className="font-medium text-night">{snapshot.indicator_name}</Td>
            <Td className="text-muted">{snapshot.unit_name}</Td>
            <Td className="text-muted">
              {formatDay(snapshot.on_date)}
            </Td>
            <Td className="tabular text-right">{snapshot.value}</Td>
            <Td>
              <Badge tone={snapshot.met_target ? "success" : "neutral"}>
                {snapshot.met_target ? "Yes" : "No"}
              </Badge>
            </Td>
          </tr>
        ))}
      </tbody>
    </Table>
  );
}

/** Finance summaries: period roll-up of income, expenses, receivables. */
function FinanceSummariesTable({
  summaries,
}: {
  summaries: UnitFinanceSummary[];
}) {
  return (
    <Table>
      <thead>
        <tr>
          <Th>Unit</Th>
          <Th>Period</Th>
          <Th className="text-right">Income</Th>
          <Th className="text-right">Expenses</Th>
          <Th className="text-right">Receivables</Th>
        </tr>
      </thead>
      <tbody>
        {summaries.map((summary) => (
          <tr key={summary.id}>
            <Td className="font-medium text-night">{summary.unit_name}</Td>
            <Td className="text-muted">
              {formatDay(summary.period_start)} — {formatDay(summary.period_end)}
            </Td>
            <Td className="tabular text-right">{summary.income_minor}</Td>
            <Td className="tabular text-right">{summary.expenses_minor}</Td>
            <Td className="tabular text-right">{summary.receivables_minor}</Td>
          </tr>
        ))}
      </tbody>
    </Table>
  );
}

/** An error card for one panel, so one refusal never blanks the page. */
function PanelError({ title, error }: { title: string; error: ApiError }): ReactNode {
  return (
    <div className="p-[22px]">
      <ProblemNotice error={error} title={title} />
    </div>
  );
}

export default async function UnitsPage({
  searchParams,
}: {
  searchParams: Promise<{
    kind?: string;
    status?: string;
    active?: string;
  }>;
}) {
  const params = await searchParams;
  const kind = KIND_FILTERS.find((k) => k === params.kind);
  const status = params.status === "active" || params.status === "retired" ? params.status : undefined;
  const active = params.active === "true" || params.active === "false" ? params.active === "true" : undefined;

  // The shell already guaranteed a session; this is the same read the API makes.
  await requireUser();

  // Who is looking decides which write controls appear. Go still authorises.
  const user = await requireUser();
  const held = new Set(user.roles);
  const isTeam = held.has("ceo_commercial") || held.has("ceo_internal");
  const capabilities: string[] = isTeam
    ? ["units.unit.manage"]
    : [];

  const [unitsPage, integrationsPage, definitionsPage, snapshotsPage, summariesPage] =
    await Promise.all([
      load(() => listUnits({ kind, status, active, limit: 50 })),
      load(() => listIntegrations({ limit: 50 })),
      load(() => listIndicatorDefinitions({ limit: 50 })),
      load(() => listSnapshots({ limit: 50 })),
      load(() => listFinanceSummaries({ limit: 50 })),
    ]);

  const unitOptions = (unitsPage.value?.items ?? []).map((unit) => ({
    id: unit.id,
    name: unit.name,
  }));

  return (
    <div className="space-y-6">
      <PageHeader
        title="Units"
        description="Management view into each business unit — integrations, indicators and finance summaries."
        actions={
          <>
            <NewUnitButton
              people={[]}
              me={user.person_id}
              capabilities={capabilities}
              variant="secondary"
            />
            <NewIntegrationButton
              units={unitOptions}
              people={[]}
              me={user.person_id}
              capabilities={capabilities}
            />
            <NewIndicatorDefinitionButton
              units={unitOptions}
              people={[]}
              me={user.person_id}
              capabilities={capabilities}
            />
          </>
        }
      />

      <StatGrid label="Units overview">
        <Stat
          label="Units"
          value={unitsPage.value?.items.length ?? "—"}
          icon="building"
          iconTone="info"
          hint={
            unitsPage.value
              ? `${unitsPage.value.items.filter((u) => u.status === "active").length} active`
              : "Units could not be read"
          }
        />
        <Stat
          label="Integrations"
          value={integrationsPage.value?.items.length ?? "—"}
          icon="signal"
          iconTone="neutral"
          hint={
            integrationsPage.value
              ? `${integrationsPage.value.items.filter((i) => i.last_sync_status === "ok").length} syncing OK`
              : "Integrations could not be read"
          }
        />
        <Stat
          label="Indicator definitions"
          value={definitionsPage.value?.items.length ?? "—"}
          icon="sliders"
          iconTone="neutral"
          hint="What we measure for each unit"
        />
        <Stat
          label="Snapshots"
          value={snapshotsPage.value?.items.length ?? "—"}
          icon="calendar"
          iconTone="neutral"
          hint="Dated indicator values per unit"
        />
      </StatGrid>

      <Card
        title="Business units"
        description="Each unit is a scope — unit managers see theirs, CEOs see all."
        padded={false}
      >
        {unitsPage.error ? (
          <PanelError title="We could not load the units" error={unitsPage.error} />
        ) : !unitsPage.value || unitsPage.value.items.length === 0 ? (
          <div className="p-[22px]">
            <EmptyState
              icon="building"
              iconTone="info"
              title="No units yet"
            >
              Business units appear here once they are registered. The school is the first unit.
            </EmptyState>
          </div>
        ) : (
          <>
            <div className="px-[22px] pb-1 pt-4">
              <Chips label="Filter units by kind">
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
            <UnitsTable
              units={unitsPage.value.items}
              people={[]}
              me={user.person_id}
              capabilities={capabilities}
            />
            <PageNote count={unitsPage.value.items.length} hasMore={unitsPage.value.has_more} />
          </>
        )}
      </Card>

      <Card
        title="Integrations"
        description="How Atlas connects to each unit's system of record — endpoint and credential reference only, never the secret."
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
              Integrations appear here once they are configured. Sync failures raise
              units.sync.failed.v1 (§8.7).
            </EmptyState>
          </div>
        ) : (
          <>
            <IntegrationsTable
              integrations={integrationsPage.value.items}
              people={[]}
              me={user.person_id}
              capabilities={capabilities}
              unitOptions={unitOptions}
            />
            <PageNote count={integrationsPage.value.items.length} hasMore={integrationsPage.value.has_more} />
          </>
        )}
      </Card>

      <Card
        title="Indicator definitions"
        description="What we measure for each unit — name, formula, target, thresholds."
        padded={false}
      >
        {definitionsPage.error ? (
          <PanelError title="We could not load indicator definitions" error={definitionsPage.error} />
        ) : !definitionsPage.value || definitionsPage.value.items.length === 0 ? (
          <div className="p-[22px]">
            <EmptyState
              icon="sliders"
              iconTone="info"
              title="No indicator definitions yet"
            >
              Indicator definitions appear here once they are configured. Indicator breaches raise
              units.indicator.breached.v1 (§8.7).
            </EmptyState>
          </div>
        ) : (
          <>
            <IndicatorDefinitionsTable
              definitions={definitionsPage.value.items}
              people={[]}
              me={user.person_id}
              capabilities={capabilities}
              unitOptions={unitOptions}
            />
            <PageNote count={definitionsPage.value.items.length} hasMore={definitionsPage.value.has_more} />
          </>
        )}
      </Card>

      <Card
        title="Snapshots"
        description="A dated indicator set for a unit — one row per indicator per date."
        padded={false}
      >
        {snapshotsPage.error ? (
          <PanelError title="We could not load snapshots" error={snapshotsPage.error} />
        ) : !snapshotsPage.value || snapshotsPage.value.items.length === 0 ? (
          <div className="p-[22px]">
            <EmptyState
              icon="calendar"
              iconTone="info"
              title="No snapshots yet"
            >
              Snapshots appear here once they are recorded by the sync worker.
            </EmptyState>
          </div>
        ) : (
          <>
            <SnapshotsTable snapshots={snapshotsPage.value.items} />
            <PageNote count={snapshotsPage.value.items.length} hasMore={snapshotsPage.value.has_more} />
          </>
        )}
      </Card>

      <Card
        title="Finance summaries"
        description="Period roll-up of income, expenses and receivables per unit. Money is integer minor units."
        padded={false}
      >
        {summariesPage.error ? (
          <PanelError title="We could not load finance summaries" error={summariesPage.error} />
        ) : !summariesPage.value || summariesPage.value.items.length === 0 ? (
          <div className="p-[22px]">
            <EmptyState
              icon="card"
              iconTone="info"
              title="No finance summaries yet"
            >
              Finance summaries appear here once they are computed for each period.
            </EmptyState>
          </div>
        ) : (
          <>
            <FinanceSummariesTable summaries={summariesPage.value.items} />
            <PageNote count={summariesPage.value.items.length} hasMore={summariesPage.value.has_more} />
          </>
        )}
      </Card>
    </div>
  );
}
