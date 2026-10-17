-- +goose Up
-- +goose StatementBegin
-- Phase 2 correction: migration 00011 created the `work` schema but never
-- granted it to the runtime roles. Every work route and the atlas-worker
-- escalation sweep therefore failed with "permission denied for schema work"
-- (the Go tests connect as the schema owner, so nothing caught it).
--
-- Grants mirror 00002's pattern and the 10.1 / 13.3 role split: full DML for
-- atlas_api and atlas_worker, SELECT for atlas_readonly, and default privileges
-- so future work tables and sequences are covered without another migration.
-- audit.events keeps INSERT+SELECT only; work is ordinary business data.
--
-- Roles are created by infra/compose/postgres/initdb.d/01-roles.sh locally and
-- by the platform elsewhere; every grant is guarded so a bare database still
-- migrates.
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'atlas_api') THEN
    GRANT USAGE ON SCHEMA work TO atlas_api;
    GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA work TO atlas_api;
    GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA work TO atlas_api;
    ALTER DEFAULT PRIVILEGES FOR ROLE atlas_migrator IN SCHEMA work GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO atlas_api;
    ALTER DEFAULT PRIVILEGES FOR ROLE atlas_migrator IN SCHEMA work GRANT USAGE, SELECT ON SEQUENCES TO atlas_api;
  END IF;

  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'atlas_worker') THEN
    GRANT USAGE ON SCHEMA work TO atlas_worker;
    GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA work TO atlas_worker;
    GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA work TO atlas_worker;
    ALTER DEFAULT PRIVILEGES FOR ROLE atlas_migrator IN SCHEMA work GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO atlas_worker;
    ALTER DEFAULT PRIVILEGES FOR ROLE atlas_migrator IN SCHEMA work GRANT USAGE, SELECT ON SEQUENCES TO atlas_worker;
  END IF;

  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'atlas_readonly') THEN
    GRANT USAGE ON SCHEMA work TO atlas_readonly;
    GRANT SELECT ON ALL TABLES IN SCHEMA work TO atlas_readonly;
    ALTER DEFAULT PRIVILEGES FOR ROLE atlas_migrator IN SCHEMA work GRANT SELECT ON TABLES TO atlas_readonly;
  END IF;
END
$$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'atlas_api') THEN
    REVOKE ALL ON ALL TABLES IN SCHEMA work FROM atlas_api;
    REVOKE ALL ON ALL SEQUENCES IN SCHEMA work FROM atlas_api;
    REVOKE ALL ON SCHEMA work FROM atlas_api;
    ALTER DEFAULT PRIVILEGES FOR ROLE atlas_migrator IN SCHEMA work REVOKE ALL ON TABLES FROM atlas_api;
    ALTER DEFAULT PRIVILEGES FOR ROLE atlas_migrator IN SCHEMA work REVOKE ALL ON SEQUENCES FROM atlas_api;
  END IF;

  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'atlas_worker') THEN
    REVOKE ALL ON ALL TABLES IN SCHEMA work FROM atlas_worker;
    REVOKE ALL ON ALL SEQUENCES IN SCHEMA work FROM atlas_worker;
    REVOKE ALL ON SCHEMA work FROM atlas_worker;
    ALTER DEFAULT PRIVILEGES FOR ROLE atlas_migrator IN SCHEMA work REVOKE ALL ON TABLES FROM atlas_worker;
    ALTER DEFAULT PRIVILEGES FOR ROLE atlas_migrator IN SCHEMA work REVOKE ALL ON SEQUENCES FROM atlas_worker;
  END IF;

  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'atlas_readonly') THEN
    REVOKE ALL ON ALL TABLES IN SCHEMA work FROM atlas_readonly;
    REVOKE ALL ON SCHEMA work FROM atlas_readonly;
    ALTER DEFAULT PRIVILEGES FOR ROLE atlas_migrator IN SCHEMA work REVOKE ALL ON TABLES FROM atlas_readonly;
  END IF;
END
$$;
-- +goose StatementEnd
