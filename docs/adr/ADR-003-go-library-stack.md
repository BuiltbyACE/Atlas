# ADR-003 — Go library stack

| | |
|---|---|
| **Status** | Proposed |
| **Date** | 2026-10-11 |
| **Deciders** | Internal & Technical CEO, Commercial CEO |
| **Phase** | Phase 0 |
| **Supersedes / Superseded by** | — |

## Context

The blueprint names an initial Go library set to be confirmed "[DECIDE in ADR-003]" (§5.4). Atlas is spec-first (§11) and uses PostgreSQL as its only required infrastructure dependency, including for jobs. Phase 0 needs a settled, stable stack so the skeleton and CI can be built once.

## Decision

Adopt the following Go stack:

- **HTTP:** `net/http` + `chi` router (plus the standard middleware package).
- **Database driver:** `pgx/v5`.
- **Queries:** `sqlc` — typed Go generated from SQL query files in `db/queries/`.
- **Migrations:** `goose` (SQL migrations in `db/migrations/`).
- **Jobs:** `River` — durable job queue on PostgreSQL (used by `atlas-worker`).
- **API codegen:** `oapi-codegen`, spec-first from OpenAPI 3.1 files in `contracts/openapi/`.
- **Logging:** `slog` — structured JSON.
- **Telemetry:** OpenTelemetry SDK.
- **Password hashing:** `argon2id` (see ADR-008).
- **Validation:** `go-playground/validator` **or** hand-written validation (module by module).

## Consequences

### Positive
- Boring, well-supported libraries; PostgreSQL remains the only infrastructure dependency (River needs no Redis/broker).
- sqlc + oapi-codegen enforce the spec-first and typed-boundary principles (§11).
- One logging and one tracing standard across the core (`slog` + OpenTelemetry).

### Negative / trade-offs
- sqlc, goose and oapi-codegen add generation steps and require CI diff checks (§16.2).
- River is PostgreSQL-backed, so heavy job throughput competes with query load on the same database.
- Leaving validation library choice open can cause inconsistency if not settled quickly.

## Alternatives considered
- **gRPC/protobuf** instead of REST + OpenAPI — rejected: browser and partner access favour HTTP/JSON (§11).
- **ORM (GORM/ent)** instead of sqlc — rejected: less transparent SQL and weaker boundary enforcement.
- **Redis-backed job broker (e.g. Asynq)** — rejected: adds infrastructure before evidence; River suffices (§5.4).
- **`gin`/`echo` router** — rejected: `chi` is lighter and closer to `net/http`.

## References
- `docs/ATLAS.md` §5.4
- `docs/ATLAS.md` §11
- `docs/ATLAS.md` §16.2
