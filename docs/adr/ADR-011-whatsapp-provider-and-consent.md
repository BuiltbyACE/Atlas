# ADR-011 — WhatsApp provider approach and consent model

| | |
|---|---|
| **Status** | Deferred (Phase 3) |
| **Date** | 2026-10-11 |
| **Deciders** | Internal & Technical CEO, Commercial CEO |
| **Phase** | Phase 3 |
| **Supersedes / Superseded by** | — |

## Context

WhatsApp is the primary commercial channel: lead conversations, reminders, daily-report prompts and executive-brief summaries (§12). It is also a compliance-sensitive channel — business-initiated messages have strict rules, and consent must be recorded under the Kenya Data Protection Act (§13.2). Meta policy and pricing change over time. The number strategy and consent wording are still open (D9, §21).

## Decision

**Deferred to Phase 3** (prompts may start in Phase 2). The intended approach (to be confirmed in Phase 3):

- Use the **WhatsApp Business Platform (Cloud API)**, wrapped in Go's `adapters/` behind a small interface (e.g. `MessageSender.SendTemplate(ctx, to, template, params, idemKey)`) with a **fake implementation for tests and staging** so nothing in non-production messages real clients (§12).
- **Business-initiated messages require approved templates**; free-form replies are limited to the customer-service window after the contact last messaged.
- Store **per-contact consent/opt-in** (marketing, WhatsApp, call recording) with timestamps and source on the `contact` record (§8.1, §13.2).
- Inbound webhooks (`/webhooks/whatsapp`) pass through the common pipeline: **app-secret signature verification** → replay/timestamp check → store raw payload in `integrations.webhook_inbox` → acknowledge fast → process asynchronously → emit domain events (§8.9, §12).
- In-app and email remain always-available notification channels (§6.4).

**Open:** D9 — one company number vs per-salesperson, and consent wording (§21). Verify current Meta pricing and policy before build.

## Consequences

### Positive
- Official API with template compliance rather than unofficial libraries.
- Consent recorded per contact; auditable delivery via `core.notifications` and integration metadata.
- One adapter interface keeps the provider swappable and staging safe.

### Negative / trade-offs
- Meta policy/pricing risk requires re-verification before build (§12).
- Template approval adds latency to new message types.
- Consent management and content retention limits add UX and data-lifecycle work (§10.4, §13.2).

## Alternatives considered
- **Unofficial WhatsApp libraries** — rejected: terms-of-service and ban risk.
- **Another Business Solution Provider/aggregator** — possible; the adapter keeps this open.
- **SMS-first** — rejected for now: lower richness; SMS remains a later fallback (§6.4).

## References
- `docs/ATLAS.md` §12
- `docs/ATLAS.md` §6.4
- `docs/ATLAS.md` §8.1
- `docs/ATLAS.md` §10.4
- `docs/ATLAS.md` §13.2
- `docs/ATLAS.md` §21 (D9)
