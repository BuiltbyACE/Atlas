-- +goose Up
-- +goose StatementBegin
-- Phase 1: two-person approvals (docs/ATLAS.md §3.3, §6.4).
--
-- The approval matrix has decisions that need two distinct people:
--
--   | Above T2 | Both CEOs (shared decisions queue) |
--
-- The base spine (00002) already has core.approval_decisions — one row per
-- approver, keyed (approval_id, approver_id) — but core.approvals had no way to
-- say how many distinct decisions it needs, so the engine could only ever
-- finalise on the first one. This column is that missing bit of state: it is
-- set from the approval policy's approver_rule ('both_ceos' → 2) at request
-- time and read by the engine's decide path. A single-person approval keeps the
-- default of 1, so nothing that exists today changes behaviour.
--
-- It is deliberately smallint, not an int: the matrix never needs more than two
-- and the check keeps a bad caller from inventing an unbounded queue.
SET search_path TO public;

ALTER TABLE core.approvals
  ADD COLUMN approvals_required smallint NOT NULL DEFAULT 1
    CHECK (approvals_required BETWEEN 1 AND 2);

COMMENT ON COLUMN core.approvals.approvals_required IS
  'Distinct approvals needed before status becomes approved (§3.3 both-CEO decisions).';

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE core.approvals DROP COLUMN IF EXISTS approvals_required;
-- +goose StatementEnd
