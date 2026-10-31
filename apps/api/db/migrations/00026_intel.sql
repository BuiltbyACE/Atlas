-- +goose Up
-- +goose StatementBegin
-- Phase 6: Executive intelligence (§8.8).
--
-- Purpose: fewer surprises; faster, evidence-based decisions. All numbers are
-- computed deterministically from records and link back to them (every figure
-- is clickable to its source rows).
--
-- Entities: decision (the company's memory of major decisions), risk
-- (client, delivery, financial, people, systems risks), brief (daily executive
-- brief), metric snapshot (point-in-time KPI figures).
--
-- Go is the sole writer (ADR-002); ids are UUIDv7; times are timestamptz in
-- UTC. Human references come from core.next_reference (DEC-2026-0001,
-- RSK-2026-0001, BRF-2026-0001, MET-2026-0001).
CREATE SCHEMA IF NOT EXISTS intel;

-- Decisions: every major decision becomes a searchable record — the company's
-- memory (§8.8).
CREATE TABLE intel.decisions (
  id              uuid PRIMARY KEY,
  ref             text NOT NULL UNIQUE,
  question        text NOT NULL,
  context_links   text[] NOT NULL DEFAULT '{}',
  options         text[] NOT NULL DEFAULT '{}',
  recommendation  text,
  requested_by    uuid REFERENCES people.persons(id),
  requested_by_name text,
  deciders        uuid[] NOT NULL DEFAULT '{}',
  due_on          date,
  outcome         text CHECK (outcome IN ('open','approved','rejected','deferred')) DEFAULT 'open',
  rationale       text,
  version         integer NOT NULL DEFAULT 1,
  created_at      timestamptz NOT NULL DEFAULT now(),
  updated_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_intel_decisions_outcome ON intel.decisions(outcome);
CREATE INDEX idx_intel_decisions_due_on  ON intel.decisions(due_on);
CREATE INDEX idx_intel_decisions_requested_by ON intel.decisions(requested_by);

-- Risks: client, delivery, financial, people, systems risks with owner,
-- likelihood, impact, mitigation (§8.8).
CREATE TABLE intel.risks (
  id              uuid PRIMARY KEY,
  ref             text NOT NULL UNIQUE,
  title           text NOT NULL,
  kind            text NOT NULL CHECK (kind IN ('client','delivery','financial','people','systems')),
  owner_person_id uuid REFERENCES people.persons(id),
  owner_name      text,
  likelihood      text NOT NULL CHECK (likelihood IN ('low','medium','high','critical')) DEFAULT 'medium',
  impact          text NOT NULL CHECK (impact IN ('low','medium','high','critical')) DEFAULT 'medium',
  mitigation      text,
  status          text NOT NULL CHECK (status IN ('open','monitoring','closed','mitigated')) DEFAULT 'open',
  version         integer NOT NULL DEFAULT 1,
  created_at      timestamptz NOT NULL DEFAULT now(),
  updated_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_intel_risks_kind      ON intel.risks(kind);
CREATE INDEX idx_intel_risks_status    ON intel.risks(status);
CREATE INDEX idx_intel_risks_owner     ON intel.risks(owner_person_id);

-- Briefs: daily executive brief generated at 07:30 EAT, delivered in-app, by
-- email and as a WhatsApp summary (§8.8).
CREATE TABLE intel.briefs (
  id          uuid PRIMARY KEY,
  ref         text NOT NULL UNIQUE,
  for_date    date NOT NULL,
  summary     text,
  sections    jsonb NOT NULL DEFAULT '{}',
  delivered   boolean NOT NULL DEFAULT false,
  delivered_at timestamptz,
  version     integer NOT NULL DEFAULT 1,
  created_at  timestamptz NOT NULL DEFAULT now(),
  updated_at  timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_intel_briefs_for_date ON intel.briefs(for_date);
CREATE INDEX idx_intel_briefs_delivered ON intel.briefs(delivered);

-- Metric snapshots: point-in-time KPI figures (§8.8).
CREATE TABLE intel.metric_snapshots (
  id          uuid PRIMARY KEY,
  ref         text NOT NULL UNIQUE,
  metric_name text NOT NULL,
  metric_value text NOT NULL,
  as_of       date NOT NULL,
  source      text,
  version     integer NOT NULL DEFAULT 1,
  created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_intel_metric_snapshots_metric ON intel.metric_snapshots(metric_name, as_of DESC);

-- updated_at triggers, the same convention every other schema uses.
CREATE TRIGGER intel_decisions_touch BEFORE UPDATE ON intel.decisions
  FOR EACH ROW EXECUTE FUNCTION core.touch_updated_at();
CREATE TRIGGER intel_risks_touch BEFORE UPDATE ON intel.risks
  FOR EACH ROW EXECUTE FUNCTION core.touch_updated_at();
CREATE TRIGGER intel_briefs_touch BEFORE UPDATE ON intel.briefs
  FOR EACH ROW EXECUTE FUNCTION core.touch_updated_at();
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TRIGGER IF EXISTS intel_briefs_touch ON intel.briefs;
DROP TRIGGER IF EXISTS intel_risks_touch ON intel.risks;
DROP TRIGGER IF EXISTS intel_decisions_touch ON intel.decisions;
DROP INDEX IF EXISTS idx_intel_metric_snapshots_metric;
DROP INDEX IF EXISTS idx_intel_briefs_delivered;
DROP INDEX IF EXISTS idx_intel_briefs_for_date;
DROP INDEX IF EXISTS idx_intel_risks_owner;
DROP INDEX IF EXISTS idx_intel_risks_status;
DROP INDEX IF EXISTS idx_intel_risks_kind;
DROP INDEX IF EXISTS idx_intel_decisions_requested_by;
DROP INDEX IF EXISTS idx_intel_decisions_due_on;
DROP INDEX IF EXISTS idx_intel_decisions_outcome;
DROP TABLE IF EXISTS intel.metric_snapshots;
DROP TABLE IF EXISTS intel.briefs;
DROP TABLE IF EXISTS intel.risks;
DROP TABLE IF EXISTS intel.decisions;
DROP SCHEMA IF EXISTS intel;
-- +goose StatementEnd
