-- +goose Up
-- +goose StatementBegin
-- Phase 5: Systems observability & integrations (docs/ATLAS.md §8.6).
--
-- Purpose: every website, app, API and integration SafariStack runs —
-- for itself or a client — has an owner, is monitored, and has an incident
-- history. Go is the sole writer (ADR-002); ids are UUIDv7; times are
-- timestamptz in UTC. Human references (SYS-2026-0007, INC-2026-0031) come
-- from core.next_reference.
--
-- A credential_record never holds the secret (§8.6): it records that a
-- credential exists and where it lives in a secrets manager. Atlas stores the
-- pointer, never the value.
--
-- This migration is the storage the read side and the monitoring worker share.
-- The tables carry the status columns and version counters the write lifecycle
-- needs, so no further migration is required to add it.
CREATE SCHEMA IF NOT EXISTS systems;

-- Systems: the register of everything we run (§8.6). organization_id is set
-- only when the system is client-hosted by us, which is also when a contract
-- and SLA link are meaningful.
CREATE TABLE systems.systems (
  id              uuid PRIMARY KEY,
  ref             text NOT NULL UNIQUE,
  name            text NOT NULL,
  system_type     text NOT NULL CHECK (system_type IN
                    ('website','web_app','api','mobile_backend','integration','background_job','database')),
  owner_person_id uuid REFERENCES people.persons(id),
  organization_id uuid REFERENCES crm.organizations(id),
  environment     text NOT NULL CHECK (environment IN ('prod','staging')) DEFAULT 'prod',
  criticality     text NOT NULL CHECK (criticality IN ('tier1','tier2','tier3')) DEFAULT 'tier3',
  contract_id     uuid REFERENCES contracts.contracts(id),
  runbook_url     text,
  repository_url  text,
  description     text,
  status          text NOT NULL CHECK (status IN ('active','retired')) DEFAULT 'active',
  version         integer NOT NULL DEFAULT 1,
  created_at      timestamptz NOT NULL DEFAULT now(),
  updated_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_systems_systems_type        ON systems.systems(system_type);
CREATE INDEX idx_systems_systems_environment ON systems.systems(environment);
CREATE INDEX idx_systems_systems_criticality ON systems.systems(criticality);
CREATE INDEX idx_systems_systems_owner       ON systems.systems(owner_person_id);
CREATE INDEX idx_systems_systems_org         ON systems.systems(organization_id);
CREATE INDEX idx_systems_systems_status      ON systems.systems(status);

-- Checks: one probe against a system (§8.6). `target` is a URL, host:port,
-- hostname or job key depending on check_type; `expect` is the content or
-- status marker a readiness probe must find — Atlas checks content, not just
-- 200 OK (§8.6). failure_threshold is the flap-protection count: an incident
-- opens only after this many consecutive failures.
CREATE TABLE systems.checks (
  id                uuid PRIMARY KEY,
  system_id         uuid NOT NULL REFERENCES systems.systems(id) ON DELETE CASCADE,
  name              text NOT NULL,
  check_type        text NOT NULL CHECK (check_type IN
                      ('http_status','http_content','tcp','tls_expiry','dns_expiry','heartbeat','latency','dependency')),
  target            text NOT NULL,
  expect            text,
  interval_seconds  integer NOT NULL DEFAULT 300 CHECK (interval_seconds > 0),
  timeout_seconds   integer NOT NULL DEFAULT 10 CHECK (timeout_seconds > 0),
  regions           text[] NOT NULL DEFAULT '{}',
  warn_ms           integer CHECK (warn_ms IS NULL OR warn_ms >= 0),
  fail_ms           integer CHECK (fail_ms IS NULL OR fail_ms >= 0),
  failure_threshold integer NOT NULL DEFAULT 2 CHECK (failure_threshold > 0),
  -- A heartbeat check's expected window: silence longer than this is a failure.
  heartbeat_window_seconds integer CHECK (heartbeat_window_seconds IS NULL OR heartbeat_window_seconds > 0),
  active            boolean NOT NULL DEFAULT true,
  version           integer NOT NULL DEFAULT 1,
  created_at        timestamptz NOT NULL DEFAULT now(),
  updated_at        timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_systems_checks_system ON systems.checks(system_id);
CREATE INDEX idx_systems_checks_type   ON systems.checks(check_type);
CREATE INDEX idx_systems_checks_active ON systems.checks(active);

-- Check results: one row per probe, partitioned by month (§8.6). The partition
-- key is part of the primary key because Postgres requires it. A listener
-- creates the month's partition ahead of time with
-- systems.ensure_check_result_partition(); the default partition is a safety
-- net so a probe can never fail to record for want of a partition.
CREATE TABLE systems.check_results (
  id         uuid NOT NULL,
  check_id   uuid NOT NULL REFERENCES systems.checks(id) ON DELETE CASCADE,
  at         timestamptz NOT NULL,
  status     text NOT NULL CHECK (status IN ('pass','warn','fail')),
  latency_ms integer CHECK (latency_ms IS NULL OR latency_ms >= 0),
  detail     text,
  source     text NOT NULL CHECK (source IN ('atlas','heartbeat','client_reported')) DEFAULT 'atlas',
  region     text,
  PRIMARY KEY (id, at)
) PARTITION BY RANGE (at);

CREATE TABLE systems.check_results_default PARTITION OF systems.check_results DEFAULT;

CREATE INDEX idx_systems_check_results_check ON systems.check_results(check_id, at DESC);
CREATE INDEX idx_systems_check_results_status ON systems.check_results(status);

-- ensure_check_result_partition creates the month partition covering p_month
-- if it is not there yet. The monitoring worker calls it as it inserts; a
-- missing partition would otherwise push a row into the default partition,
-- which is correct but unbounded.
-- +goose StatementEnd
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION systems.ensure_check_result_partition(p_month date)
RETURNS void LANGUAGE plpgsql AS $$
DECLARE
  start_month date := date_trunc('month', p_month)::date;
  end_month   date := (date_trunc('month', p_month) + interval '1 month')::date;
  part_name   text := 'check_results_' || to_char(start_month, 'YYYY_MM');
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
    WHERE n.nspname = 'systems' AND c.relname = part_name
  ) THEN
    EXECUTE format(
      'CREATE TABLE systems.%I PARTITION OF systems.check_results FOR VALUES FROM (%L) TO (%L)',
      part_name, start_month, end_month);
  END IF;
END;
$$;
-- +goose StatementEnd
-- +goose StatementBegin

-- Route the current and next month explicitly so the first results land in a
-- bounded partition rather than the default.
SELECT systems.ensure_check_result_partition(date_trunc('month', now())::date);
SELECT systems.ensure_check_result_partition((date_trunc('month', now()) + interval '1 month')::date);

-- Incidents: an outage or degradation, from detection through review (§8.6).
-- A system can appear in many incidents and an incident can cover several
-- systems, so the systems are joined through incident_systems.
CREATE TABLE systems.incidents (
  id                  uuid PRIMARY KEY,
  ref                 text NOT NULL UNIQUE,
  title               text NOT NULL,
  severity            text NOT NULL CHECK (severity IN ('sev1','sev2','sev3','sev4')),
  status              text NOT NULL CHECK (status IN ('open','acknowledged','mitigated','resolved','closed')) DEFAULT 'open',
  detected_at         timestamptz NOT NULL DEFAULT now(),
  detected_source     text NOT NULL CHECK (detected_source IN ('atlas','client_reported','manual')) DEFAULT 'atlas',
  acknowledged_at     timestamptz,
  mitigated_at        timestamptz,
  resolved_at         timestamptz,
  closed_at           timestamptz,
  commander_person_id uuid REFERENCES people.persons(id),
  client_impact       text,
  root_cause          text,
  summary             text,
  -- Set when a SEV1/SEV2 resolves: it forces the post-incident review (§8.6).
  review_required     boolean NOT NULL DEFAULT false,
  review_at           timestamptz,
  version             integer NOT NULL DEFAULT 1,
  created_at          timestamptz NOT NULL DEFAULT now(),
  updated_at          timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_systems_incidents_severity   ON systems.incidents(severity);
CREATE INDEX idx_systems_incidents_status     ON systems.incidents(status);
CREATE INDEX idx_systems_incidents_detected   ON systems.incidents(detected_at DESC);
CREATE INDEX idx_systems_incidents_commander  ON systems.incidents(commander_person_id);

CREATE TABLE systems.incident_systems (
  incident_id uuid NOT NULL REFERENCES systems.incidents(id) ON DELETE CASCADE,
  system_id   uuid NOT NULL REFERENCES systems.systems(id) ON DELETE CASCADE,
  PRIMARY KEY (incident_id, system_id)
);

CREATE INDEX idx_systems_incident_systems_system ON systems.incident_systems(system_id);

-- Timeline: what happened, in order, and who said so — the incident's memory.
CREATE TABLE systems.incident_timeline (
  id              uuid PRIMARY KEY,
  incident_id     uuid NOT NULL REFERENCES systems.incidents(id) ON DELETE CASCADE,
  at              timestamptz NOT NULL DEFAULT now(),
  actor_person_id uuid REFERENCES people.persons(id),
  entry_type      text NOT NULL CHECK (entry_type IN
                    ('detected','acknowledged','note','mitigated','resolved','closed','action')),
  note            text NOT NULL,
  created_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_systems_incident_timeline_incident ON systems.incident_timeline(incident_id, at);

-- Follow-up actions: every resolved SEV1/SEV2 review produces these, and each
-- one becomes a work task (§8.6). task_id records the task it became.
CREATE TABLE systems.incident_actions (
  id              uuid PRIMARY KEY,
  incident_id     uuid NOT NULL REFERENCES systems.incidents(id) ON DELETE CASCADE,
  title           text NOT NULL,
  owner_person_id uuid REFERENCES people.persons(id),
  due_on          date,
  status          text NOT NULL CHECK (status IN ('todo','doing','done','dropped')) DEFAULT 'todo',
  task_id         uuid REFERENCES work.tasks(id) ON DELETE SET NULL,
  version         integer NOT NULL DEFAULT 1,
  created_at      timestamptz NOT NULL DEFAULT now(),
  updated_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_systems_incident_actions_incident ON systems.incident_actions(incident_id);
CREATE INDEX idx_systems_incident_actions_status   ON systems.incident_actions(status);
CREATE INDEX idx_systems_incident_actions_owner    ON systems.incident_actions(owner_person_id);

-- Credential records: that a credential exists, where it is stored and when it
-- was last rotated — never the secret itself (§8.6).
CREATE TABLE systems.credentials (
  id               uuid PRIMARY KEY,
  system_id        uuid NOT NULL REFERENCES systems.systems(id) ON DELETE CASCADE,
  label            text NOT NULL,
  credential_kind  text NOT NULL,
  store_path       text NOT NULL,
  owner_person_id  uuid REFERENCES people.persons(id),
  rotated_on       date,
  next_rotation_on date,
  notes            text,
  active           boolean NOT NULL DEFAULT true,
  version          integer NOT NULL DEFAULT 1,
  created_at       timestamptz NOT NULL DEFAULT now(),
  updated_at       timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_systems_credentials_system ON systems.credentials(system_id);
CREATE INDEX idx_systems_credentials_rotation ON systems.credentials(next_rotation_on);

-- Domains and certificates: what expires, and when we are warned (§8.6 warns
-- at 30/14/7 days).
CREATE TABLE systems.domains (
  id              uuid PRIMARY KEY,
  name            text NOT NULL UNIQUE,
  system_id       uuid REFERENCES systems.systems(id) ON DELETE SET NULL,
  registrar       text,
  registered_on   date,
  expires_on      date,
  auto_renew      boolean NOT NULL DEFAULT true,
  owner_person_id uuid REFERENCES people.persons(id),
  notes           text,
  version         integer NOT NULL DEFAULT 1,
  created_at      timestamptz NOT NULL DEFAULT now(),
  updated_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_systems_domains_expires ON systems.domains(expires_on);
CREATE INDEX idx_systems_domains_system  ON systems.domains(system_id);

CREATE TABLE systems.certificates (
  id              uuid PRIMARY KEY,
  hostname        text NOT NULL,
  system_id       uuid REFERENCES systems.systems(id) ON DELETE SET NULL,
  issuer          text,
  issued_on       date,
  expires_on      date NOT NULL,
  auto_renew      boolean NOT NULL DEFAULT true,
  owner_person_id uuid REFERENCES people.persons(id),
  notes           text,
  version         integer NOT NULL DEFAULT 1,
  created_at      timestamptz NOT NULL DEFAULT now(),
  updated_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_systems_certificates_expires ON systems.certificates(expires_on);
CREATE INDEX idx_systems_certificates_system  ON systems.certificates(system_id);

-- updated_at triggers, the same convention every other schema uses.
CREATE TRIGGER systems_systems_touch BEFORE UPDATE ON systems.systems
  FOR EACH ROW EXECUTE FUNCTION core.touch_updated_at();
CREATE TRIGGER systems_checks_touch BEFORE UPDATE ON systems.checks
  FOR EACH ROW EXECUTE FUNCTION core.touch_updated_at();
CREATE TRIGGER systems_incidents_touch BEFORE UPDATE ON systems.incidents
  FOR EACH ROW EXECUTE FUNCTION core.touch_updated_at();
CREATE TRIGGER systems_incident_actions_touch BEFORE UPDATE ON systems.incident_actions
  FOR EACH ROW EXECUTE FUNCTION core.touch_updated_at();
CREATE TRIGGER systems_credentials_touch BEFORE UPDATE ON systems.credentials
  FOR EACH ROW EXECUTE FUNCTION core.touch_updated_at();
CREATE TRIGGER systems_domains_touch BEFORE UPDATE ON systems.domains
  FOR EACH ROW EXECUTE FUNCTION core.touch_updated_at();
CREATE TRIGGER systems_certificates_touch BEFORE UPDATE ON systems.certificates
  FOR EACH ROW EXECUTE FUNCTION core.touch_updated_at();
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TRIGGER IF EXISTS systems_certificates_touch ON systems.certificates;
DROP TRIGGER IF EXISTS systems_domains_touch ON systems.domains;
DROP TRIGGER IF EXISTS systems_credentials_touch ON systems.credentials;
DROP TRIGGER IF EXISTS systems_incident_actions_touch ON systems.incident_actions;
DROP TRIGGER IF EXISTS systems_incidents_touch ON systems.incidents;
DROP TRIGGER IF EXISTS systems_checks_touch ON systems.checks;
DROP TRIGGER IF EXISTS systems_systems_touch ON systems.systems;
DROP TABLE IF EXISTS systems.certificates;
DROP TABLE IF EXISTS systems.domains;
DROP TABLE IF EXISTS systems.credentials;
DROP TABLE IF EXISTS systems.incident_actions;
DROP TABLE IF EXISTS systems.incident_timeline;
DROP TABLE IF EXISTS systems.incident_systems;
DROP TABLE IF EXISTS systems.incidents;
DROP TABLE IF EXISTS systems.check_results;
DROP FUNCTION IF EXISTS systems.ensure_check_result_partition(date);
DROP TABLE IF EXISTS systems.checks;
DROP TABLE IF EXISTS systems.systems;
DROP SCHEMA IF EXISTS systems;
-- +goose StatementEnd
