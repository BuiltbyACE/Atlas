# ADR-019 — Timesheets: requirement and project types (D7)

| | |
|---|---|
| **Status** | Accepted |
| **Date** | 2026-10-17 |
| **Deciders** | Both CEOs (D7 of §21) |
| **Phase** | Needed by Phase 2 (§21); implemented in Phase 2 |
| **Supersedes / Superseded by** | Decides D7 of §21 and replaces the `[CONFIG]` marker on `time_entry` in §8.2 |

## Context

Decision **D7** (§21, owned jointly by both CEOs) asks *whether timesheets are
required, and for which project types*. `docs/ATLAS.md` §8.2 already listed
`time_entry` as an entity — "person, task, date, minutes, note — **enabled per
project type [CONFIG]**" — and §8.4's project-profitability formula spends people
cost as `Σ(person hours × loaded hourly cost)`, "where timesheets are not used,
allocate by assignment percentage". Neither the table nor the setting existed:
migration `00011` shipped the `work` schema without `work.time_entries`, and the
`work.time_entry.read` / `work.time_entry.write` permissions were seeded with no
route behind them.

The decision has two halves — **whether** timesheets are required, and **how**
that is enforced — and each has a real cost if it is wrong:

1. **Requirement breadth.** Mandating timesheets everywhere is a tax on internal
   work whose cost nobody bills; never requiring them starves the profitability
   view of the hours it needs on the work where margin actually matters.
2. **Enforcement strength.** Blocking task completion on logged hours turns a
   finance convenience into a delivery blocker — a person who finishes a task at
   18:00 cannot mark it done until they have logged minutes, and a project slips
   for a reason unrelated to the work. The alternative, reporting coverage
   without gatekeeping, keeps the record honest and the delivery path unblocked.

D7 is needed for the §8.4 project-P&L and its gross-margin metric, and for the
work module's `work.time_entry.*` permissions to mean something, so it had to be
decided and implemented for Phase 2.

## Decision

**Requirement (implemented).** Timesheets are **required for `client_delivery`
projects only**. Every other project type — `internal_product`,
`maintenance_support`, `business_unit`, and any type added later — has
timesheets **optional**: entries are accepted and contribute to margin when
present, but their absence is not a defect. The list of required types is the
typed setting `work.timesheet.required_project_types`, a comma-separated list
whose default is `client_delivery`; an empty value means *no* project type
requires timesheets.

| Setting | Key | Default | Meaning |
|---|---|---|---|
| Required types | `work.timesheet.required_project_types` | `client_delivery` | project types whose work requires timesheets (comma-separated; empty = none) |

- Project type is free text (§8.2), so the comparison is case-insensitive and
  whitespace-trimmed: `Client_Delivery` and `client_delivery` are the same
  requirement.
- The requirement is surfaced on the project view as `timesheet_required`, so a
  client can show the timesheet section and its coverage without re-deriving the
  setting.

**Enforcement (implemented): report-only.** Nothing in the system blocks a
transition because hours are missing. A task reaches `Done`, a project reaches
`Completed`, and a review is approved regardless of timesheet coverage. D7's
requirement is a **signal and a report**, not a gate.

- The project-profitability calculation (§8.4) uses logged `time_entry` minutes
  where they exist and falls back to the **assignment-percentage allocation**
  where they do not, exactly as §8.4 already describes. A `client_delivery`
  project with sparse timesheets therefore reports margin from the allocation,
  flagged as estimate-based rather than actual.
- Coverage — for a required type, the share of a project's effort that has
  timesheets behind it — is a **reporting** concern (§8.4 metrics), not a
  transition guard.

**Where it lives.** Time entries are ordinary work data in the `work` schema,
owned by the Go API like every other work record (ADR-002): the API is the only
writer, and `POST/PUT/DELETE /api/v1/work/time-entries` emit
`work.time_entry.logged.v1`, `work.time_entry.updated.v1` and
`work.time_entry.removed.v1` in the same transaction (ADR-009). The project is
**derived from the task** and stored on the row, so a person's week, a task's
logged effort and a project's timesheet coverage are each a single indexed read.

## Consequences

### Positive

- **Client work is measured, internal work is not taxed.** The profitability
  view gets real hours where margin is judged (client delivery) without asking
  people to account for every internal hour.
- **Delivery is never blocked by bookkeeping.** A forgotten time entry cannot
  stall a task, a review or a project; the failure mode of under-logging is a
  less precise margin number, not a slipped deadline.
- The requirement is a **typed setting** (§6.7), so the CEOs change which
  project types require timesheets through the audited settings API with no
  deploy and a readable change history.
- Entries reuse the existing `work.time_entry.read` / `work.time_entry.write`
  permissions and the work module's optimistic-concurrency and audit
  conventions, so there is no second access-control or history mechanism.

### Negative

- **`client_delivery` margin is only as good as its logging.** With enforcement
  as a report, a project can close with thin timesheets and its P&L falls back
  to allocation. The mitigation is visibility, not a gate: coverage is surfaced
  (the `timesheet_required` flag and §8.4 reporting) and the fallback is labelled
  as an estimate.
- **Coverage needs a report to be useful.** This ADR records the requirement and
  the data; the coverage dashboard and the allocation-weighted margin report are
  Phase 4 finance work (§8.4). Until then the flag and the raw entries are what
  exists.
- A project type that *should* require timesheets but is spelled differently in
  data would silently not require it. This is inherent to free-text project
  types; the case-insensitive comparison removes the common accident.

### Neutral

- No new permission is introduced: `work.time_entry.read` and `.write` were
  already seeded, and role mappings already grant them.
- Time entries are editable and deletable (a logged mistake is correctable), so
  the record is a working ledger, not an append-only journal; every change is
  audited (§6.6) and emits an event.
- Elixir only reacts to the events to invalidate views (ADR-009); the hours
  themselves are data, fetched from Go, which enforces permissions.

## Alternatives considered

| Alternative | Why not |
|---|---|
| **Require timesheets for every project type** | Taxes internal product and business-unit work whose cost is not billed, and invites low-quality entries that help no one. |
| **Require timesheets for none (capture only)** | Starves the client-delivery margin view of the hours it exists to measure; D7 would be undecided in practice. |
| **Block task/project completion without hours** | Turns finance bookkeeping into a delivery blocker and slips projects for reasons unrelated to the work. Report-only was chosen deliberately. |
| **A `work.timesheet_policies` table** | One list of project types does not justify a table; `core.settings` already carries typed values, validation, versioning and audit (§6.7). |
| **Store the requirement on each project (`projects.requires_timesheets`)** | Duplicates the policy per row, so retuning the rule means migrating or rewriting every project instead of changing one setting. |
| **Percentage-of-assignment margin only (no entries)** | Simpler, but throws away the actual hours §8.4 asks for on exactly the work where margin is judged. Entries supplement the allocation, they do not replace it. |

## References

- `docs/ATLAS.md` §6.6 (concurrency/audit), §6.7 (configuration), §8.2 (work
  entities and project types), §8.4 (project profitability and the
  assignment-percentage fallback), §21 (D7)
- `docs/PHASE2.md` §3 (work module), §9 (deviations)
- ADR-002 (Go writes, Elixir orchestrates), ADR-009 (signals, not data), ADR-010
  (UTC storage, Nairobi display)
- `db/migrations/00018_phase2_time_entries.sql`
- `docs/TODO.md` — timesheet coverage reporting and the allocation-weighted
  margin report deferred to Phase 4 finance
