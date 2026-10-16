# Phase 2 — Work orchestration (backend plan)

Blueprint: `docs/ATLAS.md` §3.2, §6.3, §6.5, §6.6, §7 (outbox), §8.10, §10.3, §11,
Phase 2 (line ~1290). Boundary rules: ADR-002 (Go is the sole writer; Elixir
orchestrates), ADR-005 (transactional outbox), ADR-009 (realtime messages are
signals, not data).

This file is the running to-do list for Phase 2. Keep it current as work lands:
tick an item only when its code, tests and migrations are in place.

## 0. Conventions (unchanged from Phase 1)

- Go is the authority. Elixir only reacts to outbox events.
- Every state change writes an audit row and, where the orchestrator or the
  browser must react, an outbox event **in the same transaction**.
- Optimistic concurrency: `version` column + `If-Match`/ETag on writes (§6.6).
- Keyset cursor pagination on listings (§6.6).
- No seeded business data; no mock data.
- Schema-per-module: the work tables live in schema `work` (migration 00011).

## 1. Schema (migration 00011 — done)

- [x] `work.projects`, `work.project_members`
- [x] `work.milestones`
- [x] `work.tasks`, `work.task_dependencies`, `work.blockers`
- [x] `work.evidence` (FK to `core.files`), `work.reviews`, `work.handovers`
- [x] `work.daily_reports`, `work.absences`
- [x] Indexes for status/assignee/due_at/project; `touch_updated_at` triggers

## 2. Work core (`internal/work`) — projects, milestones, tasks

- [x] Types + validation (status/priority enums, transition rules)
- [x] `Store`/`PGStore`: CRUD + keyset listing + dependency/blocker writes,
      all versioned (optimistic concurrency, `ErrConflict`/`ErrNotFound`)
- [x] `Service`: create/update/status, dependency cycle prevention (recursive
      CTE reachability), blocker raise/resolve, audit + outbox in-transaction
- [x] `Handler` + views: routes gated by `work.project.read/manage`,
      `work.task.read/write`, `work.blocker.read/manage` (If-Match/ETag, cursor paging)
- [x] Wire router (`Deps.Work` → `registerWork`) + `cmd/atlas-api`
- [x] Unit + DB integration tests (lifecycle, transitions, cycles, blockers,
      overdue filter, registry/permission-catalogue checks)

## 3. Evidence (`work.evidence`) — done

- [x] Link an available `core.files` row to a task or project; validate file is
      `available` (a `pending`/`quarantined`/`deleted` file is rejected with
      `ErrConflict`); `work.evidence.read`/`.write`; `work.evidence.added.v1`
- [x] Review/verify fields (`verified_by`, `verified_at`); a submitter cannot
      verify their own evidence (separation of duties, §3.3)
- [x] File availability checked through `work.FileResolver`, adapted from the
      files service in `cmd/atlas-api` (the work package does not import files)

## 4. Daily reports + absence — done

- [x] `internal/work` daily reports: submit/list/get, one per person per day
      (unique key; re-submitting updates an unreviewed day, an already-reviewed
      day is immutable); emits `work.daily_report.submitted.v1`. The report
      time/deadline policy (D5) is deliberately not enforced — it is open
      configuration, so any date is accepted for now.
- [x] Absence: request/list/get/decide/cancel (dedicated flow, not the approvals
      engine); overlapping active leave is rejected; a requester cannot decide
      their own absence, and only the requester may cancel theirs
- [x] Events `work.absence.requested/approved/declined/cancelled.v1`
- [x] Permissions already catalogued (`people.daily_report.read/.submit`,
      `people.leave.read/.request/.manage`)

## 5. Overdue escalation ladder + worker

- [x] `work.escalations` (migration `00012`): `entity_type`/`entity_id`, `level`,
      `status` (open/resolved), `reason`, `assignee_person_id`, `resolved_by`/
      `resolved_at`, `version`; partial unique index on open escalations
- [x] Detect overdue tasks (due_at < now, not done/cancelled) and raise a level-1
      escalation assigned to the project owner, in one transaction
      (`FOR UPDATE SKIP LOCKED`), so two workers take disjoint slices; the
      "current deadline" scopes an episode so a resolved escalation is not
      re-raised until the deadline moves and passes again
- [x] Escalation sweep in `atlas-worker`
      (`ATLAS_WORK_SWEEP_INTERVAL`/`ATLAS_WORK_SWEEP_BATCH`), mirroring the
      approvals sweeper: it drains in batches and logs, never dies on one bad batch
- [x] Automatic resolution: once the task is done/cancelled or no longer overdue
      the sweep closes the escalation and emits `core.escalation.resolved.v1`
- [x] Manual resolution: `POST /api/v1/work/escalations/{id}/resolve` gated by
      `work.task.review`; list/get gated by `work.task.read`
- [x] Events `work.task.overdue.v1` + `core.escalation.raised/resolved.v1`.
      The ladder's E1-E3 timing windows (L2/L3) are decision **D4** and still
      open, so only L1 is raised for now; the `level` column already exists so
      later rungs need configuration, not schema. Notification fan-out is Phase 7.

## 6. Executive dashboard updates

- [x] Add "overdue work" tile (open tasks past deadline + open escalations) to
      `GET /api/v1/dashboard/executive`; reuse `dashboard.changed`

## 7. Elixir outbox wiring

- [x] Go-side event constructors are in `internal/outbox/work.go`
      (`work.project.created/status_changed`, `work.task.created/status_changed/assigned/blocked/unblocked/overdue`,
      `work.evidence.added`, `work.daily_report.submitted`, `work.absence.*`) and
      `internal/outbox/escalation.go` (`core.escalation.raised/resolved.v1`)
- [x] `AtlasOrchestrator.Work.Signals`/`Handler`: route `work.*` and
      `core.escalation.*` to `<entity>.changed` signals (ADR-009 shape:
      `{type, id}`) on `work:project:{id}` and `user:{id}`, plus a
      `dashboard.changed` tile invalidation to `exec:dashboard`; tests
- [ ] Mount the `work:project:{id}` channel with the join check via
      `GET /internal/v1/authz/check` (§7.5, ADR-009) — the topic is broadcast to
      now, but no channel is joined until this lands (frontend/real-time work)

## 8. Frontend (after the backend above is complete and tested)

- [ ] Projects list/detail, task board, evidence upload, daily report, workload

## 9. Deviations from the blueprint (recorded deliberately)

These are places where the implementation differs from `docs/ATLAS.md`; each is
either a refinement or an open question, listed so the divergence is explicit.

- **Overdue escalation timing.** §5.8 shows *Elixir* scheduling the due/L1/L2
  timers (Oban) and calling Go's `POST /internal/v1/commands/...`
  (`work.tasks.mark_overdue`, `escalations.raise`). We instead raise overdue
  escalations from a **Go worker sweep** (`atlas-worker`), mirroring the
  approvals expiry sweeper, because §5.8's internal command API does not exist
  and time-driven Go sweeps were already accepted for approvals. The outbox
  events (`work.task.overdue.v1`, `core.escalation.raised.v1`) are identical
  either way, so the frontend is unaffected; revisiting this is a matter of
  moving the *clock*, not the data.
- **Daily reports and leave/absence placement.** §7.4 catalogues
  `people.daily_report.submitted.v1` and `people.leave.*`, and the §10.1 table
  gives those tables to the `people` module, but they live in the `work` schema
  (`work.daily_reports`, `work.absences`, migration 00011) and emit `work.*`
  events. Permissions are already `people.daily_report.*`/`people.leave.*`, so
  the permission namespace and the event namespace currently disagree.
- **Approval events** (Phase 1) are finer-grained than §7.4's single
  `core.approval.decided.v1`: `core.approval.requested/approved/declined/
  escalated/cancelled/expired.v1`.
- **L1-only escalation.** The ladder's E1-E3 windows are decision **D4** (open),
  so only level 1 is raised; the `level` column exists for L2/L3 once D4 lands.
- **Realtime signal names** follow ADR-009's `{type: "<entity>.changed", id}`
  example rather than a module-level name, and tile invalidation reuses the
  dashboard's `dashboard.changed` (`AtlasOrchestrator.Work.Handler`).
