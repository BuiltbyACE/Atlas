/**
 * Atlas web - the finance actions the UI offers (blueprint §8.4).
 *
 * Go owns authorisation: every finance route declares the permission it needs
 * (§6.2) and refuses a caller without it. This module only decides which
 * controls to *show*, because Atlas' iron rule is that hiding is never the
 * control - a hidden button is a convenience, not a guard.
 *
 * The action names below are the exact permission keys the finance routes
 * declare in `internal/platform/httpserver/router.go`, and every one is present
 * in the §6.2 catalogue (`internal/seed/catalogue.go`). They are read from the
 * `finance` destination in `GET /api/v1/me/navigation`, where Go reports each
 * key as a boolean for the caller. The web app keeps no separate capability
 * vocabulary, so a control can no longer drift from what Go enforces.
 *
 * Finance is carried by both CEOs because the company has no finance officer
 * yet (§3.2), and every one of these actions is written to the audit trail with
 * the actor's name (§6.6) - the footprint is the control, so the UI decides only
 * what to show, never what Go may permit.
 */

import type { Navigation } from "@/lib/api/atlas";

/** Every finance permission the finance screens render a control for. */
export const FINANCE_ACTIONS = [
  "finance.expense.read",
  "finance.expense.write",
  "finance.expense.approve",
  "finance.invoice.read",
  "finance.invoice.write",
  "finance.invoice.issue",
  "finance.payment.read",
  "finance.payment.record",
  "finance.budget.read",
  "finance.budget.manage",
  "finance.account.manage",
  "finance.recurring_cost.manage",
  "finance.amount.sensitive.read",
  "finance.report.read",
] as const;

export type FinanceAction = (typeof FINANCE_ACTIONS)[number];

/**
 * The finance actions Go reported as held for the viewer. The booleans come
 * from the navigation response's `finance` item; when that destination is
 * absent (for example because the navigation call failed) no finance write
 * control is offered.
 */
export function financeCapabilities(
  navigation: Navigation | null,
): FinanceAction[] {
  const finance = navigation?.sections
    .flatMap((section) => section.items)
    .find((item) => item.key === "finance");
  if (!finance) return [];
  return FINANCE_ACTIONS.filter((action) => finance.actions[action] === true);
}
