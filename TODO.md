# To-do: finish foundation (backends/engines/orchestrations) so frontend can build on top

The sequence follows: Notifications done (Go), Elixir notification handler done. Approvals engine now has policy lookup, four-eyes/two-person enforcement and notification fan-out. Next: worker sweeps, Elixir wiring, settings, files, audit views, exec dashboard v1.

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
- [ ] Extend `atlas-worker` to run approvals sweeper: claim expired (`due_at < now()`, status requested/pending) with `FOR UPDATE SKIP LOCKED`, call `ExpireWithEvent`, emit `core.approval.expired.v1` + notifications
- [ ] Backoff/poll interval for sweeps (configurable)
- [ ] Tests

## C) Settings & policies
- [ ] `internal/settings/` (typed settings, get/set, versioned, audited)
- [x] Approval policy lookup used by approvals service
- [ ] Routes + tests

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
- [ ] Register `Notifications.Handler` in the outbox event router so `core.notification.requested.v1` actually pushes to `user:{id}`
- [ ] Add `core.approval.*.v1` handlers to push signals (approvals pending changed) to relevant topics
- [ ] End-to-end test with outbox event

## H) Operational/infra polish
- [ ] Worker process docs/runbook update; ensure task to run api+worker+orchestrator+web
- [ ] Metrics/logs for dispatcher + sweeps (attempts, failures, lag)
- [ ] Contracts schema validation hook awareness (CI) — schemas already present

Current status: A core complete (service, HTTP, routes, policy lookup, four-eyes/two-person, notification fan-out, tests). B–H remaining. Next: B (worker expiry sweep), then G (Elixir wiring), C, D, E, F.
