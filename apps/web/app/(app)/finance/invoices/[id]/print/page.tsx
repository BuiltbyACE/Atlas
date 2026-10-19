import { notFound } from "next/navigation";

import { ButtonLink } from "@/components/form";
import { Badge, ProblemNotice, type Tone } from "@/components/ui";
import {
  auditHistory,
  getInvoice,
  getSetting,
  type AuditEntry,
  type FinanceInvoice,
} from "@/lib/api/atlas";
import { ApiError } from "@/lib/api/client";
import { personDirectory, personName } from "@/lib/directory";
import { formatDay, formatDateTime, formatMinor, humanise } from "@/lib/format";

import { PrintButton } from "./print-button";

/**
 * The printable invoice (`/finance/invoices/{id}/print`, §8.4). Go computes
 * every line and total and withholds any amount the viewer may not see
 * (`finance.amount.sensitive.read`), so this page lays out exactly what it is
 * given and adds nothing: the company name comes from settings, the client from
 * the invoice, and the history from the audit trail, who it is named after.
 *
 * Printing is a screen concern. The sidebar and top bar are dropped by the
 * `@media print` rules in `globals.css`, so Ctrl+P puts this document on the
 * sheet alone and "Save as PDF" is the invoice's PDF for now (§8.4 marks a
 * rendered PDF service `[DECIDE]`).
 */

export const dynamic = "force-dynamic";

/** Invoice badge tones; the work/people maps do not know these states. */
function invoiceTone(status: string): Tone {
  switch (status) {
    case "paid":
      return "success";
    case "issued":
    case "partially_paid":
    case "approved":
      return "info";
    case "overdue":
    case "disputed":
      return "danger";
    default:
      return "neutral";
  }
}

/** Amounts are withheld, never zero: say so rather than printing a dash. */
function money(minor: number | undefined, currency: string): string {
  return minor === undefined ? "Restricted" : formatMinor(minor, currency);
}

/** A date in the document's house style, with an em dash when there is none. */
function day(value?: string | null): string {
  return value ? formatDay(value) : "—";
}

/** `finance.invoice.issued` -> `issued`, for the history line's verb. */
function verb(action: string): string {
  const last = action.split(".").pop() ?? action;
  return humanise(last).toLowerCase();
}

/** The company name as configured (`company.name`), when it may be read. */
async function companyName(): Promise<string | undefined> {
  try {
    const setting = await getSetting("company.name");
    return typeof setting.value === "string" && setting.value.trim()
      ? setting.value.trim()
      : undefined;
  } catch {
    // A viewer without `core.settings.read` still prints a working invoice;
    // the header simply carries no company name.
    return undefined;
  }
}

/** Who did what to this invoice. No permission for it means no history block. */
async function invoiceHistory(id: string): Promise<AuditEntry[]> {
  try {
    return (await auditHistory("finance.invoice", id, { limit: 10 })).items;
  } catch {
    return [];
  }
}

export default async function InvoicePrintPage({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = await params;

  let invoice: FinanceInvoice;
  try {
    invoice = await getInvoice(id);
  } catch (error) {
    if (!(error instanceof ApiError)) throw error;
    if (error.status === 404) notFound();
    return (
      <div className="mx-auto max-w-[640px] space-y-4 pt-8">
        <ProblemNotice error={error} title="This invoice could not be opened" />
        <ButtonLink href="/finance" variant="secondary">
          Back to Finance
        </ButtonLink>
      </div>
    );
  }

  const [company, history, names] = await Promise.all([
    companyName(),
    invoiceHistory(invoice.id),
    personDirectory(),
  ]);

  const currency = invoice.currency || "KES";
  const lines = invoice.lines ?? [];

  // Line amounts are pre-tax; the invoice total is tax-inclusive (§8.4). Both
  // are withheld together by Go, so the arithmetic below is never half-defined.
  const subtotal =
    lines.length > 0 && lines.every((line) => line.amount_minor !== undefined)
      ? lines.reduce((sum, line) => sum + (line.amount_minor ?? 0), 0)
      : undefined;
  const taxFromLines =
    subtotal === undefined
      ? undefined
      : lines.reduce(
          (sum, line) =>
            sum + Math.round(((line.amount_minor ?? 0) * line.tax_rate_bp) / 10_000),
          0,
        );
  const total =
    invoice.total_minor ??
    (subtotal !== undefined && taxFromLines !== undefined ? subtotal + taxFromLines : undefined);
  const tax =
    invoice.total_minor !== undefined && subtotal !== undefined
      ? invoice.total_minor - subtotal
      : taxFromLines;

  return (
    <div className="mx-auto w-full max-w-[840px] pb-10">
      <div className="no-print mb-5 flex flex-wrap items-center justify-between gap-3">
        <ButtonLink href="/finance" variant="secondary">
          Back to Finance
        </ButtonLink>
        <div className="flex items-center gap-2.5">
          <span className="hidden text-[13px] text-muted sm:block">
            {invoice.ref} · {humanise(invoice.status)}
          </span>
          <PrintButton />
        </div>
      </div>

      <div className="rounded-control border border-line bg-white px-6 py-8 shadow-sm print:border-0 print:shadow-none sm:px-10 sm:py-11">
        <header className="flex flex-wrap items-start justify-between gap-x-8 gap-y-4">
          <div className="min-w-0">
            <p className="text-[19px] font-semibold tracking-tight text-night">
              {company ?? "Atlas"}
            </p>
            <p className="mt-0.5 text-[13px] text-muted">
              {company ? "Atlas · SafariStack Company OS" : "SafariStack Company OS"}
            </p>
          </div>
          <div className="text-right">
            <p className="text-[13px] font-semibold uppercase tracking-[0.14em] text-muted">
              Invoice
            </p>
            <p className="mt-1 text-[22px] font-semibold tabular tracking-tight text-night">
              {invoice.ref}
            </p>
            <div className="mt-2 flex justify-end">
              <Badge tone={invoiceTone(invoice.status)} dot>
                {humanise(invoice.status)}
              </Badge>
            </div>
          </div>
        </header>

        <div className="mt-8 grid grid-cols-1 gap-6 border-t border-line pt-6 sm:grid-cols-2">
          <div>
            <p className="text-[12px] font-semibold uppercase tracking-[0.1em] text-muted">
              Billed to
            </p>
            <p className="mt-1.5 text-[15px] font-medium text-night">
              {invoice.organization_name ?? "Unnamed organisation"}
            </p>
            {invoice.contract_ref && (
              <p className="mt-0.5 text-[13px] text-muted">
                Contract {invoice.contract_ref}
                {invoice.milestone_id ? " · milestone-linked" : ""}
              </p>
            )}
            {invoice.notes && <p className="mt-3 text-[13px] text-muted">{invoice.notes}</p>}
          </div>
          <dl className="grid grid-cols-2 gap-x-4 gap-y-2 text-[13px] sm:justify-items-start">
            <dt className="text-muted">Issue date</dt>
            <dd className="tabular text-night">{day(invoice.issue_date)}</dd>
            <dt className="text-muted">Due date</dt>
            <dd className="tabular text-night">{day(invoice.due_date)}</dd>
            {invoice.issued_at && (
              <>
                <dt className="text-muted">Issued</dt>
                <dd className="tabular text-night">{formatDateTime(invoice.issued_at)}</dd>
              </>
            )}
            {invoice.e_invoice_ref && (
              <>
                <dt className="text-muted">E-invoice</dt>
                <dd className="tabular text-night">{invoice.e_invoice_ref}</dd>
              </>
            )}
          </dl>
        </div>

        <table className="mt-8 w-full border-collapse text-left text-[14px]">
          <thead>
            <tr className="border-b border-line text-[12px] uppercase tracking-[0.08em] text-muted">
              <th className="py-2 pr-3 font-semibold">#</th>
              <th className="py-2 pr-3 font-semibold">Description</th>
              <th className="py-2 pr-3 text-right font-semibold">Qty</th>
              <th className="py-2 pr-3 text-right font-semibold">Unit price</th>
              <th className="py-2 pr-3 text-right font-semibold">Tax</th>
              <th className="py-2 text-right font-semibold">Amount</th>
            </tr>
          </thead>
          <tbody>
            {lines.length === 0 ? (
              <tr>
                <td colSpan={6} className="py-6 text-[13px] text-muted">
                  This invoice has no line items.
                </td>
              </tr>
            ) : (
              lines
                .slice()
                .sort((a, b) => a.sort_order - b.sort_order)
                .map((line, index) => (
                  <tr key={line.id} className="border-b border-line/70 align-top">
                    <td className="py-3 pr-3 tabular text-muted">{index + 1}</td>
                    <td className="py-3 pr-3 text-night">{line.description}</td>
                    <td className="py-3 pr-3 text-right tabular">{line.quantity}</td>
                    <td className="py-3 pr-3 text-right tabular">
                      {money(line.unit_price_minor, currency)}
                    </td>
                    <td className="py-3 pr-3 text-right tabular text-muted">
                      {line.tax_rate_bp > 0 ? `${line.tax_rate_bp / 100}%` : "—"}
                    </td>
                    <td className="py-3 text-right tabular text-night">
                      {money(line.amount_minor, currency)}
                    </td>
                  </tr>
                ))
            )}
          </tbody>
        </table>

        <div className="mt-6 flex justify-end">
          <dl className="w-full max-w-[340px] space-y-1.5 text-[14px]">
            <div className="flex items-baseline justify-between gap-6">
              <dt className="text-muted">Subtotal</dt>
              <dd className="tabular">{money(subtotal, currency)}</dd>
            </div>
            <div className="flex items-baseline justify-between gap-6">
              <dt className="text-muted">Tax</dt>
              <dd className="tabular">{money(tax, currency)}</dd>
            </div>
            <div className="flex items-baseline justify-between gap-6 border-t border-night/20 pt-2 text-[16px] font-semibold">
              <dt>Total</dt>
              <dd className="tabular text-night">{money(total, currency)}</dd>
            </div>
            <div className="flex items-baseline justify-between gap-6 pt-1 text-[13px]">
              <dt className="text-muted">Paid to date</dt>
              <dd className="tabular">{money(invoice.amount_paid_minor, currency)}</dd>
            </div>
            <div className="flex items-baseline justify-between gap-6 text-[13px] font-medium">
              <dt className="text-muted">Outstanding</dt>
              <dd className="tabular">{money(invoice.outstanding_minor, currency)}</dd>
            </div>
          </dl>
        </div>

        {history.length > 0 && (
          <section className="mt-9 border-t border-line pt-5">
            <h2 className="text-[12px] font-semibold uppercase tracking-[0.1em] text-muted">
              History
            </h2>
            <ul className="mt-3 space-y-1.5 text-[13px] text-muted">
              {history.map((entry) => {
                const who = personName(names, entry.actor_id);
                return (
                  <li key={entry.id} className="flex flex-wrap items-baseline gap-x-2">
                    <span className="font-medium text-night">
                      {verb(entry.action)} by {who}
                    </span>
                    <span className="tabular">{formatDateTime(entry.occurred_at)}</span>
                    {entry.reason && <span className="italic">— {entry.reason}</span>}
                  </li>
                );
              })}
            </ul>
          </section>
        )}

        <footer className="mt-9 border-t border-line pt-4 text-[12px] text-muted">
          <p>
            Printed {formatDay(new Date().toISOString())} · Atlas, the SafariStack Company OS.
          </p>
          <p className="mt-1">
            Every change to this invoice is written to the audit trail with the name of the
            person who made it.
          </p>
        </footer>
      </div>
    </div>
  );
}
