import { ButtonLink } from "@/components/form";
import { Icon } from "@/components/icons";
import { SignalRefresh } from "@/components/signal-refresh";
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
  Tag,
  Td,
  Th,
  type Tone,
} from "@/components/ui";
import {
  CONTRACT_STATUSES,
  listContracts,
  listOrganizations,
  type Contract,
} from "@/lib/api/atlas";
import { ApiError } from "@/lib/api/client";
import { CONTRACTS_TOPIC } from "@/lib/realtime/topics";
import { formatDay, formatMinor, formatMinorCompact, humanise } from "@/lib/format";

/**
 * Contracts (`GET /api/v1/contracts`, gated by `contracts.contract.read`;
 * blueprint §8.3). One page of up to 200 contracts is read once and the status
 * chips work over it. The sensitive value is withheld by Go unless the viewer
 * holds `contracts.contract_value.sensitive.read`, so a missing amount is shown
 * as restricted, not as zero.
 */

export const dynamic = "force-dynamic";

/** Contract badge tones; the shared work/people map does not know these states. */
function contractTone(status: string): Tone {
  switch (status) {
    case "active":
    case "signed":
    case "approved":
    case "renewed":
    case "completed":
      return "success";
    case "sent":
    case "internal_review":
      return "info";
    case "renewal_due":
      return "warning";
    case "terminated":
      return "danger";
    default:
      return "neutral";
  }
}

export default async function ContractsPage({
  searchParams,
}: {
  searchParams: Promise<{ status?: string }>;
}) {
  const { status } = await searchParams;
  const filter = CONTRACT_STATUSES.find((s) => s === status);

  let contracts: Contract[] = [];
  let error: ApiError | null = null;
  try {
    contracts = (await listContracts({ limit: 200 })).items;
  } catch (caught) {
    if (!(caught instanceof ApiError)) throw caught;
    error = caught;
  }

  // Organisation names are a courtesy view; a viewer without crm.organization.read
  // simply sees the short id.
  const orgNames = error
    ? new Map<string, string>()
    : await listOrganizations({ limit: 200 })
        .then((page) => new Map(page.items.map((o) => [o.id, o.name])))
        .catch(() => new Map<string, string>());

  const currency = contracts.find((c) => c.currency)?.currency ?? "KES";
  const visible = contracts.filter((c) => c.value_minor !== undefined);
  const portfolio = visible.reduce((sum, c) => sum + (c.value_minor ?? 0), 0);
  const shown = filter ? contracts.filter((c) => c.status === filter) : contracts;

  return (
    <div className="space-y-6">
      <PageHeader
        title="Contracts"
        description="Every agreement from draft to signature, with its value and term."
        actions={
          error ? undefined : (
            <>
              <SignalRefresh topic={CONTRACTS_TOPIC} quiet />
              <ButtonLink href="/crm" variant="secondary">
                <Icon name="people" />
                CRM
              </ButtonLink>
            </>
          )
        }
      />

      {error ? (
        <ProblemNotice
          error={error}
          title={
            error.status === 403
              ? "You do not have permission to view contracts"
              : "Contracts could not be loaded"
          }
        />
      ) : (
        <>
          <StatGrid label="Contracts overview">
            <Stat
              label="Contracts"
              value={contracts.length}
              icon="document"
              iconTone="accent"
              hint="Across every stage"
            />
            <Stat
              label="Active"
              value={contracts.filter((c) => c.status === "active").length}
              tone="success"
              icon="shieldCheck"
              iconTone="success"
              hint="Signed and in force"
            />
            <Stat
              label="Renewal due"
              value={contracts.filter((c) => c.status === "renewal_due").length}
              tone={contracts.some((c) => c.status === "renewal_due") ? "warning" : "neutral"}
              icon="clock"
              iconTone="warning"
              hint="Approaching their end date"
            />
            <Stat
              label="Portfolio value"
              value={
                visible.length === 0 ? "—" : formatMinorCompact(portfolio, currency)
              }
              icon="card"
              iconTone="violet"
              hint={
                visible.length === 0
                  ? "Values are restricted for your role"
                  : `${visible.length} of ${contracts.length} show a value`
              }
            />
          </StatGrid>

          <div className="flex flex-wrap items-center justify-between gap-x-5 gap-y-3">
            <Chips label="Filter contracts by status">
              <ChipLink href="/contracts" pressed={!filter} count={contracts.length}>
                All
              </ChipLink>
              {CONTRACT_STATUSES.map((s) => (
                <ChipLink
                  key={s}
                  href={`/contracts?status=${s}`}
                  pressed={filter === s}
                  count={contracts.filter((c) => c.status === s).length}
                >
                  {humanise(s)}
                </ChipLink>
              ))}
            </Chips>
            <span className="inline-flex items-center gap-2 text-sm text-muted">
              <Icon name="document" className="h-4 w-4" />
              Newest first
            </span>
          </div>

          <Card padded={false}>
            {shown.length === 0 ? (
              <div className="p-[22px]">
                <EmptyState
                  icon="check"
                  className="min-h-[240px]"
                  title={
                    filter ? `No ${humanise(filter).toLowerCase()} contracts` : "No contracts yet"
                  }
                >
                  {filter
                    ? "Contracts move here as they advance through the lifecycle."
                    : "A signed proposal becomes a contract, and it appears here."}
                </EmptyState>
              </div>
            ) : (
              <Table>
                <thead>
                  <tr>
                    <Th>Contract</Th>
                    <Th>Organization</Th>
                    <Th>Status</Th>
                    <Th className="text-right">Value</Th>
                    <Th>Term</Th>
                  </tr>
                </thead>
                <tbody>
                  {shown.map((contract) => (
                    <tr key={contract.id}>
                      <Td>
                        <div className="flex flex-col items-start gap-1">
                          <Tag>{contract.ref}</Tag>
                          <span className="text-[13px] text-muted">
                            {humanise(contract.contract_type)}
                          </span>
                        </div>
                      </Td>
                      <Td className="text-muted">
                        {orgNames.get(contract.organization_id) ??
                          `Org ${contract.organization_id.slice(0, 8)}`}
                      </Td>
                      <Td>
                        <Badge tone={contractTone(contract.status)} dot>
                          {humanise(contract.status)}
                        </Badge>
                      </Td>
                      <Td className="tabular text-right">
                        {contract.value_minor === undefined ? (
                          <span className="text-muted" title="Restricted for your role">
                            Restricted
                          </span>
                        ) : (
                          formatMinor(contract.value_minor, contract.currency || currency)
                        )}
                      </Td>
                      <Td className="text-muted">
                        {contract.start_at || contract.end_at
                          ? `${formatDay(contract.start_at)} → ${formatDay(contract.end_at)}`
                          : "—"}
                      </Td>
                    </tr>
                  ))}
                </tbody>
              </Table>
            )}
          </Card>
        </>
      )}
    </div>
  );
}
