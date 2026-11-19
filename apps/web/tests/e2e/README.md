# E2E Tests (Playwright)

These tests drive the real browser against a real Go API and a disposable local
PostgreSQL database. They do not mock API responses, skip journeys, or use a
production identity.

## What CI does

The `playwright` job in `.github/workflows/ci.yml` starts a fresh Postgres
service, creates the Atlas database roles, migrates, seeds the configuration
catalogue, provisions one test-only identity, starts the API on `127.0.0.1:8080`
and the built Next.js app on `127.0.0.1:3000`, waits for both readiness checks,
runs this suite, uploads the HTML report plus every trace/screenshot/video, and
then stops both servers. A failure in any step still runs the artifact uploads
and cleanup.

## The E2E identity

`atlas-cli e2e provision` creates or refreshes exactly one account:

- email on a reserved example domain (default `e2e@example.com`)
- role `e2e_tester`, created only by this guarded command
- exactly the permissions the suite needs (no admin, user, role, settings,
  payroll, or approval rights)
- `mfa_required = false`, because privileged roles force TOTP enrolment and a
  browser test cannot complete enrolment

The command refuses to run unless **both** guards are set:

- `ATLAS_ENV=local`
- `ATLAS_E2E_PROVISION=1`

It also refuses non-example email domains and an existing account that holds any
other live role. Never point it at a deployed database.

## Local prerequisites

Use a throwaway database, not a shared or deployed one. From the repository
root, with a disposable Atlas database and its roles already present:

```bash
cd apps/api
export ATLAS_ENV=local
export ATLAS_DATABASE_URL='postgres://atlas_migrator:<local-password>@127.0.0.1:55433/atlas?sslmode=disable'
export ATLAS_MIGRATE_DATABASE_URL="$ATLAS_DATABASE_URL"
go run ./cmd/atlas-cli migrate up
go run ./cmd/atlas-cli seed

export ATLAS_E2E_PROVISION=1
export ATLAS_E2E_PASSWORD='E2eTestPassword123!'
go run ./cmd/atlas-cli e2e provision --email e2e@example.com --name 'Atlas E2E'
```

Run the API and web app in separate terminals:

```bash
# apps/api
ATLAS_ENV=local \
ATLAS_DATABASE_URL='postgres://atlas_api:<local-password>@127.0.0.1:55433/atlas?sslmode=disable' \
ATLAS_HTTP_ADDR=127.0.0.1:8080 \
go run ./cmd/atlas-api
```

```bash
# apps/web — verify with the same build CI serves
NEXT_PUBLIC_API_BASE_URL='' \
ATLAS_API_INTERNAL_URL=http://127.0.0.1:8080 \
npm run build
NEXT_PUBLIC_API_BASE_URL='' \
ATLAS_API_INTERNAL_URL=http://127.0.0.1:8080 \
./node_modules/.bin/next start -H 127.0.0.1 -p 3000
```

Use `next start` for a full verification run: `next dev` compiles routes on
demand, and the first compile of a page can outlast a navigation assertion even
though the app is healthy. CI always serves the production build, so that is
what the suite is tuned against. `npm run dev` is fine for focused debugging of
a single page.

`NEXT_PUBLIC_API_BASE_URL=''` keeps browser calls same-origin; Next rewrites
`/api/*` to `ATLAS_API_INTERNAL_URL`.

## Environment variables

| Variable | Default | Description |
|----------|---------|-------------|
| `PLAYWRIGHT_BASE_URL` | `http://localhost:3000` | Where the browser and API requests point. |
| `PLAYWRIGHT_MANAGE_WEB_SERVER` | unset | Set to `0` when the web server is started externally, as in CI. |
| `E2E_TEST_NAME` | `Atlas E2E` | Display name of the provisioned person; used for the dashboard greeting assertion. |
| `E2E_TEST_EMAIL` | `e2e@example.com` | Test account email. Must match the provisioned identity. |
| `E2E_TEST_PASSWORD` | `E2eTestPassword123!` | Test account password. Must match `ATLAS_E2E_PASSWORD`. |
| `ATLAS_ENV` | — | Must be `local` for provisioning and for non-Secure session cookies over HTTP. |
| `ATLAS_E2E_PROVISION` | — | Must be `1` to allow `atlas-cli e2e provision`. |
| `ATLAS_E2E_PASSWORD` | — | Password passed to the provisioner (a `--password` flag is also available). |
| `ATLAS_API_INTERNAL_URL` | `http://localhost:8080` | Server-side target for Next's `/api` rewrite. |
| `NEXT_PUBLIC_API_BASE_URL` | empty | Browser API base URL; empty means same-origin. |

## Running the suite

```bash
# One-time browser install
npx playwright install --with-deps chromium

# All tests
npm run test:e2e

# Against a web server you started on another port
PLAYWRIGHT_BASE_URL=http://localhost:3100 npx playwright test

# Single file / headed debugging
npx playwright test tests/e2e/auth.spec.ts
npx playwright test tests/e2e/auth.spec.ts --headed
```

`workers: 1` is deliberate: the specs create real records in one disposable
database. Traces, screenshots and videos are always captured; CI uploads them
whether the run passes or fails.

## Coverage

| File | Coverage |
|------|----------|
| `auth.spec.ts` | Password-only sign-in, sign-out, invalid credentials, auth guard. |
| `work.spec.ts` | Project creation, task creation, board placement, task detail. |
| `crm-contracts.spec.ts` | Organization, lead and contract creation through the current forms. |
| `finance.spec.ts` | Finance page access and a persisted expense claim. |
| `daily-report.spec.ts` | Filing/updating today's report and reading it back. |

Every run creates uniquely suffixed records, so retries and repeat runs against
the same throwaway database remain deterministic.
