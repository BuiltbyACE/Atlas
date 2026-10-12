# ADR-013 — Accounting package and KRA e-invoicing integration route

| | |
|---|---|
| **Status** | Deferred (Phase 4) |
| **Date** | 2026-10-11 |
| **Deciders** | Internal & Technical CEO, Commercial CEO |
| **Phase** | Phase 4 |
| **Supersedes / Superseded by** | — |

## Context

Atlas is the **management finance** system: cash position, receivables, project profitability, budgets and expense control (§8.4). Statutory accounting and tax filing may stay in an accounting package; Atlas would then export or sync to it. Kenya's KRA eTIMS / tax e-invoicing obligations must be met. §12 lists both an accounting package and KRA eTIMS as **[DECIDE]**. The package and e-invoicing route is open (D11, §21).

## Decision

**Deferred to Phase 4.** The intended approach (to be confirmed in Phase 4 with the accountant):

- Atlas remains the **management finance** system. **Statutory books may stay in an accounting package** (e.g. QuickBooks, Xero, Odoo); Atlas **exports or syncs** to it (e.g. journals), rather than duplicating statutory bookkeeping (§8.4, §12).
- For **KRA eTIMS / e-invoicing**, choose one of: **direct integration**, **via the accounting package**, or **via an approved intermediary** (§12). The `invoice` entity carries an `e-invoicing reference` field (§8.4).
- **Confirm current obligations with the accountant before the finance build** (Phase 4) (§12).

**Open:** D11 — accounting package and e-invoicing route (§21).

## Consequences

### Positive
- Atlas stays focused on operational and management control rather than statutory compliance.
- No duplicate statutory bookkeeping; the export/sync boundary is explicit.
- Route flexibility (direct, package, intermediary) keeps options open until obligations are confirmed.

### Negative / trade-offs
- Two systems can disagree; an export/sync reconciliation process is required.
- eTIMS integration complexity and evolving obligations are uncertain until confirmed.
- The export/sync layer is additional code to build, observe and maintain.

## Alternatives considered
- **Atlas becomes the statutory book of record** — rejected for now: adds compliance surface Atlas is not designed for.
- **No accounting package at all** — kept open as *management finance only*, pending D11.
- **Direct-only eTIMS integration** — one of three routes; not chosen yet.

## References
- `docs/ATLAS.md` §8.4
- `docs/ATLAS.md` §12
- `docs/ATLAS.md` §21 (D11)
