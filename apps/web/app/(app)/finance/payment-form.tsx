"use client";

import { useRouter } from "next/navigation";
import { useState, type FormEvent } from "react";

import { Drawer } from "@/components/drawer";
import { Button, Field, Select, TextArea, TextInput } from "@/components/form";
import { Icon } from "@/components/icons";
import { useToast } from "@/components/toast";
import { cx, ProblemNotice } from "@/components/ui";
import {
  allocatePayment,
  createPayment,
  PAYMENT_METHODS,
  reconcilePayment,
  type FinanceAccount,
  type FinancePayment,
  type FinancePaymentBody,
  type FinancePaymentAllocationBody,
} from "@/lib/api/atlas";
import { formatMinor, humanise, parseMinor, today } from "@/lib/format";
import { useMutation } from "@/lib/use-mutation";

import type { FinanceCapability } from "./permissions";

/**
 * Payments (`/api/v1/finance/payments`, §8.4). A payment is recorded against an
 * account and then allocated across open invoices; each allocation can take no
 * more than the invoice still owes and no more than the payment brought in, so
 * the caps below are the same arithmetic Go applies.
 */

/** The open invoices a payment can be spread over, with what each still owes. */
export interface OpenInvoice {
  id: string;
  label: string;
  /** Minor units; absent when the amount is restricted for the viewer. */
  outstanding_minor?: number;
}

function field(data: FormData, key: string): string {
  return String(data.get(key) ?? "").trim();
}

/** The typed allocations, keeping only the rows with a positive amount. */
function allocationRows(
  amounts: Record<string, string>,
  openInvoices: OpenInvoice[],
  currency: string,
): FinancePaymentAllocationBody[] {
  return openInvoices
    .map((invoice) => ({
      invoice_id: invoice.id,
      amount_minor: parseMinor(amounts[invoice.id] ?? "", currency),
    }))
    .filter((row) => row.amount_minor > 0);
}

/** The most that can be put on one invoice: what it owes, and what is left. */
function capFor(invoice: OpenInvoice, remaining: number): number {
  const owes = invoice.outstanding_minor ?? Number.POSITIVE_INFINITY;
  return Math.max(0, Math.min(owes, remaining));
}

/** The allocation grid shared by "record payment" and "allocate". */
function AllocationGrid({
  openInvoices,
  amounts,
  onChange,
  currency,
  cap,
}: {
  openInvoices: OpenInvoice[];
  amounts: Record<string, string>;
  onChange: (invoiceId: string, value: string) => void;
  currency: string;
  /** The most that can still be allocated in total (the payment's remainder). */
  cap: number;
}) {
  if (openInvoices.length === 0) {
    return (
      <p className="text-[13.5px] text-muted">
        No open invoices to allocate against — the payment stays unallocated
        until one exists.
      </p>
    );
  }

  let remaining = cap;
  return (
    <ul className="flex flex-col gap-2.5">
      {openInvoices.map((invoice) => {
        const most = capFor(invoice, remaining);
        const value = amounts[invoice.id] ?? "";
        const entered = parseMinor(value, currency);
        remaining -= entered;
        return (
          <li
            key={invoice.id}
            className="flex flex-wrap items-center gap-3 rounded-[12px] border border-line px-3 py-2.5"
          >
            <span className="min-w-0 flex-1">
              <b className="block truncate text-[14px] font-medium">{invoice.label}</b>
              <small className="block text-[12.5px] text-muted">
                {invoice.outstanding_minor === undefined
                  ? "Outstanding restricted for your role"
                  : `${formatMinor(invoice.outstanding_minor, currency)} outstanding`}
              </small>
            </span>
            <input
              type="number"
              min="0"
              step="0.01"
              inputMode="decimal"
              aria-label={`Allocate to ${invoice.label}`}
              placeholder="0.00"
              value={value}
              onChange={(event) => onChange(invoice.id, event.target.value)}
              className="h-10 w-[130px] rounded-control border border-line-strong bg-white px-3 text-right text-[14px] outline-none focus:border-atlas focus:shadow-[0_0_0_3px_rgba(26,108,245,.15)]"
            />
            {entered > most && (
              <span className="basis-full text-[12.5px] font-medium text-[#B42318]">
                At most {formatMinor(most, currency)} can go here.
              </span>
            )}
          </li>
        );
      })}
    </ul>
  );
}

/**
 * "Record payment" (`POST /api/v1/finance/payments`, `finance.payment.create`).
 * The amount is in major units and converted to minor units by `parseMinor`;
 * any allocation made here is capped by what the invoice still owes.
 */
export function RecordPaymentButton({
  accounts,
  openInvoices,
  currency,
  capabilities,
  variant = "primary",
}: {
  accounts: FinanceAccount[];
  openInvoices: OpenInvoice[];
  currency: string;
  capabilities: FinanceCapability[];
  variant?: "primary" | "secondary";
}) {
  const router = useRouter();
  const notify = useToast();
  const { busy, problem, run, reset } = useMutation();
  const [open, setOpen] = useState(false);
  const [amount, setAmount] = useState("");
  const [allocations, setAllocations] = useState<Record<string, string>>({});

  if (!capabilities.includes("finance.payment.create")) return null;

  function close() {
    setOpen(false);
    setAmount("");
    setAllocations({});
    reset();
  }

  const paymentMinor = parseMinor(amount, currency);
  const allocated = allocationRows(allocations, openInvoices, currency).reduce(
    (sum, row) => sum + row.amount_minor,
    0,
  );
  const overAllocated = allocated > paymentMinor;

  async function onSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const data = new FormData(event.currentTarget);

    const body: FinancePaymentBody = {
      amount_minor: parseMinor(field(data, "amount"), currency),
      method: field(data, "method"),
      received_on: field(data, "received_on"),
    };
    if (currency) body.currency = currency;
    const reference = field(data, "reference");
    if (reference) body.reference = reference;
    const account = field(data, "account_id");
    if (account) body.account_id = account;
    const notes = field(data, "notes");
    if (notes) body.notes = notes;
    const rows = allocationRows(allocations, openInvoices, currency);
    if (rows.length > 0) body.allocations = rows;

    const created = await run(() => createPayment(body));
    if (created) {
      setOpen(false);
      setAmount("");
      setAllocations({});
      notify(`Recorded ${created.ref}`);
      router.refresh();
    }
  }

  return (
    <>
      <Button variant={variant} size={variant === "secondary" ? "sm" : "md"} onClick={() => setOpen(true)}>
        <Icon name="plus" className={variant === "secondary" ? "h-4 w-4" : undefined} />
        Record payment
      </Button>
      <Drawer
        open={open}
        onClose={close}
        wide
        title="Record payment"
        description="Money received, against the account it landed in. Allocations are optional and capped by what each invoice owes."
      >
        <form onSubmit={onSubmit} className="flex flex-col gap-4 px-6 py-[22px]">
          {problem && <ProblemNotice error={problem} title="The payment wasn’t recorded" />}

          <div className="grid grid-cols-1 gap-3.5 sm:grid-cols-2">
            <Field label="Amount" hint={currency} htmlFor="payment-amount">
              <TextInput
                id="payment-amount"
                name="amount"
                required
                inputMode="decimal"
                placeholder="0.00"
                value={amount}
                onChange={(event) => setAmount(event.target.value)}
                data-autofocus
              />
            </Field>
            <Field label="Received on" htmlFor="payment-received">
              <TextInput
                id="payment-received"
                name="received_on"
                type="date"
                required
                defaultValue={today()}
              />
            </Field>
            <Field label="Method" htmlFor="payment-method">
              <Select id="payment-method" name="method" defaultValue="bank">
                {PAYMENT_METHODS.map((method) => (
                  <option key={method} value={method}>
                    {humanise(method)}
                  </option>
                ))}
              </Select>
            </Field>
            <Field label="Account" hint="optional" htmlFor="payment-account">
              <Select id="payment-account" name="account_id" defaultValue="">
                <option value="">Not recorded</option>
                {accounts.map((account) => (
                  <option key={account.id} value={account.id}>
                    {account.name}
                  </option>
                ))}
              </Select>
            </Field>
            <Field label="Reference" hint="optional" htmlFor="payment-reference">
              <TextInput id="payment-reference" name="reference" placeholder="Bank slip or M-Pesa code" />
            </Field>
          </div>

          <Field label="Notes" hint="optional" htmlFor="payment-notes">
            <TextArea id="payment-notes" name="notes" rows={2} />
          </Field>

          <div className="flex flex-col gap-2.5">
            <span className="text-sm font-semibold text-night">
              Allocations <span className="font-normal text-muted">optional, in {currency}</span>
            </span>
            <AllocationGrid
              openInvoices={openInvoices}
              amounts={allocations}
              onChange={(id, value) => setAllocations((current) => ({ ...current, [id]: value }))}
              currency={currency}
              cap={paymentMinor}
            />
            {allocated > 0 && (
              <p className={cx("text-[13px]", overAllocated ? "font-medium text-[#B42318]" : "text-muted")}>
                Allocated {formatMinor(allocated, currency)} of{" "}
                {formatMinor(paymentMinor, currency)}
                {overAllocated
                  ? " — that is more than the payment brought in."
                  : ` · ${formatMinor(paymentMinor - allocated, currency)} left unallocated.`}
              </p>
            )}
          </div>

          <div className="flex flex-wrap gap-2.5">
            <Button type="submit" disabled={busy || overAllocated}>
              {busy ? "Recording…" : "Record payment"}
            </Button>
            <Button type="button" variant="secondary" onClick={close} disabled={busy}>
              Cancel
            </Button>
          </div>
        </form>
      </Drawer>
    </>
  );
}

/**
 * The actions for one payment row: split it across open invoices, and mark it
 * reconciled against the bank once it has cleared.
 */
export function PaymentActions({
  payment,
  openInvoices,
  currency,
  capabilities,
}: {
  payment: FinancePayment;
  openInvoices: OpenInvoice[];
  currency: string;
  capabilities: FinanceCapability[];
}) {
  const router = useRouter();
  const notify = useToast();
  const { busy, problem, run, reset } = useMutation();
  const [open, setOpen] = useState(false);
  const [allocations, setAllocations] = useState<Record<string, string>>({});

  function close() {
    setOpen(false);
    setAllocations({});
    reset();
  }

  // The money still to place: what came in minus what is already allocated. If
  // the amount is restricted, the caps fall back to each invoice's outstanding.
  const unallocated =
    payment.amount_minor === undefined
      ? undefined
      : Math.max(0, payment.amount_minor - (payment.allocated_minor ?? 0));

  async function onAllocate(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const rows = allocationRows(allocations, openInvoices, currency);
    const updated = await run(() => allocatePayment(payment.id, payment.version, rows));
    if (updated) {
      close();
      notify(`Allocated ${payment.ref}`);
      router.refresh();
    }
  }

  async function toggleReconciled() {
    const done = await run(() =>
      reconcilePayment(payment.id, payment.version, !payment.reconciled),
    );
    if (done) {
      notify(payment.reconciled ? "Marked un-reconciled" : "Payment reconciled");
      router.refresh();
    }
  }

  const canAllocate = capabilities.includes("finance.payment.allocate");
  const canReconcile = capabilities.includes("finance.payment.reconcile");
  const nothing = !canAllocate && !canReconcile;

  return (
    <div className="flex flex-col items-end gap-2">
      <div className="flex flex-wrap items-center justify-end gap-1.5">
        {canAllocate && (
          <Button size="sm" variant="secondary" disabled={busy} onClick={() => setOpen(true)}>
            Allocate
          </Button>
        )}
        {canReconcile && (
          <Button size="sm" variant="secondary" disabled={busy} onClick={toggleReconciled}>
            {payment.reconciled ? "Un-reconcile" : "Reconcile"}
          </Button>
        )}
        {nothing && <span className="text-muted">—</span>}
      </div>

      {problem && (
        <span className={cx("max-w-[240px] text-[12.5px] font-medium text-[#B42318]")}>
          {problem.message}
        </span>
      )}

      <Drawer
        open={open}
        onClose={close}
        wide
        title="Allocate payment"
        description={`${payment.ref} · ${payment.reconciled ? "reconciled" : "not yet reconciled"}`}
      >
        <form onSubmit={onAllocate} className="flex flex-col gap-4 px-6 py-[22px]">
          {problem && <ProblemNotice error={problem} title="The allocation wasn’t saved" />}

          {unallocated === undefined ? (
            <p className="text-[13.5px] text-muted">
              The payment amount is restricted for your role, so allocations are
              capped only by what each invoice owes.
            </p>
          ) : (
            <p className="text-[13.5px] text-muted">
              <span className="font-semibold text-night">{formatMinor(unallocated, currency)}</span>{" "}
              of {formatMinor(payment.amount_minor ?? 0, currency)} is still unallocated.
            </p>
          )}

          <AllocationGrid
            openInvoices={openInvoices}
            amounts={allocations}
            onChange={(id, value) => setAllocations((current) => ({ ...current, [id]: value }))}
            currency={currency}
            cap={unallocated ?? Number.POSITIVE_INFINITY}
          />

          <div className="flex flex-wrap gap-2.5">
            <Button
              type="submit"
              disabled={busy || allocationRows(allocations, openInvoices, currency).length === 0}
            >
              {busy ? "Allocating…" : "Save allocations"}
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
