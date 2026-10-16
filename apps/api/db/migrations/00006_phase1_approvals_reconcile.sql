-- +goose Up
-- +goose StatementBegin
-- Phase 1: reconcile core.approvals with the approvals engine (docs/ATLAS.md 6.3).
--
-- The Phase 1 foundation DDL (10.3) laid down a minimum approval spine:
-- subject_type/subject_id, a four-value status, no history table. The approvals
-- engine in internal/approvals needs more to run: role-based pending lookups,
-- deadlines for the worker sweep, optimistic concurrency, a decision reason and
-- an append-only history. Migration 00005 only added an index, so the engine's
-- SQL never matched the table — every read and write failed at runtime.
--
-- This migration aligns the table with the engine (the Go side is the
-- authority) and creates core.approval_history.
SET search_path TO public;

-- 1. subject_* -> entity_* (the engine's vocabulary).
ALTER TABLE core.approvals RENAME COLUMN subject_type TO entity_type;
ALTER TABLE core.approvals RENAME COLUMN subject_id TO entity_id;

-- 2. subject_version is superseded by `version`: the engine tracks concurrency
--    on the approval row itself and never sets subject_version.
ALTER TABLE core.approvals DROP COLUMN subject_version;

-- 3. Columns the engine reads and writes.
ALTER TABLE core.approvals
  ADD COLUMN ref           text,
  ADD COLUMN amount_minor  bigint NOT NULL DEFAULT 0,
  ADD COLUMN currency      char(3),
  ADD COLUMN approver_role text,
  ADD COLUMN approver_id   uuid,
  ADD COLUMN policy_ref    text,
  ADD COLUMN due_at        timestamptz,
  ADD COLUMN decided_by    uuid,
  ADD COLUMN reason        text,
  ADD COLUMN version       integer NOT NULL DEFAULT 1,
  ADD COLUMN updated_at    timestamptz NOT NULL DEFAULT now(),
  ADD COLUMN archived_at   timestamptz;

-- 4. Widen the status set to the engine's lifecycle.
ALTER TABLE core.approvals DROP CONSTRAINT approvals_status_check;
ALTER TABLE core.approvals ADD CONSTRAINT approvals_status_check
  CHECK (status IN ('requested','pending','approved','declined','escalated','cancelled','expired'));

-- 5. A human reference (APR-YYYY-XXXXXXXX) must be unique when present.
CREATE UNIQUE INDEX approvals_ref_key ON core.approvals (ref) WHERE ref IS NOT NULL;

-- 6. Hot paths: the role inbox and the worker's expiry sweep.
CREATE INDEX approvals_pending_role_idx
  ON core.approvals (approver_role, created_at DESC, id DESC)
  WHERE status IN ('requested','pending') AND archived_at IS NULL;

CREATE INDEX approvals_due_idx
  ON core.approvals (due_at)
  WHERE status IN ('requested','pending') AND due_at IS NOT NULL AND archived_at IS NULL;

-- Keep updated_at honest even if a future caller forgets to set it.
CREATE TRIGGER approvals_touch_updated_at
  BEFORE UPDATE ON core.approvals
  FOR EACH ROW EXECUTE FUNCTION core.touch_updated_at();

-- 7. Append-only history: one row per lifecycle transition.
CREATE TABLE core.approval_history (
  id          uuid PRIMARY KEY,
  approval_id uuid NOT NULL REFERENCES core.approvals(id) ON DELETE CASCADE,
  actor_id    uuid,
  action      text NOT NULL,
  reason      text,
  from_status text,
  to_status   text,
  created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX approval_history_approval_idx ON core.approval_history (approval_id, created_at);

-- 8. Grants for the new table (mirrors 00002; guarded so a bare database still
--    migrates). Column grants on core.approvals are unaffected by ADD COLUMN.
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'atlas_api') THEN
    GRANT SELECT, INSERT, UPDATE, DELETE ON core.approval_history TO atlas_api;
  END IF;
  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'atlas_worker') THEN
    GRANT SELECT, INSERT, UPDATE, DELETE ON core.approval_history TO atlas_worker;
  END IF;
  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'atlas_readonly') THEN
    GRANT SELECT ON core.approval_history TO atlas_readonly;
  END IF;
  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'atlas_orchestrator') THEN
    GRANT SELECT ON core.approval_history TO atlas_orchestrator;
  END IF;
END
$$;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS core.approval_history;
DROP TRIGGER IF EXISTS approvals_touch_updated_at ON core.approvals;
DROP INDEX IF EXISTS core.approvals_due_idx;
DROP INDEX IF EXISTS core.approvals_pending_role_idx;
DROP INDEX IF EXISTS core.approvals_ref_key;

ALTER TABLE core.approvals DROP CONSTRAINT IF EXISTS approvals_status_check;
ALTER TABLE core.approvals ADD CONSTRAINT approvals_status_check
  CHECK (status IN ('pending','approved','rejected','expired','cancelled'));

ALTER TABLE core.approvals
  DROP COLUMN IF EXISTS archived_at,
  DROP COLUMN IF EXISTS updated_at,
  DROP COLUMN IF EXISTS version,
  DROP COLUMN IF EXISTS reason,
  DROP COLUMN IF EXISTS decided_by,
  DROP COLUMN IF EXISTS due_at,
  DROP COLUMN IF EXISTS policy_ref,
  DROP COLUMN IF EXISTS approver_id,
  DROP COLUMN IF EXISTS approver_role,
  DROP COLUMN IF EXISTS currency,
  DROP COLUMN IF EXISTS amount_minor,
  DROP COLUMN IF EXISTS ref;

ALTER TABLE core.approvals ADD COLUMN subject_version integer NOT NULL DEFAULT 1;
ALTER TABLE core.approvals RENAME COLUMN entity_id TO subject_id;
ALTER TABLE core.approvals RENAME COLUMN entity_type TO subject_type;
-- +goose StatementEnd
