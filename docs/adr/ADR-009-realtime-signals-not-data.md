# ADR-009 — Real-time messages are signals, not data

| | |
|---|---|
| **Status** | Proposed |
| **Date** | 2026-10-11 |
| **Deciders** | Internal & Technical CEO, Commercial CEO |
| **Phase** | Phase 0 |
| **Supersedes / Superseded by** | — |

## Context

Elixir is the real-time hub, but it must never leak data and must not become a second place where permissions are enforced (§5.2, ADR-002). "Data leak via real-time channels" is a named threat, mitigated by "signals-only payloads; Go-authorised joins" (§13.3). §7.5 defines the channel topic catalogue.

## Decision

- Real-time channel pushes carry **identifiers and change type only**, never business payloads — e.g. `{type: "task.changed", id}`. The browser **refetches the actual data from Go**, which enforces permissions.
- **Exception:** low-sensitivity **presence and typing** indicators may be sent directly.
- **Join authorisation:** the realtime token carries the user's ID and a compact list of scopes. For project/incident topics, Elixir calls `GET /internal/v1/authz/check` (cached briefly) so **Go remains the authority**.
- Topics and their audiences follow §7.5 (`user:{id}`, `exec:dashboard`, `work:project:{id}`, `crm:pipeline`, `systems:status`, `incident:{id}`).

Example: a task change pushes `work:project:{id}` `{type: task.changed, id}`; the client then calls `GET /api/v1/work/tasks/{id}` (permission-checked) — as shown in the §5.8 sequence.

## Consequences

### Positive
- No business data is exposed through the socket; permissions are enforced in exactly one place.
- Push payloads are tiny and the schema is trivial to keep stable.
- Clients always render current, authorised state rather than a stale payload.

### Negative / trade-offs
- An extra round-trip per change to fetch the record.
- Busy topics can cause refetch storms — clients must coalesce/debounce invalidations.
- Slight added latency before the user sees the data.

## Alternatives considered
- **Push full business payloads** — rejected: leaks data and duplicates authorisation.
- **Push data and filter on the client** — rejected: client-side filtering is cosmetic, not security (principle 6, §2).
- **Use Go for WebSockets too** — rejected: real-time delivery is Elixir's remit (§5.2).

## References
- `docs/ATLAS.md` §5.2
- `docs/ATLAS.md` §7.5
- `docs/ATLAS.md` §5.8
- `docs/ATLAS.md` §13.3
