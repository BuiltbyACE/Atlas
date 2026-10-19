"use client";

import { useRouter } from "next/navigation";
import { useState, type FormEvent } from "react";

import { Drawer } from "@/components/drawer";
import { Button, Field, Select, TextInput } from "@/components/form";
import { Icon } from "@/components/icons";
import { PersonSelect } from "@/components/person-select";
import { useToast } from "@/components/toast";
import { cx, ProblemNotice } from "@/components/ui";
import {
  BUDGET_SCOPES,
  approveBudget,
  closeBudget,
  createBudget,
  updateBudget,
  type FinanceBudget,
  type FinanceBudgetBody,
  type FinanceExpenseCategory,
} from "@/lib/api/atlas";
import type { PersonOption } from "@/lib/directory";
import { dateInput, formatMinorInput, humanise, parseMinor, today } from "@/lib/format";
import { useMutation } from "@/lib/use-mutation";

import type { FinanceCapability } from "./permissions";

/**
 * Budgets (`/api/v1/finance/budgets`, §8.4). A ceiling for a scope and period;
 * a draft can be corrected, the CEOs approve it, and it is closed when the
 * period ends. Go owns the spend matching, so the fields below only describe the
 * ceiling, never the actuals.
 */

/** A text field, trimmed; `""` when the field is absent. */
function field(data: FormData, key: string): string {
  return String(data.get(key) ?? "").trim();
}

/** Build the create/update body from the drawer's form. */
function budgetBody(data: FormData, fallbackCurrency: string): FinanceBudgetBody {
  const currency = field(data, "currency") || fallbackCurrency;
  const body: FinanceBudgetBody = {
    scope: field(data, "scope"),
    period_start: field(data, "period_start"),
    period_end: field(data, "period_end"),
    amount_minor: parseMinor(field(data, "amount"), currency),
    currency,
  };
  const ref = field(data, "scope_ref");
  if (ref) body.scope_ref = ref;
  const category = field(data, "category_key");
  if (category) body.category_key = category;
  const owner = field(data, "owner_person_id");
  if (owner) body.owner_person_id = owner;
  return body;
}

/**
 * The fields shared by "new budget" and "edit budget". A project scope carries a
 * project id in `scope_ref` (that is how Go matches spend), so it is picked from
 * the real projects; the other scopes take a free-text reference. The amount
 * hint follows the currency field.
 */
function BudgetFields({
  budget,
  categories,
  people,
  projects,
  me,
  defaultCurrency,
}: {
  budget?: FinanceBudget;
  categories: FinanceExpenseCategory[];
  people: PersonOption[];
  projects: { id: string; name: string }[];
  me?: string;
  defaultCurrency: string;
}) {
  const [scope, setScope] = useState<string>(budget?.scope ?? BUDGET_SCOPES[0]);
  const [currency, setCurrency] = useState(budget?.currency || defaultCurrency);

  const ref = budget?.scope_ref ?? "";
  const projectKnown = !ref || projects.some((project) => project.id === ref);
  const categoryKey = budget?.category_key;
  const categoryKnown = !categoryKey || categories.some((category) => category.key === categoryKey);

  return (
    <>
      <div className="grid grid-cols-1 gap-3.5 sm:grid-cols-2">
        <Field label="Scope" htmlFor="budget-scope">
          <Select
            id="budget-scope"
            name="scope"
            value={scope}
            onChange={(event) => setScope(event.target.value)}
          >
            {BUDGET_SCOPES.map((option) => (
              <option key={option} value={option}>
                {humanise(option)}
              </option>
            ))}
          </Select>
        </Field>
        {scope === "project" ? (
          <Field label="Project" hint="its spend counts against this" htmlFor="budget-scope-ref">
            <Select id="budget-scope-ref" name="scope_ref" defaultValue={ref}>
              <option value="">No project chosen</option>
              {!projectKnown && ref && <option value={ref}>Project {ref.slice(0, 8)}</option>}
              {projects.map((project) => (
                <option key={project.id} value={project.id}>
                  {project.name}
                </option>
              ))}
            </Select>
          </Field>
        ) : (
          <Field label="Reference" hint="optional" htmlFor="budget-scope-ref">
            <TextInput
              id="budget-scope-ref"
              name="scope_ref"
              defaultValue={ref}
              placeholder="Department, unit or period label"
            />
          </Field>
        )}
        <Field label="Period start" htmlFor="budget-start">
          <TextInput
            id="budget-start"
            name="period_start"
            type="date"
            required
            defaultValue={budget ? dateInput(budget.period_start) : today()}
          />
        </Field>
        <Field label="Period end" htmlFor="budget-end">
          <TextInput
            id="budget-end"
            name="period_end"
            type="date"
            required
            defaultValue={budget ? dateInput(budget.period_end) : today()}
          />
        </Field>
        <Field label="Category" hint="optional" htmlFor="budget-category">
          <Select id="budget-category" name="category_key" defaultValue={categoryKey ?? ""}>
            <option value="">All categories</option>
            {!categoryKnown && categoryKey && (
              <option value={categoryKey}>{humanise(categoryKey)}</option>
            )}
            {categories.map((category) => (
              <option key={category.key} value={category.key}>
                {category.label}
              </option>
            ))}
          </Select>
        </Field>
        <Field label="Amount" hint={currency} htmlFor="budget-amount">
          <TextInput
            id="budget-amount"
            name="amount"
            required
            inputMode="decimal"
            placeholder="0.00"
            defaultValue={budget ? formatMinorInput(budget.amount_minor, budget.currency) : ""}
          />
        </Field>
        <Field label="Currency" hint="ISO code" htmlFor="budget-currency">
          <TextInput
            id="budget-currency"
            name="currency"
            value={currency}
            onChange={(event) => setCurrency(event.target.value.toUpperCase())}
          />
        </Field>
      </div>

      <Field label="Owner" hint="optional" htmlFor="budget-owner">
        <PersonSelect
          id="budget-owner"
          name="owner_person_id"
          people={people}
          me={me}
          defaultValue={budget?.owner_person_id ?? ""}
        />
      </Field>
    </>
  );
}

/**
 * "New budget" (`POST /api/v1/finance/budgets`, `finance.budget.create`): a
 * button that opens the budget form in a drawer.
 */
export function NewBudgetButton({
  categories,
  people,
  projects,
  me,
  currency,
  capabilities,
  variant = "primary",
}: {
  categories: FinanceExpenseCategory[];
  people: PersonOption[];
  projects: { id: string; name: string }[];
  me?: string;
  currency: string;
  capabilities: FinanceCapability[];
  variant?: "primary" | "secondary";
}) {
  const router = useRouter();
  const notify = useToast();
  const { busy, problem, run, reset } = useMutation();
  const [open, setOpen] = useState(false);

  if (!capabilities.includes("finance.budget.create")) return null;

  function close() {
    setOpen(false);
    reset();
  }

  async function onSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const created = await run(() =>
      createBudget(budgetBody(new FormData(event.currentTarget), currency)),
    );
    if (created) {
      setOpen(false);
      notify("Budget created");
      router.refresh();
    }
  }

  return (
    <>
      <Button variant={variant} size={variant === "secondary" ? "sm" : "md"} onClick={() => setOpen(true)}>
        <Icon name="plus" className={variant === "secondary" ? "h-4 w-4" : undefined} />
        New budget
      </Button>
      <Drawer
        open={open}
        onClose={close}
        title="New budget"
        description="A ceiling for a scope and period. Go matches the spend; the actuals appear once it is approved."
      >
        <form onSubmit={onSubmit} className="flex flex-col gap-4 px-6 py-[22px]">
          {problem && <ProblemNotice error={problem} title="The budget wasn’t saved" />}

          <BudgetFields
            categories={categories}
            people={people}
            projects={projects}
            me={me}
            defaultCurrency={currency}
          />

          <div className="flex flex-wrap gap-2.5">
            <Button type="submit" disabled={busy}>
              {busy ? "Saving…" : "Create budget"}
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
 * The actions for one budget row: approve it, correct a draft, or close it when
 * the period ends. Approve and close are their own routes; the edit is `PUT`
 * under `finance.budget.update`, all guarded by `If-Match`.
 */
export function BudgetActions({
  budget,
  categories,
  people,
  projects,
  me,
  currency,
  capabilities,
}: {
  budget: FinanceBudget;
  categories: FinanceExpenseCategory[];
  people: PersonOption[];
  projects: { id: string; name: string }[];
  me?: string;
  currency: string;
  capabilities: FinanceCapability[];
}) {
  const router = useRouter();
  const notify = useToast();
  const { busy, problem, run, reset } = useMutation();
  const [open, setOpen] = useState(false);

  function close() {
    setOpen(false);
    reset();
  }

  async function act(action: () => Promise<unknown>, message: string) {
    const done = await run(action);
    if (done) {
      notify(message);
      router.refresh();
    }
  }

  async function onEdit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const updated = await run(() =>
      updateBudget(
        budget.id,
        budgetBody(new FormData(event.currentTarget), currency),
        budget.version,
      ),
    );
    if (updated) {
      close();
      notify("Budget saved");
      router.refresh();
    }
  }

  const canApprove =
    capabilities.includes("finance.budget.approve") && budget.status === "draft";
  // Only a draft is still open to correction; once approved it is a commitment.
  const canEdit = capabilities.includes("finance.budget.update") && budget.status === "draft";
  const canClose = capabilities.includes("finance.budget.close") && budget.status === "approved";

  const nothing = !canApprove && !canEdit && !canClose;

  return (
    <div className="flex flex-col items-end gap-2">
      <div className="flex flex-wrap items-center justify-end gap-1.5">
        {canApprove && (
          <Button
            size="sm"
            disabled={busy}
            onClick={() => act(() => approveBudget(budget.id, budget.version), "Budget approved")}
          >
            Approve
          </Button>
        )}
        {canEdit && (
          <Button size="sm" variant="secondary" disabled={busy} onClick={() => setOpen(true)}>
            Edit
          </Button>
        )}
        {canClose && (
          <Button
            size="sm"
            variant="secondary"
            disabled={busy}
            onClick={() => act(() => closeBudget(budget.id, budget.version), "Budget closed")}
          >
            Close
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
        title="Edit budget"
        description={`${humanise(budget.scope)} · ${formatDayRange(budget)}`}
      >
        <form onSubmit={onEdit} className="flex flex-col gap-4 px-6 py-[22px]">
          {problem && <ProblemNotice error={problem} title="The change wasn’t saved" />}

          <BudgetFields
            budget={budget}
            categories={categories}
            people={people}
            projects={projects}
            me={me}
            defaultCurrency={currency}
          />

          <div className="flex flex-wrap gap-2.5">
            <Button type="submit" disabled={busy}>
              {busy ? "Saving…" : "Save budget"}
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

/** `1 Jan 2026 → 31 Mar 2026` for the edit drawer's subtitle. */
function formatDayRange(budget: FinanceBudget): string {
  const start = dateInput(budget.period_start);
  const end = dateInput(budget.period_end);
  return `${start} → ${end}`;
}
