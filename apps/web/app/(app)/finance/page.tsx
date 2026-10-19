import type { ReactNode } from "react";

import { ButtonLink } from "@/components/form";
import { Icon } from "@/components/icons";
import { SignalRefresh } from "@/components/signal-refresh";
import {
  Badge,
  Card,
  ChipLink,
  Chips,
  EmptyState,
  Label,
  Meter,
  PageHeader,
  ProblemNotice,
  Stat,
  StatGrid,
  SWATCH,
  Table,
  Tag,
  Td,
  Th,
  cx,
  type Tone,
} from "@/components/ui";
import {
  EXPENSE_STATUSES,
  INVOICE_STATUSES,
  getCashPosition,
  getFinanceOverview,
  getReceivables,
  listBudgets,
  listExpenseCategories,
  listExpenses,
  listInvoices,
  listRecurringCosts,
  type CashPosition,
  type FinanceBudget,
  type FinanceExpense,
  type FinanceExpenseCategory,
  type FinanceInvoice,
  type FinanceOverview,
  type ReceivablesAgeing,
} from "@/lib/api/atlas";
import { ApiError } from "@/lib/api/client";
import { personDirectory, personName } from "@/lib/directory";
import { FINANCE_TOPIC } from "@/lib/realtime/topics";
import {
  formatDay,
  formatMinor,
  formatMinorCompact,
  humanise,
} from "@/lib/format";

/**
 * Finance (`GET /api/v1/finance/*`, gated by `finance.*`; blueprint §8.4). The
 * page reads the headline roll-up, the cash position, receivables ageing, one
 * page of expenses, invoices and budgets, the recurring costs and the expense
 * categories, then composes the management-finance picture. Go owns every rule
 * and amount; nothing here writes.
 *
 * Every `*_minor` field is withheld by Go unless the viewer holds
 * `finance.amount.sensitive.read`, so a missing amount is shown as restricted,
 * never as zero.
 */

export const dynamic = "force-dynamic";

/** Invoice states that are no longer collectable (never counted as overdue). */
const CLOSED_INVOICE_STATUSES = new Set(["paid", "written_off"]);

/** Ageing-bucket bar colours, oldest last (worst). */
const AGE_COLORS = [
  SWATCH.success,
  SWATCH.info,
  SWATCH.warning,
  SWATCH.orange,
  SWATCH.danger,
];

/** Expense badge tones; the shared work/people map does not know these states. */
function expenseTone(status: string): Tone {
  switch (status) {
    case "approved":
    case "paid":
    case "reimbursed":
    case "reconciled":
      return "success";
    case "submitted":
      return "info";
    case "rejected":
      return "danger";
    default:
      return "neutral";
  }
}

/** Invoice badge tones. */
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

function budgetTone(status: string): Tone {
  if (status === "approved") return "success";
  if (status === "closed") return "neutral";
  return "info";
}

/** True when an invoice is past due and still open; Go's `overdue` status also counts. */
function isInvoiceOverdue(invoice: FinanceInvoice): boolean {
  if (invoice.status === "overdue") return true;
  if (CLOSED_INVOICE_STATUSES.has(invoice.status)) return false;
  if (!invoice.due_date) return false;
  return new Date(invoice.due_date).getTime() < Date.now();
}

/** A money value Go withheld (the viewer lacks `finance.amount.sensitive.read`). */
function Restricted({ className }: { className?: string }) {
  return (
    <span className={cx("text-muted", className)} title="Restricted for your role">
      Restricted
    </span>
  );
}

/** Minor units as money, or the withheld marker — never a fabricated zero. */
function Money({
  minor,
  currency,
  className,
}: {
  minor?: number;
  currency: string;
  className?: string;
}) {
  if (minor === undefined) return <Restricted className={className} />;
  return <span className={className}>{formatMinor(minor, currency)}</span>;
}

/** A compact KPI money value, or the withheld marker when Go omitted it. */
function kpiMoney(minor: number | undefined, currency: string): ReactNode {
  if (minor === undefined) return <Restricted className="text-[20px]" />;
  return formatMinorCompact(minor, currency);
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

export default async function FinancePage({
  searchParams,
}: {
  searchParams: Promise<{
    expense_status?: string;
    category?: string;
    invoice_status?: string;
    overdue?: string;
  }>;
}) {
  const params = await searchParams;
  const expenseFilter = EXPENSE_STATUSES.find((s) => s === params.expense_status);
  const invoiceFilter = INVOICE_STATUSES.find((s) => s === params.invoice_status);
  const overdueOnly = params.overdue === "1";
  const categoryParam = params.category;

  const [
    overview,
    cash,
    receivables,
    expensesPage,
    invoicesPage,
    budgetsPage,
    recurring,
    categories,
  ] = await Promise.all([
    load(() => getFinanceOverview()),
    load(() => getCashPosition()),
    load(() => getReceivables()),
    load(() => listExpenses({ limit: 200 })),
    load(() => listInvoices({ limit: 200 })),
    load(() => listBudgets({ limit: 200 })),
    load(() => listRecurringCosts()),
    load(() => listExpenseCategories()),
  ]);

  // The overview is the page: if it cannot be read, say why and stop.
  if (overview.error || !overview.value) {
    const problem = overview.error ?? { message: "The finance overview returned no data." };
    return (
      <div className="space-y-6">
        <PageHeader
          title="Finance"
          description="Cash, what is owed, what each project earns, and where spend is going."
        />
        <ProblemNotice
          error={problem}
          title={
            overview.error?.status === 403
              ? "You do not have permission to view finance"
              : "Finance could not be loaded"
          }
        />
      </div>
    );
  }

  const summary: FinanceOverview = overview.value;
  const cashPosition: CashPosition | null = cash.value;
  const ageing: ReceivablesAgeing | null = receivables.value;
  const expenses = expensesPage.value?.items ?? [];
  const invoices = invoicesPage.value?.items ?? [];
  const budgets = budgetsPage.value?.items ?? [];
  const recurringCosts = recurring.value ?? [];
  const categoryList: FinanceExpenseCategory[] = categories.value ?? [];

  const names = await personDirectory();

  const currency = summary.currency || "KES";
  const categoryFilter = categoryList.find((c) => c.key === categoryParam)?.key;
  const categoryLabel = (key: string) =>
    categoryList.find((c) => c.key === key)?.label ?? humanise(key);

  const shownExpenses = expenses.filter((e) => {
    if (expenseFilter && e.status !== expenseFilter) return false;
    if (categoryFilter && e.category_key !== categoryFilter) return false;
    return true;
  });

  const shownInvoices = invoices.filter((i) => {
    if (invoiceFilter && i.status !== invoiceFilter) return false;
    if (overdueOnly && !isInvoiceOverdue(i)) return false;
    return true;
  });

  const overdueCount = summary.overdue_invoice_count;
  const budgetNote = `${summary.active_budget_count} active budget${
    summary.active_budget_count === 1 ? "" : "s"
  }${
    summary.budgets_over_threshold > 0
      ? ` · ${summary.budgets_over_threshold} over threshold`
      : ""
  }`;

  // Build a filter URL that preserves every other active filter.
  const filterHref = (patch: Record<string, string | undefined>): string => {
    const merged: Record<string, string | undefined> = {
      expense_status: expenseFilter,
      category: categoryFilter,
      invoice_status: invoiceFilter,
      overdue: overdueOnly ? "1" : undefined,
      ...patch,
    };
    const search = new URLSearchParams();
    for (const [key, value] of Object.entries(merged)) {
      if (value) search.set(key, value);
    }
    const qs = search.toString();
    return qs ? `/finance?${qs}` : "/finance";
  };

  const ageSegments =
    ageing?.buckets
      .map((bucket, i) => ({
        value: bucket.amount_minor ?? 0,
        color: AGE_COLORS[i % AGE_COLORS.length],
      }))
      .filter((segment) => segment.value > 0) ?? [];

  return (
    <div className="space-y-6">
      <PageHeader
        title="Finance"
        description="Cash, what is owed, what each project earns, and where spend is going."
        actions={
          <>
            <SignalRefresh topic={FINANCE_TOPIC} quiet />
            <ButtonLink href="/contracts" variant="secondary">
              <Icon name="document" />
              Contracts
            </ButtonLink>
          </>
        }
      />

      <StatGrid label="Finance overview">
        <Stat
          label="Cash position"
          value={kpiMoney(summary.cash_minor, currency)}
          icon="card"
          iconTone="violet"
          hint="Across every account"
        />
        <Stat
          label="Receivables outstanding"
          value={kpiMoney(summary.receivables_outstanding_minor, currency)}
          tone={summary.receivables_overdue_minor ? "warning" : "neutral"}
          icon="document"
          iconTone="info"
          hint={
            summary.receivables_overdue_minor === undefined ? (
              "Overdue amounts are restricted for your role"
            ) : (
              <span className="font-semibold text-[#B45309]">
                {formatMinorCompact(summary.receivables_overdue_minor, currency)} overdue
              </span>
            )
          }
        />
        <Stat
          label="Expenses (30 days)"
          value={kpiMoney(summary.expenses_30d_minor, currency)}
          icon="card"
          iconTone="orange"
          hint="Approved and recorded spend"
        />
        <Stat
          label="Pending approvals"
          value={summary.pending_expenses}
          tone={summary.pending_expenses > 0 ? "warning" : "neutral"}
          icon="clock"
          iconTone="warning"
          hint="Expenses awaiting a decision"
        />
        <Stat
          label="Overdue invoices"
          value={overdueCount}
          tone={overdueCount > 0 ? "danger" : "neutral"}
          icon="alert"
          iconTone="danger"
          hint="Past their due date"
        />
        <Stat
          label="Renewals (30 days)"
          value={summary.upcoming_renewals_30d}
          icon="calendar"
          iconTone="accent"
          hint="Recurring costs renewing soon"
        />
      </StatGrid>

      <div className="grid grid-cols-1 gap-5 xl:grid-cols-2">
        <Card
          title="Cash position"
          description={
            cashPosition
              ? `As of ${formatDay(cashPosition.as_of)}`
              : "As of the last snapshot"
          }
          padded={false}
        >
          {!cashPosition ? (
            <div className="p-[22px]">
              <EmptyState
                icon="card"
                iconTone="info"
                className="min-h-[160px]"
                title="Cash position unavailable"
              >
                The cash snapshot could not be read; it may be restricted for your role.
              </EmptyState>
            </div>
          ) : (
            <div className="px-[22px] pb-[22px] pt-2">
              <dl className="grid grid-cols-2 gap-x-5 gap-y-4 border-b border-line pb-[18px] sm:grid-cols-4">
                <div>
                  <dt>
                    <Label>Total</Label>
                  </dt>
                  <dd className="tabular mt-1 text-[19px] font-semibold text-night">
                    <Money minor={cashPosition.total_minor} currency={cashPosition.currency || currency} />
                  </dd>
                </div>
                <div>
                  <dt>
                    <Label>In (30d)</Label>
                  </dt>
                  <dd className="tabular mt-1 text-[19px] font-semibold text-night">
                    <Money minor={cashPosition.inflow_30d_minor} currency={cashPosition.currency || currency} />
                  </dd>
                </div>
                <div>
                  <dt>
                    <Label>Out (30d)</Label>
                  </dt>
                  <dd className="tabular mt-1 text-[19px] font-semibold text-night">
                    <Money minor={cashPosition.outflow_30d_minor} currency={cashPosition.currency || currency} />
                  </dd>
                </div>
                <div>
                  <dt>
                    <Label>Net (30d)</Label>
                  </dt>
                  <dd className="tabular mt-1 text-[19px] font-semibold text-night">
                    <Money minor={cashPosition.net_30d_minor} currency={cashPosition.currency || currency} />
                  </dd>
                </div>
              </dl>
              {cashPosition.by_account.length === 0 ? (
                <div className="pt-[18px]">
                  <EmptyState
                    icon="card"
                    iconTone="info"
                    className="min-h-[120px]"
                    title="No accounts yet"
                  >
                    Bank, cash and M-Pesa accounts appear here once they are set up.
                  </EmptyState>
                </div>
              ) : (
                <Table>
                  <thead>
                    <tr>
                      <Th>Account</Th>
                      <Th>Type</Th>
                      <Th className="text-right">Balance</Th>
                    </tr>
                  </thead>
                  <tbody>
                    {cashPosition.by_account.map((account) => (
                      <tr key={account.account_id}>
                        <Td className="font-medium">{account.name}</Td>
                        <Td className="text-muted">{humanise(account.account_type)}</Td>
                        <Td className="tabular text-right">
                          <Money
                            minor={account.balance_minor}
                            currency={cashPosition.currency || currency}
                          />
                        </Td>
                      </tr>
                    ))}
                  </tbody>
                </Table>
              )}
            </div>
          )}
        </Card>

        <Card
          title="Receivables ageing"
          description={ageing ? `As of ${formatDay(ageing.as_of)}` : "As of the last snapshot"}
        >
          {!ageing ? (
            <EmptyState
              icon="document"
              iconTone="info"
              className="min-h-[160px]"
              title="Receivables unavailable"
            >
              The receivables roll-up could not be read; it may be restricted for your role.
            </EmptyState>
          ) : (
            <div className="space-y-4">
              {ageSegments.length > 0 && (
                <Meter label="Outstanding by ageing bucket" segments={ageSegments} />
              )}
              <Table>
                <thead>
                  <tr>
                    <Th>Ageing</Th>
                    <Th className="text-right">Invoices</Th>
                    <Th className="text-right">Outstanding</Th>
                  </tr>
                </thead>
                <tbody>
                  {ageing.buckets.map((bucket) => (
                    <tr key={bucket.key}>
                      <Td className="font-medium">{bucket.label}</Td>
                      <Td className="tabular text-right text-muted">{bucket.count}</Td>
                      <Td className="tabular text-right">
                        <Money
                          minor={bucket.amount_minor}
                          currency={ageing.currency || currency}
                        />
                      </Td>
                    </tr>
                  ))}
                </tbody>
              </Table>
              <div className="flex items-center justify-between border-t border-dashed border-line-strong pt-3.5">
                <span className="text-[13.5px] font-semibold text-night">Total outstanding</span>
                <span className="tabular font-semibold">
                  <Money
                    minor={ageing.total_outstanding_minor}
                    currency={ageing.currency || currency}
                  />
                </span>
              </div>
            </div>
          )}
        </Card>
      </div>

      <Card
        title="Expenses"
        description="Claims from the field, by category and status."
        padded={false}
      >
        <div className="flex flex-col gap-3 px-[22px] pb-3.5 pt-1">
          <Chips label="Filter expenses by status">
            <ChipLink
              href={filterHref({ expense_status: undefined })}
              pressed={!expenseFilter}
              count={expenses.length}
            >
              All
            </ChipLink>
            {EXPENSE_STATUSES.map((s) => (
              <ChipLink
                key={s}
                href={filterHref({ expense_status: s })}
                pressed={expenseFilter === s}
                count={expenses.filter((e) => e.status === s).length}
              >
                {humanise(s)}
              </ChipLink>
            ))}
          </Chips>
          {categoryList.length > 0 && (
            <Chips label="Filter expenses by category">
              <ChipLink
                href={filterHref({ category: undefined })}
                pressed={!categoryFilter}
                count={expenses.length}
              >
                All categories
              </ChipLink>
              {categoryList.map((c) => (
                <ChipLink
                  key={c.key}
                  href={filterHref({ category: c.key })}
                  pressed={categoryFilter === c.key}
                  count={expenses.filter((e) => e.category_key === c.key).length}
                >
                  {c.label}
                </ChipLink>
              ))}
            </Chips>
          )}
        </div>
        {shownExpenses.length === 0 ? (
          <div className="px-[22px] pb-[22px] pt-1">
            <EmptyState
              icon="check"
              className="min-h-[220px]"
              title={expenseFilter ? `No ${humanise(expenseFilter).toLowerCase()} expenses` : "No expenses yet"}
            >
              {expenseFilter || categoryFilter
                ? "Expenses move here as they are submitted and decided."
                : "Expenses recorded in the field and from daily reports land here."}
            </EmptyState>
          </div>
        ) : (
          <Table>
            <thead>
              <tr>
                <Th>Expense</Th>
                <Th>Category</Th>
                <Th>Claimant</Th>
                <Th>Status</Th>
                <Th>Date</Th>
                <Th className="text-right">Amount</Th>
              </tr>
            </thead>
            <tbody>
              {shownExpenses.map((expense) => (
                <ExpenseRow
                  key={expense.id}
                  expense={expense}
                  currency={currency}
                  category={categoryLabel(expense.category_key)}
                  claimant={personName(names, expense.claimant_person_id)}
                />
              ))}
            </tbody>
          </Table>
        )}
      </Card>

      <Card
        title="Invoices"
        description="Raised from contract milestones and schedules."
        padded={false}
      >
        <div className="flex flex-wrap items-center justify-between gap-x-5 gap-y-3 px-[22px] pb-3.5 pt-1">
          <Chips label="Filter invoices by status">
            <ChipLink
              href={filterHref({ invoice_status: undefined })}
              pressed={!invoiceFilter}
              count={invoices.length}
            >
              All
            </ChipLink>
            {INVOICE_STATUSES.map((s) => (
              <ChipLink
                key={s}
                href={filterHref({ invoice_status: s })}
                pressed={invoiceFilter === s}
                count={invoices.filter((i) => i.status === s).length}
              >
                {humanise(s)}
              </ChipLink>
            ))}
          </Chips>
          <ChipLink
            href={filterHref({ overdue: overdueOnly ? undefined : "1" })}
            pressed={overdueOnly}
          >
            Overdue only
          </ChipLink>
        </div>
        {shownInvoices.length === 0 ? (
          <div className="px-[22px] pb-[22px] pt-1">
            <EmptyState
              icon="document"
              iconTone="info"
              className="min-h-[220px]"
              title={
                invoiceFilter
                  ? `No ${humanise(invoiceFilter).toLowerCase()} invoices`
                  : overdueOnly
                    ? "No overdue invoices"
                    : "No invoices yet"
              }
            >
              {invoiceFilter || overdueOnly
                ? "Invoices move here as they are issued, paid and chased."
                : "A milestone flagged for invoicing creates a draft invoice here."}
            </EmptyState>
          </div>
        ) : (
          <Table>
            <thead>
              <tr>
                <Th>Invoice</Th>
                <Th>Status</Th>
                <Th>Issued</Th>
                <Th>Due</Th>
                <Th className="text-right">Total</Th>
                <Th className="text-right">Outstanding</Th>
              </tr>
            </thead>
            <tbody>
              {shownInvoices.map((invoice) => (
                <InvoiceRow
                  key={invoice.id}
                  invoice={invoice}
                  currency={currency}
                />
              ))}
            </tbody>
          </Table>
        )}
      </Card>

      <Card
        title="Budgets"
        description={budgetNote}
        padded={false}
      >
        {budgets.length === 0 ? (
          <div className="p-[22px]">
            <EmptyState
              icon="pie"
              iconTone="info"
              className="min-h-[200px]"
              title="No budgets yet"
            >
              Period, department and project budgets appear here once they are set.
            </EmptyState>
          </div>
        ) : (
          <Table>
            <thead>
              <tr>
                <Th>Budget</Th>
                <Th>Period</Th>
                <Th>Category</Th>
                <Th className="text-right">Amount</Th>
                <Th className="text-right">Actual</Th>
                <Th className="text-right">Utilisation</Th>
              </tr>
            </thead>
            <tbody>
              {budgets.map((budget) => (
                <BudgetRow
                  key={budget.id}
                  budget={budget}
                  currency={currency}
                  category={budget.category_key ? categoryLabel(budget.category_key) : "—"}
                />
              ))}
            </tbody>
          </Table>
        )}
      </Card>

      <Card
        title="Recurring costs"
        description="Standing vendor commitments and their next renewal."
        padded={false}
      >
        {recurringCosts.length === 0 ? (
          <div className="p-[22px]">
            <EmptyState
              icon="clock"
              iconTone="warning"
              className="min-h-[200px]"
              title="No recurring costs yet"
            >
              Hosting, domains and subscriptions appear here once they are recorded.
            </EmptyState>
          </div>
        ) : (
          <Table>
            <thead>
              <tr>
                <Th>Vendor</Th>
                <Th>Cycle</Th>
                <Th className="text-right">Amount</Th>
                <Th className="text-right">Monthly equiv.</Th>
                <Th>Next renewal</Th>
                <Th>Owner</Th>
                <Th>State</Th>
              </tr>
            </thead>
            <tbody>
              {recurringCosts.map((cost) => (
                <tr key={cost.id}>
                  <Td>
                    <div className="flex flex-col items-start gap-1">
                      <span className="font-medium">{cost.vendor}</span>
                      {cost.description && (
                        <span className="text-[13px] text-muted">{cost.description}</span>
                      )}
                    </div>
                  </Td>
                  <Td className="text-muted">{humanise(cost.cycle)}</Td>
                  <Td className="tabular text-right">
                    <Money minor={cost.amount_minor} currency={cost.currency || currency} />
                  </Td>
                  <Td className="tabular text-right">
                    <Money
                      minor={cost.monthly_equivalent_minor}
                      currency={cost.currency || currency}
                    />
                  </Td>
                  <Td className="text-muted">{formatDay(cost.next_renewal_on)}</Td>
                  <Td className="text-muted">
                    {cost.owner_person_id ? personName(names, cost.owner_person_id) : "—"}
                  </Td>
                  <Td>
                    <Badge tone={cost.active ? "success" : "neutral"}>
                      {cost.active ? "Active" : "Inactive"}
                    </Badge>
                  </Td>
                </tr>
              ))}
            </tbody>
          </Table>
        )}
      </Card>
    </div>
  );
}

function ExpenseRow({
  expense,
  currency,
  category,
  claimant,
}: {
  expense: FinanceExpense;
  currency: string;
  category: string;
  claimant: string;
}) {
  return (
    <tr>
      <Td>
        <div className="flex flex-col items-start gap-1">
          <Tag>{expense.ref}</Tag>
          {expense.vendor && (
            <span className="text-[13px] text-muted">{expense.vendor}</span>
          )}
        </div>
      </Td>
      <Td className="text-muted">{category}</Td>
      <Td>{claimant}</Td>
      <Td>
        <Badge tone={expenseTone(expense.status)} dot>
          {humanise(expense.status)}
        </Badge>
      </Td>
      <Td className="text-muted">{formatDay(expense.spent_on)}</Td>
      <Td className="tabular text-right">
        <Money minor={expense.amount_minor} currency={expense.currency || currency} />
      </Td>
    </tr>
  );
}

function InvoiceRow({
  invoice,
  currency,
}: {
  invoice: FinanceInvoice;
  currency: string;
}) {
  const overdue = isInvoiceOverdue(invoice);
  return (
    <tr>
      <Td>
        <div className="flex flex-col items-start gap-1">
          <Tag>{invoice.ref}</Tag>
          <span className="text-[13px] text-muted">
            {invoice.organization_name ?? `Org ${invoice.organization_id.slice(0, 8)}`}
          </span>
        </div>
      </Td>
      <Td>
        <div className="flex flex-wrap items-center gap-1.5">
          <Badge tone={invoiceTone(invoice.status)} dot>
            {humanise(invoice.status)}
          </Badge>
          {overdue && invoice.status !== "overdue" && <Badge tone="danger">Overdue</Badge>}
        </div>
      </Td>
      <Td className="text-muted">{formatDay(invoice.issue_date)}</Td>
      <Td className="text-muted">{formatDay(invoice.due_date)}</Td>
      <Td className="tabular text-right">
        <Money minor={invoice.total_minor} currency={invoice.currency || currency} />
      </Td>
      <Td className="tabular text-right">
        <Money minor={invoice.outstanding_minor} currency={invoice.currency || currency} />
      </Td>
    </tr>
  );
}

function BudgetRow({
  budget,
  currency,
  category,
}: {
  budget: FinanceBudget;
  currency: string;
  category: string;
}) {
  return (
    <tr>
      <Td>
        <div className="flex flex-col items-start gap-1">
          <span className="font-medium">{humanise(budget.scope)}</span>
          <div className="flex flex-wrap items-center gap-1.5">
            {budget.scope_ref && <Tag>{budget.scope_ref}</Tag>}
            <Badge tone={budgetTone(budget.status)}>{humanise(budget.status)}</Badge>
          </div>
        </div>
      </Td>
      <Td className="text-muted">
        {formatDay(budget.period_start)} → {formatDay(budget.period_end)}
      </Td>
      <Td className="text-muted">{category}</Td>
      <Td className="tabular text-right">
        <Money minor={budget.amount_minor} currency={budget.currency || currency} />
      </Td>
      <Td className="tabular text-right">
        <Money minor={budget.actual_minor} currency={budget.currency || currency} />
      </Td>
      <Td className="text-right">
        <BudgetUsage budget={budget} currency={currency} />
      </Td>
    </tr>
  );
}

/** Utilisation as a percent badge: amber at 90%, red once over budget. */
function BudgetUsage({ budget, currency }: { budget: FinanceBudget; currency: string }) {
  const { amount_minor, actual_minor, variance_minor } = budget;
  if (amount_minor === undefined || actual_minor === undefined) {
    return <Restricted className="text-[13.5px]" />;
  }
  if (amount_minor <= 0) {
    return <span className="text-muted">—</span>;
  }
  const ratio = actual_minor / amount_minor;
  const tone: Tone = ratio >= 1 ? "danger" : ratio >= 0.9 ? "warning" : "success";
  const rowCurrency = budget.currency || currency;
  return (
    <div className="flex flex-col items-end gap-1">
      <Badge tone={tone}>{Math.round(ratio * 100)}%</Badge>
      {variance_minor !== undefined && (
        <span className="tabular text-[12.5px] text-muted">
          {formatMinor(variance_minor, rowCurrency)}
        </span>
      )}
    </div>
  );
}
