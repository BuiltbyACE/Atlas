# ADR-010 — Money as integer minor units; UUIDv7; UTC storage, Nairobi display

| | |
|---|---|
| **Status** | Proposed |
| **Date** | 2026-10-11 |
| **Deciders** | Internal & Technical CEO, Commercial CEO |
| **Phase** | Phase 0 |
| **Supersedes / Superseded by** | — |

## Context

§6.6 lists the common conventions that every module inherits; §10.3 shows them in the foundation DDL; §8.4 depends on them for finance. Money, identifiers and time are the three conventions hardest to change later, so they are fixed before any tables are written. The finance module must reconcile with M-Pesa, bank and card providers, and Atlas stores KES by default with USD supported (§6.6).

## Decision

Adopt the §6.6 conventions as binding:

- **Money:** `amount_minor bigint` + `currency char(3)` (KES default, USD supported). **Never floats.** Applies to every monetary column (budgets, expenses, invoices, payments, contract values).
- **Identifiers:** **UUIDv7** primary keys (time-ordered).
- **Time:** `timestamptz` stored in **UTC**; displayed in `Africa/Nairobi` by default, with a per-user timezone setting.
- Related conventions that follow from these: sequenced human references (`LEAD-2026-0042`, `PRJ-2026-007`, `INV-2026-0113`, `EXP-2026-0390`, `INC-2026-0012`); tax rates as effective-dated configuration; optimistic locking via a `version` column with `If-Match`/ETag; cursor-based pagination; soft delete via `archived_at`.

## Consequences

### Positive
- Integer minor units eliminate floating-point money errors and match provider APIs.
- UUIDv7 keeps B-tree index locality while avoiding sequence/enumeration leakage.
- UTC storage avoids timezone/DST ambiguity; a single display default matches the company's operating timezone.

### Negative / trade-offs
- Every amount must be converted for display, and currency minor-unit exponents differ — conversion helpers must be shared.
- Timezone rendering logic is needed wherever times are shown.
- UUIDv7 requires generator support in Go and Elixir; human references must be generated through a sequenced service.

## Alternatives considered
- **`numeric`/`decimal` money** — rejected: permission for drift and rounding ambiguity; bigint minor units are simpler.
- **UUIDv4** — rejected: random keys hurt index locality.
- **Auto-increment integer keys** — rejected: leaks volume and complicates future distribution.
- **`timestamp` without timezone or local-time storage** — rejected: DST and zone ambiguity.

## References
- `docs/ATLAS.md` §6.6
- `docs/ATLAS.md` §8.4
- `docs/ATLAS.md` §10.3
