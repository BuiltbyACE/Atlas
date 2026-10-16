# To-do: finish foundation (backends/engines/orchestrations) so frontend can build on top

The sequence follows: Notifications done (Go), Elixir notification handler done. Approvals engine has policy lookup, four-eyes/two-person enforcement, notification fan-out and a worker expiry sweep; the Elixir outbox consumer routes notification/approval/file events to realtime signals. Settings, files, audit views and exec dashboard are done; worker/runbook/logging polished. Next: Phase 2.

## A) Approvals engine — core complete
- [x] Service layer `internal/approvals/service.go`: create approval with policy checks, decide (approve/decline), escalate, cancel; optimistic concurrency; emits correct outbox events; writes audit
- [x] HTTP layer `internal/approvals/http.go` + views (request, pending lists, approve/decline/escalate/cancel, get, history)
- [x] Wire approvals into router (`httpserver/router.go`) + api/main (`cmd/atlas-api/main.go`) with permissions
- [x] Policy lookup (`Policy`/`LookupPolicy`, `core.approval_policies`): resolves approver role + required approvals when the caller omits them
- [x] Separation of duties: `authz.FourEyes` on decide (no self-approval); `authz.TwoPersonRule` for two-person decisions
- [x] Two-person decisions: `approvals_required` (migration 00007) + `core.approval_decisions`; first approval stays `pending`, second distinct person completes it
- [x] Unit tests: policy rules (`policy_test.go`); integration tests: policy resolution, four-eyes, two-person, fan-out
- [x] Integration: approvals create notification intents (request → approvers, decide → requester, escalate → target), best-effort after commit

## B) Worker sweeps (expiry/escalation)
- [x] Extend `atlas-worker` to run approvals sweeper: claim expired (`due_at < now()`, status requested/pending) with `FOR UPDATE SKIP LOCKED`, expire with event, emit `core.approval.expired.v1` + notifications
- [x] Backoff/poll interval for sweeps (configurable): `ATLAS_APPROVAL_SWEEP_INTERVAL`, `ATLAS_APPROVAL_SWEEP_BATCH`
- [x] Tests: sweep expiry end-to-end (status/event/history/fan-out), batch limit, ref uniqueness, sweeper loop unit tests
- [ ] Escalation sweep (auto-escalate before expiry) — deferred to Phase 2 work orchestration

## C) Settings & policies
- [x] `internal/settings/` (typed settings, get/set, versioned, audited): `definitions` catalogue (string/bool/int/enum/string_list), `Store` (List/Get/Set/History) with `FOR UPDATE` optimistic concurrency, `Service` (validate + best-effort audit). Migration `00008` adds `core.settings.version` + `core.setting_changes`
- [x] Approval policy lookup used by approvals service
- [x] Routes + tests: `GET /api/v1/settings`, `GET/PUT /api/v1/settings/{key}` (ETag/If-Match → 428/412), `GET /api/v1/settings/{key}/history` (cursor); read-only `GET /api/v1/approval-policies`; gated by `core.settings.read`/`core.settings.manage`/`core.approval_policy.read`
- [ ] Policy **change** workflow (request → both-CEO approval → apply), and seeding real thresholds — deferred to Phase 2 (depends on decision D4; §6.7)

## D) Files
- [x] `internal/files/` metadata, pre-signed upload URLs, classification: `Store` (create/get/list/complete/archive) over `core.files`, `Service` (initiate upload → pending, confirm via storage `Stat`, download URL, archive), routes gated by `core.file.read`/`core.file.write`
- [x] S3 config (`S3_*` env, `config.ObjectStorageConfigured`), S3-compatible adapter (`ObjectStore`: presign PUT/GET, Stat, Delete) targeting Cloudflare R2 and SeaweedFS (migration `00009`: `core.files.updated_at` + touch trigger + keyset indexes); virus-scan placeholder (`Scanner`/`NoopScanner`, `status=quarantined`)
- [x] Events (`core.file.uploaded.v1`, `core.file.deleted.v1`) written in the same tx as the change + best-effort audit; unit, DB integration, and env-gated S3 round-trip tests

## E) Audit views
- [x] Global list `GET /api/v1/audit` (filters: `entity_type`, `entity_id`, `actor_type`, `actor_id`, `action`, `since`, `until`; keyset cursor) and per-record history `GET /api/v1/audit/{entity_type}/{entity_id}`; both gated by `core.audit.read` (auditor + both CEOs, §6.3)
- [x] Redaction enforcement: the Writer redacts sensitive fields before insert (§6.2), so the stored row and every view are already redacted; the read side returns the stored JSON verbatim with no bypass. Migration `00010` adds `(occurred_at DESC, id DESC)` and `(entity_type, entity_id, occurred_at DESC, id DESC)` keyset indexes and drops the two subsumed originals
- [x] Tests: handler contract (envelope, limit+1 paging, cursor/filter/id validation), service defaults, and DB integration (redaction in the raw column and via the reader, newest-first ordering, keyset pagination without overlap, actor/action/time filters); verified live: both routes mounted and 401 unauthenticated

## F) Exec dashboard v1 (Phase 1)
- [x] Tiles: staff status (people by status), pending approvals (requested/pending + overdue + by role), escalated approvals, recent audit highlights (10 newest) — as implemented per ATLAS.md Phase 1
- [x] Fed via queries (Go read model `internal/dashboard`) + realtime invalidations: approvals events push `dashboard.changed` on `exec:dashboard`; file events push `dashboard.changed` on `exec:dashboard` (Elixir wiring)
- [x] Events for tile changes via outbox → orchestrator → `exec:dashboard` channel

## G) Elixir outbox consumer wiring (complete)
- [x] Register handlers in `Outbox.Handlers.dispatch/1`: `core.notification.requested.v1` routes to `Notifications.Handler`, `core.approval.*.v1` to `Approvals.Handler`, `core.file.*.v1` routes to `Dashboard.Handler`
- [x] `AtlasOrchestrator.Approvals.Signals` (`approval.changed`) + `Approvals.Handler`: dashboard on every approval event, named approver on request, named target on escalation; `AtlasOrchestrator.Dashboard.Signals`/`Handler` broadcasts `dashboard.changed` to `exec:dashboard` on file events
- [x] Tests: signals shape, handler routing/topics, and router dispatch (32 orchestrator tests)

## H) Operational/infra polish (complete)
- [x] Worker process docs/runbook update; ensure task to run api+worker+orchestrator+web
- [x] Structured metrics/logs for dispatcher + sweeps (attempts, failures); lag/telemetry deferred to Phase 2
- [x] Contracts schema validation hook awareness (CI) — schemas already present

## Phase 2 - Work orchestration (backend; plan in docs/PHASE2.md)

- [x] Migration 00011: `work` schema - projects, project_members, milestones,
      tasks, task_dependencies, blockers, evidence, reviews, handovers,
      daily_reports, absences (goose StatementBegin/End; schema_version 11)
- [x] `internal/outbox/work.go` - work.* event constructors (project
      created/status_changed; task created/status_changed/assigned/blocked/unblocked)
- [x] Work core: types + validation, PGStore (versioned CRUD, keyset listing,
      dependency cycle prevention, blockers), Service (transitions, audit +
      outbox in-tx), Handler + gated routes (work.project/task/blocker), wired
      into router + cmd/atlas-api
- [x] Tests: unit (validation, transitions) + DB integration (lifecycle,
      cycles, blockers, overdue filter) + registry/permission-catalogue checks
- [x] Evidence: link an available core.files row to a task/project
      (work.evidence.read/.write, work.evidence.added.v1); verify fields with
      separation of duties (cannot verify own evidence)
- [x] Daily reports: submit/list/get, one per person per day (unique key,
      re-submit updates an unreviewed day), work.daily_report.submitted.v1;
      D5 policy intentionally not enforced (open decision)
- [x] Absences: request/list/get/decide/cancel, overlap rejection, no
      self-decision (work.absence.requested/approved/declined/cancelled.v1)

### Remaining Phase 2 (next)
- [ ] Overdue escalation ladder + worker sweep (feeds exec dashboard)
- [ ] Exec dashboard "overdue work" tile
- [ ] Elixir outbox wiring for work.* events (signals + dashboard invalidation)
- [ ] Frontend (after backend above is complete)

Current status: Phase 1 (A-H) complete. Phase 2 work core, evidence, daily
reports and absences complete (schema 11; routes wired; tests green). Next:
overdue escalation ladder + worker sweep, then the exec "overdue work" tile,
then Elixir outbox wiring for work.*.
