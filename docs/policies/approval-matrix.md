# Approval Matrix Policy

**Source:** ATLAS.md §3.3 — Approval matrix [CONFIG]  
**Owner:** Both CEOs (joint approval required for changes)  
**Status:** Active  
**Last reviewed:** 2026-11-01  
**Next review:** 2027-05-01 (quarterly access review)

---

## Purpose

This policy defines who may approve what, and under what thresholds, across Atlas. It is the single source of truth for separation of duties and four-eyes (two-person rule) enforcement.

All approvals are enforced in Go (§6.2 authorisation). The UI may hide buttons, but Go rejects any unauthorised action regardless of the frontend.

---

## Approval Thresholds [CONFIG]

| Decision | Below T1 (KES 10,000) | T1 – T2 (KES 10,000 – 100,000) | Above T2 (KES 100,000+) |
|---|---|---|---|
| **Staff expense** | Project lead or line manager | Domain CEO | Both CEOs |
| **Purchase / subscription** | Domain CEO | Domain CEO | Both CEOs |
| **Proposal discount** | Commercial CEO (≤ D1 = 10%) | Commercial CEO + Internal CEO informed | Both CEOs |
| **Contract signature** | — | Commercial CEO | Both CEOs (above C2 = KES 500,000, or non-standard terms) |
| **Change request (scope)** | Project lead (no price change) | Internal CEO (effort change) | Both CEOs (price or deadline change) |
| **Hiring / termination** | — | — | Both CEOs |
| **Write-off of receivable** | — | — | Both CEOs |

T1 = KES 10,000 · T2 = KES 100,000 · D1 = 10% · C2 = KES 500,000

These values are stored in `core.approval_policies` and may only be changed jointly by both CEOs through Atlas. Each change creates an audit entry.

---

## Rules That Are Not Configurable

| Rule | Rationale |
|---|---|
| Nobody approves their own request | Separation of duties — prevents self-approval of expense, contract, or hiring |
| "Both CEOs" means two distinct approvals | The second approver sees the first's comment before deciding |
| Approvals expire if the underlying record changes materially | An amount or scope change after approval invalidates the prior decision |
| Approvals past their deadline expire automatically | The Go worker sweeps overdue rows, marks them `expired`, and emits `core.approval.expired.v1` |
| `approvals_required` is 1 or 2 | Enforced via `core.approval_decisions` — two distinct approvers required for "both CEOs" |

---

## Approval Workflow

```
Submit (with receipt, if applicable)
    ↓
Policy engine selects approver(s) from matrix based on decision type + amount
    ↓
Approver #1 reviews and decides (approve / decline / escalate)
    ↓
If "both CEOs" required: Approver #2 reviews and decides
    ↓
Finance officer pays / reimburses / reconciles
    ↓
Audit entry written (who, what, when, before/after)
```

Reminders to approvers fire after **[CONFIG]** 24 hours of inaction.

---

## Permissions Required

| Permission | Held By |
|---|---|
| `finance.expense.approve` | Both CEOs, finance_officer (day-staffed only) |
| `contracts.contract.sign` | Both CEOs |
| `core.approval.decide` | Role-dependent, per matrix |

---

## Audit Trail

Every approval decision writes to:
- `core.approval_history` — one row per transition (request, approve, decline, escalate, cancel, expire)
- `core.approval_decisions` — one row per approver per approval (enforces distinct approvers)
- `audit.events` — full before/after snapshot with actor, IP, request ID

---

## Change Control

Changes to this policy (thresholds, approver rules, configurable values) require:
1. Joint approval by both CEOs through Atlas
2. An audit entry recording old vs new values
3. Effective date range recorded in `core.approval_policies`

---

## Related Documents

- `docs/policies/accountability-policy.md` — staff accountability and surveillance boundaries
- `docs/policies/data-retention.md` — retention schedule for financial and audit records
- `docs/ATLAS.md §3.3` — architectural definition of the approval matrix
- `docs/ATLAS.md §6.2` — authorisation model (RBAC with scopes)
