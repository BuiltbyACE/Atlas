# ADR-004 — PostgreSQL schema-per-module and cross-schema FK policy

| | |
|---|---|
| **Status** | Proposed |
| **Date** | 2026-10-11 |
| **Deciders** | Internal & Technical CEO, Commercial CEO |
| **Phase** | Phase 0 |
| **Supersedes / Superseded by** | — |

## Context

Atlas is a modular monolith (ADR-001) whose module boundaries must be enforced somewhere real, not only by convention. §10.1 defines the data architecture and §5.6 the database roles. Without an explicit schema and foreign-key policy, modules would reach into each other's tables and the database would silently become the integration layer.

## Decision

One PostgreSQL cluster (16+), one database `atlas`, **one schema per module** (§10.1):

`core`, `people`, `crm`, `contracts`, `work`, `finance`, `systems`, `units`, `intel`, `integrations`, `audit`, `outbox`, `orchestrator`.

Distinct database roles (§5.6): `atlas_api` (read/write business schemas), `atlas_worker`, `atlas_orchestrator` (read/write `orchestrator`; **read-only** on `outbox`), `atlas_readonly` (reporting/BI), `atlas_migrator`.

**Cross-schema foreign keys are allowed only** to:
- `core` and `people` (identity of users/persons), and
- shared anchors: `crm.organizations` (client), `contracts.contracts`, `work.projects`.

Everything else connects by **events or service calls**. `audit` is append-only, enforced by grants and a trigger (§10.3). Row-level security is not required initially (authorisation lives in Go).

## Consequences

### Positive
- Module boundaries are visible and enforceable through database grants and FK policy.
- Supports least-privilege roles per process (`api`, `worker`, `orchestrator`, `readonly`).
- Each schema has an obvious owner; extraction path retained if a module ever splits.

### Negative / trade-offs
- Cross-module queries require service calls or events rather than ad-hoc joins — more code.
- Shared anchors still create real coupling; changes to those tables ripple.
- Migration ordering across schemas and reporting joins need care (mitigated by `atlas_readonly` and `intel` snapshots).

## Alternatives considered
- **One flat schema with naming conventions** — rejected: boundaries not enforceable by grants.
- **Separate database per module** — rejected: contradicts the single-database modular monolith (§5.6).
- **RLS-enforced multi-tenancy** — rejected as unnecessary now; authorisation lives in Go (§5.6).

## References
- `docs/ATLAS.md` §10
- `docs/ATLAS.md` §5.6
- `docs/ATLAS.md` §6.6
