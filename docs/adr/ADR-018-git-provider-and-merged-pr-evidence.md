# ADR-018 — Git provider and merged-PR evidence integration (D6)

| | |
|---|---|
| **Status** | Accepted |
| **Date** | 2026-10-17 |
| **Deciders** | Internal CEO (D6 of §21) |
| **Phase** | Needed by Phase 2 (§21); implemented in Phase 2 |
| **Supersedes / Superseded by** | Decides D6 of §21 and replaces the `[DECIDE]` marker on the "GitHub / GitLab" row in §12 |

## Context

Decision **D6** (§21, owned by the Internal CEO) asks *which* git provider
Atlas integrates with for completion evidence. §12 already described the need —
"Evidence of completion: PR merged, CI status, release tags" — and the exact
mechanism: *"Webhooks link PRs to tasks via the task ref in branch name or PR
title (`PRJ-2026-007/T-0142`)."* §6.5 lists `pull_request` (and `git_commit`)
among the evidence types. None of it existed: migration `00011` shipped
`work.evidence` with only the file shape (`file_id NOT NULL`), there was no
`ref` column on projects or tasks, and there was no webhook route.

Four things had to be decided, and each is expensive to reverse afterwards:

1. **Which provider.** §12 left it open (`GitHub / GitLab [DECIDE]`). The
   integration code, the payload parser and the operators' expectations all
   depend on the answer.
2. **How a completed external event links to an Atlas task.** The only durable
   join key is a human-readable ref in the branch name or PR title; a
   provider-internal PR number means nothing without the integration's own
   bookkeeping.
3. **How an inbound account-less webhook is authenticated and replayed.** There
   is no user session on `/webhooks/github`; a secret and a signature stand in
   for a principal, and GitHub redelivers.
4. **Where the fact lives.** ADR-002 (Go writes, Elixir orchestrates) and
   ADR-009 (events are signals) constrain the answer: Go records the link, the
   outbox carries it, Elixir only nudges the UI.

## Decision

### 1. Provider: GitHub

GitHub is the provider. §12 listed GitHub/GitLab; the team's repositories are on
GitHub, so it is what is integrated and what operators configure. The inbound
pipeline degrades gracefully to another provider (§8.9), but no GitLab adapter
is built: an unbuilt integration decides nothing, and adding GitLab later is a
new adapter, not a new architecture.

### 2. Human refs are the join key

`work.projects` and `work.tasks` now carry a **`ref`**, exactly as §8.2's entity
tables already described and as §12's branch convention assumes:

| Entity | Ref | Example |
|---|---|---|
| project | `PRJ-<YYYY>-<NNN>` | `PRJ-2026-007` |
| task | `<project ref>/T-<NNNN>` | `PRJ-2026-007/T-0142` |

Refs are allocated by Go at insert through `work.ref_counters(scope, value)`
(`INSERT … ON CONFLICT DO UPDATE … RETURNING`), inside the caller's transaction,
so a rolled-back create burns no number. The project counter is scoped per year
(`PRJ-<YYYY>`); the task counter is a single global scope (`T`), so two projects
never share a task ordinal by accident. **A ref is never edited** (ADR-010): a
branch keeps naming one task for the record's life.

### 3. The webhook surface

`POST /webhooks/github` (§11) is a **public** route: GitHub has no Atlas
principal, so the HMAC signature is the gate and the body is only parsed after
it verifies.

- GitHub signs the raw body with `X-Hub-Signature-256`; Atlas checks
  HMAC-SHA256 in constant time and accepts no other scheme.
- The shared secret is `ATLAS_GITHUB_WEBHOOK_SECRET`, **process configuration**,
  not a business setting (§2.10, §6.7): it is not tuned at runtime and must not
  travel through the settings API. When it is empty the route answers `503`
  rather than trusting an unsigned body — a configuration gap, never a
  fail-open.
- Bodies are capped at 1 MiB.
- Replay is by delivery id, not by a timestamp: GitHub sends no verifiable
  signed time, and `X-GitHub-Delivery` (with its signature) is what it
  redelivers. `integrations.github_deliveries(delivery_id PK, event_type,
  received_at)` records each delivery; a duplicate is a no-op, and a processing
  error removes the claim and returns `500` so the provider's retry is
  processed rather than swallowed.

### 4. What a pull request does

Only a **merged** pull request is completion evidence:

- `pull_request` events with `action = closed` and `merged = true` are read.
- Refs are extracted from both the PR title and the `head` branch name
  (case-insensitively — branches are often lowercase), deduped, in order.
- The first ref that resolves to a task records `pull_request` evidence on that
  task, with the reference set to the PR URL. A PR that matches no Atlas ref is
  acknowledged (not an error); other event types and unmerged PRs are ignored.

Attaching evidence is a normal Go write, so the Atlas rule holds: a merged PR
**records evidence**, it does not complete a task. The `Done` transition still
runs through review (§8.2).

### 5. Evidence is polymorphic

`work.evidence` gains `evidence_type` (the seven §6.5 values), `reference`, and a
nullable `file_id`, with a check on the §6.5 taxonomy and a second that a `file`
row names a file and no reference, and every other type names a reference and no
file. `uploader_person_id` becomes nullable: an integration has no Atlas
submitter, so a nil uploader is the honest record, and a nil uploader never
equals an actor so such evidence is still verifiable by a person.

### 6. The fact is an event

`integrations.git.pull_request.merged.v1` (§7.4) is written in the same
transactions as the link, and carries identifiers only
(evidence/task/project, provider, repository, number, URL, sha, timestamps). The
aggregate is the **evidence** row the link created — the pull request is
external and has no Atlas id. The integrations package (not the work module)
emits it because reaching the edge is the integrations module's job (§8.9);
Elixir reacts to it like every other signal (ADR-009).

Linkage is idempotent by `(task, evidence_type, reference)`: a redelivered
webhook reuses the evidence row it already recorded, and a partial failure can
be retried without duplicating it.

## Consequences

### Positive

- **One stable join key.** The ref in the branch or PR title is the same string
  a person reads on the task, so linking needs no provider bookkeeping and
  survives a PR being edited (`ref` is immutable; §8.2 always named it).
- **The §6.5 taxonomy is finally real.** `url`, `git_commit`, `pull_request`,
  `test_run`, `client_approval` and `payment_reference` all fit the one
  `work.evidence` shape, so a merged PR slots into the existing evidence,
  review and permission machinery with no second table.
- **A signed, replay-protected, account-less boundary.** The secret is
  configuration, the signature is constant-time, deliveries are de-duplicated,
  and a failure is retried rather than dropped.
- **Signals, not data.** The event carries ids; consumers re-read Go under
  Atlas's permissions (ADR-009), so the link is one fact with one reader.

### Negative

- **Processing is synchronous**, where §8.9 sketches *"store raw payload
  (`integrations.webhook_inbox`) → acknowledge fast → process asynchronously →
  emit domain event"*. A delivery is handled inline and the raw payload is not
  kept. Acceptable at Phase 2 volumes; the §8.9 inbox and worker are the
  documented next step, and the delivery log already gives the idempotence the
  pipeline needs.
- **At-least-once.** A failure after the evidence write removes the claim, so
  GitHub's retry re-runs the delivery. The link is idempotent by reference, but
  `integrations.git.pull_request.merged.v1` can be emitted twice if the failure
  lands after it. Consumers are invalidations, so a duplicate is harmless — the
  cost of the simpler route, recorded here.
- **The delivery log grows.** It is not pruned by signature age (there is no
  signed timestamp); retention is an operational choice.
- **The ref convention is a contract.** A branch or title that does not carry a
  well-formed Atlas ref links nothing; Atlas acknowledges the delivery and
  leaves the task alone. This is honesty, not a silent mis-link.

### Neutral

- The `integrations` schema is owned by `atlas_api` (as §10.1 allows),
  alongside `units`; nothing else writes the delivery log.
- GitLab is not implemented and no phase commits to it; if it is chosen, the
  parser and route are additive and `integrations.git.*` already namespaces the
  events.
- Files remain the only evidence stored in object storage; every other type is
  an external reference.

## Alternatives considered

| Alternative | Why not |
|---|---|
| **Integrate GitLab first (or both)** | The repositories are on GitHub; building a provider nobody connects yet spends the phase's budget on the wrong adapter. §8.9 makes the second a small addition. |
| **Authenticate by IP allowlist instead of a signature** | GitHub's published egress ranges are broad and change; a signature is per-body and constant-time, and is what §12 leads with ("Webhooks verified by app-secret signature"). |
| **Trust any unauthenticated POST when no secret is set** | Fail-open on a public route: anyone could fabricate a merged PR and evidence. `503` until configured was chosen deliberately. |
| **A timestamp/nonce replay window instead of the delivery log** | GitHub signs no timestamp for the `sha256` scheme, so such a window cannot be built from the payload. The delivery id is signed and redelivered, which is what is needed. |
| **A separate `pull_requests` (or `git_commits`) table** | Over-models one reference; §6.5 already gives evidence a type and a reference, and the PR is external (no Atlas id). A table would need its own permissions and listing for no reader. |
| **Link on PR `opened`** | The evidence §8.2 requires is a *merged* deliverable; an open PR is work in progress, not proof. Merged is the fact. |
| **Complete the task on merge** | Reverses §8.2 — a task reaches `Done` through review with evidence. A merged PR is evidence, and Atlas records it; the human decides. |
| **A `ATLAS_GIT_PROVIDER` toggle** | One provider is implemented; a config switch with one live option is untested code. A new provider is a new adapter, chosen through a new ADR. |

## References

- `docs/ATLAS.md` §6.5 (evidence types), §7.4 (event catalogue), §8.2 (work
  entities and the `evidence`/`ref` columns), §8.9 (integrations hub
  pipeline), §10.1 (schemas), §11 (API standards and the webhook routes), §12
  (the GitHub/GitLab integration), §21 (D6)
- `docs/PHASE2.md` §3.6 (implementation), §9 (deviations: synchronous
  processing vs the §8.9 inbox)
- ADR-002 (Go writes, Elixir orchestrates), ADR-009 (signals, not data),
  ADR-010 (stable ids; human refs)
- `db/migrations/00019_phase2_git_evidence.sql`
- `internal/integrations/`, `internal/outbox/integrations.go`
