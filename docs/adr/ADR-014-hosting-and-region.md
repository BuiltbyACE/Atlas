# ADR-014 — Hosting and region

| | |
|---|---|
| **Status** | Proposed (pending D2) |
| **Date** | 2026-10-11 |
| **Deciders** | Internal & Technical CEO, Commercial CEO |
| **Phase** | Phase 0 |
| **Supersedes / Superseded by** | — |

## Context

Atlas must be hosted somewhere by Phase 0. §16.4 gives the recommended starting position; §14 sets SLOs (99.5% API availability, outbox lag p99 < 10 s); §13.1 requires encrypted off-site backups and recovery. "Single point of failure (one VM)" and "ransomware / data loss" are named risks (§20, §13.3), mitigated by managed PostgreSQL with point-in-time recovery. The final provider and region is open (D2, §21).

## Decision

**Proposed (pending D2).** The recommended starting position per §16.4:

- **One production VM (or small managed container platform)** running **Caddy + Next.js + Go + Elixir**.
- **Managed PostgreSQL with PITR** (continuous WAL archiving, encrypted, off-site — §10.4).
- **S3-compatible object storage** (private buckets, encryption at rest — §12).
- **External uptime monitor** independent of Atlas infrastructure (Atlas cannot alert on its own outage — §8.6, §14).
- **Region chosen for lowest latency to Kenya.**
- **Scale out** (separate worker/orchestrator hosts, read replica) only when metrics demand it.

**Alternatives and trade-offs:**

| Option | Trade-offs |
|---|---|
| **AWS af-south-1 (Cape Town)** | Closest hyperscaler to Kenya; full managed services incl. RDS PITR; higher cost and operational complexity. `eu-west` is a fallback at higher latency. |
| **DigitalOcean (AMS/FRA or BLR)** | Simple, lower cost, managed PostgreSQL with backups; BLR (Bangalore) lower latency than Europe; fewer regions and services. |
| **Hetzner (EU)** | Very good price/performance; **no managed PostgreSQL**, so PITR must be self-run; moderate latency to Kenya. |
| **Fly.io** | Easy multi-region deploys, good for a small team; managed Postgres with less mature PITR; region options limited. |
| **Local Kenyan providers** | Lowest latency and possible data-residency appeal; managed-service, backup and PITR maturity varies and must be verified. |

## Consequences

### Positive
- Simple and low-cost to operate for a 2–4 engineer team.
- Managed PostgreSQL with PITR satisfies the main data-loss mitigation.
- An independent uptime monitor covers Atlas's self-outage blind spot (§8.6).

### Negative / trade-offs
- A single VM is a single point of failure until scale-out (§20).
- Region choice is a cost-vs-latency trade-off; the closest region is not always the cheapest.
- Region may carry data-residency implications to review with legal advice (§13.2).

## Alternatives considered
- **Multi-region / HA from day one** — rejected: contradicts "start simple, scale on metrics" (§16.4).
- **Self-managed PostgreSQL on a plain VM** — rejected for production: PITR maturity risk.
- **Serverless-only architecture** — rejected: long-running Go workers and Elixir workflows fit poorly.

## References
- `docs/ATLAS.md` §16.4
- `docs/ATLAS.md` §14
- `docs/ATLAS.md` §10.4
- `docs/ATLAS.md` §13.2
- `docs/ATLAS.md` §20
- `docs/ATLAS.md` §21 (D2)
