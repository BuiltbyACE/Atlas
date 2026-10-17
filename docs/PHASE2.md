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
- Schema-per-module: work tables live in schema `work` (migration 00011); the
  people tables (daily reports, leave) live in schema `people` (migration 00013).

## 1. Schema (migration 00011 — done)

- [x] `work.projects`, `work.project_members`
- [x] `work.milestones`
- [x] `work.tasks`, `work.task_dependencies`, `work.blockers`
- [x] `work.evidence` (FK to `core.files`), `work.reviews`, `work.handovers`
- [x] `people.daily_reports`, `people.leave_requests` — moved into the `people`
      schema from `work` in migration `00013` (module ownership, §10.1)
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

## 4. People module — daily reports, leave, availability, offboarding, session activity (`internal/people`) — done

- [x] `internal/people` module (migration `00013` moves the tables home):
      daily reports: submit/list/get + `GET /api/v1/daily-reports/today`, one per
      person per day (unique key; re-submitting updates an unreviewed day, an
      already-reviewed day is immutable); emits
      `people.daily_report.submitted.v1`. The report time/deadline policy (D5) is
      deliberately not enforced — it is open configuration, so any date is
      accepted for now.
- [x] Leave: request/list/get/decide/cancel (dedicated flow, not the approvals
      engine); overlapping active leave is rejected; a requester cannot decide
      their own request, and only the requester may cancel theirs
- [x] Events `people.leave.requested/approved/declined/cancelled.v1`
- [x] Routes per §8.5: `POST/GET /api/v1/daily-reports`,
      `GET /api/v1/daily-reports/{id}`, `GET /api/v1/daily-reports/today`,
      `POST/GET /api/v1/people/leave-requests`, `GET /api/v1/people/leave-requests/{id}`,
      `POST /api/v1/people/leave-requests/{id}/decide`,
      `POST /api/v1/people/leave-requests/{id}/cancel`
- [x] Permissions already catalogued (`people.daily_report.read/.submit`,
      `people.leave.read/.request/.manage`)
- [x] Availability: `GET /api/v1/people/availability?date=` (defaults to today)
      gated by `people.person.read` — every non-exited person with the approved
      leave covering the day attached
- [x] Offboarding: `POST /api/v1/people/{id}/offboard` gated by
      `people.person.manage` — one transaction exits the person with a last
      working day, deactivates the linked account and revokes its sessions and
      role grants; emits `people.person.offboarded.v1`. The blueprint's
      offboarding checklist and work handover are deferred (no table yet)
- [x] Session activity (**not in the blueprint** — requested directly): who is
      signed in now, and how long each person was active. Read-only and derived
      from the `core.sessions` rows auth already keeps (migration `00014` adds
      the roster index); no new business state.
      - `GET /api/v1/people/presence` gated by `people.session.read` — live,
        unexpired sessions last seen within `ATLAS_SESSION_PRESENCE_WINDOW`
        (default 2m), newest activity first.
      - `GET /api/v1/people/sessions?person_id=&from_date=&to_date=` — a keyset
        page of the day's sign-ins (ordered by `last_seen_at`), each with
        `online`, `active_seconds` and an `ended_at` when explicitly revoked.
        "From when to when" is `started_at` → (`ended_at` or `last_active_at`).
      - `POST /api/v1/auth/heartbeat` (Authenticated, 204) — an open client pings
        every minute, refreshing `last_seen_at` so an idle-but-open tab stays
        online; the web shell sends it from `PresenceHeartbeat`
      - Web: `/people/activity` — a polling live roster plus a per-person,
        per-day roll-up. Polling (30s) rather than a realtime topic keeps this
        from adding session-start/end outbox events and an orchestrator join map.

## 5. Overdue escalation ladder + worker

- [x] `work.escalations` (migration `00012`): `entity_type`/`entity_id`, `level`,
      `status` (open/resolved), `reason`, `assignee_person_id`, `resolved_by`/
      `resolved_at`, `version`; partial unique index on open escalations
- [x] Detect overdue tasks (due_at < now, not done/cancelled) and raise a
      escalation at the rung decision **D4** says the overdue time has reached
      (ADR-016), in one transaction (`FOR UPDATE SKIP LOCKED`), so two workers
      take disjoint slices; the "current deadline" scopes an episode so a
      resolved escalation is not re-raised until the deadline moves and passes
      again
- [x] Escalation sweep in `atlas-worker`
      (`ATLAS_WORK_SWEEP_INTERVAL`/`ATLAS_WORK_SWEEP_BATCH`), mirroring the
      approvals sweeper: it drains in batches and logs, never dies on one bad batch
- [x] Automatic resolution: once the task is done/cancelled or no longer overdue
      the sweep closes the escalation and emits `core.escalation.resolved.v1`
- [x] Manual resolution: `POST /api/v1/work/escalations/{id}/resolve` gated by
      `work.task.review`; list/get gated by `work.task.read`
- [x] Events `work.task.overdue.v1` + `core.escalation.raised/resolved.v1`.
      The full ladder is implemented (decision **D4**, `docs/adr/ADR-016`): L1
      after E1 overdue to the project owner, L2 after E2 to the domain CEO
      (`escalation.l2_role`, default `ceo_internal`), L3 after E2+E3 to the
      shared both-CEO queue with no single assignee. The windows and the L2 role
      are typed settings (§6.7), read on every sweep pass; a first sweep long
      after the deadline lands directly on the right rung, later rungs promote
      the open escalation in place (one queue entry per item, one audit per
      transition). Notification fan-out is Phase 7.

## 6. Executive dashboard updates

- [x] Add "overdue work" tile (open tasks past deadline + open escalations) to
      `GET /api/v1/dashboard/executive`; reuse `dashboard.changed`

## 7. Elixir outbox wiring

- [x] Go-side event constructors: `internal/outbox/work.go`
      (`work.project.created/status_changed`, `work.task.created/status_changed/assigned/blocked/unblocked/overdue`,
      `work.evidence.added`) and `internal/outbox/people.go`
      (`people.daily_report.submitted`, `people.leave.*`), plus
      `internal/outbox/escalation.go` (`core.escalation.raised/resolved.v1`)
- [x] `AtlasOrchestrator.Work.Signals`/`Handler`: route `work.*` and
      `core.escalation.*` to `<entity>.changed` signals (ADR-009 shape:
      `{type, id}`) on `work:project:{id}` and `user:{id}`, plus a
      `dashboard.changed` tile invalidation to `exec:dashboard`; tests
- [x] `AtlasOrchestrator.People.Signals`/`Handler`: route `people.*` to
      `<entity>.changed` signals on `user:{id}` (when the event names a person)
      and a `dashboard.changed` tile invalidation to `exec:dashboard`; tests
- [x] Go join check: `GET /internal/v1/authz/check?user_id=&permission=` (§7.5),
      service-to-service and gated by `core.authz.check`; resolves the user's
      effective permissions from their grants so the orchestrator can authorise
      a topic join without knowing roles→permissions itself
- [x] Static service credential: `atlas-cli service create` (upsert the
      `svc:orchestrator` identity and its permissions) and `atlas-cli service
      token` (mint a long-lived token signed with the API key, `ATLAS_SERVICE_TOKEN`),
      plus `auth.EnsureService`; integration test of the mint→resolve round trip
- [x] `work:project:{id}` channel mounted (`Realtime.ProjectChannel`), joined only
      after Go's internal join check agrees (§7.5, ADR-009). `AtlasOrchestrator.Authz`
      owns the call: a brief decision cache (30s) over an injectable client
      (`Authz.HTTP`, `ATLAS_SERVICE_TOKEN`), failing closed on any fault; tests
      cover the cache, the fail-closed paths and the channel join
- [x] Personal realtime topics address the **account**, not the person (§7.5,
      §10.1): producers resolve person→account at write time through
      `internal/identity` and carry a nullable `*_user_id` beside the person id
      in the outbox payload (nil when the person has no login); handlers push
      `user:{account_id}`, the id `UserSocket` assigns from the token `sub`.
      Payloads, handler maps, contracts and tests updated across
      work/people/approvals/notifications

## 8. Frontend (after the backend above is complete and tested)

- [x] Projects list/detail, task board, evidence upload, daily report, workload —
      plus escalations, the people availability roster with leave/offboarding, the
      session-activity view (`/people/activity`: live roster + per-person daily
      roll-up, fed by the heartbeat and a 30s poll), and real
      executive-dashboard tiles. A hand-written typed API layer
      (`apps/web/lib/api/atlas.ts`) mirrors the Go JSON shapes; `SignalRefresh`
      re-reads server components on realtime signals. `generate:api` is still a
      placeholder because the public OpenAPI document covers auth + internal
      endpoints only.

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
- **Daily reports and leave/absence placement — resolved.** §7.4 catalogues
  `people.daily_report.submitted.v1` and `people.leave.*`, and the §10.1 table
  gives those tables to the `people` module; migration `00013` moves
  `work.daily_reports`/`work.absences` into `people` (`people.daily_reports`,
  `people.leave_requests`), the `internal/people` package owns them, and the
  events are emitted as `people.*`. Permission and event namespaces now agree.
  The two remaining §8.5 people APIs (`GET /api/v1/people/availability`,
  `POST /api/v1/people/{id}/offboard`) are implemented too.
- **Offboarding scope.** `POST /api/v1/people/{id}/offboard` covers the access
  lifecycle — the person exits and their account's sessions and role grants are
  revoked immediately (the account lifecycle, §7.1/§8.5). The blueprint's
  `offboarding_checklist` entity has no table yet, so the checklist, asset
  return and work handover are not modelled; they can be added without changing
  this endpoint's contract.
- **Approval events** (Phase 1) are finer-grained than §7.4's single
  `core.approval.decided.v1`: `core.approval.requested/approved/declined/
  escalated/cancelled/expired.v1`.
- **Realtime join authorisation** (§7.5) is evaluated as the topic's read
  permission (`work:project:{id}` → `work.task.read`, `exec:dashboard` →
  `intel.dashboard.read`) rather than literal project membership. This matches
  the REST visibility the frontend already has, and a signal carries ids, not
  data (ADR-009). Narrowing a project channel to `work.project_members` would
  make it stricter than the REST API; that refinement is deferred.
- **L1-only escalation — resolved.** The ladder's E1-E3 windows were decision
  **D4**; D4 is decided (`docs/adr/ADR-016`) and the full ladder ships as typed
  settings: L1 after E1 to the project owner, L2 after E2 to the
  `escalation.l2_role` holder (default `ceo_internal`), L3 after E2+E3 to the
  shared both-CEO queue. Note the one timing consequence: E3 is measured against
  the task's deadline (`due_at + E2 + E3`) rather than the L2 row's age, which
  is "L2 unresolved after E3" to within one sweep interval and keeps a single
  timestamp scoping the episode. §3.4's flag/risk-driven rungs (blocker "needs
  management", cross-domain blocker) are not modelled yet — blockers carry no
  such flag — and reuse the same machinery when they land. Approval thresholds
  T1/T2, D1, C2 are decided and stored as typed settings but not enforced; see
  ADR-016 for why.
- **Realtime signal names** follow ADR-009's `{type: "<entity>.changed", id}`
  example rather than a module-level name, and tile invalidation reuses the
  dashboard's `dashboard.changed` (`AtlasOrchestrator.Work.Handler`).
