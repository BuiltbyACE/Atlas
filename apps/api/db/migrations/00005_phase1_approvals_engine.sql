-- +goose Up
-- +goose StatementBegin
-- Phase 1: approvals engine tuning (§6.3). Add indexes used by pending/expiry
-- queries; keep it additive (no schema changes to existing tables).
SET search_path TO public;

-- Pending approvals by approver role and due date (fast for worker + UI).
CREATE INDEX IF NOT EXISTS approvals_status_approver_due_idx
  ON core.approvals (status, approver_role, due_at)
  WHERE status IN ('requested', 'pending');

-- Requested-by history for the requester view.
CREATE INDEX IF NOT EXISTS approvals_requested_by_created_idx
  ON core.approvals (requested_by, created_at DESC);

-- Find approvals for a specific entity quickly.
CREATE INDEX IF NOT EXISTS approvals_entity_idx
  ON core.approvals (entity_type, entity_id);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS core.approvals_status_approver_due_idx;
DROP INDEX IF EXISTS core.approvals_requested_by_created_idx;
DROP INDEX IF EXISTS core.approvals_entity_idx;
-- +goose StatementEnd
