"use client";

import { useRouter } from "next/navigation";
import { useEffect, useState, type FormEvent } from "react";

import { Drawer } from "@/components/drawer";
import { Button, Field, Select, TextArea, TextInput } from "@/components/form";
import { Icon } from "@/components/icons";
import { useToast } from "@/components/toast";
import { cx, ProblemNotice } from "@/components/ui";
import {
  approveInvoice,
  createInvoice,
  disputeInvoice,
  issueInvoice,
  listContractMilestones,
  writeOffInvoice,
  type ContractMilestone,
  type FinanceInvoice,
  type FinanceInvoiceBody,
  type FinanceInvoiceLineBody,
} from "@/lib/api/atlas";
import { dateInput, formatMinor, humanise, parseMinor, today } from "@/lib/format";
import { useMutation } from "@/lib/use-mutation";

import type { FinanceCapability } from "./permissions";

/**
 * Invoices (`/api/v1/finance/invoices`, §8.4). The finance team raises one from
 * an organisation, optionally tied to a contract and one of its milestones;
 * approve, issue, dispute and write-off are each one decision by whoever holds
 * the permission, and every one of them writes the actor's name to the audit
 * trail (§6.6) - the footprint is the control. A write-off also demands a
 * reason, which is stored on the record (§3.3).
 */

function field(data: FormData, key: string): string {
  return String(data.get(key) ?? "").trim();
}

/** A typed percent (`"16"` / `"16.5"`) as the basis points Go stores. */
function toBasisPoints(percent: string): number {
  const value = Number(percent.trim());
  if (!Number.isFinite(value) || value < 0) return 0;
  return Math.round(value * 100);
}

/** One row of the line editor, held as typed strings until it is submitted. */
interface LineDraft {
  key: number;
  description: string;
  quantity: string;
  unitPrice: string;
  taxRate: string;
}

function emptyLine(key: number): LineDraft {
  return { key, description: "", quantity: "1", unitPrice: "", taxRate: "0" };
}

/** Pre-tax line total in minor units, from the typed quantity and unit price. */
function lineSubtotal(line: LineDraft, currency: string): number {
  const quantity = Number(line.quantity.trim());
  if (!Number.isFinite(quantity) || quantity <= 0) return 0;
  return Math.round(quantity * parseMinor(line.unitPrice, currency));
}

/** Tax on a subtotal, from the typed percent. */
function lineTax(line: LineDraft, subtotal: number): number {
  return Math.round((subtotal * toBasisPoints(line.taxRate)) / 10_000);
}

/**
 * "New invoice" (`POST /api/v1/finance/invoices`, `finance.invoice.create`).
 * The line editor keeps a live pre-tax subtotal per line and a total, so the
 * raiser sees the figure before Go does.
 */
export function NewInvoiceButton({
  organizations,
  contracts,
  currency,
  capabilities,
  variant = "primary",
}: {
  organizations: { id: string; name: string }[];
  contracts: { id: string; label: string }[];
  currency: string;
  capabilities: FinanceCapability[];
  variant?: "primary" | "secondary";
}) {
  const router = useRouter();
  const notify = useToast();
  const { busy, problem, run, reset } = useMutation();
  const [open, setOpen] = useState(false);
  const [contractId, setContractId] = useState("");
  const [milestones, setMilestones] = useState<ContractMilestone[]>([]);
  const [nextKey, setNextKey] = useState(2);
  const [lines, setLines] = useState<LineDraft[]>([emptyLine(1)]);

  // A contract's milestones are only known once one is chosen; fetch them from
  // the contracts API rather than inventing a list.
  useEffect(() => {
    if (!contractId) {
      setMilestones([]);
      return;
    }
    let live = true;
    listContractMilestones(contractId)
      .then((items) => {
        if (live) setMilestones(items);
      })
      .catch(() => {
        if (live) setMilestones([]);
      });
    return () => {
      live = false;
    };
  }, [contractId]);

  if (!capabilities.includes("finance.invoice.create")) return null;

  function close() {
    setOpen(false);
    setLines([emptyLine(1)]);
    setNextKey(2);
    setContractId("");
    reset();
  }

  function patchLine(key: number, patch: Partial<LineDraft>) {
    setLines((current) =>
      current.map((line) => (line.key === key ? { ...line, ...patch } : line)),
    );
  }

  async function onSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const data = new FormData(event.currentTarget);

    const body: FinanceInvoiceBody = {
      organization_id: field(data, "organization_id"),
      lines: lines
        .filter((line) => line.description.trim() !== "")
        .map((line, index): FinanceInvoiceLineBody => {
          const unitPriceMinor = parseMinor(line.unitPrice, currency);
          return {
            description: line.description.trim(),
            quantity: Number(line.quantity.trim()) || 0,
            unit_price_minor: unitPriceMinor,
            tax_rate_bp: toBasisPoints(line.taxRate),
            sort_order: index,
          };
        }),
    };
    const contract = field(data, "contract_id");
    if (contract) body.contract_id = contract;
    const milestone = field(data, "milestone_id");
    if (milestone) body.milestone_id = milestone;
    const issueDate = field(data, "issue_date");
    if (issueDate) body.issue_date = issueDate;
    const dueDate = field(data, "due_date");
    if (dueDate) body.due_date = dueDate;
    if (currency) body.currency = currency;
    const notes = field(data, "notes");
    if (notes) body.notes = notes;

    const created = await run(() => createInvoice(body));
    if (created) {
      setOpen(false);
      setLines([emptyLine(1)]);
      setContractId("");
      notify(`Raised ${created.ref}`);
      router.refresh();
    }
  }

  const subtotal = lines.reduce((sum, line) => sum + lineSubtotal(line, currency), 0);
  const tax = lines.reduce((sum, line) => sum + lineTax(line, lineSubtotal(line, currency)), 0);
  const hasLine = lines.some((line) => line.description.trim() !== "");

  return (
    <>
      <Button variant={variant} size={variant === "secondary" ? "sm" : "md"} onClick={() => setOpen(true)}>
        <Icon name="plus" className={variant === "secondary" ? "h-4 w-4" : undefined} />
        New invoice
      </Button>
      <Drawer
        open={open}
        onClose={close}
        wide
        title="New invoice"
        description="Braced against an organisation. Lines carry the tax; the total is Go’s to confirm."
      >
        {organizations.length === 0 ? (
          <p className="px-6 py-6 text-sm text-muted">
            No organisations exist yet. An invoice names one, so raise the
            organisation in CRM first.
          </p>
        ) : (
          <form onSubmit={onSubmit} className="flex flex-col gap-4 px-6 py-[22px]">
            {problem && <ProblemNotice error={problem} title="The invoice wasn’t raised" />}

            <Field label="Organisation" htmlFor="invoice-org">
              <Select id="invoice-org" name="organization_id" required data-autofocus>
                {organizations.map((organization) => (
                  <option key={organization.id} value={organization.id}>
                    {organization.name}
                  </option>
                ))}
              </Select>
            </Field>

            {contracts.length > 0 && (
              <div className="grid grid-cols-1 gap-3.5 sm:grid-cols-2">
                <Field label="Contract" hint="optional" htmlFor="invoice-contract">
                  <Select
                    id="invoice-contract"
                    name="contract_id"
                    value={contractId}
                    onChange={(event) => setContractId(event.target.value)}
                  >
                    <option value="">Not against a contract</option>
                    {contracts.map((contract) => (
                      <option key={contract.id} value={contract.id}>
                        {contract.label}
                      </option>
                    ))}
                  </Select>
                </Field>
                <Field label="Milestone" hint="optional" htmlFor="invoice-milestone">
                  <Select
                    id="invoice-milestone"
                    name="milestone_id"
                    defaultValue=""
                    disabled={milestones.length === 0}
                  >
                    <option value="">
                      {milestones.length === 0 ? "No contract chosen" : "Not against a milestone"}
                    </option>
                    {milestones.map((milestone) => (
                      <option key={milestone.id} value={milestone.id}>
                        {milestone.title}
                      </option>
                    ))}
                  </Select>
                </Field>
              </div>
            )}

            <div className="grid grid-cols-1 gap-3.5 sm:grid-cols-2">
              <Field label="Issue date" hint="optional" htmlFor="invoice-issue">
                <TextInput id="invoice-issue" name="issue_date" type="date" />
              </Field>
              <Field label="Due date" hint="optional" htmlFor="invoice-due">
                <TextInput id="invoice-due" name="due_date" type="date" />
              </Field>
            </div>

            <div className="flex flex-col gap-2.5">
              <span className="text-sm font-semibold text-night">
                Lines <span className="font-normal text-muted">amounts in {currency}</span>
              </span>
              <ul className="flex flex-col gap-2.5">
                {lines.map((line, index) => (
                  <li key={line.key} className="rounded-[12px] border border-line p-3">
                    <div className="flex items-start gap-2">
                      <div className="min-w-0 flex-1">
                        <label
                          htmlFor={`invoice-line-${line.key}-description`}
                          className="sr-only"
                        >
                          Line {index + 1} description
                        </label>
                        <TextInput
                          id={`invoice-line-${line.key}-description`}
                          value={line.description}
                          onChange={(event) => patchLine(line.key, { description: event.target.value })}
                          placeholder="What is being billed"
                          required={index === 0}
                        />
                      </div>
                      {lines.length > 1 && (
                        <Button
                          size="sm"
                          variant="ghost"
                          aria-label={`Remove line ${index + 1}`}
                          onClick={() =>
                            setLines((current) => current.filter((row) => row.key !== line.key))
                          }
                        >
                          <Icon name="close" className="h-4 w-4" />
                        </Button>
                      )}
                    </div>
                    <div className="mt-2.5 grid grid-cols-2 gap-2.5 sm:grid-cols-3">
                      <Field label="Qty" htmlFor={`invoice-line-${line.key}-qty`}>
                        <TextInput
                          id={`invoice-line-${line.key}-qty`}
                          type="number"
                          min="0"
                          step="0.01"
                          inputMode="decimal"
                          value={line.quantity}
                          onChange={(event) => patchLine(line.key, { quantity: event.target.value })}
                        />
                      </Field>
                      <Field label="Unit price" htmlFor={`invoice-line-${line.key}-price`}>
                        <TextInput
                          id={`invoice-line-${line.key}-price`}
                          inputMode="decimal"
                          placeholder="0.00"
                          value={line.unitPrice}
                          onChange={(event) => patchLine(line.key, { unitPrice: event.target.value })}
                        />
                      </Field>
                      <Field label="Tax %" htmlFor={`invoice-line-${line.key}-tax`}>
                        <TextInput
                          id={`invoice-line-${line.key}-tax`}
                          inputMode="decimal"
                          placeholder="16"
                          value={line.taxRate}
                          onChange={(event) => patchLine(line.key, { taxRate: event.target.value })}
                        />
                      </Field>
                    </div>
                    <p className="mt-2 text-right text-[13px] text-muted">
                      Pre-tax subtotal{" "}
                      <span className="tabular font-semibold text-night">
                        {formatMinor(lineSubtotal(line, currency), currency)}
                      </span>
                    </p>
                  </li>
                ))}
              </ul>
              <Button
                size="sm"
                variant="secondary"
                className="self-start"
                onClick={() => {
                  setLines((current) => [...current, emptyLine(nextKey)]);
                  setNextKey((key) => key + 1);
                }}
              >
                <Icon name="plus" className="h-4 w-4" />
                Add line
              </Button>
            </div>

            <dl className="flex flex-col gap-1.5 border-t border-dashed border-line-strong pt-3.5 text-[14px]">
              <div className="flex items-center justify-between">
                <dt className="text-muted">Subtotal</dt>
                <dd className="tabular font-medium">{formatMinor(subtotal, currency)}</dd>
              </div>
              <div className="flex items-center justify-between">
                <dt className="text-muted">Tax</dt>
                <dd className="tabular font-medium">{formatMinor(tax, currency)}</dd>
              </div>
              <div className="flex items-center justify-between text-[15.5px]">
                <dt className="font-semibold text-night">Total</dt>
                <dd className="tabular font-semibold">{formatMinor(subtotal + tax, currency)}</dd>
              </div>
            </dl>

            <Field label="Notes" hint="optional" htmlFor="invoice-notes">
              <TextArea id="invoice-notes" name="notes" rows={3} />
            </Field>

            <div className="flex flex-wrap gap-2.5">
              <Button type="submit" disabled={busy || !hasLine}>
                {busy ? "Raising…" : "Raise invoice"}
              </Button>
              <Button type="button" variant="secondary" onClick={close} disabled={busy}>
                Cancel
              </Button>
            </div>
          </form>
        )}
      </Drawer>
    </>
  );
}

/** Invoices still open enough to be disputed or written off. */
const OPEN_FOR_DECISION = new Set(["issued", "partially_paid", "overdue", "disputed"]);

type InvoiceStep = "issue" | "dispute" | "write-off";

/**
 * The lifecycle actions for one invoice row. Approve and issue follow the
 * lifecycle; a write-off is destructive and needs a reason, so it is styled as
 * a danger and labelled as such. Print is the one control everybody with the
 * invoice may use - it opens the rendered document (§8.4).
 */
export function InvoiceActions({
  invoice,
  capabilities,
}: {
  invoice: FinanceInvoice;
  capabilities: FinanceCapability[];
}) {
  const router = useRouter();
  const notify = useToast();
  const { busy, problem, run, reset } = useMutation();
  const [step, setStep] = useState<InvoiceStep | null>(null);

  function close() {
    setStep(null);
    reset();
  }

  async function act(action: () => Promise<unknown>, message: string) {
    const done = await run(action);
    if (done) {
      notify(message);
      router.refresh();
    }
  }

  async function onStep(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const data = new FormData(event.currentTarget);

    if (step === "issue") {
      const issueDate = field(data, "issue_date");
      const dueDate = field(data, "due_date");
      const updated = await run(() =>
        issueInvoice(
          invoice.id,
          invoice.version,
          issueDate || dueDate
            ? {
                ...(issueDate ? { issue_date: issueDate } : {}),
                ...(dueDate ? { due_date: dueDate } : {}),
              }
            : undefined,
        ),
      );
      if (updated) {
        close();
        notify(`Issued ${updated.ref}`);
        router.refresh();
      }
      return;
    }

    const reason = field(data, "reason");
    if (step === "write-off") {
      const updated = await run(() => writeOffInvoice(invoice.id, invoice.version, reason));
      if (updated) {
        close();
        notify(`Wrote off ${updated.ref}`);
        router.refresh();
      }
      return;
    }

    const updated = await run(() =>
      disputeInvoice(invoice.id, invoice.version, reason || undefined),
    );
    if (updated) {
      close();
      notify("Invoice marked disputed");
      router.refresh();
    }
  }

  const canApprove =
    capabilities.includes("finance.invoice.approve") &&
    (invoice.status === "scheduled" || invoice.status === "draft");
  const canIssue = capabilities.includes("finance.invoice.issue") && invoice.status === "approved";
  const openForDecision = OPEN_FOR_DECISION.has(invoice.status);
  const canDispute = capabilities.includes("finance.invoice.dispute") && openForDecision;
  // A write-off rides on `finance.invoice.issue`: it closes the same lifecycle,
  // and the audit row naming the actor is what guards it (§8.4).
  const canWriteOff = capabilities.includes("finance.invoice.issue") && openForDecision;

  const drawerTitle =
    step === "issue" ? "Issue invoice" : step === "dispute" ? "Dispute invoice" : "Write off invoice";

  return (
    <div className="flex flex-col items-end gap-2">
      <div className="flex flex-wrap items-center justify-end gap-1.5">
        {canApprove && (
          <Button
            size="sm"
            disabled={busy}
            onClick={() =>
              act(() => approveInvoice(invoice.id, invoice.version), `Approved ${invoice.ref}`)
            }
          >
            Approve
          </Button>
        )}
        {canIssue && (
          <Button size="sm" variant="secondary" disabled={busy} onClick={() => setStep("issue")}>
            Issue
          </Button>
        )}
        {canDispute && (
          <Button size="sm" variant="secondary" disabled={busy} onClick={() => setStep("dispute")}>
            Dispute
          </Button>
        )}
        {canWriteOff && (
          <Button size="sm" variant="danger" disabled={busy} onClick={() => setStep("write-off")}>
            Write off
          </Button>
        )}
        <Button
          size="sm"
          variant="secondary"
          onClick={() =>
            window.open(`/finance/invoices/${invoice.id}/print`, "_blank", "noopener,noreferrer")
          }
        >
          <Icon name="printer" className="h-4 w-4" />
          Print
        </Button>
      </div>

      {problem && (
        <span className={cx("max-w-[240px] text-[12.5px] font-medium text-[#B42318]")}>
          {problem.message}
        </span>
      )}

      <Drawer
        open={step !== null}
        onClose={close}
        title={drawerTitle}
        description={`${invoice.ref} · ${humanise(invoice.status)}`}
      >
        <form onSubmit={onStep} className="flex flex-col gap-4 px-6 py-[22px]">
          {problem && <ProblemNotice error={problem} title="That didn’t go through" />}

          {step === "issue" ? (
            <>
              <div className="grid grid-cols-1 gap-3.5 sm:grid-cols-2">
                <Field label="Issue date" hint="optional" htmlFor="invoice-issue-date">
                  <TextInput
                    id="invoice-issue-date"
                    name="issue_date"
                    type="date"
                    defaultValue={dateInput(invoice.issue_date) || today()}
                  />
                </Field>
                <Field label="Due date" hint="optional" htmlFor="invoice-due-date">
                  <TextInput
                    id="invoice-due-date"
                    name="due_date"
                    type="date"
                    defaultValue={dateInput(invoice.due_date)}
                  />
                </Field>
              </div>
              <p className="text-[13.5px] text-muted">
                Leave both blank to let Go date it from the contract’s terms.
              </p>
            </>
          ) : step === "dispute" ? (
            <Field label="Reason" hint="optional" htmlFor="invoice-dispute-reason">
              <TextArea
                id="invoice-dispute-reason"
                name="reason"
                rows={3}
                placeholder="What the client is querying."
                data-autofocus
              />
            </Field>
          ) : (
            <>
              <p className="rounded-control border border-[#F5C2BD] bg-[#FDECEC] px-3.5 py-3 text-[13.5px] text-[#B42318]">
                Writing off an invoice removes it from receivables for good. It is
                one decision by whoever holds the permission, and both the
                decision and this reason are written to the audit trail against
                your name (§6.6).
              </p>
              <Field label="Why is it being written off?" htmlFor="invoice-writeoff-reason">
                <TextArea
                  id="invoice-writeoff-reason"
                  name="reason"
                  required
                  rows={3}
                  data-autofocus
                />
              </Field>
            </>
          )}

          {step === "write-off" || step === "dispute" ? null : (
            <p className="text-[13.5px] text-muted">
              Check the dates before issuing — Go refuses a due date before the
              issue date.
            </p>
          )}

          <div className="flex flex-wrap gap-2.5">
            <Button
              type="submit"
              variant={step === "write-off" ? "danger" : "primary"}
              disabled={busy}
            >
              {busy ? "Saving…" : drawerTitle}
            </Button>
            <Button type="button" variant="secondary" onClick={close} disabled={busy}>
              Cancel
            </Button>
          </div>
        </form>
      </Drawer>
    </div>
  );
}
