-- +goose Up
-- +goose StatementBegin
-- Phase 1: audit views (docs/ATLAS.md §6.3, §6.6).
--
-- §6.3 makes the trail viewable per record ("History" tab) and globally for
-- auditors and CEOs. Both are keyset pages ordered newest first, so the indexes
-- below match those ORDER BYs exactly.
--
-- audit.events (00002) carried single-column indexes for occurred_at and for
-- (entity_type, entity_id). The composite indexes here subsume both, so the
-- originals are dropped rather than left to be maintained on every insert into
-- an append-only table.
SET search_path TO public;

CREATE INDEX IF NOT EXISTS audit_events_occurred_id_idx
  ON audit.events (occurred_at DESC, id DESC);

CREATE INDEX IF NOT EXISTS audit_events_entity_occurred_idx
  ON audit.events (entity_type, entity_id, occurred_at DESC, id DESC);

DROP INDEX IF EXISTS audit.audit_events_occurred_idx;
DROP INDEX IF EXISTS audit.audit_events_entity_idx;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
SET search_path TO public;
CREATE INDEX IF NOT EXISTS audit_events_occurred_idx ON audit.events (occurred_at);
CREATE INDEX IF NOT EXISTS audit_events_entity_idx ON audit.events (entity_type, entity_id);
DROP INDEX IF EXISTS audit.audit_events_entity_occurred_idx;
DROP INDEX IF EXISTS audit.audit_events_occurred_id_idx;
-- +goose StatementEnd
