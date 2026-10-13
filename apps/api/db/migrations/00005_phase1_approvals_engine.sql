-- +goose Up
-- +goose StatementBegin
-- Phase 1: approvals engine tuning (§6.3). Adjust to existing schema.
SET search_path TO public;

-- Pending approvals for lookup.
CREATE INDEX IF NOT EXISTS approvals_status_idx_phase1
  ON core.approvals (status, requested_by, created_at DESC);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS core.approvals_status_idx_phase1;
-- +goose StatementEnd
