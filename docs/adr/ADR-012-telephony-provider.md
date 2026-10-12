# ADR-012 — Telephony provider

| | |
|---|---|
| **Status** | Deferred (Phase 3) |
| **Date** | 2026-10-11 |
| **Deciders** | Internal & Technical CEO, Commercial CEO |
| **Phase** | Phase 3 |
| **Supersedes / Superseded by** | — |

## Context

The CRM behaves like a lightweight sales call centre (§8.1). Calls must be logged as `interaction` records; optional recording may be added later. §12 lists a local provider with voice APIs (e.g. Africa's Talking) and an international one (e.g. Twilio). The decision depends on cost, Kenyan number support, and whether call recording is wanted (D10, §21). Recording is consent-sensitive (§13.2).

## Decision

**Deferred to Phase 3.** The intended approach (to be confirmed in Phase 3):

- **Start with manual call logging** if budget or timing requires — log the call as an `interaction` (type `call`, direction, duration, outcome, next action) with no provider integration (§12, §8.1).
- If an integration is adopted, choose between a **local voice API with Kenyan number support (e.g. Africa's Talking)** or an **international provider (e.g. Twilio)**, deciding on **cost, native Kenyan number support, and call logs via webhook**.
- Provider calls go through Go's `adapters/telephony` behind a small interface; inbound webhooks are signature-verified and persisted before processing (§8.9).
- **Recording only with announced consent**, retained for the shortest practical period (§12, §10.4).

**Open:** D10 — manual logging vs local voice API vs international provider, and whether call recording is enabled (§21).

## Consequences

### Positive
- Deferring avoids premature provider lock-in while CRM ships.
- Manual logging works immediately and requires no new infrastructure.
- An adapter interface keeps providers swappable; consent-first recording protects data subjects.

### Negative / trade-offs
- Manual logging gives no automatic duration/recording and relies on salesperson discipline.
- Provider choice affects recurring cost and Kenyan number availability.
- Recording introduces Kenya Data Protection Act obligations and retention constraints (§13.2, §10.4).

## Alternatives considered
- **No telephony integration ever (manual logging only)** — still viable; kept as fallback.
- **Build on a local aggregator** — constrains provider flexibility.
- **A softphone/VoIP product** — heavier than required for call logging.

## References
- `docs/ATLAS.md` §12
- `docs/ATLAS.md` §8.1
- `docs/ATLAS.md` §8.9
- `docs/ATLAS.md` §10.4
- `docs/ATLAS.md` §13.2
- `docs/ATLAS.md` §21 (D10)
