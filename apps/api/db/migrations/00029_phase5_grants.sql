-- +goose Up
-- +goose StatementBegin
-- Phase 5 grants (docs/ATLAS.md §10.1, §13.3).
--
-- Migrations 00024–00026 create the systems, units and intel schemas for the
-- Systems register (§8.6), business units (§8.9) and executive intelligence
-- (§8.8), but they never granted the runtime roles access to them. The API runs
-- as atlas_api, so every read of those schemas failed with
-- "permission denied for schema systems (SQLSTATE 42501)" and surfaced as a 500.
--
-- This migration grants exactly the same shape as the earlier phases: atlas_api
-- and atlas_worker read and write, atlas_readonly reads. Default privileges are
-- extended too, so tables added to these schemas by later migrations are usable
-- without another grant migration.
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'atlas_api') THEN
    GRANT USAGE ON SCHEMA systems, units, intel TO atlas_api;
    GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA systems, units, intel TO atlas_api;
    GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA systems, units, intel TO atlas_api;
    ALTER DEFAULT PRIVILEGES FOR ROLE atlas_migrator IN SCHEMA systems
      GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO atlas_api;
    ALTER DEFAULT PRIVILEGES FOR ROLE atlas_migrator IN SCHEMA units
      GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO atlas_api;
    ALTER DEFAULT PRIVILEGES FOR ROLE atlas_migrator IN SCHEMA intel
      GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO atlas_api;
    ALTER DEFAULT PRIVILEGES FOR ROLE atlas_migrator IN SCHEMA systems, units, intel
      GRANT USAGE, SELECT ON SEQUENCES TO atlas_api;
  END IF;

  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'atlas_worker') THEN
    GRANT USAGE ON SCHEMA systems, units, intel TO atlas_worker;
    GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA systems, units, intel TO atlas_worker;
    GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA systems, units, intel TO atlas_worker;
    ALTER DEFAULT PRIVILEGES FOR ROLE atlas_migrator IN SCHEMA systems
      GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO atlas_worker;
    ALTER DEFAULT PRIVILEGES FOR ROLE atlas_migrator IN SCHEMA units
      GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO atlas_worker;
    ALTER DEFAULT PRIVILEGES FOR ROLE atlas_migrator IN SCHEMA intel
      GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO atlas_worker;
    ALTER DEFAULT PRIVILEGES FOR ROLE atlas_migrator IN SCHEMA systems, units, intel
      GRANT USAGE, SELECT ON SEQUENCES TO atlas_worker;
  END IF;

  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'atlas_readonly') THEN
    GRANT USAGE ON SCHEMA systems, units, intel TO atlas_readonly;
    GRANT SELECT ON ALL TABLES IN SCHEMA systems, units, intel TO atlas_readonly;
    ALTER DEFAULT PRIVILEGES FOR ROLE atlas_migrator IN SCHEMA systems
      GRANT SELECT ON TABLES TO atlas_readonly;
    ALTER DEFAULT PRIVILEGES FOR ROLE atlas_migrator IN SCHEMA units
      GRANT SELECT ON TABLES TO atlas_readonly;
    ALTER DEFAULT PRIVILEGES FOR ROLE atlas_migrator IN SCHEMA intel
      GRANT SELECT ON TABLES TO atlas_readonly;
  END IF;
END
$$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'atlas_api') THEN
    ALTER DEFAULT PRIVILEGES FOR ROLE atlas_migrator IN SCHEMA systems, units, intel
      REVOKE ALL ON TABLES FROM atlas_api;
    ALTER DEFAULT PRIVILEGES FOR ROLE atlas_migrator IN SCHEMA systems, units, intel
      REVOKE ALL ON SEQUENCES FROM atlas_api;
    REVOKE ALL ON ALL TABLES IN SCHEMA systems, units, intel FROM atlas_api;
    REVOKE ALL ON ALL SEQUENCES IN SCHEMA systems, units, intel FROM atlas_api;
    REVOKE USAGE ON SCHEMA systems, units, intel FROM atlas_api;
  END IF;

  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'atlas_worker') THEN
    ALTER DEFAULT PRIVILEGES FOR ROLE atlas_migrator IN SCHEMA systems, units, intel
      REVOKE ALL ON TABLES FROM atlas_worker;
    ALTER DEFAULT PRIVILEGES FOR ROLE atlas_migrator IN SCHEMA systems, units, intel
      REVOKE ALL ON SEQUENCES FROM atlas_worker;
    REVOKE ALL ON ALL TABLES IN SCHEMA systems, units, intel FROM atlas_worker;
    REVOKE ALL ON ALL SEQUENCES IN SCHEMA systems, units, intel FROM atlas_worker;
    REVOKE USAGE ON SCHEMA systems, units, intel FROM atlas_worker;
  END IF;

  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'atlas_readonly') THEN
    ALTER DEFAULT PRIVILEGES FOR ROLE atlas_migrator IN SCHEMA systems, units, intel
      REVOKE ALL ON TABLES FROM atlas_readonly;
    REVOKE ALL ON ALL TABLES IN SCHEMA systems, units, intel FROM atlas_readonly;
    REVOKE USAGE ON SCHEMA systems, units, intel FROM atlas_readonly;
  END IF;
END
$$;
-- +goose StatementEnd
