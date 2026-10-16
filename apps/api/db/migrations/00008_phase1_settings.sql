-- +goose Up
-- +goose StatementBegin
-- Phase 1: the settings store (docs/ATLAS.md §6.6, §6.7).
--
-- §6.6 fixes optimistic concurrency for every editable resource: a `version`
-- column, If-Match on update, 412 on a stale version. core.settings (00002)
-- predates that rule, so it gains the column here.
--
-- §6.7 calls the typed key/value store a *versioned* one. The current value
-- lives in core.settings; each change is appended to core.setting_changes with
-- the previous and the new value, so "what was this last month, and who changed
-- it?" survives the overwrite. The history row is written by the settings
-- service in the same transaction as the value it describes.
SET search_path TO public;

ALTER TABLE core.settings
  ADD COLUMN IF NOT EXISTS version integer NOT NULL DEFAULT 1;

CREATE TABLE IF NOT EXISTS core.setting_changes (
  id          uuid PRIMARY KEY,
  key         text NOT NULL REFERENCES core.settings(key) ON DELETE CASCADE,
  version     integer NOT NULL,
  old_value   jsonb,
  new_value   jsonb NOT NULL,
  changed_by  uuid,
  changed_at  timestamptz NOT NULL DEFAULT now(),
  reason      text
);

-- The API reads a setting's history in (changed_at, id) order, so the keyset
-- index matches the ORDER BY exactly.
CREATE INDEX IF NOT EXISTS setting_changes_key_idx
  ON core.setting_changes (key, changed_at DESC, id DESC);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
SET search_path TO public;
DROP TABLE IF EXISTS core.setting_changes;
ALTER TABLE core.settings DROP COLUMN IF EXISTS version;
-- +goose StatementEnd
