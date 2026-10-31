-- +goose Up
-- +goose StatementBegin
-- Phase 5: Business-unit oversight (§8.7).
--
-- Purpose: management visibility into each business unit (starting with the
-- school) without rebuilding its systems or over-collecting its data. The
-- school management system remains the system of record; Atlas stores
-- aggregates and indicators, not student-level personal data, unless a
-- specific, approved need exists.
--
-- Each unit has its own scope: unit managers see their unit; CEOs see all.
-- Go is the sole writer (ADR-002); ids are UUIDv7; times are timestamptz in
-- UTC. Human references come from core.next_reference (UNI-2026-0001).
CREATE SCHEMA IF NOT EXISTS units;

-- Business units: each unit is a scope — unit managers see theirs, CEOs see
-- all. The school is the first unit.
CREATE TABLE units.business_units (
  id              uuid PRIMARY KEY,
  ref             text NOT NULL UNIQUE,
  name            text NOT NULL,
  code            text NOT NULL UNIQUE,
  kind            text NOT NULL CHECK (kind IN ('school','department','project','region')),
  owner_person_id uuid REFERENCES people.persons(id),
  description     text,
  active          boolean NOT NULL DEFAULT true,
  version         integer NOT NULL DEFAULT 1,
  created_at      timestamptz NOT NULL DEFAULT now(),
  updated_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_units_business_units_kind   ON units.business_units(kind);
CREATE INDEX idx_units_business_units_owner  ON units.business_units(owner_person_id);
CREATE INDEX idx_units_business_units_active ON units.business_units(active);

-- Unit integrations: how Atlas connects to a unit's system of record.
-- Endpoint, credential reference (never the secret), schedule, last sync status.
CREATE TABLE units.unit_integrations (
  id               uuid PRIMARY KEY,
  unit_id          uuid NOT NULL REFERENCES units.business_units(id) ON DELETE CASCADE,
  name             text NOT NULL,
  kind             text NOT NULL CHECK (kind IN ('api','webhook','file','sftp')),
  endpoint         text NOT NULL,
  credential_ref   text NOT NULL,
  schedule         text,
  last_sync_at     timestamptz,
  last_sync_status text CHECK (last_sync_status IN ('ok','failed','never')),
  active           boolean NOT NULL DEFAULT true,
  version          integer NOT NULL DEFAULT 1,
  created_at       timestamptz NOT NULL DEFAULT now(),
  updated_at       timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_units_unit_integrations_unit      ON units.unit_integrations(unit_id);
CREATE INDEX idx_units_unit_integrations_last_sync ON units.unit_integrations(last_sync_at);
CREATE INDEX idx_units_unit_integrations_active    ON units.unit_integrations(active);

-- Unit indicator definitions: what we measure for each unit — name, formula /
-- source field, target, thresholds.
CREATE TABLE units.unit_indicator_definitions (
  id             uuid PRIMARY KEY,
  unit_id        uuid NOT NULL REFERENCES units.business_units(id) ON DELETE CASCADE,
  name           text NOT NULL,
  kind           text NOT NULL CHECK (kind IN ('count','ratio','amount','boolean')),
  source_field   text NOT NULL,
  formula        text,
  target         text,
  warning_threshold text,
  critical_threshold text,
  active         boolean NOT NULL DEFAULT true,
  version        integer NOT NULL DEFAULT 1,
  created_at     timestamptz NOT NULL DEFAULT now(),
  updated_at     timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_units_unit_indicator_definitions_unit ON units.unit_indicator_definitions(unit_id);
CREATE INDEX idx_units_unit_indicator_definitions_active ON units.unit_indicator_definitions(active);

-- Unit snapshots: a dated indicator set for a unit. One row per indicator per
-- date, so a unit's history is a time series without over-collecting personal
-- data (§8.7).
CREATE TABLE units.unit_snapshots (
  id           uuid PRIMARY KEY,
  unit_id      uuid NOT NULL REFERENCES units.business_units(id) ON DELETE CASCADE,
  on_date      date NOT NULL,
  indicator_id uuid NOT NULL REFERENCES units.unit_indicator_definitions(id) ON DELETE CASCADE,
  value        text NOT NULL,
  met_target   boolean NOT NULL DEFAULT false,
  notes        text,
  created_at   timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_units_unit_snapshots_unit_date ON units.unit_snapshots(unit_id, on_date DESC);
CREATE INDEX idx_units_unit_snapshots_indicator ON units.unit_snapshots(indicator_id);
CREATE UNIQUE INDEX idx_units_unit_snapshots_one_per_indicator_per_day
  ON units.unit_snapshots(unit_id, indicator_id, on_date);

-- Unit finance summaries: period roll-up of income, expenses, receivables.
-- Money is integer minor units (ADR-010).
CREATE TABLE units.unit_finance_summaries (
  id             uuid PRIMARY KEY,
  unit_id        uuid NOT NULL REFERENCES units.business_units(id) ON DELETE CASCADE,
  period_start   date NOT NULL,
  period_end     date NOT NULL,
  income_minor   bigint NOT NULL DEFAULT 0,
  expenses_minor bigint NOT NULL DEFAULT 0,
  receivables_minor bigint NOT NULL DEFAULT 0,
  created_at     timestamptz NOT NULL DEFAULT now(),
  updated_at     timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_units_unit_finance_summaries_unit_period ON units.unit_finance_summaries(unit_id, period_start DESC);
CREATE UNIQUE INDEX idx_units_unit_finance_summaries_one_per_unit_per_period
  ON units.unit_finance_summaries(unit_id, period_start, period_end);

-- updated_at triggers, the same convention every other schema uses.
CREATE TRIGGER units_business_units_touch BEFORE UPDATE ON units.business_units
  FOR EACH ROW EXECUTE FUNCTION core.touch_updated_at();
CREATE TRIGGER units_unit_integrations_touch BEFORE UPDATE ON units.unit_integrations
  FOR EACH ROW EXECUTE FUNCTION core.touch_updated_at();
CREATE TRIGGER units_unit_indicator_definitions_touch BEFORE UPDATE ON units.unit_indicator_definitions
  FOR EACH ROW EXECUTE FUNCTION core.touch_updated_at();
CREATE TRIGGER units_unit_finance_summaries_touch BEFORE UPDATE ON units.unit_finance_summaries
  FOR EACH ROW EXECUTE FUNCTION core.touch_updated_at();
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TRIGGER IF EXISTS units_unit_finance_summaries_touch ON units.unit_finance_summaries;
DROP TRIGGER IF EXISTS units_unit_indicator_definitions_touch ON units.unit_indicator_definitions;
DROP TRIGGER IF EXISTS units_unit_integrations_touch ON units.unit_integrations;
DROP TRIGGER IF EXISTS units_business_units_touch ON units.business_units;
DROP INDEX IF EXISTS idx_units_unit_finance_summaries_unit_period;
DROP INDEX IF EXISTS idx_units_unit_finance_summaries_one_per_unit_per_period;
DROP INDEX IF EXISTS idx_units_unit_snapshots_one_per_indicator_per_day;
DROP INDEX IF EXISTS idx_units_unit_snapshots_indicator;
DROP INDEX IF EXISTS idx_units_unit_snapshots_unit_date;
DROP INDEX IF EXISTS idx_units_unit_indicator_definitions_active;
DROP INDEX IF EXISTS idx_units_unit_indicator_definitions_unit;
DROP INDEX IF EXISTS idx_units_unit_integrations_active;
DROP INDEX IF EXISTS idx_units_unit_integrations_last_sync;
DROP INDEX IF EXISTS idx_units_unit_integrations_unit;
DROP INDEX IF EXISTS idx_units_business_units_active;
DROP INDEX IF EXISTS idx_units_business_units_owner;
DROP INDEX IF EXISTS idx_units_business_units_kind;
DROP TABLE IF EXISTS units.unit_finance_summaries;
DROP TABLE IF EXISTS units.unit_snapshots;
DROP TABLE IF EXISTS units.unit_indicator_definitions;
DROP TABLE IF EXISTS units.unit_integrations;
DROP TABLE IF EXISTS units.business_units;
DROP SCHEMA IF EXISTS units;
-- +goose StatementEnd
