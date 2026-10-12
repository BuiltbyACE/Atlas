# ADR-002 — Go is the sole writer of business data and sole caller of external providers; Elixir orchestrates via commands

| | |
|---|---|
| **Status** | Proposed |
| **Date** | 2026-10-11 |
| **Deciders** | Internal & Technical CEO, Commercial CEO |
| **Phase** | Phase 0 |
| **Supersedes / Superseded by** | — |

## Context

Design principle 3 states: "Go owns the truth; Elixir owns time and motion" (§2). The greatest named technical risk is that "two backends drift into duplication" (§20). Atlas runs Go, Elixir and Next.js alongside each other with a shared PostgreSQL database, so without an explicit rule it is easy for Elixir or Next.js to write data, enforce permissions, or call providers — producing divergence, duplicated credentials and unauditable changes.

## Decision

The §5.2 boundary rules are **architectural law**; breaking one requires an ADR.

- **Go** is the only writer of business schemas, the owner of business rules, validation, state transitions, approvals, calculations and permissions, the owner of authentication/authorisation, the sole caller of external providers, the owner of inbound webhooks, and the owner of transactional background jobs (`atlas-worker`).
- **Elixir** owns time-based orchestration (reminders, SLA timers, escalations, daily-report prompts), event processing and cross-module reactions, real-time delivery, and notification routing/fan-out. It decides *when*, then calls Go's internal command API `/internal/v1/commands/*` to *do*.
- **Next.js** renders UI and fetches from Go; it never writes and holds no business rules.
- Elixir → Go calls authenticate with a service identity token (`sub=svc:orchestrator`, scoped permissions) plus an `Idempotency-Key` (§5.7).

Two explicit consequences: Elixir never leaks data (see ADR-009), and Go stays deterministic — anything that waits lives in Elixir.

## Consequences

### Positive
- One place for credentials, rate limits, retries, audit and business rules (§5.2).
- No dual-write drift: exactly one writer per business fact.
- Permission enforcement stays in Go, so it cannot be bypassed by another service.

### Negative / trade-offs
- Go is a chokepoint: every orchestrated action becomes a versioned, idempotent command API surface.
- Adds a network hop between Elixir decisions and their execution.
- Discipline required in review, or Elixir gradually accumulates logic.

## Alternatives considered
- Elixir writing business tables directly — rejected: two writers, divergence risk.
- Next.js server actions containing business rules — rejected: rules must live in the backend (§2).
- Shared rules library duplicated in Go and Elixir — rejected: two implementations drift.

## References
- `docs/ATLAS.md` §2
- `docs/ATLAS.md` §5.2
- `docs/ATLAS.md` §5.7
- `docs/ATLAS.md` §20
