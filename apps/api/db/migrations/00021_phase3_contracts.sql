-- +goose Up
-- +goose StatementBegin
-- Phase 3: Contracts & commitments (docs/ATLAS.md §8.3).
--
-- A contract is drafted from an accepted proposal (§8.1). Signing
-- (`contracts.contract.signed.v1`) triggers the handover and the financial
-- schedule (§9.1, implemented in later phases). Money is integer minor units
-- (ADR-010); human refs come from core.next_reference (CTR-…, CR-…).
CREATE SCHEMA IF NOT EXISTS contracts;

CREATE TABLE contracts.contracts (
  id               uuid PRIMARY KEY,
  ref              text NOT NULL UNIQUE,
  organization_id  uuid NOT NULL REFERENCES crm.organizations(id),
  opportunity_id   uuid REFERENCES crm.opportunities(id),
  contract_type    text NOT NULL CHECK (contract_type IN ('fixed_price_project','retainer','support_sla','licence','hosting')),
  status           text NOT NULL CHECK (status IN ('draft','internal_review','approved','sent','signed','active','renewal_due','renewed','completed','terminated')) DEFAULT 'draft',
  value_minor      bigint,
  currency         char(3) NOT NULL DEFAULT 'KES',
  start_at         date,
  end_at           date,
  payment_terms    text,
  renewal_terms    text,
  -- governing documents and signatories are small structured lists carried on
  -- the contract itself (document bytes live in core.files).
  governing_docs   jsonb NOT NULL DEFAULT '[]'::jsonb,
  signatories      jsonb NOT NULL DEFAULT '[]'::jsonb,
  signed_at        timestamptz,
  version          integer NOT NULL DEFAULT 1,
  created_at       timestamptz NOT NULL DEFAULT now(),
  updated_at       timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_contracts_contracts_org ON contracts.contracts(organization_id);
CREATE INDEX idx_contracts_contracts_opportunity ON contracts.contracts(opportunity_id);
CREATE INDEX idx_contracts_contracts_status ON contracts.contracts(status);
CREATE INDEX idx_contracts_contracts_end_at ON contracts.contracts(end_at);

-- Every revision of the document, with a change summary.
CREATE TABLE contracts.contract_versions (
  id               uuid PRIMARY KEY,
  contract_id      uuid NOT NULL REFERENCES contracts.contracts(id) ON DELETE CASCADE,
  version_no       integer NOT NULL,
  document_file_id uuid REFERENCES core.files(id),
  change_summary   text,
  created_by       uuid REFERENCES people.persons(id),
  created_at       timestamptz NOT NULL DEFAULT now(),
  UNIQUE (contract_id, version_no)
);

CREATE INDEX idx_contracts_versions_contract ON contracts.contract_versions(contract_id);

-- Billing milestones: a due date and an amount, optionally linked to the work
-- milestone that triggers the invoice (§8.2, §9.1).
CREATE TABLE contracts.milestones (
  id                   uuid PRIMARY KEY,
  contract_id          uuid NOT NULL REFERENCES contracts.contracts(id) ON DELETE CASCADE,
  title                text NOT NULL,
  due_at               date,
  amount_minor         bigint,
  currency             char(3) NOT NULL DEFAULT 'KES',
  project_milestone_id uuid REFERENCES work.milestones(id),
  version              integer NOT NULL DEFAULT 1,
  created_at           timestamptz NOT NULL DEFAULT now(),
  updated_at           timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_contracts_milestones_contract ON contracts.milestones(contract_id);

-- Deliverables: what the contract promises, with acceptance criteria.
CREATE TABLE contracts.deliverables (
  id                  uuid PRIMARY KEY,
  contract_id         uuid NOT NULL REFERENCES contracts.contracts(id) ON DELETE CASCADE,
  description         text NOT NULL,
  acceptance_criteria text,
  milestone_id        uuid REFERENCES contracts.milestones(id),
  version             integer NOT NULL DEFAULT 1,
  created_at          timestamptz NOT NULL DEFAULT now(),
  updated_at          timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_contracts_deliverables_contract ON contracts.deliverables(contract_id);

-- Obligations: who owes what, by when (SafariStack or the client). A client
-- obligation has an owner on our side who chases it; overdue ones become
-- contract risks (§8.3).
CREATE TABLE contracts.obligations (
  id               uuid PRIMARY KEY,
  contract_id      uuid NOT NULL REFERENCES contracts.contracts(id) ON DELETE CASCADE,
  party            text NOT NULL CHECK (party IN ('safaristack','client')),
  description      text NOT NULL,
  due_at           date,
  recurrence       text,
  status           text NOT NULL CHECK (status IN ('open','met','overdue','waived')) DEFAULT 'open',
  owner_person_id  uuid REFERENCES people.persons(id),
  version          integer NOT NULL DEFAULT 1,
  created_at       timestamptz NOT NULL DEFAULT now(),
  updated_at       timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_contracts_obligations_contract ON contracts.obligations(contract_id);
CREATE INDEX idx_contracts_obligations_status ON contracts.obligations(status);

-- SLA terms: a measurable promise (availability, response time) tied to the
-- systems that measure it (§8.6).
CREATE TABLE contracts.sla_terms (
  id                  uuid PRIMARY KEY,
  contract_id         uuid NOT NULL REFERENCES contracts.contracts(id) ON DELETE CASCADE,
  metric              text NOT NULL,
  target              text NOT NULL,
  measurement_window  text,
  system_ids          uuid[] NOT NULL DEFAULT '{}',
  version             integer NOT NULL DEFAULT 1,
  created_at          timestamptz NOT NULL DEFAULT now(),
  updated_at          timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_contracts_sla_terms_contract ON contracts.sla_terms(contract_id);

-- Change requests: a scope/price/deadline change against a signed contract. A
-- change to price or client deadlines needs both CEOs; approval creates a new
-- contract version (§8.3).
CREATE TABLE contracts.change_requests (
  id                  uuid PRIMARY KEY,
  ref                 text NOT NULL UNIQUE,
  contract_id         uuid NOT NULL REFERENCES contracts.contracts(id) ON DELETE CASCADE,
  requested_by        text,
  description         text NOT NULL,
  effort_impact       text,
  price_impact_minor  bigint,
  currency            char(3) NOT NULL DEFAULT 'KES',
  deadline_impact_days integer,
  status              text NOT NULL CHECK (status IN ('draft','submitted','approved','rejected','implemented')) DEFAULT 'draft',
  approval_id         uuid REFERENCES core.approvals(id),
  version             integer NOT NULL DEFAULT 1,
  created_at          timestamptz NOT NULL DEFAULT now(),
  updated_at          timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_contracts_change_requests_contract ON contracts.change_requests(contract_id);
CREATE INDEX idx_contracts_change_requests_status ON contracts.change_requests(status);

CREATE TRIGGER contracts_touch BEFORE UPDATE ON contracts.contracts
  FOR EACH ROW EXECUTE FUNCTION core.touch_updated_at();
CREATE TRIGGER contracts_milestones_touch BEFORE UPDATE ON contracts.milestones
  FOR EACH ROW EXECUTE FUNCTION core.touch_updated_at();
CREATE TRIGGER contracts_deliverables_touch BEFORE UPDATE ON contracts.deliverables
  FOR EACH ROW EXECUTE FUNCTION core.touch_updated_at();
CREATE TRIGGER contracts_obligations_touch BEFORE UPDATE ON contracts.obligations
  FOR EACH ROW EXECUTE FUNCTION core.touch_updated_at();
CREATE TRIGGER contracts_sla_terms_touch BEFORE UPDATE ON contracts.sla_terms
  FOR EACH ROW EXECUTE FUNCTION core.touch_updated_at();
CREATE TRIGGER contracts_change_requests_touch BEFORE UPDATE ON contracts.change_requests
  FOR EACH ROW EXECUTE FUNCTION core.touch_updated_at();

DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'atlas_api') THEN
    GRANT USAGE ON SCHEMA contracts TO atlas_api;
    GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA contracts TO atlas_api;
    GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA contracts TO atlas_api;
    ALTER DEFAULT PRIVILEGES FOR ROLE atlas_migrator IN SCHEMA contracts GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO atlas_api;
    ALTER DEFAULT PRIVILEGES FOR ROLE atlas_migrator IN SCHEMA contracts GRANT USAGE, SELECT ON SEQUENCES TO atlas_api;
  END IF;

  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'atlas_worker') THEN
    GRANT USAGE ON SCHEMA contracts TO atlas_worker;
    GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA contracts TO atlas_worker;
    GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA contracts TO atlas_worker;
    ALTER DEFAULT PRIVILEGES FOR ROLE atlas_migrator IN SCHEMA contracts GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO atlas_worker;
    ALTER DEFAULT PRIVILEGES FOR ROLE atlas_migrator IN SCHEMA contracts GRANT USAGE, SELECT ON SEQUENCES TO atlas_worker;
  END IF;

  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'atlas_readonly') THEN
    GRANT USAGE ON SCHEMA contracts TO atlas_readonly;
    GRANT SELECT ON ALL TABLES IN SCHEMA contracts TO atlas_readonly;
    ALTER DEFAULT PRIVILEGES FOR ROLE atlas_migrator IN SCHEMA contracts GRANT SELECT ON TABLES TO atlas_readonly;
  END IF;
END
$$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'atlas_api') THEN
    REVOKE ALL ON ALL TABLES IN SCHEMA contracts FROM atlas_api;
    REVOKE ALL ON ALL SEQUENCES IN SCHEMA contracts FROM atlas_api;
    ALTER DEFAULT PRIVILEGES FOR ROLE atlas_migrator IN SCHEMA contracts REVOKE ALL ON TABLES FROM atlas_api;
    ALTER DEFAULT PRIVILEGES FOR ROLE atlas_migrator IN SCHEMA contracts REVOKE ALL ON SEQUENCES FROM atlas_api;
  END IF;

  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'atlas_worker') THEN
    REVOKE ALL ON ALL TABLES IN SCHEMA contracts FROM atlas_worker;
    REVOKE ALL ON ALL SEQUENCES IN SCHEMA contracts FROM atlas_worker;
    ALTER DEFAULT PRIVILEGES FOR ROLE atlas_migrator IN SCHEMA contracts REVOKE ALL ON TABLES FROM atlas_worker;
    ALTER DEFAULT PRIVILEGES FOR ROLE atlas_migrator IN SCHEMA contracts REVOKE ALL ON SEQUENCES FROM atlas_worker;
  END IF;

  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'atlas_readonly') THEN
    REVOKE ALL ON ALL TABLES IN SCHEMA contracts FROM atlas_readonly;
    ALTER DEFAULT PRIVILEGES FOR ROLE atlas_migrator IN SCHEMA contracts REVOKE ALL ON TABLES FROM atlas_readonly;
  END IF;
END
$$;

DROP SCHEMA IF EXISTS contracts CASCADE;
-- +goose StatementEnd
