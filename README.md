# Atlas

**Atlas** is the internal operating system of **SafariStack Solutions** — the single place where strategy becomes work, work becomes evidence, and evidence becomes decisions.

> The founding blueprint lives at [`docs/ATLAS.md`](docs/ATLAS.md). It is the source of truth for what Atlas is, how it is built, and in which order. When this repo and the blueprint disagree, one of them is wrong — fix it explicitly (an ADR, or a blueprint edit). Never let them drift silently.

---

## Stack

| Layer | Technology | Location |
|---|---|---|
| UI + thin BFF | Next.js (App Router, TypeScript) | `apps/web` |
| Core backend (source of truth) | Go (modular monolith) | `apps/api` |
| Orchestrator (time & motion) | Elixir/OTP + Phoenix Channels | `apps/orchestrator` |
| Data | PostgreSQL 16 (schema per module) | `infra/compose` |
| Edge | Caddy (TLS, routing, rate limits) | `infra/caddy` |

Architectural law (see `docs/ATLAS.md` §5.2 and `docs/adr/ADR-002`): **Go owns business data, rules, permissions and every external provider call. Elixir owns time and motion — timers, workflows, escalation and real-time signals — and asks Go to act. Next.js renders; it holds no business logic and never touches the database.**

## Repository layout

```
atlas/
├── apps/
│   ├── web/            # Next.js (App Router, TypeScript)
│   ├── api/            # Go module: atlas-api, atlas-worker, atlas-cli
│   └── orchestrator/   # Elixir/Phoenix app: AtlasOrchestrator
├── contracts/
│   ├── openapi/        # public.yaml, internal.yaml  (spec-first)
│   └── events/         # one JSON Schema per event type/version
├── infra/
│   ├── compose/        # local Docker Compose (Postgres, SeaweedFS, Mailpit)
│   ├── caddy/          # Caddyfile (edge routing)
│   └── docker/         # Dockerfiles for staging/prod images
├── docs/
│   ├── ATLAS.md        # the founding blueprint
│   ├── adr/            # ADR-001 … ADR-015
│   ├── runbooks/       # incident, restore, rotate secrets, onboard staff
│   └── policies/       # approval matrix, accountability, data retention
├── scripts/
└── Taskfile.yml        # the build/run entry point
```

## Prerequisites

- Go 1.24+ (`go version`)
- Elixir 1.18+ / Erlang/OTP 27+ (`elixir --version`)
- Node.js 20+ and npm (`node --version`)
- Docker Desktop with Compose (`docker compose version`)
- [Task](https://taskfile.dev) (`task --version`) — `winget install Task.Task`, `brew install go-task`, or `scoop install task`

## Quick start

```powershell
Copy-Item .env.example .env      # first time only
task up                          # start Postgres, SeaweedFS, Mailpit (+ edge)
task migrate                     # apply database migrations
task api:run                     # Go API on :8080   (separate terminal)
task orch:serve                  # Elixir orchestrator (separate terminal)
task web:dev                     # Next.js on :3000  (separate terminal)
```

Then check health:

```powershell
curl http://localhost:8080/api/v1/health
curl http://localhost:8080/api/v1/health/ready
```

Run `task` (no arguments) to list every available task.

## Phase 0 acceptance ("hello event")

Phase 0 is done when a `core.system.hello.v1` event, written by Go **inside a transaction**, is consumed by Elixir and delivered to the browser as a **live signal** — and CI blocks on failing tests. See `docs/ATLAS.md` §17 and Appendix A.

**Status: DONE.** Demonstrate it end to end:

```powershell
task up && task migrate
task api:run      # Go API on :8080
task orch:serve   # Elixir orchestrator on :4000
curl -X POST http://localhost:8080/api/v1/dev/hello -H "content-type: application/json" -d "{\"message\":\"hi\"}"
# 202 { ..., "seq": N, "note": "written to outbox; ..." }
```

The orchestrator consumes the event, advances `orchestrator.consumer_cursors`, records idempotency in `orchestrator.processed_events` and broadcasts a `signal` on `user:demo` / `exec:dashboard`. Watch it live in a browser at `http://localhost:4000/dev/hello`, or play the browser role headlessly:

```powershell
node apps/web/scripts/e2e-realtime.cjs
# connecting … joined user:demo ✓ → POST … -> 202 → SIGNAL on user:demo ✓
```

## Working agreement

- Branch names carry the Atlas task ref (e.g. `PRJ-2026-007/T-0142-...`).
- Permissions are enforced in Go, never in the UI.
- Every write use case runs in **one transaction** that includes its audit entry and its outbox event(s).
- Completion requires linked evidence.
- Configure policy; do not hard-code it.

## Status

Phase 0 — Groundwork. See `docs/ATLAS.md` §17 for phase exit criteria and `docs/adr/` for the decisions taken so far.
