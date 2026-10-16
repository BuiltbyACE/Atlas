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

## 4. Daily reports + absence

- [ ] `internal/work` daily reports: submit/list/get, one per person per day,
      policy hooks for D5 (report time/deadline) left as configuration
- [ ] Absence: request/list/decide via the approvals engine or a dedicated flow
- [ ] Permissions already catalogued (`people.daily_report.*`, `people.leave.*`)

## 5. Overdue escalation ladder + worker

- [ ] Detect overdue tasks (due_at < now, not done/cancelled)
- [ ] Escalation sweep in `atlas-worker` (claim + escalate in one transaction,
      `FOR UPDATE SKIP LOCKED`), configurable interval/batch
- [ ] Events + notification fan-out; feeds the exec dashboard

## 6. Executive dashboard updates

- [ ] Add "overdue work" tile (Phase 2 exit needs it); reuse `dashboard.changed`

## 7. Elixir outbox wiring

- [ ] Go-side event constructors are in `internal/outbox/work.go`
      (`work.project.created/status_changed`, `work.task.created/status_changed/assigned/blocked/unblocked`,
      `work.evidence.added`)
- [ ] Route `work.*` events to signals; push invalidation on `exec:dashboard`
      and named recipients; tests

## 8. Frontend (after the backend above is complete and tested)

- [ ] Projects list/detail, task board, evidence upload, daily report, workload
