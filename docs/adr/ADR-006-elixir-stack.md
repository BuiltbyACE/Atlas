# ADR-006 — Elixir stack: Phoenix Channels (no LiveView), Oban, Ecto on its own schema

| | |
|---|---|
| **Status** | Proposed |
| **Date** | 2026-10-11 |
| **Deciders** | Internal & Technical CEO, Commercial CEO |
| **Phase** | Phase 0 |
| **Supersedes / Superseded by** | — |

## Context

Elixir is part of Atlas from Phase 1 with a narrow, valuable job: time-based orchestration, event processing, notification routing and real-time delivery (§5.5). It must not become a second backend (ADR-002) or be over-engineered before metrics demand it (§20).

## Decision

The orchestrator (`atlas-orchestrator`) uses:

- **Elixir + OTP** with supervision for long-lived workflow processes.
- **Phoenix** — **Channels and PubSub only; no LiveView** (the UI is Next.js, ADR-007).
- **Ecto** on its own `orchestrator` schema only (never business schemas).
- **Oban** for durable timers/jobs, stored in the orchestrator schema so they survive restarts.
- **Postgrex notifications** to LISTEN on `atlas_outbox` and wake the consumer.
- **`Req`** HTTP client to call Go's internal command API with a service token.
- **`:telemetry` + OpenTelemetry** for observability.

Responsibilities (§5.5): outbox consumer, workflow processes (rehydratable from `orchestrator.workflow_instances`), durable timers, notification routing, real-time hub on `/socket`, daily rhythms.

**It must not:** write business tables, call external providers directly, re-implement permission logic, or become the place where business rules hide (§5.5).

## Consequences

### Positive
- OTP supervision gives resilient, long-running workflows; Oban timers survive restarts.
- Phoenix Channels are a natural real-time hub for dashboards, boards and incident rooms.
- Ecto confined to its own schema keeps the write boundary (ADR-002) intact.

### Negative / trade-offs
- A second language and runtime to operate and staff.
- "Over-engineering Elixir/infra too early" is a named risk (§20) — mitigated by the narrow scope.
- No LiveView means UI concerns stay split across two stacks; rehydration and idempotency add complexity.

## Alternatives considered
- **Do all orchestration in Go** (cron / River) — rejected: Go should stay deterministic and not hold waiting workflows (§5.2).
- **LiveView for the UI** — rejected: UI is Next.js (§5.3, ADR-007).
- **A Temporal-style external engine** — rejected: heavier than the job requires.

## References
- `docs/ATLAS.md` §5.5
- `docs/ATLAS.md` §3.4
- `docs/ATLAS.md` §9
- `docs/ATLAS.md` §20
