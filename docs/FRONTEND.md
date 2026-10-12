# Atlas — frontend brief and system preview

**Audience:** the frontend team.
**Status:** Phase 1 foundation is in place. Authentication (Go) is built,
committed and running; the UI is still a shell. Your first task is the **login
journey against the real backend**.
**Authority:** [`docs/ATLAS.md`](ATLAS.md) is the source of truth. This document
is the frontend lens on it. Where the two disagree, ATLAS.md and the code win —
raise it.

---

## 1. The system in one page

Atlas is the internal operating system of SafariStack Solutions. Three
applications and one database:

```
        ┌──────────────┐
        │   Browser    │
        └──────┬───────┘
               │  https://<one origin>            (D1 [DECIDE])
        ┌──────▼───────┐  Caddy: TLS, routing, rate limits
        │    Caddy     │
        └───┬──────┬───┘
   /api/*   │      │   everything else, /socket
   ┌────────▼──┐  ┌▼───────────────┐
   │  Go API   │  │  Next.js (BFF) │  ← you are here (apps/web)
   │ apps/api  │  │   apps/web     │
   └────┬──────┘  └────────────────┘
        │                 │
        │  Postgres       │  Phoenix Channels (signals only)
   ┌────▼────────┐  ┌─────▼──────────┐
   │ PostgreSQL  │  │ Elixir/OTP     │  timers, workflows, escalation
   │  (schemas)  │  │ apps/orchestr. │
   └─────────────┘  └────────────────┘
```

Three rules that decide everything (ATLAS.md §5.2, ADR-002):

1. **Go owns business data, rules, permissions and every external provider
   call.** It is the only writer to business schemas.
2. **Elixir owns time and motion** — timers, workflows, escalation, realtime
   signals. It never writes business data.
3. **Next.js is UI plus a thin BFF.** Server components fetch from Go; route
   handlers/server actions only as thin pass-throughs. **No business logic, no
   direct database access, ever.**

Key consequences you will feel daily:

- **One origin.** Caddy routes `/api/*` to Go and everything else to Next.js, so
  cookies are first-party and there is no CORS. Locally, a `next.config.ts`
  rewrite proxies `/api/*` to Go (see §5).
- **The session cookie is the only client auth token.** Never store it in
  JavaScript; it is HttpOnly. Just let the browser send it.
- **Realtime carries signals, not data.** A push says "task 123 changed"; you
  then refetch task 123 from Go, which authorises you. (ADR-009)
- **Server components** call Go over the internal network, forwarding the
  cookie. **Client components** call `/api/*` directly.

Read only what you need, in this order: ATLAS.md §5.2–5.3, §6.1, §11, §13.1;
ADR-002, ADR-007, ADR-008, ADR-009.

---

## 2. Your first task: the login journey

**Status: built.** `apps/web/app/(auth)/login/` now runs the whole journey
below against Go (`login-flow.tsx`, calls in `lib/auth/flow.ts`), styled from
the design prototype: a milk-white page (`bg-milk`) with the SafariStack ink
splash, responsive from 320 px phones to wide desktops. It was verified end to
end against Postgres: phone and email sign-in, enrolment with a locally rendered
QR, recovery codes, TOTP and recovery-code verify, reload, redirects, sign-out.

### Definition of done

- A user signs in with **email or E.164 phone** plus password.
- If the account requires MFA, the same page collects a TOTP (or recovery) code
  and completes the sign-in.
- If the account has no MFA yet but the role is privileged, the page runs
  **enrolment** (show the `otpauth://` URI as a QR, confirm one code, show
  recovery codes once).
- After success the user lands on `/dashboard`, the shell renders their name,
  and reloading stays signed in.
- Signed-out: visiting a protected route redirects to `/login`.
- `/login` with a valid session redirects to `/dashboard`.
- Sign out works; the cookie is cleared.

### The flow (state machine)

```
                     POST /api/v1/auth/login  {identifier, password}
                                     │
             ┌───────────────────────┼────────────────────────┐
     200 {user, cookie}      200 {requires_mfa,         401 / 429
             │                 mfa_token,                     │
             │                 enrollment_required}           ▼
             │                       │                    show error
             │            ┌──────────┴───────────┐
             │   enrollment_required=true   enrollment_required=false
             │            │                        │
             │  POST /mfa/enroll            POST /api/v1/auth/mfa/verify
             │   → {secret, otpauth_url}          {mfa_token, code}
             │   show QR                                    │
             │            │                       200 {user, cookie}
             │   POST /api/v1/auth/mfa/activate             │
             │     {mfa_token, code}                       │
             │   201 {recovery_codes, cookie}              │
             └────────────┬───────────────────────────────┘
                          ▼
                    signed in → /dashboard
```

The **`mfa_token`** is a 5-minute challenge token issued by login. It is **not**
a session: no cookie is set until MFA completes. Treat it as opaque and keep it
in component state only.

### Deliberate gaps (no backend for them yet)

- **Forgot password** says plainly that self-service reset does not exist yet
  and to contact a system administrator. There is no reset endpoint and no
  email provider (§8); the UI does not pretend to send anything.
- **Remember me** from the prototype is omitted: session lifetime is fixed by
  Go (12 h idle / 7 d absolute), so a checkbox would be a promise nothing keeps.

---

## 3. The authentication contract (exact)

Base URL `http://localhost:8080`. All bodies are JSON. `identifier` is an email
address, or a phone number that normalises to E.164 (a bare Kenyan `07…`
becomes `+2547…`; the default country code is `+254`, pending D1/D2).

### 2.1 Endpoints

| Method | Path | Auth | Success |
|---|---|---|---|
| POST | `/api/v1/auth/login` | none | `200 {user}` **or** `{requires_mfa,…}` + cookie |
| POST | `/api/v1/auth/mfa/verify` | mfa_token | `200 {user}` + cookie |
| POST | `/api/v1/auth/mfa/enroll` | mfa_token | `200 {secret, otpauth_url}` |
| POST | `/api/v1/auth/mfa/activate` | mfa_token | `201 {recovery_codes}` + cookie |
| POST | `/api/v1/auth/logout` | session | `200 {status}` and clears the cookie |
| GET | `/api/v1/auth/me` | session | `200 User` |
| GET | `/api/v1/auth/sessions` | session | `200 {sessions: Session[]}` |
| DELETE | `/api/v1/auth/sessions/{id}` | session | `200 {status}` / `404` |
| POST | `/api/v1/auth/sessions/revoke-all` | session | `200 {revoked: n}` |
| POST | `/api/v1/auth/realtime-token` | session | `200 {token, expires_at}` |
| POST | `/api/v1/auth/invites` | session + `core.user.manage` | `201 {token, email, expires_at}` |
| POST | `/api/v1/auth/invites/accept` | none | `201 {id, email}` |

`GET /api/v1/`, `/api/v1/health`, `/api/v1/health/ready` also exist (system).

### JSON shapes

| Schema | Fields |
|---|---|
| `LoginRequest` | `identifier`, `password` |
| `LoginResponse` | `requires_mfa?`, `enrollment_required?`, `mfa_token?`, `user?` |
| `User` | `id`, `email`, `name`, `subject`, `person_id?`, `roles[]` |
| `MfaTokenRequest` | `mfa_token` |
| `MfaCodeRequest` | `mfa_token`, `code` |
| `MfaEnrollResponse` | `secret`, `otpauth_url` |
| `MfaActivateResponse` | `recovery_codes[]` |
| `Session` | `id`, `current`, `ip?`, `user_agent?`, `created_at`, `last_seen_at`, `expires_at` |
| `RealtimeToken` | `token`, `expires_at` |
| `AcceptInviteRequest` | `token`, `password` (min 12), `full_name?` |
| `InviteRequest` | `email`, `full_name?`, `employment_type?`, `role_grants[]`, `ttl_hours?` |
| `GrantSpec` | `role_key`, `scope_type` (`company`\|`business_unit`\|`department`\|`project`\|`own`), `scope_id?` |

The full spec is `contracts/openapi/public.yaml`. Generate a typed client from it
(§5) — do not hand-write these types.

### The session cookie

- Name **`atlas_session`**; `HttpOnly`, `SameSite=Lax`, `Path=/`.
- `Secure` is off locally and **on** outside local development — a staging run
  over plain HTTP will silently drop the cookie.
- Idle timeout **12 h**; absolute lifetime **7 d** (defaults). The cookie's
  `Expires` matches the absolute deadline.
- You cannot read it from JS, and you do not need to: the browser sends it to
  same-origin `/api/*`. In server components use `cookies()`; `lib/api/client.ts`
  already forwards it.

### Errors — RFC 9457

Every non-2xx is `application/problem+json` (ATLAS.md §11):

```json
{
  "type": "https://atlas.safaristack.co.ke/problems/unauthenticated",
  "title": "Unauthorized",
  "status": 401,
  "detail": "Authentication is required.",
  "instance": "/api/v1/auth/me",
  "code": "UNAUTHENTICATED",
  "request_id": "01a12fc4-2496-..."
}
```

- `code` is the stable, machine-readable value — **switch on `code`, never on
  `detail`**.
- `errors[]` carries field-level validation when present.
- `request_id` (also the `X-Request-Id` header) — surface it in bug reports.
- Replies include `X-Frame-Options: DENY`, `X-Content-Type-Options: nosniff`,
  `Referrer-Policy: strict-origin-when-cross-origin`.

Codes you will meet on the auth surface:

| `code` | HTTP | Meaning |
|---|---|---|
| `UNAUTHENTICATED` | 401 | Bad credentials / no session (deliberately the same for both) |
| `RATE_LIMITED` | 429 | Too many failed attempts; back off |
| `MFA_REQUIRED` | 400 | Tried to verify before enrolling |
| `MFA_ALREADY_ACTIVE` | 409 | Enrolment attempted twice |
| `VALIDATION_FAILED` | 400 | Malformed body / password under 12 chars |
| `UNSUPPORTED_MEDIA_TYPE` | 415 | A JSON endpoint received a body not sent as `application/json` |
| `NOT_FOUND` | 404 | Unknown session id, unknown route |
| `INTERNAL_ERROR` | 500 | Log it with the `request_id` |

### CSRF

There is **no CSRF token**. Protection is `SameSite=Lax` plus the requirement
that mutations be JSON with `Content-Type: application/json` (a cross-site form
cannot do that without an origin check). Go enforces the media type in
`httpx.DecodeJSON`: anything else gets `415 UNSUPPORTED_MEDIA_TYPE`, even a
`text/plain` body that happens to be valid JSON (the classic form-CSRF shape).
Consequence for you:

- **Use `fetch`/XHR with JSON.** Do not submit credential forms as native HTML
  `POST` form bodies.
- `apiFetch` in `apps/web/lib/api/client.ts` already serialises JSON.

---

## 3b. A worked example (curl)

```bash
# 1. sign in — cookies land in the jar
curl -sS -c jar.txt -H "Content-Type: application/json" \
  -d '{"identifier":"you@safaristack.co.ke","password":"..."}' \
  http://localhost:8080/api/v1/auth/login

# 2. who am I
curl -sS -b jar.txt http://localhost:8080/api/v1/auth/me

# 3. sign out
curl -sS -b jar.txt -X POST http://localhost:8080/api/v1/auth/logout
```

Against a working account this prints the `User`, then `{"status":"signed_out"}`.

---

## 4. Running it locally

Full detail is in [`docs/runbooks/local-development.md`](runbooks/local-development.md).
The short version:

```powershell
task up            # Postgres :55433, SeaweedFS S3 :9000, Mailpit
task migrate       # Go schema (currently v3)
task seed          # roles + permissions (idempotent)
task keygen        # ONLY if you want stable chat/realtime keys in .env
task api:run       # Go API on :8080
cd apps/web; npm install; npm run dev   # Next.js on :3000
```

The web app reaches Go through the `next.config.ts` rewrite, so in the browser
`/api/v1/...` is same-origin on `:3000` — the cookie and `SameSite=Lax` behave
exactly as in production. You do **not** need CORS locally.

Keep `NEXT_PUBLIC_API_BASE_URL` **empty** (root `.env`, which `task web:dev`
loads, and `apps/web/.env.local`). Set to `http://localhost:8080` it sends the
browser to Go cross-origin, where there is no CORS, and sign-in fails.

### You need an account

Atlas has no self-service sign-up, and every normal account starts from an
*invitation* — which itself requires an authenticated user. So an operator
creates the first account:

```powershell
cd apps/api
$env:ATLAS_BOOTSTRAP_PASSWORD = "a-real-password-of-12-plus"
go run ./cmd/atlas-cli bootstrap `
  --email "you@safaristack.co.ke" `
  --name  "Your Real Name" `
  --role  ceo_internal
```

Then sign in with that email and password. Notes:

- The role key comes from ATLAS.md §3.2 (`ceo_commercial`, `ceo_internal`,
  `finance_officer`, `sales_rep`, `project_lead`, `engineer`, `staff`,
  `unit_manager`, `auditor`, `system_admin`). Default: `system_admin`.
- Privileged roles must enrol TOTP — the login response will say
  `enrollment_required: true`, and your UI must handle it (§2).
- **Use real people and real passwords.** No mock business data, ever.

### Generating the typed client

`generate:api` is a placeholder. Target behaviour:

```jsonc
// apps/web/package.json
"generate:api": "openapi-typescript ../../contracts/openapi/public.yaml -o lib/api/generated/schema.d.ts"
```

`lib/api/generated/` is gitignore'd except its README; CI is intended to run a
generation-diff check. Until it is wired, `apiFetch` plus the hand types in
`lib/auth/session.ts` are enough for the login journey.

---

## 5. Where things live, and what is broken

```
apps/web/
├── app/
│   ├── layout.tsx                 # root <html>
│   ├── page.tsx                   # redirects to /dashboard
│   ├── (auth)/login/page.tsx      # ← REWRITE THIS (currently a fake form)
│   └── (app)/                     # authenticated shell
│       ├── layout.tsx             # sidebar + header; calls requireUser()
│       ├── dashboard/page.tsx     # placeholder cards
│       └── settings/page.tsx      # placeholder
├── lib/
│   ├── api/client.ts              # apiFetch: JSON, cookie forwarding, ApiError
│   ├── api/generated/             # generated client goes here
│   ├── auth/session.ts            # getCurrentUser / requireUser
│   └── realtime/socket.ts         # Phoenix client + hooks (done)
├── next.config.ts                 # /api/* rewrite to Go
└── README.md
```

**Already true**

- `lib/realtime/socket.ts` is implemented: realtime token fetch, `/socket`
  connection, `joinTopic`/`useRealtimeSignals`/`useExecDashboard`/
  `useUserChannel`, and signal-payload validation.
- `lib/api/client.ts` (`apiFetch`, `ApiError`) forwards the cookie on the server
  and throws `ApiError` carrying the Problem body.
- `(app)/layout.tsx` calls `requireUser()` and redirects to `/login`.

**Fixed**

- `lib/auth/session.ts` read `/api/v1/me`; the real path is
  `/api/v1/auth/me`. Its `CurrentUser` shape now matches the contract (no
  invented `permissions`/`scopes`).
- `ProblemDetails` in `lib/api/client.ts` typed `errors` as a map; Go sends
  `[{field, code, detail}]`. It also now carries `request_id`.

**Done**

1. `(auth)/login` is a real client flow (§2), with redirect-if-authenticated.
2. A sign-out control in the shell (`components/sign-out-button.tsx`).

**Yours to do**

1. `(app)/settings` — the session-management page (`GET /auth/sessions`,
   `DELETE /auth/sessions/{id}`, `POST /auth/sessions/revoke-all`).
2. Accept-invite page (`POST /api/v1/auth/invites/accept`).
3. Honest stubs for the module routes in the sidebar (`/crm`, `/work`, …):
   today they 404.
4. Wire `generate:api` and replace hand types with generated ones.

---

## 6. Full system preview

This is the whole product, so you can see where the modules sit. **Only the
auth surface and health endpoints exist today.** Everything below is specified in
ATLAS.md §8 and will arrive module by module — build the shell and the login
journey now, and **never invent data** to fill a screen.

The sidebar in `(app)/layout.tsx` already lists the modules.

| Module | Route | API prefix | Primary persona | First screens |
|---|---|---|---|---|
| Dashboard | `/dashboard` | (computed from all) | CEO | cash, pipeline, delivery health, approvals, incidents |
| CRM | `/crm` | `/api/v1/crm/*` | Commercial CEO, Sales | My day call list, lead record, pipeline board, WhatsApp inbox |
| Work | `/work` | `/api/v1/work/*` | Project Lead, Engineer | My work, project board, project overview, blockers, review queue |
| Contracts | `/contracts` | `/api/v1/contracts*` | Commercial CEO | register, contract record, obligations, change requests |
| Finance | `/finance` | `/api/v1/finance/*` | Finance Officer, CEOs | cash position, receivables ageing, expenses, profitability |
| People | `/people` | `/api/v1/people/*`, `/api/v1/daily-reports` | Internal CEO, all staff | availability, daily reports, leave, reviews |
| Systems | `/systems` | `/api/v1/systems/*` | Internal CEO | systems register, checks, incident timeline |
| Units | `/units` | `/api/v1/units/*` | Unit Manager, CEOs | unit indicators, sync status |
| Intel | `/intel` | `/api/v1/intel/*` | CEOs | daily brief, decisions, risks, metrics |
| Settings | `/settings` | (core) | System Administrator | users, roles, settings, sessions |

Plus, across every module: notifications, approvals, audit, files, and search —
the same patterns repeated.

**What "full preview" means for you now:** build the **shell** (chrome,
navigation, layout primitives) and the **auth journey**. Stub module routes with
an honest empty state ("Not built yet — arriving in Phase X"), not with fake
records.

### Personas and scoping (§3.2)

- `ceo_commercial` / `ceo_internal` — company-wide.
- `finance_officer`, `sales_rep`, `project_lead`, `engineer`, `staff` — mostly
  own/assigned scopes.
- `unit_manager` — one business unit only.
- `auditor` — read-only.
- `system_admin` — technical admin, **not** an approval role.

Permissions are checked by Go on every request. The UI may *hide* what a user
cannot do, but hiding is never the control — Go is. `User.roles` is what you
have today; a permissions list is on the roadmap.

### Realtime topics (ADR-009)

- `user:{id}` — per-user notifications, approvals, escalations. (`user_socket`
  strips the `user:` prefix.)
- `exec:dashboard` — CEOs only; signals for the exec dashboard.

Signals look like `{type, entity, id, change, seq?}`. **Refetch from Go.** The
helpers in `lib/realtime/socket.ts` already do the token dance
(`POST /api/v1/auth/realtime-token`, 5-minute Ed25519 token, verified by Elixir
against Go's JWKS).

Two caveats:

- `next.config.ts` rewrites do **not** proxy WebSockets. In dev, the browser
  connects straight to `NEXT_PUBLIC_REALTIME_URL` (the orchestrator on `:4000`);
  in production Caddy proxies `/socket`.
- For a socket to authenticate, the **Go API must be running** (Elixir fetches
  its JWKS from `http://localhost:8080/internal/v1/.well-known/jwks.json`).

---

## 7. Guardrails

- **No business logic in Next.js.** If you are tempted to compute a status,
  validate a rule, or "just" query the database — stop. It belongs in Go.
- **No mock business records.** Real people, real data. A screen with no data
  yet should say so, not show invented rows.
- **Never render a realtime payload as data.** Refetch.
- **TypeScript strict**, Tailwind for styling, App Router. Match the surrounding
  code; the existing files show the house style.
- **Accessibility and honesty**: keyboard-navigable, labels on inputs, and
  error text that the user can act on. Finance and people screens are sensitive
  (§13).
- **Server components by default; `"use client"` only where you need state or
  event handlers.**

## 8. Open decisions that touch the UI

Tracked in ATLAS.md §21. Most relevant to you:

- **D1 — domain** (`atlas.safaristack.co.ke` appears already in Problem `type`
  URIs). **D2 — hosting/region** (latency to Kenya).
- **UI kit [DECIDE]** — Tailwind is chosen; a component kit (e.g. shadcn/ui) is
  proposed in ATLAS.md §15 but not decided.
- **Email provider** (invitations, password resets) — not yet chosen; the UI
  should not assume a delivery channel.
- The default **country code `+254`** for phone sign-in is pending D1/D2.

## 9. Suggested order for the frontend team

1. Login (`email`/`phone` + password) against the real backend.
2. MFA verify, then enrolment (QR + recovery codes).
3. Sign-out and the session list in `/settings`.
4. Invite acceptance page (needs a `core.user.manage` account to issue invites).
5. Wire `generate:api` and migrate to generated types.
6. Dashboard and the authenticated shell polish — with honest empty states —
   ready for the first data modules.
