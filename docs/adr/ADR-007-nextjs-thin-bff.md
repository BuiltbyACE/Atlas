# ADR-007 — Next.js as UI + thin BFF; same-origin routing via Caddy; generated TS client

| | |
|---|---|
| **Status** | Proposed |
| **Date** | 2026-10-11 |
| **Deciders** | Internal & Technical CEO, Commercial CEO |
| **Phase** | Phase 0 |
| **Supersedes / Superseded by** | — |

## Context

§5.3 defines how Next.js and Go fit together. The frontend must share one contract with the backend (§11), keep cookies first-party, and never bypass Go's authorisation (principle 6, §2). The web app also needs a real-time client for Phoenix Channels (§7.5).

## Decision

Next.js (App Router, TypeScript) is **UI + a thin BFF only**:

- **Same origin, two services.** Caddy routes on one domain (e.g. `atlas.safaristack.co.ke` **[DECIDE]**): `/api/*` → Go, `/socket` (WebSocket) → Elixir, everything else → Next.js.
- **Server components** call Go over the internal network, forwarding the user's session cookie; Go authorises every call as that user.
- **Client components** call `/api/*` directly via a TypeScript client **generated from Go's OpenAPI spec**, so frontend and backend share one contract.
- **Route handlers / server actions** exist only as thin pass-throughs (e.g. file-upload streaming). They never contain business rules and never talk to PostgreSQL.
- UI kit e.g. shadcn/ui + Tailwind **[DECIDE]**; realtime client in `lib/realtime/` (§15).

## Consequences

### Positive
- Cookies stay first-party; no CORS complexity.
- One contract (OpenAPI 3.1) shared by frontend and backend; generation removes hand-written clients.
- UI cannot bypass Go's authz; server rendering always reflects real permissions.

### Negative / trade-offs
- Two processes run behind one proxy, raising local/staging setup (Docker Compose covers this).
- Client-side calls add a hop through Caddy; refetch behaviour must be managed (ADR-009).
- Route handlers tempt developers to add logic — must be policed in review.

## Alternatives considered
- **Cross-origin Next.js → Go** — rejected: cookie/CORS complexity.
- **Next.js as a pure SPA** calling `/api/*` only — rejected: loses server-rendered permission-aware pages.
- **Business logic in Next.js server actions** — rejected: violates §5.2 and §5.3.
- **Separate subdomains for app and API** — rejected: third-party cookie and config burden.

## References
- `docs/ATLAS.md` §5.3
- `docs/ATLAS.md` §5.1
- `docs/ATLAS.md` §11
- `docs/ATLAS.md` §15
