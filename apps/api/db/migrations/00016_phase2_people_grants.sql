-- +goose Up
-- +goose StatementBegin
-- Phase 2 correction: the tables 00013 moved into `people` never carried grants.
--
-- 00002 granted "ALL TABLES IN SCHEMA people" to atlas_api/atlas_worker, but
-- daily_reports and leave_requests were created in `work` by 00011 and only
-- later relocated with ALTER TABLE ... SET SCHEMA. A table keeps its ACLs when
-- it changes schema, so the relocation carried the empty `work` ACLs with it and
-- the people module (daily reports, leave, availability) failed at runtime with
-- "permission denied for table daily_reports". The Go tests connect as the
-- schema owner, so nothing caught it — the same blind spot as migration 00015.
--
-- Grants mirror 00002/00015: full DML for atlas_api and atlas_worker, SELECT for
-- atlas_readonly, and default privileges so future people tables and sequences
-- are covered without another migration.
--
-- Roles are created by infra/compose/postgres/initdb.d/01-roles.sh locally and
-- by the platform elsewhere; every grant is guarded so a bare database still
-- migrates.
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'atlas_api') THEN
    GRANT USAGE ON SCHEMA people TO atlas_api;
    GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA people TO atlas_api;
    GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA people TO atlas_api;
    ALTER DEFAULT PRIVILEGES FOR ROLE atlas_migrator IN SCHEMA people GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO atlas_api;
    ALTER DEFAULT PRIVILEGES FOR ROLE atlas_migrator IN SCHEMA people GRANT USAGE, SELECT ON SEQUENCES TO atlas_api;
  END IF;

  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'atlas_worker') THEN
    GRANT USAGE ON SCHEMA people TO atlas_worker;
    GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA people TO atlas_worker;
    GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA people TO atlas_worker;
    ALTER DEFAULT PRIVILEGES FOR ROLE atlas_migrator IN SCHEMA people GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO atlas_worker;
    ALTER DEFAULT PRIVILEGES FOR ROLE atlas_migrator IN SCHEMA people GRANT USAGE, SELECT ON SEQUENCES TO atlas_worker;
  END IF;

  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'atlas_readonly') THEN
    GRANT USAGE ON SCHEMA people TO atlas_readonly;
    GRANT SELECT ON ALL TABLES IN SCHEMA people TO atlas_readonly;
    ALTER DEFAULT PRIVILEGES FOR ROLE atlas_migrator IN SCHEMA people GRANT SELECT ON TABLES TO atlas_readonly;
  END IF;

  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'atlas_orchestrator') THEN
    GRANT USAGE ON SCHEMA people TO atlas_orchestrator;
    GRANT SELECT ON ALL TABLES IN SCHEMA people TO atlas_orchestrator;
    ALTER DEFAULT PRIVILEGES FOR ROLE atlas_migrator IN SCHEMA people GRANT SELECT ON TABLES TO atlas_orchestrator;
  END IF;
END
$$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- Revoke only the grants this migration adds for the relocated tables. The
-- schema-level USAGE and default privileges that 00002 already granted are left
-- in place, because 00002 owns them.
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'atlas_api') THEN
    REVOKE SELECT, INSERT, UPDATE, DELETE ON people.daily_reports, people.leave_requests FROM atlas_api;
  END IF;

  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'atlas_worker') THEN
    REVOKE SELECT, INSERT, UPDATE, DELETE ON people.daily_reports, people.leave_requests FROM atlas_worker;
  END IF;

  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'atlas_readonly') THEN
    REVOKE SELECT ON people.daily_reports, people.leave_requests FROM atlas_readonly;
  END IF;

  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'atlas_orchestrator') THEN
    REVOKE SELECT ON people.daily_reports, people.leave_requests FROM atlas_orchestrator;
  END IF;
END
$$;
-- +goose StatementEnd
