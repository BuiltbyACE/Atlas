-- +goose Up
-- +goose StatementBegin
-- Phase 1: file and evidence storage conventions (docs/ATLAS.md §6.5, §6.6).
--
-- core.files (00002) already holds the metadata. §6.6 fixes an `updated_at` on
-- every edited row; a file's status moves pending -> available (or quarantined)
-- -> deleted, so it gains one here, kept current by the shared touch trigger.
SET search_path TO public;

ALTER TABLE core.files
  ADD COLUMN IF NOT EXISTS updated_at timestamptz NOT NULL DEFAULT now();

DROP TRIGGER IF EXISTS files_touch ON core.files;
CREATE TRIGGER files_touch
  BEFORE UPDATE ON core.files
  FOR EACH ROW EXECUTE FUNCTION core.touch_updated_at();

-- Listings are keyset pages on (created_at DESC, id DESC): "my files", the
-- files attached to one entity, and the status-filtered views.
CREATE INDEX IF NOT EXISTS files_uploaded_by_idx
  ON core.files (uploaded_by, created_at DESC, id DESC);

CREATE INDEX IF NOT EXISTS files_entity_created_idx
  ON core.files (entity_type, entity_id, created_at DESC, id DESC);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
SET search_path TO public;
DROP INDEX IF EXISTS core.files_entity_created_idx;
DROP INDEX IF EXISTS core.files_uploaded_by_idx;
DROP TRIGGER IF EXISTS files_touch ON core.files;
ALTER TABLE core.files DROP COLUMN IF EXISTS updated_at;
-- +goose StatementEnd
