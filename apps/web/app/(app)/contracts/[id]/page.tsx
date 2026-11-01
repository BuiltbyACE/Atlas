"use client";

import Link from "next/link";
import { useState } from "react";

import { Button, ButtonLink, Field, Select, TextInput, TextArea } from "@/components/form";
import { Icon } from "@/components/icons";
import { SignalRefresh } from "@/components/signal-refresh";
import {
  Badge,
  Card,
  EmptyState,
  PageHeader,
  ProblemNotice,
  Stat,
  StatGrid,
  Table,
  Td,
  Th,
} from "@/components/ui";
import {
  getContract,
  listContractDeliverables,
  listContractMilestones,
  listContractObligations,
  listOrganizations,
  setContractStatus,
  signContract,
  type Contract,
  type ContractDeliverable,
  type ContractMilestone,
  type ContractObligation,
  type CrmOrganization,
} from "@/lib/api/atlas";
import { ApiError } from "@/lib/api/client";
import { personDirectory, personName } from "@/lib/directory";
import { formatDay, formatMinor, humanise } from "@/lib/format";
import { useMutation } from "@/lib/use-mutation";

/**
 * Contract detail. Reads the contract and its milestones, deliverables,
 * obligations and SLA terms from Go, then offers status change and signing
 * actions.
 */
export const dynamic = "force-dynamic";

async function safe<T>(promise: Promise<T>, fallback: T): Promise<T> {
  try {
    return await promise;
  } catch (error) {
    if (error instanceof ApiError) return fallback;
    throw error;
  }
}

export default async function ContractDetailPage({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = await params;

  let contract: Contract;
  try {
    contract = await getContract(id);
  } catch (error) {
    if (!(error instanceof ApiError)) throw error;
    return (
      <div className="space-y-6">
        <PageHeader title="Contract" />
        <ProblemNotice
          error={error}
          title={
            error.status === 404
              ? "That contract does not exist"
              : "The contract could not be loaded"
          }
        />
      </div>
    );
  }

  const [milestones, obligations, deliverables, organizations, names] =
    await Promise.all([
      safe<ContractMilestone[]>(listContractMilestones(id), []),
      safe<ContractObligation[]>(listContractObligations(id), []),
      safe<ContractDeliverable[]>(listContractDeliverables(id), []),
      safe<{ items: CrmOrganization[] }>(listOrganizations(), { items: [] }),
      personDirectory(),
    ]);

  const organization = organizations.items.find(
    (o) => o.id === contract.organization_id,
  );

  const isSigned = contract.status === "signed" || contract.status === "active";

  return (
    <div className="space-y-6">
      <PageHeader
        title={contract.ref}
        eyebrow={
          <span className="flex flex-wrap items-center gap-2">
            <Badge
              tone={
                {
                  active: "success",
                  signed: "success",
                  approved: "info",
                  sent: "info",
                  internal_review: "info",
                  renewal_due: "warning",
                  terminated: "danger",
                }[contract.status] as import("@/components/ui").Tone
              }
            >
              {humanise(contract.status)}
            </Badge>
            {humanise(contract.contract_type)}
          </span>
        }
        description={
          <span className="flex flex-wrap items-center gap-x-2 gap-y-1">
            <span>
              {organization ? organization.name : contract.organization_id.slice(0, 8)}
            </span>
            <SignalRefresh topic="contracts" quiet />
          </span>
        }
        actions={
          <>
            <ButtonLink href="/contracts" variant="secondary">
              <Icon name="chevronLeft" />
              Back to contracts
            </ButtonLink>
          </>
        }
      />

      <StatGrid label="Contract at a glance">
        <Stat
          label="Value"
          value={
            contract.value_minor !== undefined
              ? formatMinor(contract.value_minor, contract.currency)
              : "Restricted"
          }
          icon="card"
          iconTone="violet"
          hint={
            contract.value_minor === undefined
              ? "Value restricted for your role"
              : contract.currency
          }
        />
        <Stat
          label="Term"
          value={
            contract.start_at && contract.end_at
              ? `${formatDay(contract.start_at)} → ${formatDay(contract.end_at)}`
              : "—"
          }
          icon="clock"
          iconTone="neutral"
        />
      </StatGrid>

      <div className="grid grid-cols-1 gap-5 xl:grid-cols-[minmax(0,1.7fr)_minmax(0,1fr)]">
        <div className="flex flex-col gap-5">
          <Card title="Details">
            <Table>
              <tbody>
                <tr>
                  <Th className="text-muted w-[30%]">Organization</Th>
                  <Td className="tabular">
                    {organization ? (
                      <Link
                        href={`/crm/organizations/${organization.id}`}
                        className="font-semibold text-night hover:underline"
                      >
                        {organization.name}
                      </Link>
                    ) : (
                      <span className="text-muted">{contract.organization_id.slice(0, 8)}</span>
                    )}
                  </Td>
                </tr>
                <tr>
                  <Th className="text-muted">Type</Th>
                  <Td className="tabular text-muted">
                    {humanise(contract.contract_type)}
                  </Td>
                </tr>
                <tr>
                  <Th className="text-muted">Payment terms</Th>
                  <Td className="tabular text-muted">
                    {contract.payment_terms ?? "—"}
                  </Td>
                </tr>
                <tr>
                  <Th className="text-muted">Renewal terms</Th>
                  <Td className="tabular text-muted">
                    {contract.renewal_terms ?? "—"}
                  </Td>
                </tr>
                {contract.signed_at && (
                  <tr>
                    <Th className="text-muted">Signed at</Th>
                    <Td className="tabular text-muted">
                      {formatDay(contract.signed_at)}
                    </Td>
                  </tr>
                )}
              </tbody>
            </Table>
          </Card>

          {milestones.length > 0 && (
            <Card title="Milestones">
              <Table>
                <thead>
                  <tr>
                    <Th>Milestone</Th>
                    <Th className="text-right">Due</Th>
                    <Th className="text-right">Amount</Th>
                  </tr>
                </thead>
                <tbody>
                  {milestones.map((m) => (
                    <tr key={m.id}>
                      <Td className="tabular">
                        {m.title}
                      </Td>
                      <Td className="tabular text-right text-muted">
                        {formatDay(m.due_at ?? "—")}
                      </Td>
                      <Td className="tabular text-right">
                        {m.amount_minor !== undefined
                          ? formatMinor(m.amount_minor, m.currency)
                          : "—"}
                      </Td>
                    </tr>
                  ))}
                </tbody>
              </Table>
            </Card>
          )}

          {obligations.length > 0 && (
            <Card title="Obligations">
              <Table>
                <thead>
                  <tr>
                    <Th>Party</Th>
                    <Th>Description</Th>
                    <Th className="text-right">Due</Th>
                    <Th>Status</Th>
                  </tr>
                </thead>
                <tbody>
                  {obligations.map((o) => (
                    <tr key={o.id}>
                      <Td className="tabular text-muted">{o.party}</Td>
                      <Td className="tabular whitespace-pre-wrap text-[13px]">
                        {o.description}
                      </Td>
                      <Td className="tabular text-right text-muted">
                        {formatDay(o.due_at ?? "—")}
                      </Td>
                      <Td>
                        <Badge tone="neutral">{humanise(o.status)}</Badge>
                      </Td>
                    </tr>
                  ))}
                </tbody>
              </Table>
            </Card>
          )}

          {deliverables.length > 0 && (
            <Card title="Deliverables">
              <Table>
                <thead>
                  <tr>
                    <Th>Deliverable</Th>
                    <Th>Acceptance criteria</Th>
                  </tr>
                </thead>
                <tbody>
                  {deliverables.map((d) => (
                    <tr key={d.id}>
                      <Td className="tabular whitespace-pre-wrap text-[13px]">
                        {d.description}
                      </Td>
                      <Td className="tabular text-muted">
                        {d.acceptance_criteria ?? "—"}
                      </Td>
                    </tr>
                  ))}
                </tbody>
              </Table>
            </Card>
          )}
        </div>

        <ContractEditor contract={contract} isSigned={isSigned} />
      </div>

      <p className="text-xs text-[#7A879C]">
        Created {formatDay(contract.created_at)} · updated {formatDay(contract.updated_at)} ·
        version {contract.version}
      </p>
    </div>
  );
}

function ContractEditor({
  contract,
  isSigned,
}: {
  contract: Contract;
  isSigned: boolean;
}) {
  const { busy, problem, run, reset } = useMutation();
  const [status, setStatus] = useState(contract.status);

  async function onSave(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const updated = await run(async () => {
      return setContractStatus(contract.id, contract.version, {
        status: status as any,
      });
    });
    if (updated) {
      reset();
    }
  }

  async function onSign(event: React.MouseEvent<HTMLButtonElement>) {
    event.preventDefault();
    const signed = await run(async () => {
      return signContract(contract.id, contract.version, {
        name: "",
        role: "",
        method: "",
      });
    });
    if (signed) {
      reset();
    }
  }

  return (
    <Card title="Manage contract">
      {problem && <ProblemNotice error={problem} title="The contract wasn't updated" />}

      <form onSubmit={onSave} className="flex flex-col gap-4 px-6 py-[22px]">
        <Field label="Status">
          <Select
            name="status"
            value={status}
            onChange={(e) => setStatus(e.target.value as any)}
          >
            <option value="draft">Draft</option>
            <option value="sent">Sent</option>
            <option value="internal_review">Internal review</option>
            <option value="approved">Approved</option>
            <option value="signed">Signed</option>
            <option value="active">Active</option>
            <option value="renewal_due">Renewal due</option>
            <option value="terminated">Terminated</option>
          </Select>
        </Field>

        {!isSigned && (
          <div className="flex items-end gap-2 pt-4">
            <Button type="submit" variant="primary" disabled={busy}>
              {busy ? "Saving…" : "Save status"}
            </Button>
            <Button
              type="button"
              variant="primary"
              onClick={onSign}
              disabled={busy}
            >
              {busy ? "Signing…" : "Sign contract"}
            </Button>
          </div>
        )}

        {isSigned && (
          <div className="flex items-end gap-2 pt-4">
            <Button type="submit" variant="primary" disabled={busy}>
              {busy ? "Saving…" : "Save status"}
            </Button>
          </div>
        )}
      </form>
    </Card>
  );
}
