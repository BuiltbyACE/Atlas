-- +goose Up
-- +goose StatementBegin
-- Phase 4: Finance & commercial control (docs/ATLAS.md §8.4).
--
-- Atlas is the management-finance system: cash position, receivables, project
-- profitability and spending. Go is the sole writer (ADR-002). Money is stored
-- as integer minor units with a currency (ADR-010) — never floats, never a bare
-- amount without a currency; times are timestamptz in UTC; ids are UUIDv7.
-- Human references (EXP-2026-0142, INV-…, PAY-…) come from core.next_reference.
--
-- The expense categories are [CONFIG] (§8.4): the table ships empty and the seed
-- package inserts the initial rows, so an operator edit later is preserved.
--
-- This migration is the read side's storage. The write lifecycle
-- (expenses.submit/decide, invoices.issue, payments.record) lands in a later
-- pass; the tables carry the status columns and version counters it needs so no
-- further migration is required to add it.
CREATE SCHEMA IF NOT EXISTS finance;

-- Config: the spend categories an expense is filed under (§8.4 [CONFIG]).
-- operational, salaries, hosting/cloud, software subscriptions, transport,
-- marketing, contractors, equipment.
CREATE TABLE finance.expense_categories (
  key         text PRIMARY KEY,
  label       text NOT NULL,
  sort_order  integer NOT NULL DEFAULT 0,
  active      boolean NOT NULL DEFAULT true,
  created_at  timestamptz NOT NULL DEFAULT now(),
  updated_at  timestamptz NOT NULL DEFAULT now()
);

-- Accounts: where cash sits — bank, cash, M-Pesa till/paybill, card (§8.4).
-- opening_balance_minor is the balance before Atlas recorded anything; the
-- current balance is derived from payments and paid expenses at read time.
CREATE TABLE finance.accounts (
  id                    uuid PRIMARY KEY,
  name                  text NOT NULL,
  account_type          text NOT NULL CHECK (account_type IN ('bank','cash','mpesa_till','mpesa_paybill','card')),
  currency              char(3) NOT NULL DEFAULT 'KES',
  opening_balance_minor bigint,
  active                boolean NOT NULL DEFAULT true,
  version               integer NOT NULL DEFAULT 1,
  created_at            timestamptz NOT NULL DEFAULT now(),
  updated_at            timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_finance_accounts_type ON finance.accounts(account_type);
CREATE INDEX idx_finance_accounts_active ON finance.accounts(active);

-- Budgets: a ceiling for a scope and period (§8.4). scope_ref names the
-- department, project or unit the scope points at — for a project it is the
-- work.projects id as text, so a budget survives the project row being moved.
CREATE TABLE finance.budgets (
  id              uuid PRIMARY KEY,
  scope           text NOT NULL CHECK (scope IN ('company_period','department','project','unit')),
  scope_ref       text,
  period_start    date NOT NULL,
  period_end      date NOT NULL,
  category_key    text REFERENCES finance.expense_categories(key),
  amount_minor    bigint,
  currency        char(3) NOT NULL DEFAULT 'KES',
  status          text NOT NULL CHECK (status IN ('draft','approved','closed')) DEFAULT 'draft',
  owner_person_id uuid REFERENCES people.persons(id),
  version         integer NOT NULL DEFAULT 1,
  created_at      timestamptz NOT NULL DEFAULT now(),
  updated_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_finance_budgets_scope ON finance.budgets(scope);
CREATE INDEX idx_finance_budgets_status ON finance.budgets(status);
CREATE INDEX idx_finance_budgets_period ON finance.budgets(period_start, period_end);
CREATE INDEX idx_finance_budgets_category ON finance.budgets(category_key);
CREATE INDEX idx_finance_budgets_owner ON finance.budgets(owner_person_id);

-- Expenses: a claim with a category, an optional project, a date and a status
-- (§8.4). status follows Draft → Submitted → Approved → Paid / Reimbursed →
-- Reconciled, or Rejected with a reason. account_id is the account a paid
-- expense left from, so the account balance can be derived.
CREATE TABLE finance.expenses (
  id                  uuid PRIMARY KEY,
  ref                 text NOT NULL UNIQUE,
  claimant_person_id  uuid NOT NULL REFERENCES people.persons(id),
  category_key        text NOT NULL REFERENCES finance.expense_categories(key),
  project_id          uuid REFERENCES work.projects(id),
  amount_minor        bigint,
  currency            char(3) NOT NULL DEFAULT 'KES',
  spent_on            date NOT NULL,
  vendor              text,
  payment_method      text CHECK (payment_method IN ('bank','mpesa','card','cash')),
  account_id          uuid REFERENCES finance.accounts(id),
  status              text NOT NULL CHECK (status IN ('draft','submitted','approved','rejected','paid','reimbursed','reconciled')) DEFAULT 'draft',
  submitted_at        timestamptz,
  decided_by          uuid REFERENCES people.persons(id),
  decided_at          timestamptz,
  decision_reason     text,
  notes               text,
  version             integer NOT NULL DEFAULT 1,
  created_at          timestamptz NOT NULL DEFAULT now(),
  updated_at          timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_finance_expenses_status ON finance.expenses(status);
CREATE INDEX idx_finance_expenses_category ON finance.expenses(category_key);
CREATE INDEX idx_finance_expenses_project ON finance.expenses(project_id);
CREATE INDEX idx_finance_expenses_claimant ON finance.expenses(claimant_person_id);
CREATE INDEX idx_finance_expenses_spent_on ON finance.expenses(spent_on);
CREATE INDEX idx_finance_expenses_account ON finance.expenses(account_id);

-- Invoices: what a client owes, from a contract milestone or a recurring
-- schedule (§8.4). milestone_id points at the billing milestone; the project a
-- milestone belongs to is reached through contracts.milestones.project_milestone_id.
CREATE TABLE finance.invoices (
  id              uuid PRIMARY KEY,
  ref             text NOT NULL UNIQUE,
  organization_id uuid NOT NULL REFERENCES crm.organizations(id),
  contract_id     uuid REFERENCES contracts.contracts(id),
  milestone_id    uuid REFERENCES contracts.milestones(id),
  issue_date      date,
  due_date        date,
  total_minor     bigint,
  currency        char(3) NOT NULL DEFAULT 'KES',
  status          text NOT NULL CHECK (status IN ('scheduled','draft','approved','issued','partially_paid','paid','overdue','disputed','written_off')) DEFAULT 'draft',
  issued_at       timestamptz,
  paid_at         timestamptz,
  e_invoice_ref   text,
  notes           text,
  version         integer NOT NULL DEFAULT 1,
  created_at      timestamptz NOT NULL DEFAULT now(),
  updated_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_finance_invoices_status ON finance.invoices(status);
CREATE INDEX idx_finance_invoices_organization ON finance.invoices(organization_id);
CREATE INDEX idx_finance_invoices_contract ON finance.invoices(contract_id);
CREATE INDEX idx_finance_invoices_milestone ON finance.invoices(milestone_id);
CREATE INDEX idx_finance_invoices_issue_date ON finance.invoices(issue_date);
CREATE INDEX idx_finance_invoices_due_date ON finance.invoices(due_date);

-- Invoice lines: the description, quantity and price that sum to a total (§8.4).
-- quantity is a decimal, not money, so numeric is right here; the *_minor
-- columns stay integer. tax_rate_bp is basis points (1600 = 16%).
CREATE TABLE finance.invoice_lines (
  id               uuid PRIMARY KEY,
  invoice_id       uuid NOT NULL REFERENCES finance.invoices(id) ON DELETE CASCADE,
  description      text NOT NULL,
  quantity         numeric(14,3) NOT NULL DEFAULT 1,
  unit_price_minor bigint,
  tax_rate_bp      integer NOT NULL DEFAULT 0,
  amount_minor     bigint,
  sort_order       integer NOT NULL DEFAULT 0,
  created_at       timestamptz NOT NULL DEFAULT now(),
  updated_at       timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_finance_invoice_lines_invoice ON finance.invoice_lines(invoice_id);

-- Payments: money received (§8.4). reconciled is the flag an operator sets once
-- the bank/M-Pesa line has been matched. Allocation to invoices lives in
-- finance.payment_allocations, so one payment can settle several invoices.
CREATE TABLE finance.payments (
  id          uuid PRIMARY KEY,
  ref         text NOT NULL UNIQUE,
  amount_minor bigint,
  currency    char(3) NOT NULL DEFAULT 'KES',
  method      text NOT NULL CHECK (method IN ('bank','mpesa','card','cash')),
  reference   text,
  received_on date NOT NULL,
  account_id  uuid REFERENCES finance.accounts(id),
  reconciled  boolean NOT NULL DEFAULT false,
  notes       text,
  version     integer NOT NULL DEFAULT 1,
  created_at  timestamptz NOT NULL DEFAULT now(),
  updated_at  timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_finance_payments_method ON finance.payments(method);
CREATE INDEX idx_finance_payments_received_on ON finance.payments(received_on);
CREATE INDEX idx_finance_payments_account ON finance.payments(account_id);

-- Payment allocations: how much of a payment settled which invoice (§8.4).
-- Unallocated money is a payment minus the sum of its allocations — the
-- reconciliation queue.
CREATE TABLE finance.payment_allocations (
  id           uuid PRIMARY KEY,
  payment_id   uuid NOT NULL REFERENCES finance.payments(id) ON DELETE CASCADE,
  invoice_id   uuid NOT NULL REFERENCES finance.invoices(id),
  amount_minor bigint NOT NULL,
  created_at   timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_finance_payment_allocations_payment ON finance.payment_allocations(payment_id);
CREATE INDEX idx_finance_payment_allocations_invoice ON finance.payment_allocations(invoice_id);

-- Recurring costs: standing vendor charges — hosting, domains, SaaS (§8.4) —
-- with the next renewal date the calendar and the 30-day reminder read.
CREATE TABLE finance.recurring_costs (
  id              uuid PRIMARY KEY,
  vendor          text NOT NULL,
  description     text,
  amount_minor    bigint,
  currency        char(3) NOT NULL DEFAULT 'KES',
  cycle           text NOT NULL CHECK (cycle IN ('monthly','quarterly','annual')),
  next_renewal_on date NOT NULL,
  owner_person_id uuid REFERENCES people.persons(id),
  active          boolean NOT NULL DEFAULT true,
  version         integer NOT NULL DEFAULT 1,
  created_at      timestamptz NOT NULL DEFAULT now(),
  updated_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_finance_recurring_costs_next_renewal ON finance.recurring_costs(next_renewal_on);
CREATE INDEX idx_finance_recurring_costs_active ON finance.recurring_costs(active);
CREATE INDEX idx_finance_recurring_costs_owner ON finance.recurring_costs(owner_person_id);

-- Exchange rates: the USD→KES rate for a date, for USD contracts (§8.4). The
-- read slice reports a single company currency and does not yet consolidate, so
-- nothing reads this table yet; it is created now so the consolidation pass has
-- its storage and does not need a new migration.
CREATE TABLE finance.exchange_rates (
  id            uuid PRIMARY KEY,
  rate_on       date NOT NULL,
  from_currency char(3) NOT NULL,
  to_currency   char(3) NOT NULL,
  rate          numeric(18,8) NOT NULL CHECK (rate > 0),
  created_at    timestamptz NOT NULL DEFAULT now(),
  updated_at    timestamptz NOT NULL DEFAULT now(),
  UNIQUE (rate_on, from_currency, to_currency)
);

CREATE INDEX idx_finance_exchange_rates_on ON finance.exchange_rates(rate_on);

CREATE TRIGGER finance_accounts_touch BEFORE UPDATE ON finance.accounts
  FOR EACH ROW EXECUTE FUNCTION core.touch_updated_at();
CREATE TRIGGER finance_expense_categories_touch BEFORE UPDATE ON finance.expense_categories
  FOR EACH ROW EXECUTE FUNCTION core.touch_updated_at();
CREATE TRIGGER finance_budgets_touch BEFORE UPDATE ON finance.budgets
  FOR EACH ROW EXECUTE FUNCTION core.touch_updated_at();
CREATE TRIGGER finance_expenses_touch BEFORE UPDATE ON finance.expenses
  FOR EACH ROW EXECUTE FUNCTION core.touch_updated_at();
CREATE TRIGGER finance_invoices_touch BEFORE UPDATE ON finance.invoices
  FOR EACH ROW EXECUTE FUNCTION core.touch_updated_at();
CREATE TRIGGER finance_invoice_lines_touch BEFORE UPDATE ON finance.invoice_lines
  FOR EACH ROW EXECUTE FUNCTION core.touch_updated_at();
CREATE TRIGGER finance_payments_touch BEFORE UPDATE ON finance.payments
  FOR EACH ROW EXECUTE FUNCTION core.touch_updated_at();
CREATE TRIGGER finance_recurring_costs_touch BEFORE UPDATE ON finance.recurring_costs
  FOR EACH ROW EXECUTE FUNCTION core.touch_updated_at();
CREATE TRIGGER finance_exchange_rates_touch BEFORE UPDATE ON finance.exchange_rates
  FOR EACH ROW EXECUTE FUNCTION core.touch_updated_at();

DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'atlas_api') THEN
    GRANT USAGE ON SCHEMA finance TO atlas_api;
    GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA finance TO atlas_api;
    GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA finance TO atlas_api;
    ALTER DEFAULT PRIVILEGES FOR ROLE atlas_migrator IN SCHEMA finance GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO atlas_api;
    ALTER DEFAULT PRIVILEGES FOR ROLE atlas_migrator IN SCHEMA finance GRANT USAGE, SELECT ON SEQUENCES TO atlas_api;
  END IF;

  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'atlas_worker') THEN
    GRANT USAGE ON SCHEMA finance TO atlas_worker;
    GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA finance TO atlas_worker;
    GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA finance TO atlas_worker;
    ALTER DEFAULT PRIVILEGES FOR ROLE atlas_migrator IN SCHEMA finance GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO atlas_worker;
    ALTER DEFAULT PRIVILEGES FOR ROLE atlas_migrator IN SCHEMA finance GRANT USAGE, SELECT ON SEQUENCES TO atlas_worker;
  END IF;

  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'atlas_readonly') THEN
    GRANT USAGE ON SCHEMA finance TO atlas_readonly;
    GRANT SELECT ON ALL TABLES IN SCHEMA finance TO atlas_readonly;
    ALTER DEFAULT PRIVILEGES FOR ROLE atlas_migrator IN SCHEMA finance GRANT SELECT ON TABLES TO atlas_readonly;
  END IF;
END
$$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'atlas_api') THEN
    REVOKE ALL ON ALL TABLES IN SCHEMA finance FROM atlas_api;
    REVOKE ALL ON ALL SEQUENCES IN SCHEMA finance FROM atlas_api;
    ALTER DEFAULT PRIVILEGES FOR ROLE atlas_migrator IN SCHEMA finance REVOKE ALL ON TABLES FROM atlas_api;
    ALTER DEFAULT PRIVILEGES FOR ROLE atlas_migrator IN SCHEMA finance REVOKE ALL ON SEQUENCES FROM atlas_api;
  END IF;

  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'atlas_worker') THEN
    REVOKE ALL ON ALL TABLES IN SCHEMA finance FROM atlas_worker;
    REVOKE ALL ON ALL SEQUENCES IN SCHEMA finance FROM atlas_worker;
    ALTER DEFAULT PRIVILEGES FOR ROLE atlas_migrator IN SCHEMA finance REVOKE ALL ON TABLES FROM atlas_worker;
    ALTER DEFAULT PRIVILEGES FOR ROLE atlas_migrator IN SCHEMA finance REVOKE ALL ON SEQUENCES FROM atlas_worker;
  END IF;

  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'atlas_readonly') THEN
    REVOKE ALL ON ALL TABLES IN SCHEMA finance FROM atlas_readonly;
    ALTER DEFAULT PRIVILEGES FOR ROLE atlas_migrator IN SCHEMA finance REVOKE ALL ON TABLES FROM atlas_readonly;
  END IF;
END
$$;

DROP SCHEMA IF EXISTS finance CASCADE;
-- +goose StatementEnd
