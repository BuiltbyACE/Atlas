"use client";

import { useRouter, useSearchParams } from "next/navigation";
import { useState, useEffect } from "react";

import { Button, Field, Select, TextInput, TextArea } from "@/components/form";
import { Icon } from "@/components/icons";
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
import { useToast } from "@/components/toast";
import {
  createExpense,
  getExpense,
  listExpenseCategories,
  listExpenses,
  submitExpense,
  type ExpenseStatus,
  type FinanceExpense,
} from "@/lib/api/atlas";
import { ApiError } from "@/lib/api/client";
import { MutationProblem } from "@/lib/use-mutation";
import { formatDay, formatMinor, humanise } from "@/lib/format";
import { useMutation } from "@/lib/use-mutation";

/**
 * My Expenses (§8.4). Employee-facing expense claims: list your own expenses,
 * create new ones, and submit drafts for approval. Amounts are shown only when
 * the viewer holds `finance.amount.sensitive.read`; otherwise they appear as
 * restricted.
 */
export const dynamic = "force-dynamic";

export default function MyExpensesPage() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const [expenses, setExpenses] = useState<FinanceExpense[]>([]);
  const [error, setError] = useState<ApiError | null>(null);
  const [editingId, setEditingId] = useState<string | null>(null);
  const [creating, setCreating] = useState(false);

  const statusFilter = searchParams.get("status") ?? "";

  useEffect(() => {
    const query: Record<string, string> = { limit: "200" };
    if (statusFilter) query.status = statusFilter;
    listExpenses(query).then(
      (page) => setExpenses(page.items.filter((e) => e.claimant_person_id)),
      (err) => {
        if (err instanceof ApiError) setError(err);
        else throw err;
      },
    );
  }, [statusFilter]);

  const { busy, problem, run, reset } = useMutation();

  async function onSubmitExpense(body: {
    category_key: string;
    amount_minor: number;
    currency: string;
    spent_on: string;
    vendor?: string;
    payment_method?: string;
    notes?: string;
  }) {
    const created = await run(() => createExpense(body));
    if (created) {
      reset();
      router.refresh();
    }
  }

  async function onSubmit(id: string, version: number) {
    const submitted = await run(() => submitExpense(id, version));
    if (submitted) {
      reset();
      router.refresh();
    }
  }

  const totalAmount = expenses.reduce(
    (sum, e) => sum + (e.amount_minor ?? 0),
    0,
  );
  const pendingAmount = expenses
    .filter((e) => e.status === "submitted")
    .reduce((sum, e) => sum + (e.amount_minor ?? 0), 0);

  return (
    <div className="space-y-6">
      <PageHeader
        title="My Expenses"
        description="Your expense claims: draft, submit, and track reimbursement."
        actions={
          <>
            {creating ? null : (
              <Button
                type="button"
                variant="primary"
                size="sm"
                onClick={() => setCreating(true)}
              >
                <Icon name="plus" className="h-4 w-4" />
                New expense
              </Button>
            )}
          </>
        }
      />

      {error && (
        <ProblemNotice
          error={error}
          title={
            error.status === 403
              ? "You do not have permission to view expenses"
              : "Expenses could not be loaded"
          }
        />
      )}

      <StatGrid label="My expense claims">
        <Stat
          label="Total claimed"
          value={formatMinor(totalAmount)}
          icon="card"
          iconTone="info"
          hint="All expenses"
        />
        <Stat
          label="Pending approval"
          value={formatMinor(pendingAmount)}
          icon="clock"
          iconTone="warning"
          hint={`${expenses.filter((e) => e.status === "submitted").length} submitted`}
        />
      </StatGrid>

      <div className="flex flex-wrap items-center justify-between gap-x-5 gap-y-3">
        <div className="flex flex-wrap gap-2">
          {["draft", "submitted", "approved", "paid", "rejected"].map((s) => (
            <Button
              key={s}
              type="button"
              variant="secondary"
              size="sm"
              onClick={() => {
                const qs = new URLSearchParams(searchParams);
                qs.set("status", s);
                router.push(`/finance/my-expenses?${qs.toString()}`);
              }}
            >
              {humanise(s)}
            </Button>
          ))}
          <Button
            type="button"
            variant="secondary"
            size="sm"
            onClick={() => {
              const qs = new URLSearchParams(searchParams);
              qs.delete("status");
              router.push(`/finance/my-expenses?${qs.toString()}`);
            }}
          >
            All
          </Button>
        </div>
      </div>

      {creating || editingId ? (
        <ExpenseForm
          initial={
            editingId
              ? expenses.find((e) => e.id === editingId) ?? undefined
              : undefined
          }
          onSave={onSubmitExpense}
          onCancel={() => {
            setEditingId(null);
            setCreating(false);
          }}
          busy={busy}
          problem={problem}
        />
      ) : (
        <Card padded={false}>
          {expenses.length === 0 ? (
            <div className="p-[22px]">
              <EmptyState
                icon="card"
                iconTone="info"
                className="min-h-[220px]"
                title="No expenses yet"
              >
                Create your first expense claim to get started.
              </EmptyState>
            </div>
          ) : (
            <Table>
              <thead>
                <tr>
                  <Th>Ref</Th>
                  <Th>Category</Th>
                  <Th className="text-right">Amount</Th>
                  <Th>Date</Th>
                  <Th>Status</Th>
                  <Th className="text-right">Actions</Th>
                </tr>
              </thead>
              <tbody>
                {expenses.map((e) => (
                  <tr key={e.id}>
                    <Td className="tabular text-muted font-mono text-[13px]">
                      {e.ref}
                    </Td>
                    <Td className="tabular text-muted">
                      {e.category_key}
                    </Td>
                    <Td className="tabular text-right">
                      {e.amount_minor !== undefined
                        ? formatMinor(e.amount_minor, e.currency)
                        : <span className="text-muted">Restricted</span>}
                    </Td>
                    <Td className="tabular text-muted">
                      {formatDay(e.spent_on)}
                    </Td>
                    <Td>
                      <Badge
                        tone={
                          {
                            draft: "neutral",
                            submitted: "info",
                            approved: "success",
                            paid: "success",
                            reimbursed: "success",
                            rejected: "danger",
                            reconciled: "success",
                          }[e.status as ExpenseStatus] as import("@/components/ui").Tone
                        }
                      >
                        {humanise(e.status)}
                      </Badge>
                    </Td>
                    <Td className="tabular text-right">
                      {e.status === "draft" ? (
                        <>
                          <Button
                            type="button"
                            variant="secondary"
                            size="sm"
                            onClick={() => setEditingId(e.id)}
                          >
                            Edit
                          </Button>
                          <Button
                            type="button"
                            variant="primary"
                            size="sm"
                            onClick={() => onSubmit(e.id, e.version)}
                          >
                            Submit
                          </Button>
                        </>
                      ) : (
                        <Button
                          type="button"
                          variant="ghost"
                          size="sm"
                          disabled
                        >
                          View
                        </Button>
                      )}
                    </Td>
                  </tr>
                ))}
              </tbody>
            </Table>
          )}
        </Card>
      )}
    </div>
  );
}

function ExpenseForm({
  initial,
  onSave,
  onCancel,
  busy,
  problem,
}: {
  initial?: FinanceExpense;
  onSave: (body: {
    category_key: string;
    amount_minor: number;
    currency: string;
    spent_on: string;
    vendor?: string;
    payment_method?: string;
    notes?: string;
  }) => void;
  onCancel: () => void;
  busy: boolean;
  problem: MutationProblem | null;
}) {
  const [categoryKey, setCategoryKey] = useState(initial?.category_key ?? "");
  const [amountMinor, setAmountMinor] = useState(
    initial?.amount_minor?.toString() ?? "",
  );
  const [currency, setCurrency] = useState(initial?.currency ?? "KES");
  const [spentOn, setSpentOn] = useState(initial?.spent_on ?? "");
  const [vendor, setVendor] = useState(initial?.vendor ?? "");
  const [paymentMethod, setPaymentMethod] = useState(
    initial?.payment_method ?? "",
  );
  const [notes, setNotes] = useState(initial?.notes ?? "");
  const [categories, setCategories] = useState<{ key: string; label: string }[]>(
    [],
  );

  useEffect(() => {
    listExpenseCategories().then((cs) =>
      setCategories(cs.map((c) => ({ key: c.key, label: c.label }))),
    );
  }, []);

  async function onSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    onSave({
      category_key: categoryKey,
      amount_minor: parseInt(amountMinor, 10) || 0,
      currency,
      spent_on: spentOn,
      vendor: vendor || undefined,
      payment_method: paymentMethod || undefined,
      notes: notes || undefined,
    });
  }

  return (
    <Card title="New expense" description="Claim money you spent on behalf of the company.">
      <form onSubmit={onSubmit} className="flex flex-col gap-4 px-6 py-[22px]">
        {problem && (
          <ProblemNotice error={problem} title="The expense wasn't created" />
        )}
        <Field label="Category" htmlFor="expense-category">
          <Select
            id="expense-category"
            name="category_key"
            value={categoryKey}
            onChange={(e) => setCategoryKey(e.target.value)}
            required
            data-autofocus
          >
            <option value="">Select a category</option>
            {categories.map((c) => (
              <option key={c.key} value={c.key}>
                {c.label}
              </option>
            ))}
          </Select>
        </Field>
        <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
          <Field label="Amount" htmlFor="expense-amount">
            <TextInput
              id="expense-amount"
              name="amount_minor"
              type="number"
              min="1"
              value={amountMinor}
              onChange={(e) => setAmountMinor(e.target.value)}
              placeholder="0"
            />
            <span className="text-[11.5px] text-muted">Minor units (e.g. 1500 = KES 15.00)</span>
          </Field>
          <Field label="Currency" htmlFor="expense-currency">
            <Select
              id="expense-currency"
              name="currency"
              value={currency}
              onChange={(e) => setCurrency(e.target.value)}
            >
              <option value="KES">KES</option>
              <option value="USD">USD</option>
              <option value="EUR">EUR</option>
            </Select>
          </Field>
        </div>
        <Field label="Date spent" htmlFor="expense-date">
          <TextInput
            id="expense-date"
            name="spent_on"
            type="date"
            value={spentOn}
            onChange={(e) => setSpentOn(e.target.value)}
          />
        </Field>
        <Field label="Vendor" htmlFor="expense-vendor">
          <TextInput
            id="expense-vendor"
            name="vendor"
            value={vendor}
            onChange={(e) => setVendor(e.target.value)}
            placeholder="e.g. Safaricom, Jumia"
          />
        </Field>
        <Field label="Payment method" htmlFor="expense-payment">
          <TextInput
            id="expense-payment"
            name="payment_method"
            value={paymentMethod}
            onChange={(e) => setPaymentMethod(e.target.value)}
            placeholder="e.g. M-Pesa, card, cash"
          />
        </Field>
        <Field label="Notes" htmlFor="expense-notes">
          <TextArea
            id="expense-notes"
            name="notes"
            value={notes}
            onChange={(e) => setNotes(e.target.value)}
            rows={3}
            placeholder="Business purpose of this expense"
          />
        </Field>
        <div className="flex justify-end gap-2 pt-4">
          <Button type="button" variant="secondary" onClick={onCancel}>
            Cancel
          </Button>
          <Button type="submit" variant="primary" disabled={busy}>
            {busy ? "Creating…" : "Create expense"}
          </Button>
        </div>
      </form>
    </Card>
  );
}
