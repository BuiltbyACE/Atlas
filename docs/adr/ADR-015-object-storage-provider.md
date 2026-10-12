# ADR-015 — Object storage provider

| | |
|---|---|
| **Status** | Accepted (Phase 0) |
| **Date** | 2026-10-11 |
| **Deciders** | Internal & Technical CEO (Phase 0 scaffolding) |
| **Phase** | Phase 0 |
| **Supersedes / Superseded by** | Supersedes the prose assumption in §12 that MinIO is the local object store |

## Context

Atlas's document management requires S3-compatible object storage (§12, §15): documents for contracts, invoices and KYC files, stored at rest in private buckets. For local development the blueprint assumed a MinIO container (`infra/compose/...`).

In late September 2026 the official `minio/minio` Docker image was removed from Docker Hub. The alternatives evaluated: `quay.io/minio` returns 404/401 for the image; Chainguard's MinIO images are not published to Docker Hub; `pgsty/silo` (a MinIO fork) is viable but adds a less-mainstream dependency. The local dev storage needs are modest — a private S3 endpoint, a handful of buckets, no replication, no tiering.

SeaweedFS (`chrislusf/seaweedfs`) provides an S3-compatible gateway (AWS Signature Version 4) in a single, actively maintained container, is widely used for exactly this dev/edge role, and is a credible base for the production object store (or for its gateway in front of a managed store).

## Decision

- Use **SeaweedFS** (`chrislusf/seaweedfs:latest`) as the local S3-compatible object store in Docker Compose.
- Single container: the S3 gateway on host port **8333** (mapped to container port 9000), with the disk volume mounted under `infra/compose/data/seaweedfs`.
- Anonymized buckets configured for Atlas use are declared in `infra/compose/seaweedfs/s3.json`; the default admin identity is `atlas` / `atlasminio` (matching the blueprint's local MinIO identity) in `_env`-overridable `S3_*` variables.
- Buckets are created on `task storage:init` (idempotent, `curl` SigV4 `PUT` returning 200 or 409).
- Production object storage decision remains open (D2) — AWS S3/R2 alternative considered; the S3 protocol surface is identical either way, so nothing in the contract layer changes.

**Alternatives and trade-offs:**

| Option | Trade-offs |
|---|---|
| **SeaweedFS** | Single small container, real S3 SigV4, actively maintained; not a MinIO drop-in for exotic features (versioning UI, replication) we don't need locally. |
| **pgsty/silo** | MinIO-compatible fork; heavier config surface; less-mainstream community. |
| **localstack / moto** | S3 emulation, not a durable store; fine for mocking, wrong shape for a local "real" store. |
| **Chainguard minio** | Images not available on Docker Hub; extra registry setup with no benefit here. |

## Consequences

### Positive
- Local and CI can exercise real S3 SigV4 semantics (signing, multipart, listing) against a maintained server.
- One container, declarative bucket policy via `s3.json`, `task storage:init` idempotence.
- The S3 protocol boundary keeps the provider swappable for production (ADR-001: repository pattern boundary in Go).

### Negative
- SeaweedFS is not a MinIO feature-for-feature clone; anything relying on MinIO-specific console/features must be avoided.
- Another third-party image pinned in the compose manifest; image availability risk exists for any Docker Hub image (same risk class as MinIO's removal, listed in §20 risks).

### Neutral
- `S3_*` env vars and the `storage:init` task keep the developer contract stable if the provider changes later.