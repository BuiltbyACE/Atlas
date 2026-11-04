-- +goose Up
-- +goose StatementBegin
-- Phase 6: GitHub Engineering Intelligence (§8.9).
--
-- Purpose: durable storage for GitHub App installations, employee identity
-- mappings, authorized repositories, and full webhook event payloads. Go is
-- the sole writer (ADR-002); ids are UUIDv7; times are timestamptz in UTC.
--
-- With employees on personal GitHub accounts, each employee installs the
-- GitHub App on their own account and authorizes specific repositories. SCOS
-- stores the installation mapping and uses the App's private key to mint
-- short-lived installation access tokens for API calls.

-- GitHub App installations: one per employee's personal account (or one per
-- company org if Scenario A is adopted later).
CREATE TABLE integrations.github_installations (
  id              uuid PRIMARY KEY,
  installation_id bigint NOT NULL UNIQUE,                 -- GitHub's numeric installation id
  account_type    text NOT NULL CHECK (account_type IN ('personal','organization')),
  account_login   text NOT NULL,                          -- GitHub login (org name or user login)
  account_id      bigint NOT NULL,                        -- GitHub's numeric account id
  person_id       uuid REFERENCES people.persons(id),     -- SCOS person who installed / owns it
  status          text NOT NULL CHECK (status IN ('active','suspended','revoked')) DEFAULT 'active',
  authorized_repos text[] NOT NULL DEFAULT '{}',          -- repo full_names the installation may access
  created_at      timestamptz NOT NULL DEFAULT now(),
  updated_at      timestamptz NOT NULL DEFAULT now(),
  revoked_at      timestamptz
);

CREATE INDEX idx_integrations_github_installations_installation_id ON integrations.github_installations(installation_id);
CREATE INDEX idx_integrations_github_installations_person_id     ON integrations.github_installations(person_id);
CREATE INDEX idx_integrations_github_installations_status        ON integrations.github_installations(status);

-- Employee identity mapping: SCOS person <-> GitHub numeric user id.
-- A person may have multiple GitHub identities (e.g. work + personal), so this
-- is a many-to-many mapping keyed by person + github_user_id.
CREATE TABLE integrations.github_user_mappings (
  id             uuid PRIMARY KEY,
  person_id      uuid NOT NULL REFERENCES people.persons(id),
  github_user_id bigint NOT NULL,
  github_login   text NOT NULL,
  installation_id bigint REFERENCES integrations.github_installations(installation_id),
  is_primary     boolean NOT NULL DEFAULT false,          -- the identity used for attribution
  created_at     timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX idx_integrations_github_user_mappings_unique
  ON integrations.github_user_mappings(person_id, github_user_id);
CREATE INDEX idx_integrations_github_user_mappings_github_id
  ON integrations.github_user_mappings(github_user_id);
CREATE INDEX idx_integrations_github_user_mappings_person_id
  ON integrations.github_user_mappings(person_id);

-- Authorized repositories: which repos each installation may access, and which
-- SCOS project they map to (for task-to-code traceability via PRJ-YYYY-NNN refs).
CREATE TABLE integrations.github_repositories (
  id               uuid PRIMARY KEY,
  installation_id  bigint NOT NULL REFERENCES integrations.github_installations(installation_id),
  github_repo_id   bigint NOT NULL,
  full_name        text NOT NULL,                          -- owner/repo
  name             text NOT NULL,                          -- repo short name
  is_private       boolean NOT NULL DEFAULT true,
  project_id       uuid REFERENCES work.projects(id),      -- optional SCOS project link
  last_pushed_at   timestamptz,
  created_at       timestamptz NOT NULL DEFAULT now(),
  updated_at       timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX idx_integrations_github_repositories_unique
  ON integrations.github_repositories(installation_id, github_repo_id);
CREATE INDEX idx_integrations_github_repositories_project_id
  ON integrations.github_repositories(project_id);
CREATE INDEX idx_integrations_github_repositories_full_name
  ON integrations.github_repositories(full_name);

-- Durable full event store: every GitHub webhook payload, not just merged PRs.
-- Push, pull_request (all actions), check_run, workflow_run, deployment_status,
-- installation events are all stored here for replay and reconciliation.
CREATE TABLE integrations.github_events (
  id          uuid PRIMARY KEY,
  installation_id bigint NOT NULL REFERENCES integrations.github_installations(installation_id),
  delivery_id  text NOT NULL,
  event_type   text NOT NULL,                              -- push, pull_request, check_run, ...
  action       text,                                       -- opened, closed, merged, ...
  repository   text NOT NULL,                              -- owner/repo
  sender_login text NOT NULL,                              -- GitHub actor who triggered the event
  sender_id    bigint,                                     -- GitHub numeric actor id
  payload      jsonb NOT NULL,                             -- full webhook body
  processed_at timestamptz,
  created_at   timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_integrations_github_events_delivery_id   ON integrations.github_events(delivery_id);
CREATE INDEX idx_integrations_github_events_event_type    ON integrations.github_events(event_type);
CREATE INDEX idx_integrations_github_events_created_at    ON integrations.github_events(created_at);
CREATE INDEX idx_integrations_github_events_repository    ON integrations.github_events(repository);
CREATE INDEX idx_integrations_github_events_sender_login  ON integrations.github_events(sender_login);

-- updated_at triggers.
CREATE TRIGGER integrations_github_installations_touch BEFORE UPDATE ON integrations.github_installations
  FOR EACH ROW EXECUTE FUNCTION core.touch_updated_at();
CREATE TRIGGER integrations_github_repositories_touch BEFORE UPDATE ON integrations.github_repositories
  FOR EACH ROW EXECUTE FUNCTION core.touch_updated_at();

-- Grants: mirror 00019 and 00027. atlas_api writes and reads; atlas_readonly reads.
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'atlas_api') THEN
    GRANT USAGE ON SCHEMA integrations TO atlas_api;
    GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA integrations TO atlas_api;
    ALTER DEFAULT PRIVILEGES FOR ROLE atlas_migrator IN SCHEMA integrations
      GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO atlas_api;
  END IF;
  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'atlas_readonly') THEN
    GRANT USAGE ON SCHEMA integrations TO atlas_readonly;
    GRANT SELECT ON ALL TABLES IN SCHEMA integrations TO atlas_readonly;
    ALTER DEFAULT PRIVILEGES FOR ROLE atlas_migrator IN SCHEMA integrations
      GRANT SELECT ON TABLES TO atlas_readonly;
  END IF;
END
$$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TRIGGER IF EXISTS integrations_github_repositories_touch ON integrations.github_repositories;
DROP TRIGGER IF EXISTS integrations_github_installations_touch ON integrations.github_installations;
DROP INDEX IF EXISTS idx_integrations_github_events_sender_login;
DROP INDEX IF EXISTS idx_integrations_github_events_repository;
DROP INDEX IF EXISTS idx_integrations_github_events_created_at;
DROP INDEX IF EXISTS idx_integrations_github_events_event_type;
DROP INDEX IF EXISTS idx_integrations_github_events_delivery_id;
DROP TABLE IF EXISTS integrations.github_events;
DROP INDEX IF EXISTS idx_integrations_github_repositories_full_name;
DROP INDEX IF EXISTS idx_integrations_github_repositories_project_id;
DROP INDEX IF EXISTS idx_integrations_github_repositories_unique;
DROP TABLE IF EXISTS integrations.github_repositories;
DROP INDEX IF EXISTS idx_integrations_github_user_mappings_person_id;
DROP INDEX IF EXISTS idx_integrations_github_user_mappings_github_id;
DROP INDEX IF EXISTS idx_integrations_github_user_mappings_unique;
DROP TABLE IF EXISTS integrations.github_user_mappings;
DROP INDEX IF EXISTS idx_integrations_github_installations_status;
DROP INDEX IF EXISTS idx_integrations_github_installations_person_id;
DROP INDEX IF EXISTS idx_integrations_github_installations_installation_id;
DROP TABLE IF EXISTS integrations.github_installations;
-- +goose StatementEnd
