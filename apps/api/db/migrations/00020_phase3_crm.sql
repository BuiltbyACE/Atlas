-- +goose Up
-- +goose StatementBegin
-- Phase 3: CRM & client acquisition (docs/ATLAS.md §8.1).
--
-- Go is the sole writer (ADR-002). Money is stored as integer minor units with
-- a currency (ADR-010); ids are UUIDv7; times are timestamptz in UTC. Human
-- references (LEAD-2026-0042, OPP-…, PROP-…) come from core.next_reference.
--
-- Lead sources and pipeline stages are [CONFIG] (§8.1): the tables ship empty
-- and the seed package inserts the initial rows, so an operator edit later is
-- preserved.
CREATE SCHEMA IF NOT EXISTS crm;

-- Config: where leads come from (§8.1). Website form, referral, event, inbound
-- call, WhatsApp, outbound, partner, existing client.
CREATE TABLE crm.lead_sources (
  key         text PRIMARY KEY,
  label       text NOT NULL,
  sort_order  integer NOT NULL DEFAULT 0,
  active      boolean NOT NULL DEFAULT true,
  created_at  timestamptz NOT NULL DEFAULT now(),
  updated_at  timestamptz NOT NULL DEFAULT now()
);

-- Config: ordered pipeline stages (§8.1). Discovery → Solution design →
-- Proposal sent → Negotiation → Contract → Won / Lost.
CREATE TABLE crm.pipeline_stages (
  key         text PRIMARY KEY,
  label       text NOT NULL,
  sort_order  integer NOT NULL DEFAULT 0,
  is_won      boolean NOT NULL DEFAULT false,
  is_lost     boolean NOT NULL DEFAULT false,
  active      boolean NOT NULL DEFAULT true,
  created_at  timestamptz NOT NULL DEFAULT now(),
  updated_at  timestamptz NOT NULL DEFAULT now()
);

-- Organizations: a client or prospect company (§8.1).
CREATE TABLE crm.organizations (
  id               uuid PRIMARY KEY,
  name             text NOT NULL,
  -- name_normalized is lower(trim(name)) kept by Go; duplicate detection on
  -- organization name compares against it (§8.1 "duplicate detection").
  name_normalized  text NOT NULL,
  industry         text,
  size             text,
  location         text,
  kra_pin          text,
  status           text NOT NULL CHECK (status IN ('prospect','client','former')) DEFAULT 'prospect',
  owner_person_id  uuid REFERENCES people.persons(id),
  version          integer NOT NULL DEFAULT 1,
  created_at       timestamptz NOT NULL DEFAULT now(),
  updated_at       timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_crm_organizations_status ON crm.organizations(status);
CREATE INDEX idx_crm_organizations_owner ON crm.organizations(owner_person_id);
CREATE INDEX idx_crm_organizations_name_norm ON crm.organizations(name_normalized);

-- Contacts: a person at an organization, with consent flags, timestamps and
-- source (§8.1, ADR-011). Phones and emails are arrays so a contact can hold
-- several; the WhatsApp number is the single consented business number.
CREATE TABLE crm.contacts (
  id                      uuid PRIMARY KEY,
  organization_id         uuid REFERENCES crm.organizations(id),
  name                    text NOT NULL,
  role                    text,
  phones                  text[] NOT NULL DEFAULT '{}',
  emails                  text[] NOT NULL DEFAULT '{}',
  whatsapp_number         text,
  consent_marketing             boolean NOT NULL DEFAULT false,
  consent_marketing_at          timestamptz,
  consent_marketing_source      text,
  consent_whatsapp              boolean NOT NULL DEFAULT false,
  consent_whatsapp_at           timestamptz,
  consent_whatsapp_source       text,
  consent_call_recording        boolean NOT NULL DEFAULT false,
  consent_call_recording_at     timestamptz,
  consent_call_recording_source text,
  version                 integer NOT NULL DEFAULT 1,
  created_at              timestamptz NOT NULL DEFAULT now(),
  updated_at              timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_crm_contacts_organization ON crm.contacts(organization_id);

-- Opportunities: a qualified deal in the pipeline (§8.1).
CREATE TABLE crm.opportunities (
  id                uuid PRIMARY KEY,
  ref               text NOT NULL UNIQUE,
  organization_id   uuid REFERENCES crm.organizations(id),
  title             text NOT NULL,
  value_minor       bigint,
  currency          char(3) NOT NULL DEFAULT 'KES',
  probability       integer CHECK (probability BETWEEN 0 AND 100),
  stage_key         text NOT NULL REFERENCES crm.pipeline_stages(key),
  expected_close_at date,
  owner_person_id   uuid REFERENCES people.persons(id),
  lost_reason       text CHECK (lost_reason IN ('price','timing','competitor','no_response','scope_fit','other')),
  version           integer NOT NULL DEFAULT 1,
  created_at        timestamptz NOT NULL DEFAULT now(),
  updated_at        timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_crm_opportunities_org ON crm.opportunities(organization_id);
CREATE INDEX idx_crm_opportunities_stage ON crm.opportunities(stage_key);
CREATE INDEX idx_crm_opportunities_owner ON crm.opportunities(owner_person_id);

-- Leads: an enquiry on its way to qualified (§8.1). A lead may carry an
-- organization/contact that already exists, or the raw details of a new one
-- until it is converted.
CREATE TABLE crm.leads (
  id                       uuid PRIMARY KEY,
  ref                      text NOT NULL UNIQUE,
  source_key               text NOT NULL REFERENCES crm.lead_sources(key),
  source_detail            text,
  organization_id          uuid REFERENCES crm.organizations(id),
  contact_id               uuid REFERENCES crm.contacts(id),
  contact_name             text,
  contact_phone            text,
  contact_email            text,
  need_summary             text,
  estimated_value_minor    bigint,
  currency                 char(3) NOT NULL DEFAULT 'KES',
  priority                 text NOT NULL CHECK (priority IN ('low','normal','high','urgent')) DEFAULT 'normal',
  owner_person_id          uuid REFERENCES people.persons(id),
  status                   text NOT NULL CHECK (status IN ('new','assigned','contacted','qualified','nurture','disqualified','converted')) DEFAULT 'new',
  next_action              text,
  next_action_due_at       timestamptz,
  qualification_notes      text,
  lost_reason              text CHECK (lost_reason IN ('price','timing','competitor','no_response','scope_fit','other')),
  converted_opportunity_id uuid REFERENCES crm.opportunities(id),
  version                  integer NOT NULL DEFAULT 1,
  created_at               timestamptz NOT NULL DEFAULT now(),
  updated_at               timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_crm_leads_status ON crm.leads(status);
CREATE INDEX idx_crm_leads_owner ON crm.leads(owner_person_id);
CREATE INDEX idx_crm_leads_source ON crm.leads(source_key);
CREATE INDEX idx_crm_leads_next_action_due ON crm.leads(next_action_due_at);
CREATE INDEX idx_crm_leads_contact_phone ON crm.leads(contact_phone);
CREATE INDEX idx_crm_leads_contact_email ON crm.leads(contact_email);

-- Interactions: a call, WhatsApp message, email, meeting or note (§8.1).
-- Recording/file links are only attached with consent.
CREATE TABLE crm.interactions (
  id                 uuid PRIMARY KEY,
  interaction_type   text NOT NULL CHECK (interaction_type IN ('call','whatsapp','email','meeting','note')),
  direction          text NOT NULL CHECK (direction IN ('inbound','outbound')) DEFAULT 'outbound',
  contact_id         uuid REFERENCES crm.contacts(id),
  lead_id            uuid REFERENCES crm.leads(id),
  opportunity_id     uuid REFERENCES crm.opportunities(id),
  occurred_at        timestamptz NOT NULL,
  duration_minutes   integer CHECK (duration_minutes IS NULL OR duration_minutes >= 0),
  outcome            text NOT NULL,
  summary            text,
  next_action        text,
  next_action_due_at timestamptz,
  recording_file_id  uuid REFERENCES core.files(id),
  recorded_by        uuid NOT NULL REFERENCES people.persons(id),
  version            integer NOT NULL DEFAULT 1,
  created_at         timestamptz NOT NULL DEFAULT now(),
  updated_at         timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_crm_interactions_lead ON crm.interactions(lead_id);
CREATE INDEX idx_crm_interactions_opportunity ON crm.interactions(opportunity_id);
CREATE INDEX idx_crm_interactions_contact ON crm.interactions(contact_id);

-- Follow-ups: a scheduled next touch (§8.1). A missed one escalates.
CREATE TABLE crm.followups (
  id               uuid PRIMARY KEY,
  lead_id          uuid REFERENCES crm.leads(id),
  opportunity_id   uuid REFERENCES crm.opportunities(id),
  owner_person_id  uuid NOT NULL REFERENCES people.persons(id),
  due_at           timestamptz NOT NULL,
  channel          text NOT NULL CHECK (channel IN ('call','whatsapp','email','meeting','other')),
  reason           text NOT NULL,
  status           text NOT NULL CHECK (status IN ('scheduled','done','missed','cancelled')) DEFAULT 'scheduled',
  outcome          text,
  completed_at     timestamptz,
  version          integer NOT NULL DEFAULT 1,
  created_at       timestamptz NOT NULL DEFAULT now(),
  updated_at       timestamptz NOT NULL DEFAULT now(),
  CHECK (lead_id IS NOT NULL OR opportunity_id IS NOT NULL)
);

CREATE INDEX idx_crm_followups_lead ON crm.followups(lead_id);
CREATE INDEX idx_crm_followups_opportunity ON crm.followups(opportunity_id);
CREATE INDEX idx_crm_followups_owner_due ON crm.followups(owner_person_id, due_at);
CREATE INDEX idx_crm_followups_status ON crm.followups(status);

-- Proposals: a priced offer against an opportunity (§8.1). A discount above the
-- threshold requires approval (crm.proposal.discount_approve, §3.3).
CREATE TABLE crm.proposals (
  id               uuid PRIMARY KEY,
  ref              text NOT NULL UNIQUE,
  opportunity_id   uuid NOT NULL REFERENCES crm.opportunities(id),
  version_no       integer NOT NULL DEFAULT 1,
  scope_summary    text,
  line_items       jsonb NOT NULL DEFAULT '[]'::jsonb,
  price_minor      bigint NOT NULL DEFAULT 0,
  currency         char(3) NOT NULL DEFAULT 'KES',
  discount_minor   bigint NOT NULL DEFAULT 0,
  valid_until      date,
  status           text NOT NULL CHECK (status IN ('draft','sent','accepted','declined','expired')) DEFAULT 'draft',
  document_file_id uuid REFERENCES core.files(id),
  approval_id      uuid REFERENCES core.approvals(id),
  version          integer NOT NULL DEFAULT 1,
  created_at       timestamptz NOT NULL DEFAULT now(),
  updated_at       timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_crm_proposals_opportunity ON crm.proposals(opportunity_id);
CREATE INDEX idx_crm_proposals_status ON crm.proposals(status);

-- Touch triggers keep updated_at honest for every mutable table.
CREATE TRIGGER crm_organizations_touch BEFORE UPDATE ON crm.organizations
  FOR EACH ROW EXECUTE FUNCTION core.touch_updated_at();
CREATE TRIGGER crm_contacts_touch BEFORE UPDATE ON crm.contacts
  FOR EACH ROW EXECUTE FUNCTION core.touch_updated_at();
CREATE TRIGGER crm_opportunities_touch BEFORE UPDATE ON crm.opportunities
  FOR EACH ROW EXECUTE FUNCTION core.touch_updated_at();
CREATE TRIGGER crm_leads_touch BEFORE UPDATE ON crm.leads
  FOR EACH ROW EXECUTE FUNCTION core.touch_updated_at();
CREATE TRIGGER crm_interactions_touch BEFORE UPDATE ON crm.interactions
  FOR EACH ROW EXECUTE FUNCTION core.touch_updated_at();
CREATE TRIGGER crm_followups_touch BEFORE UPDATE ON crm.followups
  FOR EACH ROW EXECUTE FUNCTION core.touch_updated_at();
CREATE TRIGGER crm_proposals_touch BEFORE UPDATE ON crm.proposals
  FOR EACH ROW EXECUTE FUNCTION core.touch_updated_at();
CREATE TRIGGER crm_lead_sources_touch BEFORE UPDATE ON crm.lead_sources
  FOR EACH ROW EXECUTE FUNCTION core.touch_updated_at();
CREATE TRIGGER crm_pipeline_stages_touch BEFORE UPDATE ON crm.pipeline_stages
  FOR EACH ROW EXECUTE FUNCTION core.touch_updated_at();

-- Grants mirror the §10.1 / §13.3 role split and 00015: full DML for atlas_api
-- and atlas_worker, SELECT for atlas_readonly, plus default privileges so future
-- crm tables are covered. Roles are created by the platform; every grant is
-- guarded so a bare database still migrates.
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'atlas_api') THEN
    GRANT USAGE ON SCHEMA crm TO atlas_api;
    GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA crm TO atlas_api;
    GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA crm TO atlas_api;
    ALTER DEFAULT PRIVILEGES FOR ROLE atlas_migrator IN SCHEMA crm GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO atlas_api;
    ALTER DEFAULT PRIVILEGES FOR ROLE atlas_migrator IN SCHEMA crm GRANT USAGE, SELECT ON SEQUENCES TO atlas_api;
  END IF;

  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'atlas_worker') THEN
    GRANT USAGE ON SCHEMA crm TO atlas_worker;
    GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA crm TO atlas_worker;
    GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA crm TO atlas_worker;
    ALTER DEFAULT PRIVILEGES FOR ROLE atlas_migrator IN SCHEMA crm GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO atlas_worker;
    ALTER DEFAULT PRIVILEGES FOR ROLE atlas_migrator IN SCHEMA crm GRANT USAGE, SELECT ON SEQUENCES TO atlas_worker;
  END IF;

  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'atlas_readonly') THEN
    GRANT USAGE ON SCHEMA crm TO atlas_readonly;
    GRANT SELECT ON ALL TABLES IN SCHEMA crm TO atlas_readonly;
    ALTER DEFAULT PRIVILEGES FOR ROLE atlas_migrator IN SCHEMA crm GRANT SELECT ON TABLES TO atlas_readonly;
  END IF;
END
$$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'atlas_api') THEN
    REVOKE ALL ON ALL TABLES IN SCHEMA crm FROM atlas_api;
    REVOKE ALL ON ALL SEQUENCES IN SCHEMA crm FROM atlas_api;
    ALTER DEFAULT PRIVILEGES FOR ROLE atlas_migrator IN SCHEMA crm REVOKE ALL ON TABLES FROM atlas_api;
    ALTER DEFAULT PRIVILEGES FOR ROLE atlas_migrator IN SCHEMA crm REVOKE ALL ON SEQUENCES FROM atlas_api;
  END IF;

  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'atlas_worker') THEN
    REVOKE ALL ON ALL TABLES IN SCHEMA crm FROM atlas_worker;
    REVOKE ALL ON ALL SEQUENCES IN SCHEMA crm FROM atlas_worker;
    ALTER DEFAULT PRIVILEGES FOR ROLE atlas_migrator IN SCHEMA crm REVOKE ALL ON TABLES FROM atlas_worker;
    ALTER DEFAULT PRIVILEGES FOR ROLE atlas_migrator IN SCHEMA crm REVOKE ALL ON SEQUENCES FROM atlas_worker;
  END IF;

  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'atlas_readonly') THEN
    REVOKE ALL ON ALL TABLES IN SCHEMA crm FROM atlas_readonly;
    ALTER DEFAULT PRIVILEGES FOR ROLE atlas_migrator IN SCHEMA crm REVOKE ALL ON TABLES FROM atlas_readonly;
  END IF;
END
$$;

DROP SCHEMA IF EXISTS crm CASCADE;
-- +goose StatementEnd
