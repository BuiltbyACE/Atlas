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

## Sign in

**Status: built.** Atlas signs a person in with an email address or an E.164
phone number plus an argon2id password. Privileged roles then complete a TOTP
challenge — an authenticator code, or a recovery code if the app is lost — and a
first-time privileged user enrols with a QR code rendered in the browser (the
secret never leaves the page) and is shown one-time recovery codes. It is a
milk-white page over the SafariStack ink splash, responsive from 320 px phones to
wide desktops.

| Desktop | Phone |
|---|---|
| ![Atlas sign-in on desktop](<images/Atlas — Sign in.png>) | ![Atlas sign-in on a phone](<images/Atlas — Sign in (phone).png>) |

The flow lives in `apps/web/app/(auth)/login/` and talks JSON to the Go API;
[`docs/FRONTEND.md`](docs/FRONTEND.md) documents the journey and the contract.

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
task up                          # start Postgres, SeaweedFS, Mailpit (waits until healthy)
task migrate                     # Go migrations (goose)
task orch:migrate                # orchestrator migrations (Ecto)
task seed                        # roles + permission catalogue (§3.2, §6.2)
task bootstrap -- --email you@safaristack.co.ke --name "Your Name" --role ceo_internal   # first account, once
task api:run                     # Go API on :8080            (separate terminal)
task orch:serve                  # Elixir orchestrator :4000  (separate terminal)
task web:dev                     # Next.js on :3000           (separate terminal)
```

Then check health:

```powershell
curl http://localhost:8080/api/v1/health
curl http://localhost:8080/api/v1/health/ready
```

Run `task` (no arguments) to list every task. The full local-development guide —
ports, startup order, reset and troubleshooting — is in
[`docs/runbooks/local-development.md`](docs/runbooks/local-development.md).

## Foundation (Phase 1)

The foundation is built before any business module. `docs/ATLAS.md` §6 is the
source of truth: *"The foundation is Phase 1. Every later module depends on it."*

**In place now**

- **Event spine** (§7.1): `outbox.events` + a NOTIFY trigger. Go writes inside a
  transaction; Elixir consumes and signals the browser.
- **Database foundation** (§10.1, §10.3): `people` (departments, persons),
  `core` (user accounts, sessions, roles, permissions, role grants, service
  identities, approvals, settings, sequences, notifications, files) and the
  append-only `audit.events`. Invariants live in the database — UUIDv7 keys,
  `citext`, `updated_at` triggers, an append-only audit trigger, and
  human-reference sequences (`core.next_reference` → `LEAD-2026-0042`).
- **Least-privilege DB roles** (§10.1, §13.3): `atlas_migrator`, `atlas_api`,
  `atlas_worker`, `atlas_orchestrator`, `atlas_readonly`.
- **Configuration seed** (`task seed`): the §3.2 roles and the §6.2 permission
  catalogue, written idempotently. No mock business records and no people.
- **Go API skeleton** (§11): every route is declared in a routing registry with
  its method, pattern, access level and required permission; a test walks the
  mounted router and fails the build if a route is missing from the registry or
  names a permission the seed does not contain. `httpx` supplies RFC 9457
  problem responses, cursor pagination, ETags and concurrency control.
- **Authentication** (§6.1, ADR-008): sign in with an email address or an E.164
  phone number plus an argon2id password; TOTP MFA for privileged roles; opaque
  sessions stored only as a hash, with idle and absolute expiry; and 5-minute
  Ed25519 realtime tokens whose JWKS the orchestrator fetches and verifies.
- **Sign-in UI** (§6.1): the Next.js login journey — email/phone + password, TOTP
  and recovery-code verification, first-run enrolment with a browser-rendered QR
  and one-time recovery codes, and sign-out from the authenticated shell. Go
  authorises every call; the UI holds no rules.
- **First-account bootstrap** (`task bootstrap`): because Atlas has no
  self-service sign-up, an operator creates the first real account by hand;
  privileged roles are created with MFA required.

**Next pillars:** idempotency keys and rate limiting on the write path, then the
governed domain modules.

### Event-spine smoke test

`core.system.hello.v1` is no longer a demo endpoint; it is an operator canary for
the event spine:

```powershell
task smoke:event
# → {"event_id":"…","type":"core.system.hello.v1","seq":N}
```

Go writes it inside a transaction, the orchestrator consumes it, advances
`orchestrator.consumer_cursors`, records idempotency in
`orchestrator.processed_events` and broadcasts a `signal` on `user:{id}` /
`exec:dashboard`.

## Working agreement

- Branch names carry the Atlas task ref (e.g. `PRJ-2026-007/T-0142-...`).
- Permissions are enforced in Go, never in the UI.
- Every write use case runs in **one transaction** that includes its audit entry and its outbox event(s).
- Completion requires linked evidence.
- Configure policy; do not hard-code it.

## Status

Phase 1 — foundation. The event spine, database foundation, configuration seed,
Go API skeleton, authentication and the sign-in UI are in place; idempotency keys
and rate limiting on the write path, then the governed modules, come next. See
`docs/ATLAS.md` §17 for phase exit criteria, `docs/ATLAS.md` §6 for the
foundation, and `docs/adr/` for the decisions taken so far.
