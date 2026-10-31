-- +goose Up
-- +goose StatementBegin
-- Phase 6: Integrations hub (§8.9).
--
-- Purpose: one consistent way to connect anything to Atlas. The integration
-- registry records identity, owner, direction, credential reference, scopes,
-- status, last success and error rate for every integration. The webhook inbox
-- stores raw inbound payloads; the outbound log records outbound call attempts.
-- Go is the sole writer (ADR-002); ids are UUIDv7; times are timestamptz in
-- UTC.
CREATE SCHEMA IF NOT EXISTS integrations;

-- Integration registry: every connection to an external system (§8.9).
CREATE TABLE integrations.integrations (
  id              uuid PRIMARY KEY,
  ref             text NOT NULL UNIQUE,
  name            text NOT NULL,
  kind            text NOT NULL CHECK (kind IN ('api','webhook','file','sftp')),
  direction       text NOT NULL CHECK (direction IN ('in','out','both')),
  owner_person_id uuid REFERENCES people.persons(id),
  credential_ref  text NOT NULL,
  scopes          text[] NOT NULL DEFAULT '{}',
  status          text NOT NULL CHECK (status IN ('active','paused','error')) DEFAULT 'active',
  last_success    timestamptz,
  error_rate      real NOT NULL DEFAULT 0 CHECK (error_rate >= 0 AND error_rate <= 1),
  version         integer NOT NULL DEFAULT 1,
  created_at      timestamptz NOT NULL DEFAULT now(),
  updated_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_integrations_integrations_kind   ON integrations.integrations(kind);
CREATE INDEX idx_integrations_integrations_dir    ON integrations.integrations(direction);
CREATE INDEX idx_integrations_integrations_owner  ON integrations.integrations(owner_person_id);
CREATE INDEX idx_integrations_integrations_status ON integrations.integrations(status);

-- Webhook inbox: raw inbound provider payloads, acknowledged fast and processed
-- asynchronously (§8.9, §11).
CREATE TABLE integrations.webhook_inbox (
  id             uuid PRIMARY KEY,
  provider       text NOT NULL,
  event_type     text NOT NULL,
  delivery_id    text NOT NULL,
  payload        jsonb NOT NULL,
  headers        jsonb,
  received_at    timestamptz NOT NULL DEFAULT now(),
  processed_at   timestamptz,
  processing_status text NOT NULL CHECK (processing_status IN ('pending','processed','failed')) DEFAULT 'pending',
  error_message  text
);

CREATE INDEX idx_integrations_webhook_inbox_provider   ON integrations.webhook_inbox(provider);
CREATE INDEX idx_integrations_webhook_inbox_delivery   ON integrations.webhook_inbox(delivery_id);
CREATE INDEX idx_integrations_webhook_inbox_received   ON integrations.webhook_inbox(received_at);
CREATE INDEX idx_integrations_webhook_inbox_status     ON integrations.webhook_inbox(processing_status);

-- Outbound log: outbound call attempts with timeouts, retries and circuit state (§8.9).
CREATE TABLE integrations.outbound_log (
  id             uuid PRIMARY KEY,
  provider       text NOT NULL,
  endpoint       text NOT NULL,
  method         text NOT NULL,
  status_code    int,
  error_message  text,
  attempt        int NOT NULL DEFAULT 1,
  max_attempts   int NOT NULL DEFAULT 3,
  last_attempt_at timestamptz NOT NULL DEFAULT now(),
  circuit_state  text NOT NULL CHECK (circuit_state IN ('closed','open','half_open')) DEFAULT 'closed',
  created_at     timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_integrations_outbound_log_provider   ON integrations.outbound_log(provider);
CREATE INDEX idx_integrations_outbound_log_endpoint   ON integrations.outbound_log(endpoint);
CREATE INDEX idx_integrations_outbound_log_last_attempt ON integrations.outbound_log(last_attempt_at);
CREATE INDEX idx_integrations_outbound_log_circuit    ON integrations.outbound_log(circuit_state);

-- updated_at triggers, the same convention every other schema uses.
CREATE TRIGGER integrations_integrations_touch BEFORE UPDATE ON integrations.integrations
  FOR EACH ROW EXECUTE FUNCTION core.touch_updated_at();
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TRIGGER IF EXISTS integrations_integrations_touch ON integrations.integrations;
DROP INDEX IF EXISTS idx_integrations_outbound_log_circuit;
DROP INDEX IF EXISTS idx_integrations_outbound_log_last_attempt;
DROP INDEX IF EXISTS idx_integrations_outbound_log_endpoint;
DROP INDEX IF EXISTS idx_integrations_outbound_log_provider;
DROP INDEX IF EXISTS idx_integrations_webhook_inbox_status;
DROP INDEX IF EXISTS idx_integrations_webhook_inbox_received;
DROP INDEX IF EXISTS idx_integrations_webhook_inbox_delivery;
DROP INDEX IF EXISTS idx_integrations_webhook_inbox_provider;
DROP INDEX IF EXISTS idx_integrations_integrations_status;
DROP INDEX IF EXISTS idx_integrations_integrations_owner;
DROP INDEX IF EXISTS idx_integrations_integrations_dir;
DROP INDEX IF EXISTS idx_integrations_integrations_kind;
DROP TABLE IF EXISTS integrations.outbound_log;
DROP TABLE IF EXISTS integrations.webhook_inbox;
DROP TABLE IF EXISTS integrations.integrations;
DROP TABLE IF EXISTS integrations.github_deliveries;
DROP SCHEMA IF EXISTS integrations;
-- +goose StatementEnd
