# Phase 3 — CRM & contracts (backend plan)

Blueprint: `docs/ATLAS.md` §8.1 (CRM & client acquisition) and §8.3 (Contracts &
commitments). Boundary rules: ADR-002 (Go is the sole writer; Elixir
orchestrates), ADR-005 (transactional outbox), ADR-009 (realtime messages are
signals, not data), ADR-010 (money as integer minor units + `currency`, UUIDv7,
UTC).

This file is the running to-do list for Phase 3. Tick an item only when its code,
tests and migrations are in place.

Scope decision (D-Phase3): the WhatsApp and telephony providers stay **deferred**
(ADR-011, ADR-012). CRM records interactions manually and exposes the provider
seam as an interface (`crm.WhatsAppSender`, no-op by default) so a real Cloud API
adapter can be plugged in without a redesign. The website-form webhook is
implemented; the WhatsApp inbound webhook is a stub.

The web pages are built by the frontend team; this phase delivers the Go API,
outbox events and Elixir realtime wiring only.

## 0. Conventions (unchanged)

- Go is the authority. Elixir only reacts to outbox events.
- Every state change writes an audit row and, where the orchestrator or the
  browser must react, an outbox event **in the same transaction**.
- Optimistic concurrency: `version` column + `If-Match`/ETag on writes (§6.6).
- Keyset cursor pagination on listings.
- Human refs via `core.next_reference(type, year)` (LEAD-2026-0042, OPP-…,
  PROP-…, CTR-…, CR-…).
- No seeded business data; config tables (lead sources, pipeline stages) carry
  defaults only.

## 1. Schema

- [ ] Migration `00020_phase3_crm.sql` — schema `crm`: `lead_sources`,
      `pipeline_stages`, `organizations`, `contacts`, `opportunities`, `leads`,
      `interactions`, `followups`, `proposals` (+ grants, touch triggers).
- [ ] Migration `00021_phase3_contracts.sql` — schema `contracts`: `contracts`,
      `contract_versions`, `milestones`, `deliverables`, `obligations`,
      `sla_terms`, `change_requests` (+ grants, touch triggers).
- [ ] Config defaults for `lead_sources` and `pipeline_stages` in the seed
      package (idempotent, operator edits preserved).

## 2. CRM module (`internal/crm`)

- [ ] `crm.go` — types, enums, transitions, validation (lead lifecycle,
      interaction/follow-up rules, duplicate detection helpers).
- [ ] `store.go` — PGStore: organizations, contacts, opportunities, leads,
      interactions, follow-ups, proposals; versioned writes; keyset listings;
      ref allocation in-tx.
- [ ] `service.go` — Service: audit + outbox in-tx; lead assign/qualify/convert;
      opportunity win/lose; proposal send; follow-up complete; missed-follow-up
      sweep hook.
- [ ] `http.go` — Handler + views + error mapping; ETag/If-Match.
- [ ] Routes in `httpserver/router.go` (`registerCRM`), wiring in
      `cmd/atlas-api/main.go`.
- [ ] Website-form webhook `POST /webhooks/website-forms` (public, shared
      secret) creating a lead.

## 3. Contracts module (`internal/contracts`)

- [ ] `contracts.go` — types, lifecycle (Draft → … → Signed → Active → …),
      validation.
- [ ] `store.go` — PGStore for contracts, versions, milestones, deliverables,
      obligations, SLA terms, change requests.
- [ ] `service.go` — create from proposal; submit/sign (emits signed event and
      triggers handover + finance schedule later); change requests (both CEOs).
- [ ] `http.go` — Handler + views.
- [ ] Routes (`registerContracts`) + main wiring.

## 4. Events & contracts

- [ ] `outbox/crm.go`, `outbox/contracts.go` — event types + constructors.
- [ ] JSON schemas under `contracts/events/` for each new event.

## 5. Elixir wiring

- [ ] `crm/handler.ex` + `crm/signals.ex`; `contracts/handler.ex` +
      `contracts/signals.ex`.
- [ ] Dispatch in `outbox/handlers.ex`; tests.

## 6. Tests

- [ ] Go unit tests (validation/transitions) + DB integration tests.
- [ ] Schema-grants guard tests (mirror `TestWorkSchemaGrants`).
- [ ] Route registry / permission guard stays green.
- [ ] Orchestrator ExUnit tests.

## Open decisions

- **D9** WhatsApp number strategy & consent wording (Commercial CEO) — deferred;
  adapter seam in place.
- **D10** Telephony & call recording (Commercial CEO) — deferred; `interaction`
  records a call manually with `recording_file_id` null.
- Client obligations → contract risks surfacing (Phase 5/6).
