-- +goose Up
-- +goose StatementBegin
-- Phase 4: the write lifecycle's missing columns (docs/ATLAS.md §8.4).
--
-- 00022 carried every status and version column the lifecycle needs. Three
-- fields the expense claim carries are only written once a claim is created:
--   * receipt_file_id — the uploaded receipt (§8.4: required above a configured
--     amount). The bytes live in object storage; this is the core.files id.
--   * tax_invoice_ref — the tax-compliant invoice reference, where applicable.
--   * paid_on — the day an approved claim was paid or reimbursed. Without it
--     the settlement step could not say when the money actually left, which is
--     what reconciliation is checked against.
ALTER TABLE finance.expenses
  ADD COLUMN IF NOT EXISTS receipt_file_id uuid REFERENCES core.files(id) ON DELETE SET NULL,
  ADD COLUMN IF NOT EXISTS tax_invoice_ref text,
  ADD COLUMN IF NOT EXISTS paid_on date;

CREATE INDEX IF NOT EXISTS idx_finance_expenses_receipt ON finance.expenses(receipt_file_id);
CREATE INDEX IF NOT EXISTS idx_finance_expenses_paid_on ON finance.expenses(paid_on);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS finance.idx_finance_expenses_paid_on;
DROP INDEX IF EXISTS finance.idx_finance_expenses_receipt;

ALTER TABLE finance.expenses
  DROP COLUMN IF EXISTS paid_on,
  DROP COLUMN IF EXISTS tax_invoice_ref,
  DROP COLUMN IF EXISTS receipt_file_id;
-- +goose StatementEnd
