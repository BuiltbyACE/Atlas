# ADR-001 — Modular monolith in Go as the single core backend

| | |
|---|---|
| **Status** | Proposed (accepted in this blueprint) |
| **Date** | 2026-10-11 |
| **Deciders** | Internal & Technical CEO, Commercial CEO |
| **Phase** | Phase 0 |
| **Supersedes / Superseded by** | — |

## Context

Atlas is built by a small team (2–4 engineers) who also deliver client work (§17, §20), so operational surface area is a first-class cost. Design principle 9 requires a **modular monolith first**, split only when evidence demands it (§2). The backend must own every business rule and every write (§2, §5.2). A microservice-per-module topology from day one would multiply deployment, observability and integration cost before any evidence justifies it.

## Decision

The Atlas core backend is a **single modular monolith written in Go** — one Go module producing three binaries built from the same code (§5.4):

- `atlas-api` — public REST API (`/api/v1`), internal command API (`/internal/v1`), webhooks (`/webhooks/*`).
- `atlas-worker` — durable job processing (River on PostgreSQL), health probes, scheduled data jobs.
- `atlas-cli` — migrations, seed data, audited break-glass operations.

Modules (`crm`, `work`, `contracts`, `finance`, `people`, `systems`, `units`, `intel`, …) each have `domain` / `store` / `service` / `http` layers. A module may call another **only** through that module's exported `service` interface — never its tables or `store`. Cross-module side effects that can be asynchronous go through **events**, not direct calls. Split into separate services only when evidence demands it.

## Consequences

### Positive
- One deployable, one CI pipeline, one in-process transaction boundary; audit entry and outbox event are written in the same transaction as the change (§5.4).
- Small-team operational load; matches the "modular monolith first" principle (§2).
- Strict internal module boundaries preserve a future extraction path if a module must split out.

### Negative / trade-offs
- A single process is the scaling unit until split; a hot module scales with the whole core.
- Boundary discipline can erode over time — needs code review and a checklist item (§20 "two backends drift").
- All business code shares one language and runtime; no per-module technology freedom.

## Alternatives considered
- **Microservices per module from the start** — rejected: cost/complexity not justified without evidence.
- **Polyglot backend** (business logic spread across languages) — rejected: violates "Go owns the truth" (§5.2).
- **Single binary with no internal module boundaries** — rejected: produces a big ball of mud.

## References
- `docs/ATLAS.md` §2
- `docs/ATLAS.md` §5.4
- `docs/ATLAS.md` §11
- `docs/ATLAS.md` §15
- `docs/ATLAS.md` §20
