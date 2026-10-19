/**
 * Atlas web - the finance write capabilities the UI offers (blueprint §8.4).
 *
 * Go owns authorisation: every finance route declares the permission it needs
 * (§6.2) and refuses a caller without it. This module only decides which
 * controls to *show*, because Atlas' iron rule is that hiding is never the
 * control - a hidden button is a convenience, not a guard.
 *
 * The web app has no permission list yet: `GET /api/v1/auth/me` returns `roles`
 * and nothing finer (FRONTEND.md §6, "a permissions list is on the roadmap").
 * Each capability below is therefore named after the action it performs, and
 * the roles that hold it are read off the role table in §3.2. When Go exposes a
 * real permission list for the caller, replace `financeCapabilities` with it and
 * delete the role map.
 *
 * Finance is carried by both CEOs because the company has no finance officer
 * yet (§3.2), and every one of these actions is written to the audit trail with
 * the actor's name (§6.6) - the footprint is the control, so the UI decides only
 * what to show, never what Go may permit.
 */

/** Every finance write capability the finance screens offer. */
export const FINANCE_CAPABILITIES = [
  "finance.expense.create",
  "finance.expense.submit",
  "finance.expense.approve",
  "finance.expense.settle",
  "finance.expense.reconcile",
  "finance.invoice.create",
  "finance.invoice.update",
  "finance.invoice.approve",
  "finance.invoice.issue",
  "finance.invoice.dispute",
  "finance.payment.create",
  "finance.payment.allocate",
  "finance.payment.reconcile",
  "finance.budget.create",
  "finance.budget.update",
  "finance.budget.approve",
  "finance.budget.close",
  "finance.account.create",
  "finance.account.manage",
  "finance.recurring_cost.create",
  "finance.recurring_cost.update",
] as const;

export type FinanceCapability = (typeof FINANCE_CAPABILITIES)[number];

/** Roles from §3.2, spelled as Go stores them. */
type Role =
  | "ceo_commercial"
  | "ceo_internal"
  | "finance_officer"
  | "sales_rep"
  | "project_lead"
  | "engineer"
  | "staff"
  | "unit_manager"
  | "auditor"
  | "system_admin";

/**
 * The finance team. Both CEOs carry the whole module because the company has
 * no finance officer yet; the `finance_officer` role keeps the same powers for
 * the day it is staffed (§3.2).
 */
const FINANCE_TEAM: Role[] = ["finance_officer", "ceo_commercial", "ceo_internal"];

/**
 * Who may raise an expense claim: everyone who spends on the company's behalf
 * (`finance.expense.write`), plus the finance team.
 */
const CLAIMANTS: Role[] = [...FINANCE_TEAM, "project_lead", "engineer", "staff"];

/**
 * Who may decide a claim. Approvals are one audited decision: whoever holds the
 * permission decides — including on their own claim — and the audit row names
 * them (§6.6). A project lead still decides for their own team (§3.3).
 */
const EXPENSE_APPROVERS: Role[] = [...FINANCE_TEAM, "project_lead"];

/**
 * The roles that hold each capability. These mirror the role table in
 * `apps/api/internal/seed/mapping.go`, which is where the routes' permissions are
 * actually granted: a role not listed holds nothing, so an `auditor` (read-only),
 * a `sales_rep` or a `system_admin` sees no write control - which is also what Go
 * would answer. When Go exposes a real permission list for the caller, replace
 * `financeCapabilities` with it and delete `GRANTS`.
 */
const GRANTS: Record<FinanceCapability, Role[]> = {
  "finance.expense.create": CLAIMANTS,
  "finance.expense.submit": CLAIMANTS,
  "finance.expense.approve": EXPENSE_APPROVERS,
  "finance.expense.settle": FINANCE_TEAM,
  "finance.expense.reconcile": FINANCE_TEAM,

  // A write-off rides on `finance.invoice.issue`: it closes the same lifecycle
  // and there is no separate `finance.invoice.write_off` permission (§8.4).
  "finance.invoice.create": FINANCE_TEAM,
  "finance.invoice.update": FINANCE_TEAM,
  "finance.invoice.approve": FINANCE_TEAM,
  "finance.invoice.issue": FINANCE_TEAM,
  "finance.invoice.dispute": FINANCE_TEAM,

  "finance.payment.create": FINANCE_TEAM,
  "finance.payment.allocate": FINANCE_TEAM,
  "finance.payment.reconcile": FINANCE_TEAM,

  "finance.budget.create": FINANCE_TEAM,
  "finance.budget.update": FINANCE_TEAM,
  "finance.budget.approve": FINANCE_TEAM,
  "finance.budget.close": FINANCE_TEAM,

  "finance.account.create": FINANCE_TEAM,
  "finance.account.manage": FINANCE_TEAM,

  "finance.recurring_cost.create": FINANCE_TEAM,
  "finance.recurring_cost.update": FINANCE_TEAM,
};

/**
 * The capabilities the viewer holds, from the roles on their session. Returned
 * as a plain array so it crosses the server/client boundary unchanged.
 */
export function financeCapabilities(
  roles: readonly string[],
): FinanceCapability[] {
  const held = new Set(roles);
  return FINANCE_CAPABILITIES.filter((capability) =>
    GRANTS[capability].some((role) => held.has(role)),
  );
}
