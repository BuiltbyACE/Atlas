"use client";

import { useRouter } from "next/navigation";
import { useState, type FormEvent } from "react";

import { Drawer } from "@/components/drawer";
import { Button, Field, Select, TextArea, TextInput } from "@/components/form";
import { Icon } from "@/components/icons";
import { PersonSelect } from "@/components/person-select";
import { useToast } from "@/components/toast";
import { cx, ProblemNotice } from "@/components/ui";
import {
  createExpense,
  decideExpense,
  PAYMENT_METHODS,
  reconcileExpense,
  settleExpense,
  submitExpense,
  type FinanceAccount,
  type FinanceExpense,
  type FinanceExpenseBody,
  type FinanceExpenseCategory,
  uploadFile,
} from "@/lib/api/atlas";
import { ApiError } from "@/lib/api/client";
import type { PersonOption } from "@/lib/directory";
import { humanise, parseMinor, today } from "@/lib/format";
import { useMutation } from "@/lib/use-mutation";

import type { FinanceCapability } from "./permissions";

/**
 * Expenses (`/api/v1/finance/expenses`, §8.4). Recording a claim takes
 * `finance.expense.create`; approving takes `finance.expense.approve` and is one
 * audited decision — whoever may approve may approve any claim, including their
 * own, and the audit row names them (§6.6). These components only offer the
 * steps the status and the viewer's role allow.
 */

/** A text field, trimmed; `""` when the field is absent. */
function field(data: FormData, key: string): string {
  return String(data.get(key) ?? "").trim();
}

/**
 * Push a receipt to object storage (§6.5): the bytes go to a pre-signed URL and
 * Go only ever records the id. A storage failure is re-thrown as the same
 * problem shape the API returns, so the drawer says what actually went wrong
 * instead of the generic "something went wrong".
 */
async function uploadReceipt(file: File): Promise<string> {
  try {
    const record = await uploadFile(file, { classification: "confidential" });
    return record.id;
  } catch (error) {
    if (error instanceof ApiError) throw error;
    throw new ApiError(502, {
      title: "The receipt didn’t upload",
      detail: error instanceof Error ? error.message : String(error),
      code: "FINANCE_RECEIPT_UPLOAD_FAILED",
    });
  }
}

/**
 * Build the create body from the drawer's form. `receiptFileId` is the file
 * already in storage: §8.4 asks for a receipt on a claim above a configured
 * amount, so the upload happens first and the claim is created against it.
 */
function expenseBody(data: FormData, currency: string, receiptFileId?: string): FinanceExpenseBody {
  const body: FinanceExpenseBody = {
    category_key: field(data, "category_key"),
    amount_minor: parseMinor(field(data, "amount"), currency),
    spent_on: field(data, "spent_on"),
  };
  const claimant = field(data, "claimant_person_id");
  if (claimant) body.claimant_person_id = claimant;
  const project = field(data, "project_id");
  if (project) body.project_id = project;
  if (currency) body.currency = currency;
  const vendor = field(data, "vendor");
  if (vendor) body.vendor = vendor;
  const method = field(data, "payment_method");
  if (method) body.payment_method = method;
  const taxRef = field(data, "tax_invoice_ref");
  if (taxRef) body.tax_invoice_ref = taxRef;
  const notes = field(data, "notes");
  if (notes) body.notes = notes;
  if (receiptFileId) body.receipt_file_id = receiptFileId;
  return body;
}

/**
 * "New expense" (`POST /api/v1/finance/expenses`, `finance.expense.create`): a
 * button that opens the claim form in a drawer. The claimant defaults to the
 * signed-in person, who is usually the one claiming.
 */
export function NewExpenseButton({
  people,
  me,
  categories,
  projects,
  currency,
  capabilities,
  variant = "primary",
}: {
  people: PersonOption[];
  me?: string;
  categories: FinanceExpenseCategory[];
  projects: { id: string; name: string }[];
  currency: string;
  capabilities: FinanceCapability[];
  variant?: "primary" | "secondary";
}) {
  const router = useRouter();
  const notify = useToast();
  const { busy, problem, run, reset } = useMutation();
  const [open, setOpen] = useState(false);

  if (!capabilities.includes("finance.expense.create")) return null;

  function close() {
    setOpen(false);
    reset();
  }

  async function onSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const data = new FormData(event.currentTarget);
    const created = await run(async () => {
      const receipt = data.get("receipt");
      const receiptFileId =
        receipt instanceof File && receipt.size > 0
          ? await uploadReceipt(receipt)
          : undefined;
      return createExpense(expenseBody(data, currency, receiptFileId));
    });
    if (created) {
      setOpen(false);
      notify(`Recorded ${created.ref}`);
      router.refresh();
    }
  }

  return (
    <>
      <Button variant={variant} size={variant === "secondary" ? "sm" : "md"} onClick={() => setOpen(true)}>
        <Icon name="plus" className={variant === "secondary" ? "h-4 w-4" : undefined} />
        New expense
      </Button>
      <Drawer
        open={open}
        onClose={close}
        title="New expense"
        description="Every claim lands against the person who recorded it. An approver decides it in one step, and that decision is audited."
      >
        {categories.length === 0 ? (
          <p className="px-6 py-6 text-sm text-muted">
            No expense categories are configured yet, so a claim has nothing to
            sit under. An administrator sets them up in Atlas settings.
          </p>
        ) : (
          <form onSubmit={onSubmit} className="flex flex-col gap-4 px-6 py-[22px]">
            {problem && <ProblemNotice error={problem} title="The claim wasn’t saved" />}

            <Field label="Claimant" hint="you, by default" htmlFor="expense-claimant">
              <PersonSelect
                id="expense-claimant"
                name="claimant_person_id"
                people={people}
                me={me}
                defaultValue={me ?? ""}
                emptyLabel={me ? undefined : "Choose a person"}
              />
            </Field>

            <Field label="Category" htmlFor="expense-category">
              <Select id="expense-category" name="category_key" required data-autofocus>
                {categories.map((category) => (
                  <option key={category.key} value={category.key}>
                    {category.label}
                  </option>
                ))}
              </Select>
            </Field>

            <div className="grid grid-cols-1 gap-3.5 sm:grid-cols-2">
              <Field label="Amount" hint={currency} htmlFor="expense-amount">
                <TextInput
                  id="expense-amount"
                  name="amount"
                  required
                  inputMode="decimal"
                  placeholder="0.00"
                />
              </Field>
              <Field label="Spent on" htmlFor="expense-spent">
                <TextInput
                  id="expense-spent"
                  name="spent_on"
                  type="date"
                  required
                  defaultValue={today()}
                />
              </Field>
              <Field label="Paid by" hint="optional" htmlFor="expense-method">
                <Select id="expense-method" name="payment_method" defaultValue="">
                  <option value="">Not recorded</option>
                  {PAYMENT_METHODS.map((method) => (
                    <option key={method} value={method}>
                      {humanise(method)}
                    </option>
                  ))}
                </Select>
              </Field>
              <Field label="Vendor" hint="optional" htmlFor="expense-vendor">
                <TextInput id="expense-vendor" name="vendor" placeholder="e.g. Bolt, Kenindia" />
              </Field>
              {projects.length > 0 && (
                <Field label="Project" hint="optional" htmlFor="expense-project">
                  <Select id="expense-project" name="project_id" defaultValue="">
                    <option value="">Not against a project</option>
                    {projects.map((project) => (
                      <option key={project.id} value={project.id}>
                        {project.name}
                      </option>
                    ))}
                  </Select>
                </Field>
              )}
              <Field label="Tax invoice ref" hint="optional" htmlFor="expense-tax">
                <TextInput id="expense-tax" name="tax_invoice_ref" placeholder="VAT invoice number" />
              </Field>
            </div>

            <Field
              label="Receipt"
              hint="required above a configured amount"
              htmlFor="expense-receipt"
            >
              <TextInput
                id="expense-receipt"
                name="receipt"
                type="file"
                accept="image/*,application/pdf"
              />
            </Field>

            <Field label="Notes" hint="optional" htmlFor="expense-notes">
              <TextArea id="expense-notes" name="notes" rows={3} placeholder="What this was for." />
            </Field>

            <div className="flex flex-wrap gap-2.5">
              <Button type="submit" disabled={busy}>
                {busy ? "Recording…" : "Record expense"}
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

/** Which step the row is asking for input on. */
type ExpenseStep = "reject" | "settle-paid" | "settle-reimbursed";

/**
 * The lifecycle actions for one expense row. Each is offered only when the
 * status allows it and the viewer's capabilities cover it; Go still has the
 * final say, and a refusal is shown as it was given.
 */
export function ExpenseActions({
  expense,
  me,
  accounts,
  currency,
  capabilities,
}: {
  expense: FinanceExpense;
  me?: string;
  accounts: FinanceAccount[];
  currency: string;
  capabilities: FinanceCapability[];
}) {
  const router = useRouter();
  const notify = useToast();
  const { busy, problem, run, reset } = useMutation();
  const [step, setStep] = useState<ExpenseStep | null>(null);

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

  async function approve() {
    const updated = await run(() => decideExpense(expense.id, expense.version, "approve"));
    if (updated) {
      notify("Expense approved");
      router.refresh();
    }
  }

  async function onStep(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const data = new FormData(event.currentTarget);

    if (step === "reject") {
      const reason = field(data, "reason");
      const updated = await run(() =>
        decideExpense(expense.id, expense.version, "reject", reason || undefined),
      );
      if (updated) {
        close();
        notify("Expense rejected");
        router.refresh();
      }
      return;
    }

    const method = step === "settle-reimbursed" ? "reimbursed" : "paid";
    const account = field(data, "account_id");
    const paidOn = field(data, "paid_on");
    const updated = await run(() =>
      settleExpense(expense.id, expense.version, {
        method,
        account_id: account || undefined,
        paid_on: paidOn || undefined,
      }),
    );
    if (updated) {
      close();
      notify(method === "paid" ? "Expense marked paid" : "Expense marked for reimbursement");
      router.refresh();
    }
  }

  const canSubmit =
    capabilities.includes("finance.expense.submit") &&
    (expense.status === "draft" || expense.status === "rejected");
  // Who decides is a matter of permission, not of distance from the claim: the
  // footprint on the audit row is the control (§6.6). `me` only defaults the
  // claimant above, so self-approval is offered where it is permitted.
  const canDecide =
    capabilities.includes("finance.expense.approve") && expense.status === "submitted";
  const canSettle =
    capabilities.includes("finance.expense.settle") && expense.status === "approved";
  const canReconcile =
    capabilities.includes("finance.expense.reconcile") &&
    (expense.status === "paid" || expense.status === "reimbursed");

  const nothing = !canSubmit && !canDecide && !canSettle && !canReconcile;

  const drawerTitle =
    step === "reject"
      ? "Reject expense"
      : step === "settle-paid"
        ? "Mark expense paid"
        : "Reimburse expense";

  return (
    <div className="flex flex-col items-end gap-2">
      <div className="flex flex-wrap items-center justify-end gap-1.5">
        {canSubmit && (
          <Button
            size="sm"
            variant="secondary"
            disabled={busy}
            onClick={() => act(() => submitExpense(expense.id, expense.version), "Sent for approval")}
          >
            Submit
          </Button>
        )}
        {canDecide && (
          <>
            <Button size="sm" disabled={busy} onClick={approve}>
              Approve
            </Button>
            <Button size="sm" variant="danger" disabled={busy} onClick={() => setStep("reject")}>
              Reject
            </Button>
          </>
        )}
        {canSettle && (
          <>
            <Button size="sm" variant="secondary" disabled={busy} onClick={() => setStep("settle-paid")}>
              Mark paid
            </Button>
            <Button
              size="sm"
              variant="secondary"
              disabled={busy}
              onClick={() => setStep("settle-reimbursed")}
            >
              Reimburse
            </Button>
          </>
        )}
        {canReconcile && (
          <Button
            size="sm"
            variant="secondary"
            disabled={busy}
            onClick={() => act(() => reconcileExpense(expense.id, expense.version), "Expense reconciled")}
          >
            Reconcile
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
        open={step !== null}
        onClose={close}
        title={drawerTitle}
        description={`${expense.ref} · ${humanise(expense.status)}`}
      >
        <form onSubmit={onStep} className="flex flex-col gap-4 px-6 py-[22px]">
          {problem && <ProblemNotice error={problem} title="That didn’t go through" />}

          {step === "reject" ? (
            <Field label="Why is it rejected?" htmlFor="expense-reject-reason">
              <TextArea
                id="expense-reject-reason"
                name="reason"
                required
                rows={3}
                placeholder="The claimant sees this, so be specific."
                data-autofocus
              />
            </Field>
          ) : (
            <>
              <div className="grid grid-cols-1 gap-3.5 sm:grid-cols-2">
                <Field label="Paid on" hint="optional" htmlFor="expense-paid-on">
                  <TextInput
                    id="expense-paid-on"
                    name="paid_on"
                    type="date"
                    defaultValue={today()}
                  />
                </Field>
                <Field label="Account" hint="optional" htmlFor="expense-account">
                  <Select id="expense-account" name="account_id" defaultValue="">
                    <option value="">Not recorded</option>
                    {accounts.map((account) => (
                      <option key={account.id} value={account.id}>
                        {account.name}
                      </option>
                    ))}
                  </Select>
                </Field>
              </div>
              <p className="text-[13.5px] text-muted">
                <span className="font-semibold text-night">
                  {currency}
                </span>{" "}
                · the move is recorded against you and appears in the audit trail.
              </p>
            </>
          )}

          <div className="flex flex-wrap gap-2.5">
            <Button type="submit" variant={step === "reject" ? "danger" : "primary"} disabled={busy}>
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
