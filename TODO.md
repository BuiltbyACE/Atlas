# To-do: finish foundation (backends/engines/orchestrations) so frontend can build on top

The sequence follows: Notifications done (Go), Elixir notification handler done. Approvals engine now has policy lookup, four-eyes/two-person enforcement, notification fan-out and a worker expiry sweep, and the Elixir outbox consumer routes notification and approval events to realtime signals. Next: settings, files, audit views, exec dashboard v1.

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
- [ ] `internal/files/` metadata, pre-signed upload URLs, classification
- [ ] S3 config, virus scan placeholder
- [ ] Events + tests

## E) Audit views
- [ ] Global audit list (auditor/CEOs), per-record history endpoints
- [ ] Redaction enforcement
- [ ] Tests

## F) Exec dashboard v1
- [ ] Tiles: pipeline, approvals pending, incidents, overdue work
- [ ] Fed via queries + realtime invalidations
- [ ] Events for tile changes

## G) Elixir outbox consumer wiring (complete)
- [x] Register handlers in `Outbox.Handlers.dispatch/1`: `core.notification.requested.v1` routes to `Notifications.Handler`, `core.approval.*.v1` to `Approvals.Handler`
- [x] `AtlasOrchestrator.Approvals.Signals` (`approval.changed`) + `Approvals.Handler`: dashboard on every approval event, named approver on request, named target on escalation (requester/role nudges already come via the notifications engine)
- [x] Tests: signals shape, handler routing/topics, and router dispatch for notification + approval events (24 orchestrator tests)

## H) Operational/infra polish
- [ ] Worker process docs/runbook update; ensure task to run api+worker+orchestrator+web
- [ ] Metrics/logs for dispatcher + sweeps (attempts, failures, lag)
- [ ] Contracts schema validation hook awareness (CI) — schemas already present

Current status: A core complete (service, HTTP, routes, policy lookup, four-eyes/two-person, notification fan-out, tests), B complete (worker expiry sweep, configurable poll/batch, tests) and G complete (Elixir outbox routing + approval signals). C–F, H remaining. Next: C (settings), D (files), E (audit views), F (exec dashboard).
