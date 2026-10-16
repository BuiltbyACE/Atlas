# Runbook â€” local development

How to run Atlas on a developer machine: infrastructure in Docker, applications
on the host with hot reload. This is the runbook to follow when standing up a
fresh machine or when something will not start.

Blueprint references: Â§16 (environments and infrastructure), Â§10 (data),
Appendix A (Phase 0/1).

## Verified toolchain

Checked with `task doctor` on the reference machine (Windows):

| Tool | Version |
|---|---|
| Go | 1.27.0 |
| Erlang/OTP | 29 (erts 17.1) |
| Elixir / Mix | 1.20.4 |
| Node.js | 24.21.0 |
| npm | 11.19.0 |
| Docker | 29.8.0 |
| Docker Compose | v5.5.1 |
| Task | 3.54.0 |

Optional tooling (not installed by default; only needed for specific tasks):

- `sqlc` â€” only for `task generate` (SQL â†’ Go codegen). The module builds and
  runs without it.
- `staticcheck` â€” only for `task api:lint`'s second command. `go vet` always runs.

`goose` is **not** required on the PATH: migrations run through the `atlas-cli`
binary, which embeds goose as a library.

## Ports

| Service | Host | Container | Notes |
|---|---|---|---|
| PostgreSQL | 55433 | 5432 | Deliberately not 5432, to avoid clashing with a local Postgres. |
| SeaweedFS S3 API | 9000 | 8333 | `S3_ENDPOINT=http://localhost:9000` |
| SeaweedFS master | 9333 | 9333 | Cluster status. |
| SeaweedFS filer | 8888 | 8888 | Bucket/object browser. |
| Mailpit SMTP | 1025 | 1025 | |
| Mailpit UI | 8025 | 8025 | http://localhost:8025 |
| Go API | 8080 | â€” | Runs on the host. |
| Elixir orchestrator | 4000 | â€” | Runs on the host. |
| Next.js web | 3000 | â€” | Runs on the host. |
| Caddy (optional `edge` profile) | 80 / 443 | 80 / 443 | `docker compose --profile edge up -d` |

Apps run on the host and reach Docker services on `localhost` at the ports above.

## Startup order

```powershell
Copy-Item .env.example .env   # first time only
task doctor                   # confirm tooling and .env
task up                       # 1. infrastructure (waits until healthy, inits the bucket)
task migrate                  # 2. Go schema (goose)
task orch:migrate             # 3. orchestrator schema (Ecto, `orchestrator` prefix)
task seed                     # 4. role/permission catalogue (Â§3.2, Â§6.2)

task api:run                  # 5. Go API          (:8080)
task orch:serve               # 7. orchestrator    (:4000)  (own terminal)
task api:worker               # 6. Go worker        (runs approvals expiry/sweeper + notifications dispatcher)  (own terminal)
task orch:serve               # 7. orchestrator    (:4000)  (own terminal)
task web:dev                  # 8. Next.js web     (:3000)  (own terminal)
```

`task up` is genuinely one command: it starts the three infrastructure services,
waits for their health checks, then creates the object-storage bucket (retrying
until the SeaweedFS S3 gateway is ready).

## Health checks

```powershell
curl http://localhost:8080/api/v1/health        # liveness (process up)
curl http://localhost:8080/api/v1/health/ready  # readiness (DB reachable)
curl http://localhost:4000/health               # orchestrator liveness
curl http://localhost:4000/health/ready         # orchestrator readiness (Repo)
```

`task ps` shows container health. All three infrastructure services expose a
Docker healthcheck.

## Authentication and realtime tokens

Sign-in is a session cookie (opaque token, stored hashed). Realtime tokens are
signed by Go and verified by the orchestrator against Go's JWKS (ADR-008).

Local development needs no key setup: when `ATLAS_ENCRYPTION_KEY` and
`ATLAS_REALTIME_PRIVATE_KEY` are empty, `atlas-api` generates ephemeral keys at
boot. To keep the same keys across restarts (for example so the orchestrator's
cached JWKS keeps verifying), generate stable ones once:

```powershell
task keygen          # prints ATLAS_REALTIME_PRIVATE_KEY and ATLAS_ENCRYPTION_KEY
# paste the output into .env, then restart the API
```

The orchestrator fetches keys from `ATLAS_JWKS_URL`
(`http://localhost:8080/internal/v1/.well-known/jwks.json`), so the API must be
running before a socket can be authenticated. The URL is served only inside the
deployment â€” Caddy returns 404 for `/internal/*`.

### The orchestrator's service credential

When the orchestrator calls Go's internal API (for example the realtime join
check `GET /internal/v1/authz/check`), it presents a static service token. Because
the token is signed with the API's key, set `ATLAS_REALTIME_PRIVATE_KEY` first
(run `task keygen` and paste it into `.env`), then create the identity and mint a
token once:

```powershell
task service -- create --id svc:orchestrator `
  --permission core.authz.check --permission core.notification.manage
task service -- token --id svc:orchestrator    # paste into ATLAS_SERVICE_TOKEN
```

Re-mint the token whenever the signing key rotates (or the token's
`ATLAS_SERVICE_TOKEN_TTL`, 30 days by default, lapses).

Inspect the published keys:

```powershell
curl http://localhost:8080/internal/v1/.well-known/jwks.json
```

Sign in with an email address (`ceo@safaristack.co.ke`) or an E.164 phone
number (`+254â€¦`). Accounts that require MFA answer with a short-lived challenge
token instead of a session.

### Creating the first account

Atlas has no self-service sign-up (ATLAS.md Â§6.1): every ordinary account comes
from an invitation, and an invitation can only be created by an authenticated
user. The very first account is therefore created deliberately by an operator,
with a real name and password â€” never a fixture:

```powershell
$env:ATLAS_BOOTSTRAP_PASSWORD = "a-long-password"
task bootstrap -- --email you@safaristack.co.ke --name "Your Name" --role ceo_internal
```

`--role` defaults to `system_admin`; `--phone` and `--scope` are optional.
Privileged roles (the two CEOs, `finance_officer`, `system_admin`, `auditor`)
are created with `mfa_required`, so the first sign-in is a TOTP enrolment. The
command refuses to overwrite an existing email.

## Daily commands

| Task | Purpose |
|---|---|
| `task up` / `task down` | Start / stop infrastructure. |
| `task ps` / `task logs` | Inspect containers. |
| `task psql` | Open `psql` on the Atlas database. |
| `task migrate` / `task migrate:status` | Go migrations. |
| `task orch:migrate` | Orchestrator (Ecto) migrations. |
| `task seed` | Write the role/permission catalogue (idempotent). |
| `task keygen` | Print fresh local secrets for `.env` (Ed25519 + AES-256). |
| `task bootstrap` | Create the first account (a real person), then sign in. |
| `task test` | Go + Elixir tests. |
| `task lint` | Go vet, Credo, ESLint + typecheck. |

## Troubleshooting

**`task up` fails on `storage:init` with `exit status 56`.**
The SeaweedFS S3 gateway starts a few seconds after the master. `storage:init`
now retries for up to 30 seconds, so this should self-resolve. If it persists,
`docker logs atlas-storage` and re-run `task storage:init`.

**Port already in use.**
Change `POSTGRES_PORT` in `.env` and update the port in
`ATLAS_DATABASE_URL`, `ATLAS_MIGRATE_DATABASE_URL` and
`ORCHESTRATOR_DATABASE_URL` to match, then `task down && task up`.

**`ORCHESTRATOR_DATABASE_URL is not set` when running `mix` directly.**
Run Elixir commands through Task (`task orch:serve`, `task orch:test`), which
loads the repo-root `.env`; or export the variable yourself.

**Reset everything.**
`task nuke` removes the containers **and** volumes (Postgres data and object
storage). Then `task up` and re-run the migrate/seed steps. This is destructive
to local data only.

**Stale images.**
`docker compose -f infra/compose/docker-compose.yml pull` then `task up`.
