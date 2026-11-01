/**
 * Atlas web - the finance write capabilities the UI offers (blueprint §8.4).
 *
 * Go owns authorisation: every finance route declares the permission it needs
 * (§6.2) and refuses a caller without it. This module only decides which
 * controls to *show*, because Atlas' iron rule is that hiding is never the
 * control - a hidden button is a convenience, not a guard.
 *
 * Each capability below is therefore named after the permission it requires,
 * matching the permission keys in `internal/outbox/units.go` and the route
 * declarations in `internal/platform/httpserver/router.go`. `GET /api/v1/auth/me`
 * returns the caller's permissions (§6.2); `financeCapabilities` filters the
 * full capability list down to those the caller holds.
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

/**
 * The capabilities the viewer holds, derived from the permissions on their
 * session (`GET /api/v1/auth/me`). Each capability key matches its permission
 * key, so a viewer holds a capability exactly when they hold the matching
 * permission.
 */
export function financeCapabilities(
  permissions: readonly string[],
): FinanceCapability[] {
  const held = new Set(permissions);
  return FINANCE_CAPABILITIES.filter((capability) => held.has(capability));
}
