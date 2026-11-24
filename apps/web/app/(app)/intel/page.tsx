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
  listBriefs,
  listDecisions,
  listMetricSnapshots,
  listRisks,
  type Brief,
  type Decision,
  type MetricSnapshot,
  type Risk,
} from "@/lib/api/atlas";
import { ApiError } from "@/lib/api/client";
import { requireUser } from "@/lib/auth/session";
import { formatDay, humanise } from "@/lib/format";
import type { PersonOption } from "@/lib/directory";
import { NewDecisionButton, DecisionActions } from "./decision-form";
import { NewRiskButton, RiskActions } from "./risk-form";
import { NewBriefButton, BriefActions } from "./brief-form";
import { NewMetricSnapshotButton } from "./metric-snapshot-form";

/**
 * Intel (`GET /api/v1/intel/*`, gated by `intel.*`; blueprint §8.8). The page
 * lists decisions, risks, briefs and metric snapshots — the executive
 * intelligence view for fewer surprises and faster, evidence-based decisions.
 *
 * Go owns every rule and every authorisation. The page only presents what Go
 * answered.
 */

export const dynamic = "force-dynamic";

/** Decision outcome filters. */
const OUTCOME_FILTERS = ["open", "approved", "rejected", "deferred"] as const;

/** Risk kind filters. */
const KIND_FILTERS = ["client", "delivery", "financial", "people", "systems"] as const;

/** Decision outcome tones. */
function outcomeTone(outcome: string): Tone {
  switch (outcome) {
    case "approved":
      return "success";
    case "rejected":
      return "danger";
    case "deferred":
      return "warning";
    default:
      return "neutral";
  }
}

/** Risk kind tones. */
function riskKindTone(kind: string): Tone {
  switch (kind) {
    case "client":
      return "info";
    case "delivery":
      return "warning";
    case "financial":
      return "danger";
    case "people":
      return "neutral";
    default:
      return "neutral";
  }
}

/** Risk likelihood/impact tones. */
function riskSeverityTone(severity: string): Tone {
  switch (severity) {
    case "critical":
      return "danger";
    case "high":
      return "warning";
    case "medium":
      return "info";
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

/** Decisions table: the company's memory of major decisions. */
function DecisionsTable({
  decisions,
  people,
  me,
  capabilities,
}: {
  decisions: Decision[];
  people: PersonOption[];
  me?: string;
  capabilities: string[];
}) {
  return (
    <Table>
      <thead>
        <tr>
          <Th>Decision</Th>
          <Th>Outcome</Th>
          <Th>Deciders</Th>
          <Th>Due</Th>
          <Th className="text-right">Actions</Th>
        </tr>
      </thead>
      <tbody>
        {decisions.map((decision) => (
          <tr key={decision.id}>
            <Td className="font-medium text-night">{decision.question}</Td>
            <Td>
              <Badge tone={outcomeTone(decision.outcome)}>
                {humanise(decision.outcome)}
              </Badge>
            </Td>
            <Td className="text-muted">{decision.deciders.length}</Td>
            <Td className="text-muted">{decision.due_on ?? "—"}</Td>
            <Td className="text-right">
              <DecisionActions
                decision={decision}
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

/** Risks table: client, delivery, financial, people, systems risks. */
function RisksTable({
  risks,
  people,
  me,
  capabilities,
}: {
  risks: Risk[];
  people: PersonOption[];
  me?: string;
  capabilities: string[];
}) {
  return (
    <Table>
      <thead>
        <tr>
          <Th>Risk</Th>
          <Th>Kind</Th>
          <Th>Likelihood</Th>
          <Th>Impact</Th>
          <Th>Status</Th>
          <Th className="text-right">Actions</Th>
        </tr>
      </thead>
      <tbody>
        {risks.map((risk) => (
          <tr key={risk.id}>
            <Td className="font-medium text-night">{risk.title}</Td>
            <Td>
              <Badge tone={riskKindTone(risk.kind)}>{humanise(risk.kind)}</Badge>
            </Td>
            <Td>
              <Badge tone={riskSeverityTone(risk.likelihood)}>{humanise(risk.likelihood)}</Badge>
            </Td>
            <Td>
              <Badge tone={riskSeverityTone(risk.impact)}>{humanise(risk.impact)}</Badge>
            </Td>
            <Td>
              <Badge tone={risk.status === "open" ? "warning" : "neutral"}>
                {humanise(risk.status)}
              </Badge>
            </Td>
            <Td className="text-right">
              <RiskActions risk={risk} people={people} capabilities={capabilities} />
            </Td>
          </tr>
        ))}
      </tbody>
    </Table>
  );
}

/** Briefs table: daily executive briefs. */
function BriefsTable({
  briefs,
  people,
  me,
  capabilities,
}: {
  briefs: Brief[];
  people: PersonOption[];
  me?: string;
  capabilities: string[];
}) {
  return (
    <Table>
      <thead>
        <tr>
          <Th>Date</Th>
          <Th>Summary</Th>
          <Th>Delivered</Th>
          <Th className="text-right">Actions</Th>
        </tr>
      </thead>
      <tbody>
        {briefs.map((brief) => (
          <tr key={brief.id}>
            <Td className="font-medium text-night">{brief.for_date}</Td>
            <Td className="text-muted"><span title={brief.summary}>{brief.summary ?? "—"}</span></Td>
            <Td>
              <Badge tone={brief.delivered ? "success" : "neutral"}>
                {brief.delivered ? "Delivered" : "Pending"}
              </Badge>
            </Td>
            <Td className="text-right">
              <BriefActions brief={brief} people={people} capabilities={capabilities} />
            </Td>
          </tr>
        ))}
      </tbody>
    </Table>
  );
}

/** Metric snapshots table: point-in-time KPI figures. */
function MetricSnapshotsTable({ snapshots }: { snapshots: MetricSnapshot[] }) {
  return (
    <Table>
      <thead>
        <tr>
          <Th>Metric</Th>
          <Th>Value</Th>
          <Th>As of</Th>
          <Th>Source</Th>
        </tr>
      </thead>
      <tbody>
        {snapshots.map((snapshot) => (
          <tr key={snapshot.id}>
            <Td className="font-medium text-night">{snapshot.metric_name}</Td>
            <Td className="tabular text-right">{snapshot.metric_value}</Td>
            <Td className="text-muted">{snapshot.as_of}</Td>
            <Td className="text-muted">{snapshot.source ?? "—"}</Td>
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

export default async function IntelPage({
  searchParams,
}: {
  searchParams: Promise<{
    outcome?: string;
    kind?: string;
  }>;
}) {
  const params = await searchParams;
  const outcome = OUTCOME_FILTERS.find((o) => o === params.outcome);
  const kind = KIND_FILTERS.find((k) => k === params.kind);

  // The shell already guaranteed a session; this is the same read the API makes.
  await requireUser();

  // Who is looking decides which write controls appear. Go still authorises.
  const user = await requireUser();
  const held = new Set(user.roles);
  const isTeam = held.has("ceo_commercial") || held.has("ceo_internal");
  const capabilities: string[] = isTeam
    ? [
        "intel.decision.manage",
        "intel.risk.manage",
        "intel.brief.manage",
        "intel.metric.manage",
      ]
    : [];

  const [decisionsPage, risksPage, briefsPage, snapshotsPage] =
    await Promise.all([
      load(() => listDecisions({ outcome, limit: 50 })),
      load(() => listRisks({ kind, limit: 50 })),
      load(() => listBriefs({ limit: 50 })),
      load(() => listMetricSnapshots({ limit: 50 })),
    ]);

  return (
    <div className="space-y-6">
      <PageHeader
        title="Intel"
        description="Executive intelligence: decisions, risks, briefs and metrics."
        actions={
          <>
            <NewDecisionButton
              people={[]}
              me={user.person_id}
              capabilities={capabilities}
              variant="secondary"
            />
            <NewRiskButton
              people={[]}
              me={user.person_id}
              capabilities={capabilities}
              variant="secondary"
            />
            <NewBriefButton
              people={[]}
              me={user.person_id}
              capabilities={capabilities}
            />
            <NewMetricSnapshotButton
              people={[]}
              me={user.person_id}
              capabilities={capabilities}
            />
          </>
        }
      />

      <StatGrid label="Intel overview">
        <Stat
          label="Decisions"
          value={decisionsPage.value?.items.length ?? "—"}
          icon="sliders"
          iconTone="info"
          hint={
            decisionsPage.value
              ? `${decisionsPage.value.items.filter((d) => d.outcome === "open").length} open`
              : "Decisions could not be read"
          }
        />
        <Stat
          label="Risks"
          value={risksPage.value?.items.length ?? "—"}
          icon="alert"
          iconTone={risksPage.value && risksPage.value.items.some((r) => r.likelihood === "critical") ? "danger" : "neutral"}
          hint={
            risksPage.value
              ? `${risksPage.value.items.filter((r) => r.status === "open").length} open`
              : "Risks could not be read"
          }
        />
        <Stat
          label="Briefs"
          value={briefsPage.value?.items.length ?? "—"}
          icon="calendar"
          iconTone="neutral"
          hint={
            briefsPage.value
              ? `${briefsPage.value.items.filter((b) => b.delivered).length} delivered`
              : "Briefs could not be read"
          }
        />
        <Stat
          label="Metric snapshots"
          value={snapshotsPage.value?.items.length ?? "—"}
          icon="signal"
          iconTone="neutral"
          hint="Point-in-time KPI figures"
        />
      </StatGrid>

      <Card
        title="Decisions"
        description="Every major decision becomes a searchable record — the company's memory (§8.8)."
        padded={false}
      >
        <div className="px-[22px] pb-1 pt-4">
          <Chips label="Filter decisions by outcome">
            <ChipLink href="#" pressed={outcome === undefined}>
              Every outcome
            </ChipLink>
            {OUTCOME_FILTERS.map((o) => (
              <ChipLink key={o} href="#" pressed={outcome === o}>
                {humanise(o)}
              </ChipLink>
            ))}
          </Chips>
        </div>
        {decisionsPage.error ? (
          <PanelError title="We could not load decisions" error={decisionsPage.error} />
        ) : !decisionsPage.value || decisionsPage.value.items.length === 0 ? (
          <div className="p-[22px]">
            <EmptyState
              icon="sliders"
              iconTone="info"
              title="No decisions yet"
            >
              Decisions appear here once they are recorded. Each decision records the question,
              options, recommendation, deciders and outcome.
            </EmptyState>
          </div>
        ) : (
          <>
            <DecisionsTable
              decisions={decisionsPage.value.items}
              people={[]}
              me={user.person_id}
              capabilities={capabilities}
            />
            <PageNote count={decisionsPage.value.items.length} hasMore={decisionsPage.value.has_more} />
          </>
        )}
      </Card>

      <Card
        title="Risks"
        description="Client, delivery, financial, people and systems risks with owner, likelihood and impact (§8.8)."
        padded={false}
      >
        <div className="px-[22px] pb-1 pt-4">
          <Chips label="Filter risks by kind">
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
        {risksPage.error ? (
          <PanelError title="We could not load risks" error={risksPage.error} />
        ) : !risksPage.value || risksPage.value.items.length === 0 ? (
          <div className="p-[22px]">
            <EmptyState
              icon="alert"
              iconTone="info"
              title="No risks yet"
            >
              Risks appear here once they are recorded. Indicator breaches raise
              units.indicator.breached.v1 (§8.7).
            </EmptyState>
          </div>
        ) : (
          <>
            <RisksTable
              risks={risksPage.value.items}
              people={[]}
              me={user.person_id}
              capabilities={capabilities}
            />
            <PageNote count={risksPage.value.items.length} hasMore={risksPage.value.has_more} />
          </>
        )}
      </Card>

      <Card
        title="Briefs"
        description="Daily executive brief generated at 07:30 EAT, delivered in-app, by email and as a WhatsApp summary (§8.8)."
        padded={false}
      >
        {briefsPage.error ? (
          <PanelError title="We could not load briefs" error={briefsPage.error} />
        ) : !briefsPage.value || briefsPage.value.items.length === 0 ? (
          <div className="p-[22px]">
            <EmptyState
              icon="calendar"
              iconTone="info"
              title="No briefs yet"
            >
              Briefs appear here once they are generated each working day.
            </EmptyState>
          </div>
        ) : (
          <>
            <BriefsTable
              briefs={briefsPage.value.items}
              people={[]}
              me={user.person_id}
              capabilities={capabilities}
            />
            <PageNote count={briefsPage.value.items.length} hasMore={briefsPage.value.has_more} />
          </>
        )}
      </Card>

      <Card
        title="Metric snapshots"
        description="Point-in-time KPI figures — every figure links back to its source rows."
        padded={false}
      >
        {snapshotsPage.error ? (
          <PanelError title="We could not load metric snapshots" error={snapshotsPage.error} />
        ) : !snapshotsPage.value || snapshotsPage.value.items.length === 0 ? (
          <div className="p-[22px]">
            <EmptyState
              icon="signal"
              iconTone="info"
              title="No metric snapshots yet"
            >
              Metric snapshots appear here once they are recorded.
            </EmptyState>
          </div>
        ) : (
          <>
            <MetricSnapshotsTable snapshots={snapshotsPage.value.items} />
            <PageNote count={snapshotsPage.value.items.length} hasMore={snapshotsPage.value.has_more} />
          </>
        )}
      </Card>
    </div>
  );
}
