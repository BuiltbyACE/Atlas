# Atlas Architecture Decision Records

This directory holds the Architecture Decision Records (ADRs) for **Atlas**, the SafariStack Company Operating System (SCOS). The source of truth for the architecture is [`../ATLAS.md`](../ATLAS.md); the initial ADR list is defined in §19 and the open items in §21.

An ADR records a decision that is expensive to reverse. When the blueprint and the code disagree, the code is wrong or the blueprint must be changed **through an ADR** (§0). Breaking an ADR marked as "architectural law" (§5.2) requires a new ADR.

## Index

| ADR | Title | Status | Phase |
|---|---|---|---|
| [ADR-001](ADR-001-modular-monolith-go.md) | Modular monolith in Go as the single core backend | Proposed (accepted in this blueprint) | Phase 0 |
| [ADR-002](ADR-002-go-writes-elixir-orchestrates.md) | Go is the sole writer of business data and sole caller of external providers; Elixir orchestrates via commands | Proposed | Phase 0 |
| [ADR-003](ADR-003-go-library-stack.md) | Go library stack | Proposed | Phase 0 |
| [ADR-004](ADR-004-postgres-schema-per-module.md) | PostgreSQL schema-per-module and cross-schema FK policy | Proposed | Phase 0 |
| [ADR-005](ADR-005-transactional-outbox.md) | Transactional outbox with pg_notify + polling; no message broker until justified | Proposed | Phase 0 |
| [ADR-006](ADR-006-elixir-stack.md) | Elixir stack: Phoenix Channels (no LiveView), Oban, Ecto on its own schema | Proposed | Phase 0 |
| [ADR-007](ADR-007-nextjs-thin-bff.md) | Next.js as UI + thin BFF; same-origin routing via Caddy; generated TS client | Proposed | Phase 0 |
| [ADR-008](ADR-008-auth-and-realtime-tokens.md) | Session-cookie auth, MFA for privileged roles, short-lived realtime tokens | Proposed | Phase 0 |
| [ADR-009](ADR-009-realtime-signals-not-data.md) | Real-time messages are signals, not data | Proposed | Phase 0 |
| [ADR-010](ADR-010-money-ids-time.md) | Money as integer minor units; UUIDv7; UTC storage, Nairobi display | Proposed | Phase 0 |
| [ADR-011](ADR-011-whatsapp-provider-and-consent.md) | WhatsApp provider approach and consent model | Deferred (Phase 3) | Phase 3 |
| [ADR-012](ADR-012-telephony-provider.md) | Telephony provider | Deferred (Phase 3) | Phase 3 |
| [ADR-013](ADR-013-accounting-and-einvoicing.md) | Accounting package and KRA e-invoicing integration route | Deferred (Phase 4) | Phase 4 |
| [ADR-014](ADR-014-hosting-and-region.md) | Hosting and region | Proposed (pending D2) | Phase 0 |
| [ADR-015](ADR-015-object-storage-provider.md) | Object storage provider (SeaweedFS local store) | Accepted | Phase 0 |

## Status values

- **Proposed** — written, awaiting joint CEO agreement.
- **Proposed (accepted in this blueprint)** — agreed as part of the founding blueprint (§19).
- **Proposed (pending D<N>)** — agreed in principle but gated on the named open decision (§21).
- **Deferred (Phase N)** — intentionally not decided yet; must be resolved before the phase that needs it (§0, §19).
- **Accepted / Rejected / Superseded** — set once a decision is final. ADRs are never edited to change a past decision; write a new ADR and mark the old one superseded.

## Related open decisions (§21)

| # | Decision | Owner | ADR |
|---|---|---|---|
| D2 | Hosting provider and region | Internal CEO | ADR-014 |
| D3 | SSO vs Atlas-native accounts | Internal CEO | ADR-008 |
| D9 | WhatsApp number strategy and consent wording | Commercial CEO | ADR-011 |
| D10 | Telephony approach and call recording | Commercial CEO | ADR-012 |
| D11 | Accounting package and e-invoicing route | Both CEOs + accountant | ADR-013 |

## Format

Each ADR follows a fixed template: Status / Date / Deciders / Phase / Supersedes table, then **Context**, **Decision**, **Consequences** (positive, negative), **Alternatives considered**, and **References** to `ATLAS.md` sections. Keep each ADR tight and cite section numbers.

Path: `docs/adr/`.
