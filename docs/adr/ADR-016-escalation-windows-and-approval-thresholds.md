# ADR-016 — Escalation windows E1–E3 and approval thresholds T1/T2, D1, C2 (D4)

| | |
|---|---|
| **Status** | Accepted |
| **Date** | 2026-10-17 |
| **Deciders** | Both CEOs (D4 of §21) |
| **Phase** | Needed by Phase 1 (§21); implemented in Phase 2 |
| **Supersedes / Superseded by** | Decides D4 of §21 and replaces the illustrative starting values of §3.3 |

## Context

Two of Atlas's policy surfaces are configurable by decision **D4**:

- The **escalation ladder** (§3.4, §5.8) needs the three timing windows E1–E3 and a
  definition of "domain CEO". Until this ADR the sweep raised **level 1
  immediately** on overdue and assigned it to the project owner
  (`docs/PHASE2.md` §9, "L1-only escalation"), because the level column existed
  but the windows did not.
- The **approval matrix** (§3.3) needs thresholds T1, T2 (the amount bands) and
  D1, C2 (the discount and contract-value thresholds). The database carried only
  two uncapped policies (`expense → ceo_commercial`, `purchase → ceo_internal`)
  and the matrix's other rules were unset.

D4 is owned by **Both CEOs** and marked as needed by Phase 1: it blocks the L2/L3
rungs and the money decisions. This ADR records the decided numbers.

Two implementation facts constrain what "enforce" can mean today:

1. §5.8 shows *Elixir* scheduling the due/L1/L2 timers and calling Go commands
   that do not exist. The accepted deviation (`docs/PHASE2.md` §9) puts the clock
   in the **Go worker sweep**, which already mirrors the approvals sweeper.
2. The approvals engine resolves an approver rule only when it names a single
   concrete role. §3.3's `line_manager` and `domain_ceo` rules have no resolver —
   there is no reporting line and no commercial/internal domain map in the
   schema (projects carry a `department_id`, not a domain). Policy bands using
   those rules would make policy-derived requests fail unless the caller names a
   role explicitly.

## Decision

**Escalation windows (implemented, `docs/PHASE2.md` §5).**

| Window | Value | Effect |
|---|---|---|
| E1 | 24 hours overdue | raise **L1** to the project owner |
| E2 | 48 hours overdue | promote to **L2**, the domain CEO |
| E3 | 48 hours after L2 | promote to **L3**, both CEOs' shared queue |

- L1 goes to the **project owner** — the "owner's lead" of §3.4: a task lives in
  one project and its owner is that lead in this data model.
- L2 goes to the **holder of the `escalation.l2_role` setting**, default
  `ceo_internal` (work, people and systems are the internal CEO's domain, §3.1,
  §3.2). Resolved from company-scoped role grants at sweep time; when nobody
  holds the role, L2 degrades to shared visibility rather than to nobody.
- L3 is the **shared both-CEO queue** and deliberately has **no single
  assignee**; the event still invalidates the executive dashboard.
- E3 is measured against the **task's deadline**: L3 fires at
  `due_at + E2 + E3`. Because the sweep raises L2 as soon as E2 passes, this is
  "L2 unresolved after E3" to within one sweep interval, and it keeps one
  timestamp (`due_at`) scoping the whole episode — the mechanism the L1-only
  implementation already used. A sweep that was down for days lands a task
  directly on the rung its overdue time has reached; it never walks rungs one
  pass at a time.
- The rungs are **promoted in place** (same escalation row: level, assignee and
  version change), so one overdue item is one queue entry and every transition
  is audited. The partial unique index on open escalations is untouched.
- The windows and the L2 role are **typed settings** (§6.7) —
  `escalation.e1_hours`, `escalation.e2_hours`, `escalation.e3_hours`,
  `escalation.l2_role` — read on every sweep pass, so they are retuned through
  the settings API (versioned, audited) without a deploy. No new table: three
  numbers and a role do not yet justify one, and `core.settings` already carries
  the change history §6.7 wants.

**Approval thresholds (decided and stored, not yet enforced).**

| Threshold | Value | Meaning |
|---|---|---|
| T1 | KES 10,000 | below: project lead / line manager |
| T2 | KES 100,000 | above: both CEOs |
| D1 | 10% | maximum proposal discount the Commercial CEO approves alone |
| C2 | KES 500,000 | contract value above which both CEOs sign |

- Stored as typed settings `approvals.t1_minor`, `approvals.t2_minor`,
  `approvals.discount_max_pct`, `approvals.contract_ceo_min_minor` (minor units
  per ADR-010), so the decision is on record and editable through the same
  audited API.
- **Enforcement is deferred** until the approvals engine can express §3.3:
  `line_manager`/`domain_ceo` need resolvers that do not exist (Context 2), and
  the policy change workflow (§6.7, TODO) is not built. Seeding
  `core.approval_policies` bands today would turn working policy-derived
  requests into errors, so the numbers wait in settings and the two existing
  uncapped policies keep governing.
- §3.4's non-time-driven rungs — a blocker flagged "needs management" (L2) and
  cross-domain blockers or client-impacting risk (L3) — are **out of scope**
  here: blockers have no such flag today. Those are separate triggers on the
  same ladder; the rung machinery this ADR ships is what they will reuse.

## Consequences

### Positive
- D4 is decided; the ladder and the money thresholds are no longer open items
  and the L2/L3 rungs are live.
- Retuning the ladder is an audited settings write, not a deploy; E1 ≤ E2 ≤ … is
  normalised so a misconfigured ladder degrades its timing, never its order.
- One escalation row per overdue item keeps counts, the shared queues and the
  dashboard honest, and the audit trail records each rung transition.

### Negative
- L3 has no single assignee, so no per-person realtime nudge reaches both CEOs
  from the event alone; they see it through the dashboard invalidation and the
  shared queue. Notification fan-out (Phase 7, §6.4) is where both CEOs get a
  direct nudge.
- L1 targets the project owner because that is the only "lead" the work schema
  has; a task-specific escalation to a reporting line needs a reporting-line
  model that does not exist yet.
- `work.task.overdue.v1` now rides with the first escalation (at E1) rather than
  firing at the deadline. The dashboard's overdue tile derives from `due_at`
  directly, so overdue work is still visible immediately; only the realtime
  refresh signal moves by E1.
- The approval thresholds are inert until enforcement lands — deliberately, and
  recorded here so the gap is a decision, not an oversight.

### Neutral
- Values are KES minor units and whole percentages (ADR-010); no currency column
  is needed because the thresholds are company-wide and single-currency today.
- When the policy change workflow (§6.7) lands, changing these numbers becomes a
  both-CEO approval; the settings keys remain the storage.

## Alternatives considered

| Option | Trade-offs |
|---|---|
| **Elixir Oban timers for the ladder** (§5.8 literal) | The §5.8 command API does not exist; the Go sweep is already accepted for approvals (`docs/PHASE2.md` §9). Moving the clock later changes no data. |
| **One escalation row per level** | Keeps an explicit row per rung, but inflates the queue and dashboard counts (L1+L2+L3 open for one item) and needs extra dedup in every read. In-place promotion keeps the record singular. |
| **A `core.escalation_policies` table** (§6.7) | Faithful to the table list, but adds a table, its CRUD and its change workflow for three numbers and a role. Typed settings already provide versioning, auditing and validation. |
| **Seed the §3.3 bands now** | Enforces T1/T2 immediately, but `line_manager`/`domain_ceo` cannot resolve, so policy-derived requests would break — a correctness regression for unenforced policy. |
| **E3 measured from the L2 row's age** | Exact "L2 unresolved after E3", but needs a level-transition timestamp column and splits episode scoping across two clocks. Deadline arithmetic is within one sweep interval and keeps one clock. |

## References

- `docs/ATLAS.md` §3.1 (leadership domains), §3.2 (roles), §3.3 (approval
  matrix), §3.4 (escalation ladder), §5.8 (overdue example), §6.3 (approvals),
  §6.4 (notifications), §6.6 (concurrency/audit), §6.7 (configuration), §21 (D4)
- `docs/PHASE2.md` §5 (ladder implementation), §9 (deviations)
- ADR-002 (Go writes), ADR-009 (signals, not data), ADR-010 (minor units)
- `docs/TODO.md` — policy change workflow deferred with enforcement
