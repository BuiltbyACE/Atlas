# ADR-005 — Transactional outbox with pg_notify + polling; no message broker until justified

| | |
|---|---|
| **Status** | Proposed |
| **Date** | 2026-10-11 |
| **Deciders** | Internal & Technical CEO, Commercial CEO |
| **Phase** | Phase 0 |
| **Supersedes / Superseded by** | — |

## Context

Design principle 2 says events connect modules; principle 3 splits Go (truth) from Elixir (time and motion) (§2). Go must notify Elixir of business changes without the unsafe pattern of writing to a database **and** a message broker (dual-write). The event catalogue is large (§7.4) but the initial load is modest and the team is small.

## Decision

Use a **transactional outbox** in PostgreSQL (§7.1):

- Go writes the business change, its `audit.events` row, and its `outbox.events` row(s) in **one transaction**.
- A trigger runs `PERFORM pg_notify('atlas_outbox', NEW.seq::text)` after insert.
- Elixir's `Outbox.Listener` wakes on `NOTIFY` and its `Outbox.Consumer` also polls every few seconds as a safety net. It reads `seq > cursor` in batches, dispatches to handlers, advances the cursor in `orchestrator.consumer_cursors`, and records handled event IDs for idempotency.
- Events carry the envelope in §7.2 and follow `module.aggregate.past_tense_verb.vN` naming (§7.3). JSON Schemas live in `contracts/events/` and are validated in CI on both producer and consumer sides.
- Retention: archive events older than **[CONFIG]** 90 days to `outbox.events_archive`.

**Add a broker (NATS/RabbitMQ/Kafka) only if** a second independent consumer needs separate delivery semantics, throughput exceeds what polling handles comfortably, or an external partner needs event subscriptions — recorded as a future ADR (§7.1).

## Consequences

### Positive
- No extra infrastructure; PostgreSQL is the only dependency.
- Atomicity guaranteed: a change and its event cannot diverge.
- A single global `seq` gives ordered delivery; simple for a small team to reason about and operate.

### Negative / trade-offs
- Polling adds latency (bounded by the interval) and background database load.
- A single ordered stream could bottleneck at very high throughput.
- `pg_notify` payload is only a sequence number (8 KB limit) and offers no broker-level fan-out, retention or external replay.

## Alternatives considered
- **Dual-write to DB and broker** — rejected: can lose or duplicate events on partial failure.
- **Direct synchronous Go→Elixir calls** — rejected: couples availability and loses the durable record.
- **Broker from day one** — rejected: unnecessary infrastructure for the initial scale (§5.1, §20).

## References
- `docs/ATLAS.md` §7.1
- `docs/ATLAS.md` §7.2
- `docs/ATLAS.md` §7.3
- `docs/ATLAS.md` §5.1
- `docs/ATLAS.md` §5.5
