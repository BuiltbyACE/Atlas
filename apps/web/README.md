# @atlas/web

The Atlas web app: the UI for **Atlas, the SafariStack Company Operating
System**. Next.js (App Router) + TypeScript + Tailwind CSS v3.

This is the **Phase 0 groundwork** shell: an app-router skeleton, an auth
layout, a generated-API-client seam and a Phoenix Channels hook. It contains
**no business logic** - Go owns all writes, rules and permissions (blueprint
section 5.2).

> **Frontend team, start here:** [`docs/FRONTEND.md`](../../docs/FRONTEND.md) is
> the full brief and system preview. Authentication (Go) is built and running,
> and the sign-in journey is live in `app/(auth)/login/`. Next: the sessions
> page, invite acceptance, honest module-route stubs, and the generated API
> client.

## Architecture in one paragraph

One origin, two services. In production **Caddy** routes `/api/*` to the Go API
and everything else to this Next.js app, so cookies stay first-party. Server
components fetch from Go over the private network, forwarding the session
cookie; client components call `/api/*` through the typed client. Real-time
arrives over `/socket` (Phoenix Channels) as **signals only** - entity id plus
change type - and the client refetches real data from Go (ADR-009).

## Prerequisites

- Node.js 24+ and npm 11+.
- For full-stack development: the Go API on `http://localhost:8080` and the
  Elixir orchestrator on `http://localhost:4000` (see repo `infra/`).

## Setup

```bash
cd apps/web
npm install
cp .env.example .env.local   # adjust as needed
npm run dev
```

`npm run dev` serves the app on http://localhost:3000. A development `rewrites`
rule in `next.config.ts` proxies `/api/*` to `ATLAS_API_INTERNAL_URL`
(default `http://localhost:8080`).

## Scripts

| Script | Description |
|---|---|
| `npm run dev` | Start the dev server with hot reload. |
| `npm run build` | Production build. |
| `npm run start` | Serve the production build. |
| `npm run lint` | ESLint (`next/core-web-vitals`). |
| `npm run typecheck` | `tsc --noEmit` (strict). |
| `npm run generate:api` | Generate the typed API client (placeholder - see below). |

## Routing layout

```
app/
├── layout.tsx              # root layout, Outfit font, metadata "Atlas · SafariStack"
├── page.tsx                # redirects to /dashboard
├── (auth)/login/           # the sign-in journey (client flow + page shell)
└── (app)/                  # authenticated shell: sidebar + header
    ├── dashboard/          # executive dashboard v1 shell
    └── settings/           # minimal settings shell
```

The sidebar lists the modules from blueprint section 4: Dashboard, CRM, Work,
Contracts, Finance, People, Systems, Units, Intel, Settings. Only Dashboard and
Settings have pages; the rest are linked as future routes.

## API client and codegen

The typed API client is **generated** from the contract-first OpenAPI 3.1 spec
at `contracts/openapi/public.yaml` (blueprint sections 5.3 and 11). Generated
output lives in `lib/api/generated/` and must **not** be hand-edited; it is
gitignored and only `README.md` + `.gitkeep` are committed.

`public.yaml` does not exist yet in Phase 0, so `npm run generate:api` is a
placeholder. Once the spec lands, generate with
[`openapi-typescript`](https://openapi-ts.dev/):

```bash
npx openapi-typescript ../../contracts/openapi/public.yaml \
  -o lib/api/generated/schema.d.ts
```

Hand-written code talks to the API through `apiFetch` in
[`lib/api/client.ts`](lib/api/client.ts), which:

- resolves the base URL (`NEXT_PUBLIC_API_BASE_URL` in the browser,
  `ATLAS_API_INTERNAL_URL` on the server),
- forwards the session cookie on the server,
- throws `ApiError` carrying an RFC 9457 Problem Details body (with Atlas'
  stable `code`).

CI runs a generation-diff check, so always regenerate rather than edit
generated files (blueprint section 16.2).

## Realtime

[`lib/realtime/socket.ts`](lib/realtime/socket.ts) gets a short-lived realtime
token from `POST /api/v1/auth/realtime-token`, connects to `/socket` and exposes:

- `joinTopic(topic, onSignal, onError?)` returning an unsubscribe function,
- `useRealtimeSignals(topic, handler, options?)`,
- `useExecDashboard(handler)` for `exec:dashboard`,
- `useUserChannel(userId, handler)` for `user:{id}`.

Signals carry `{ type, entity, id, change }` only. **Refetch the real record
from Go inside the handler** - never render the signal payload as data.

## Environment

See [`.env.example`](.env.example):

- `NEXT_PUBLIC_API_BASE_URL` - browser base URL; empty means same origin.
- `ATLAS_API_INTERNAL_URL` - server-side URL of the Go API.
- `NEXT_PUBLIC_REALTIME_URL` - Phoenix socket endpoint (default `/socket`).

## Phase status

Phase 1. The Go authentication backend is complete and the **sign-in journey is
built**: `app/(auth)/login/` runs email/phone + password, TOTP and recovery-code
verification, first-run enrolment with a browser-rendered QR and one-time
recovery codes, and `/login` redirects an already-signed-in visitor on. The
authenticated shell has a working sign-out. Still to do: the sessions page,
invite acceptance, honest stubs for the module routes (they currently 404), and
wiring `generate:api` so the hand-written types are replaced by generated ones.
See [`docs/FRONTEND.md`](../../docs/FRONTEND.md).

## Brand tokens

The sign-in design tokens live in
[`tailwind.config.ts`](tailwind.config.ts): `milk` (the warm page), `ink` (the
SafariStack navy), and `atlas` (the action blue). Outfit is self-hosted from
`app/fonts/` via `next/font/local`, so sign-in makes no third-party font
request. Brand assets are in `public/brand/`.
