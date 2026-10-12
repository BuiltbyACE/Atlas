-- Queries for the transactional outbox (docs/ATLAS.md §7.1).
--
-- These are compiled by sqlc into typed Go in internal/db. Phase 0 uses a small
-- hand-written writer (internal/outbox) so the hello-event demo has no codegen
-- dependency; generated code lands in Phase 1 with the first real modules.

-- name: InsertEvent :one
INSERT INTO outbox.events (
  id, type, aggregate_type, aggregate_id, occurred_at, actor_id, correlation_id, causation_id, payload
) VALUES (
  $1, $2, $3, $4, $5, $6, $7, $8, $9
)
RETURNING seq;

-- name: ListEventsAfter :many
SELECT seq, id, type, aggregate_type, aggregate_id, occurred_at,
       actor_id, correlation_id, causation_id, payload
FROM outbox.events
WHERE seq > $1
ORDER BY seq
LIMIT $2;

-- name: MaxEventSeq :one
SELECT COALESCE(MAX(seq), 0)::bigint AS max_seq FROM outbox.events;

-- name: CountEvents :one
SELECT count(*)::bigint AS count FROM outbox.events;
