import type { ReactNode } from "react";

import {
  Badge,
  Card,
  ChipLink,
  Chips,
  EmptyState,
  Meter,
  PageHeader,
  ProblemNotice,
  Stat,
  StatGrid,
  SWATCH,
  Table,
  Td,
  Th,
  type Tone,
} from "@/components/ui";
import {
  getSystemsOverview,
  listChecks,
  listExpiring,
  listIncidents,
  listSystems,
  type AtlasSystem,
  type SystemsCheck,
  type SystemsExpiry,
  type SystemsIncident,
  type SystemsOverview,
} from "@/lib/api/atlas";
import { ApiError } from "@/lib/api/client";
import { requireUser } from "@/lib/auth/session";
import { formatDateTime, formatDay, formatDuration, humanise, timeAgo } from "@/lib/format";

/**
 * Systems (`GET /api/v1/systems/*`, gated by `systems.*`; blueprint §8.6). The
 * page reads the observability headline, the register, one page of checks, one
 * page of incidents and everything about to lapse, then composes them into the
 * "what is running, what is broken, what runs out soon" picture.
 *
 * Every figure is measured by Go from real probes and incidents; nothing here is
 * estimated. A check that has never run is shown as "never run", not as a pass,
 * and a system with no probes has no availability figure at all.
 *
 * This is the read slice: the register and the incident lifecycle are written
 * from the Systems write flows, which is a later pass. Go owns every rule and
 * every authorisation, so the page only presents what Go answered.
 */

export const dynamic = "force-dynamic";

/** Systems filters, spelled as Go's enums (`SystemType`, `Criticality`, …). */
const TIER_FILTERS = ["tier1", "tier2", "tier3"] as const;
const ENV_FILTERS = ["prod", "staging"] as const;
const CHECK_STATUS_FILTERS = ["fail", "warn", "pass"] as const;

/** Criticality tones: the tier is the risk, so tier1 shouts. */
function criticalityTone(criticality: string): Tone {
  switch (criticality) {
    case "tier1":
      return "danger";
    case "tier2":
      return "warning";
    default:
      return "neutral";
  }
}

/** A check's health is its newest result; an unprobed check has no tone. */
function checkTone(status?: string): Tone {
  switch (status) {
    case "pass":
      return "success";
    case "warn":
      return "warning";
    case "fail":
      return "danger";
    default:
      return "neutral";
  }
}

/** Incident severity: SEV1 red, SEV2 orange, SEV3 amber, SEV4 quiet. */
function severityTone(severity: string): Tone {
  switch (severity) {
    case "sev1":
      return "danger";
    case "sev2":
      return "orange";
    case "sev3":
      return "warning";
    default:
      return "neutral";
  }
}

/** Incident lifecycle: an unacknowledged incident is the one that hurts. */
function incidentTone(status: string): Tone {
  switch (status) {
    case "open":
      return "danger";
    case "acknowledged":
    case "mitigated":
      return "warning";
    case "resolved":
      return "success";
    default:
      return "neutral";
  }
}

/** Expiry urgency: the tightest band Go put it in (§8.6 warns at 30/14/7). */
function expiryTone(band: number, daysOut: number): Tone {
  if (daysOut <= 0) return "danger";
  switch (band) {
    case 7:
      return "danger";
    case 14:
      return "orange";
    case 30:
      return "warning";
    default:
      return "neutral";
  }
}

/** Availability tone against the tier-appropriate expectation. */
function availabilityTone(bp: number): Tone {
  if (bp >= 9990) return "success";
  if (bp >= 9900) return "warning";
  return "danger";
}

/** Basis points as a percentage: `9900` -> `99.00%`. */
function percent(bp: number): string {
  return `${(bp / 100).toFixed(2)}%`;
}

/** Share of the 30-day incidents Atlas caught before a client did. */
function atlasDetectionShare(summary: SystemsOverview): string | undefined {
  if (summary.incidents_30d === 0) return undefined;
  const share = Math.round((summary.detected_by_atlas_30d / summary.incidents_30d) * 100);
  return `${share}%`;
}

/** The verb of an expiry kind (`certificate` -> "Certificate"). */
function expiryKindLabel(kind: string): string {
  return humanise(kind);
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

/** A system's derived health as badges, so the register reads at a glance. */
function HealthCell({ system }: { system: AtlasSystem }) {
  if (system.checks_total === 0 && system.open_incidents === 0) {
    return <span className="text-muted">No probes</span>;
  }
  return (
    <span className="flex flex-wrap items-center gap-1.5">
      {system.checks_failing > 0 && (
        <Badge tone="danger" dot>
          {system.checks_failing} failing
        </Badge>
      )}
      {system.checks_warning > 0 && (
        <Badge tone="warning" dot>
          {system.checks_warning} warning
        </Badge>
      )}
      {system.open_incidents > 0 && (
        <Badge tone="orange" dot>
          {system.open_incidents} incident{system.open_incidents === 1 ? "" : "s"}
        </Badge>
      )}
      {system.checks_failing === 0 &&
        system.checks_warning === 0 &&
        system.open_incidents === 0 && <Badge tone="success">All clear</Badge>}
    </span>
  );
}

/** A single availability row: the meter and the precise ratio behind it. */
function AvailabilityRow({
  row,
}: {
  row: SystemsOverview["availability"][number];
}) {
  return (
    <li className="border-t border-line py-3.5 first:border-t-0">
      <div className="flex flex-wrap items-center justify-between gap-x-4 gap-y-1">
        <span className="flex min-w-0 items-center gap-2">
          <span className="truncate font-medium text-night">{row.name}</span>
          <Badge tone={criticalityTone(row.criticality)}>
            {row.criticality.replace("tier", "Tier ")}
          </Badge>
        </span>
        <span className="tabular text-[13.5px] font-semibold text-night">
          {percent(row.availability_bp)}
          <span className="ml-2 font-normal text-muted">
            {row.passed}/{row.total} probes
          </span>
        </span>
      </div>
      <Meter
        className="mt-2.5"
        label={`${row.name} availability over the last 30 days`}
        segments={[
          { value: row.passed, color: SWATCH[availabilityTone(row.availability_bp)] },
          { value: Math.max(row.total - row.passed, 0), color: "#E3E7EE" },
        ]}
      />
    </li>
  );
}

/** The register table: one row per system with its derived health. */
function SystemsTable({ systems }: { systems: AtlasSystem[] }) {
  return (
    <Table>
      <thead>
        <tr>
          <Th>System</Th>
          <Th>Type</Th>
          <Th>Env</Th>
          <Th>Criticality</Th>
          <Th>Health</Th>
          <Th className="text-right">Availability</Th>
          <Th>Owner</Th>
        </tr>
      </thead>
      <tbody>
        {systems.map((system) => (
          <tr key={system.id}>
            <Td>
              <span className="flex items-center gap-2">
                <span className="font-medium text-night">{system.name}</span>
                {system.status !== "active" && <Badge tone="neutral">{humanise(system.status)}</Badge>}
              </span>
              <span className="mt-0.5 block font-mono text-[12px] text-muted">{system.ref}</span>
            </Td>
            <Td className="text-muted">{humanise(system.system_type)}</Td>
            <Td className="text-muted">{humanise(system.environment)}</Td>
            <Td>
              <Badge tone={criticalityTone(system.criticality)}>
                {system.criticality.replace("tier", "Tier ")}
              </Badge>
            </Td>
            <Td>
              <HealthCell system={system} />
            </Td>
            <Td className="text-right">
              {system.availability_bp === undefined ? (
                <span className="text-muted">—</span>
              ) : (
                <Badge tone={availabilityTone(system.availability_bp)}>
                  {percent(system.availability_bp)}
                </Badge>
              )}
            </Td>
            <Td className="text-muted">{system.owner?.name ?? "—"}</Td>
          </tr>
        ))}
      </tbody>
    </Table>
  );
}

/** One probe with its newest result — the raw material for the health above. */
function ChecksTable({ checks }: { checks: SystemsCheck[] }) {
  return (
    <Table>
      <thead>
        <tr>
          <Th>Check</Th>
          <Th>System</Th>
          <Th>Type</Th>
          <Th>Target</Th>
          <Th>Last result</Th>
          <Th className="text-right">Failures in a row</Th>
        </tr>
      </thead>
      <tbody>
        {checks.map((check) => (
          <tr key={check.id}>
            <Td>
              <span className="flex items-center gap-2">
                <span className="font-medium text-night">{check.name}</span>
                {!check.active && <Badge tone="neutral">Paused</Badge>}
              </span>
            </Td>
            <Td className="text-muted">
              <span className="flex items-center gap-2">
                {check.system_name}
                <Badge tone={criticalityTone(check.system_criticality)}>
                  {check.system_criticality.replace("tier", "Tier ")}
                </Badge>
              </span>
            </Td>
            <Td className="text-muted">{humanise(check.check_type)}</Td>
            <Td className="max-w-[280px] truncate font-mono text-[12px] text-muted" >
              <span title={check.target}>{check.target}</span>
            </Td>
            <Td>
              {check.last_status === undefined ? (
                <Badge tone="neutral">Never run</Badge>
              ) : (
                <span className="flex flex-col gap-1">
                  <Badge tone={checkTone(check.last_status)} dot>
                    {humanise(check.last_status)}
                  </Badge>
                  <span className="text-[12.5px] text-muted">
                    {timeAgo(check.last_at)}
                    {check.last_latency_ms !== undefined && ` · ${check.last_latency_ms} ms`}
                  </span>
                </span>
              )}
            </Td>
            <Td className="tabular text-right">
              {check.consecutive_failures > 0 ? (
                <span className="font-semibold text-[#C0352B]">
                  {check.consecutive_failures}/{check.failure_threshold}
                </span>
              ) : (
                <span className="text-muted">0/{check.failure_threshold}</span>
              )}
            </Td>
          </tr>
        ))}
      </tbody>
    </Table>
  );
}

/** The incident log: what broke, how bad, who commanded it, how long it took. */
function IncidentsTable({ incidents }: { incidents: SystemsIncident[] }) {
  return (
    <Table>
      <thead>
        <tr>
          <Th>Incident</Th>
          <Th>Severity</Th>
          <Th>Status</Th>
          <Th>Systems</Th>
          <Th>Detected</Th>
          <Th className="text-right">Time to resolve</Th>
        </tr>
      </thead>
      <tbody>
        {incidents.map((incident) => (
          <tr key={incident.id}>
            <Td>
              <span className="flex items-center gap-2">
                <span className="font-medium text-night">{incident.title}</span>
                {incident.review_required && <Badge tone="violet">Review owed</Badge>}
              </span>
              <span className="mt-0.5 block font-mono text-[12px] text-muted">{incident.ref}</span>
            </Td>
            <Td>
              <Badge tone={severityTone(incident.severity)} dot>
                {incident.severity_label}
              </Badge>
            </Td>
            <Td>
              <Badge tone={incidentTone(incident.status)}>{humanise(incident.status)}</Badge>
            </Td>
            <Td className="text-muted">
              {incident.system_names.length === 0 ? "—" : incident.system_names.join(", ")}
            </Td>
            <Td className="text-muted">
              <span className="block">{formatDateTime(incident.detected_at)}</span>
              <span className="text-[12.5px]">
                {incident.detected_source === "atlas" ? "Atlas detected" : "Client reported"}
              </span>
            </Td>
            <Td className="tabular text-right">
              {incident.mttr_seconds === undefined ? (
                <Badge tone="warning">Open</Badge>
              ) : (
                <span className="font-semibold">{formatDuration(incident.mttr_seconds)}</span>
              )}
            </Td>
          </tr>
        ))}
      </tbody>
    </Table>
  );
}

/** Domains, certificates and credential rotations that lapse soon (§8.6). */
function ExpiringTable({ expiring }: { expiring: SystemsExpiry[] }) {
  return (
    <Table>
      <thead>
        <tr>
          <Th>What</Th>
          <Th>Kind</Th>
          <Th>System</Th>
          <Th>Expires</Th>
          <Th>Auto-renew</Th>
          <Th className="text-right">Window</Th>
        </tr>
      </thead>
      <tbody>
        {expiring.map((item) => (
          <tr key={`${item.kind}:${item.id}`}>
            <Td className="font-medium text-night">{item.name}</Td>
            <Td className="text-muted">{expiryKindLabel(item.kind)}</Td>
            <Td className="text-muted">{item.system?.name ?? "—"}</Td>
            <Td className="text-muted">
              <span className="block">{formatDay(item.expires_on)}</span>
              <span className="text-[12.5px]">{item.days_out <= 0 ? "lapsed" : `${item.days_out} days out`}</span>
            </Td>
            <Td className="text-muted">{item.auto_renew ? "Automatic" : "Manual"}</Td>
            <Td className="text-right">
              {item.days_out <= 0 ? (
                <Badge tone="danger" dot>
                  Lapsed
                </Badge>
              ) : (
                <Badge tone={expiryTone(item.warning_band, item.days_out)} dot>
                  {item.warning_band}-day warning
                </Badge>
              )}
            </Td>
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

export default async function SystemsPage({
  searchParams,
}: {
  searchParams: Promise<{
    tier?: string;
    env?: string;
    check_status?: string;
    incidents?: string;
  }>;
}) {
  const params = await searchParams;
  const tier = TIER_FILTERS.find((t) => t === params.tier);
  const env = ENV_FILTERS.find((e) => e === params.env);
  const checkStatus = CHECK_STATUS_FILTERS.find((s) => s === params.check_status);
  const openOnly = params.incidents === "open";
  const reviewsOnly = params.incidents === "review";

  // The shell already guaranteed a session; this is the same read the API makes.
  await requireUser();

  const [overview, systemsPage, checksPage, incidentsPage, expiringResult] = await Promise.all([
    load(() => getSystemsOverview()),
    load(() => listSystems({ criticality: tier, environment: env, limit: 50 })),
    load(() => listChecks({ status: checkStatus, limit: 50 })),
    load(() =>
      listIncidents({
        open: openOnly ? true : undefined,
        review_required: reviewsOnly ? true : undefined,
        limit: 50,
      }),
    ),
    load(() => listExpiring()),
  ]);

  const summary = overview.value;

  // Build a filter URL that preserves every other active filter.
  const filterHref = (patch: Record<string, string | undefined>): string => {
    const merged: Record<string, string | undefined> = {
      tier,
      env,
      check_status: checkStatus,
      incidents: openOnly ? "open" : reviewsOnly ? "review" : undefined,
      ...patch,
    };
    const search = new URLSearchParams();
    for (const [key, value] of Object.entries(merged)) {
      if (value) search.set(key, value);
    }
    const qs = search.toString();
    return qs ? `/systems?${qs}` : "/systems";
  };

  return (
    <div className="space-y-6">
      <PageHeader
        title="Systems"
        description="What is running, what is broken, and what is about to lapse."
      />

      {overview.error && <ProblemNotice error={overview.error} title="We could not load the systems overview" />}

      <StatGrid label="Systems overview">
        <Stat
          label="Systems"
          value={summary?.systems_total ?? "—"}
          icon="server"
          iconTone="info"
          hint={
            summary
              ? `${summary.systems_tier1} tier 1 · ${summary.systems_tier2} tier 2 · ${summary.systems_tier3} tier 3`
              : "The register could not be read"
          }
        />
        <Stat
          label="Failing checks"
          value={summary?.checks_failing ?? "—"}
          tone={summary && summary.checks_failing > 0 ? "danger" : "neutral"}
          icon="alert"
          iconTone={summary && summary.checks_failing > 0 ? "danger" : "neutral"}
          hint={
            summary
              ? `${summary.checks_warning} warning · ${summary.checks_unprobed} never run`
              : "Probes could not be read"
          }
        />
        <Stat
          label="Open incidents"
          value={summary?.open_incidents ?? "—"}
          tone={summary && summary.open_sev1 > 0 ? "danger" : summary && summary.open_incidents > 0 ? "warning" : "neutral"}
          icon="pulse"
          iconTone={summary && summary.open_incidents > 0 ? "warning" : "neutral"}
          hint={summary ? `${summary.open_sev1} SEV1 · ${summary.open_sev2} SEV2` : "Incidents could not be read"}
        />
        <Stat
          label="Reviews owed"
          value={summary?.reviews_outstanding ?? "—"}
          tone={summary && summary.reviews_outstanding > 0 ? "warning" : "neutral"}
          icon="clipboard"
          iconTone="violet"
          hint="Post-incident reviews still open"
        />
        <Stat
          label="Time to resolve"
          value={summary?.mttr_seconds === undefined ? "—" : formatDuration(summary?.mttr_seconds)}
          icon="clock"
          iconTone="success"
          hint={
            summary?.mtta_seconds === undefined
              ? "SEV1 and SEV2, last 90 days"
              : `Responded in ${formatDuration(summary.mtta_seconds)} on average`
          }
        />
        <Stat
          label="Atlas detected"
          value={summary ? (atlasDetectionShare(summary) ?? "—") : "—"}
          icon="signal"
          iconTone="accent"
          hint={
            summary
              ? summary.incidents_30d === 0
                ? "No incidents in 30 days"
                : `${summary.detected_by_atlas_30d} of ${summary.incidents_30d} incidents (30 days)`
              : "Detection could not be measured"
          }
        />
        <Stat
          label="Expiring soon"
          value={summary?.expiring_count ?? "—"}
          tone={summary && summary.expiring_count > 0 ? "warning" : "neutral"}
          icon="calendar"
          iconTone="orange"
          hint={
            summary
              ? `${summary.rotations_due_count} credential rotation${
                  summary.rotations_due_count === 1 ? "" : "s"
                } due`
              : "Expiry could not be read"
          }
        />
      </StatGrid>

      <div className="grid grid-cols-1 gap-5 xl:grid-cols-2">
        <Card
          title="Availability"
          description="Share of probes that passed over the last 30 days."
          padded={false}
        >
          {!summary ? (
            <div className="p-[22px]">
              <EmptyState
                icon="signal"
                iconTone="info"
                className="min-h-[160px]"
                title="Availability unavailable"
              >
                The rollout could not be read; it may be restricted for your role.
              </EmptyState>
            </div>
          ) : summary.availability.length === 0 ? (
            <div className="p-[22px]">
              <EmptyState
                icon="pulse"
                iconTone="info"
                className="min-h-[160px]"
                title="No probes yet"
              >
                Availability is measured from check results. Register a system and its probes first.
              </EmptyState>
            </div>
          ) : (
            <ul className="px-[22px] pb-2 pt-1">
              {summary.availability.map((row) => (
                <AvailabilityRow key={row.system_id} row={row} />
              ))}
            </ul>
          )}
        </Card>

        <Card
          title="Expiring soon"
          description={
            summary
              ? `Anything lapsing within ${summary.expiry_warning_days[0]} days, soonest first.`
              : "Anything lapsing soon."
          }
          padded={false}
        >
          {expiringResult.error ? (
            <PanelError title="We could not load expiry" error={expiringResult.error} />
          ) : !expiringResult.value || expiringResult.value.items.length === 0 ? (
            <div className="p-[22px]">
              <EmptyState
                icon="shieldCheck"
                iconTone="success"
                className="min-h-[160px]"
                title="Nothing expiring soon"
              >
                Domains, certificates and credential rotations are all comfortably in date.
              </EmptyState>
            </div>
          ) : (
            <>
              <ExpiringTable expiring={expiringResult.value.items} />
              <PageNote count={expiringResult.value.items.length} />
            </>
          )}
        </Card>
      </div>

      <Card
        title="Systems register"
        description="Every system with the health its probes and incidents give it."
        padded={false}
      >
        <div className="px-[22px] pb-1 pt-4">
          <Chips label="Filter systems by criticality">
            <ChipLink href={filterHref({ tier: undefined })} pressed={tier === undefined}>
              Every tier
            </ChipLink>
            {TIER_FILTERS.map((t) => (
              <ChipLink key={t} href={filterHref({ tier: t })} pressed={tier === t}>
                {t.replace("tier", "Tier ")}
              </ChipLink>
            ))}
          </Chips>
        </div>
        <div className="px-[22px] pb-3 pt-2">
          <Chips label="Filter systems by environment">
            <ChipLink href={filterHref({ env: undefined })} pressed={env === undefined}>
              Every environment
            </ChipLink>
            {ENV_FILTERS.map((e) => (
              <ChipLink key={e} href={filterHref({ env: e })} pressed={env === e}>
                {humanise(e)}
              </ChipLink>
            ))}
          </Chips>
        </div>
        {systemsPage.error ? (
          <PanelError title="We could not load the register" error={systemsPage.error} />
        ) : !systemsPage.value || systemsPage.value.items.length === 0 ? (
          <div className="p-[22px]">
            <EmptyState icon="server" iconTone="info" title="No systems match">
              Nothing in the register matches these filters. Register a system to start monitoring
              it.
            </EmptyState>
          </div>
        ) : (
          <>
            <SystemsTable systems={systemsPage.value.items} />
            <PageNote count={systemsPage.value.items.length} hasMore={systemsPage.value.has_more} />
          </>
        )}
      </Card>

      <Card
        title="Checks"
        description="Probes across every system, newest results first."
        padded={false}
      >
        <div className="px-[22px] pb-1 pt-4">
          <Chips label="Filter checks by last result">
            <ChipLink href={filterHref({ check_status: undefined })} pressed={checkStatus === undefined}>
              Every result
            </ChipLink>
            {CHECK_STATUS_FILTERS.map((s) => (
              <ChipLink key={s} href={filterHref({ check_status: s })} pressed={checkStatus === s}>
                {humanise(s)}
              </ChipLink>
            ))}
          </Chips>
        </div>
        {checksPage.error ? (
          <PanelError title="We could not load the checks" error={checksPage.error} />
        ) : !checksPage.value || checksPage.value.items.length === 0 ? (
          <div className="p-[22px]">
            <EmptyState icon="pulse" iconTone="info" title="No checks match">
              Nothing matches these filters. A check is what turns a system&apos;s health into a fact.
            </EmptyState>
          </div>
        ) : (
          <>
            <ChecksTable checks={checksPage.value.items} />
            <PageNote count={checksPage.value.items.length} hasMore={checksPage.value.has_more} />
          </>
        )}
      </Card>

      <Card
        title="Incidents"
        description="Outages and degradations, most recently detected first."
        padded={false}
      >
        <div className="px-[22px] pb-3 pt-4">
          <Chips label="Filter incidents">
            <ChipLink href={filterHref({ incidents: undefined })} pressed={!openOnly && !reviewsOnly}>
              All incidents
            </ChipLink>
            <ChipLink href={filterHref({ incidents: "open" })} pressed={openOnly}>
              Still open
            </ChipLink>
            <ChipLink href={filterHref({ incidents: "review" })} pressed={reviewsOnly}>
              Review owed
            </ChipLink>
          </Chips>
        </div>
        {incidentsPage.error ? (
          <PanelError title="We could not load the incident log" error={incidentsPage.error} />
        ) : !incidentsPage.value || incidentsPage.value.items.length === 0 ? (
          <div className="p-[22px]">
            <EmptyState icon="shieldCheck" iconTone="success" title="No incidents">
              Nothing matches these filters. Incidents opened by a failing check land here.
            </EmptyState>
          </div>
        ) : (
          <>
            <IncidentsTable incidents={incidentsPage.value.items} />
            <PageNote count={incidentsPage.value.items.length} hasMore={incidentsPage.value.has_more} />
          </>
        )}
      </Card>
    </div>
  );
}
