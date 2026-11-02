-- Atlas production database roles (docs/ATLAS.md §5.6, §10.1).
--
-- The local equivalent is infra/compose/postgres/initdb.d/01-roles.sh. In
-- staging and production the platform does not run init scripts, so this file
-- is applied by the one-shot bootstrap service instead. It is idempotent: it
-- creates roles that are missing and re-asserts passwords and grants.
--
-- Passwords arrive as psql variables (-v name=value), never as literals.
--
--   atlas_migrator      owns migrations; may create schemas and the goose table
--   atlas_api           read/write on business schemas
--   atlas_worker        read/write on business schemas (dispatch, sweeps)
--   atlas_orchestrator  owns its own schema; READ-ONLY on outbox
--   atlas_readonly      reporting / BI

\set ON_ERROR_STOP on

-- ── Roles ────────────────────────────────────────────────────
-- CREATE ROLE has no IF NOT EXISTS, so build the statement and execute only
-- where the role is genuinely absent.
SELECT format('CREATE ROLE %I LOGIN PASSWORD %L', t.rolname, t.rolpass)
FROM (VALUES
    ('atlas_migrator',     :'migrator_pw'),
    ('atlas_api',          :'api_pw'),
    ('atlas_worker',       :'worker_pw'),
    ('atlas_orchestrator', :'orchestrator_pw'),
    ('atlas_readonly',     :'readonly_pw')
) AS t(rolname, rolpass)
WHERE NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = t.rolname);
\gexec

-- Re-assert the password on every run so a rotation is one bootstrap run.
SELECT format('ALTER ROLE %I WITH LOGIN PASSWORD %L', t.rolname, t.rolpass)
FROM (VALUES
    ('atlas_migrator',     :'migrator_pw'),
    ('atlas_api',          :'api_pw'),
    ('atlas_worker',       :'worker_pw'),
    ('atlas_orchestrator', :'orchestrator_pw'),
    ('atlas_readonly',     :'readonly_pw')
) AS t(rolname, rolpass);
\gexec

-- ── Database-level grants ────────────────────────────────────
DO $$
BEGIN
    -- The migrator owns schema creation.
    EXECUTE format('GRANT CREATE, CONNECT ON DATABASE %I TO atlas_migrator', current_database());
    -- The orchestrator owns the `orchestrator` schema and may create it when
    -- the Go foundation migration has not run yet.
    EXECUTE format('GRANT CREATE, CONNECT ON DATABASE %I TO atlas_orchestrator', current_database());
    EXECUTE format('GRANT CONNECT ON DATABASE %I TO atlas_api, atlas_worker, atlas_readonly', current_database());

    -- PostgreSQL 15+ removed CREATE on `public` from PUBLIC, so goose cannot
    -- create its goose_db_version table there without this.
    EXECUTE 'GRANT CREATE, USAGE ON SCHEMA public TO atlas_migrator';
END
$$;

-- ── Extensions ───────────────────────────────────────────────
-- Must exist before the Go foundation migration runs: 00001 uses citext in
-- column definitions.
CREATE EXTENSION IF NOT EXISTS citext;     -- case-insensitive text (emails, names)
CREATE EXTENSION IF NOT EXISTS pgcrypto;   -- digest / gen_random_uuid helpers
CREATE EXTENSION IF NOT EXISTS pg_trgm;    -- trigram search (duplicate detection, §8.1)

-- ── Comments (operational discovery) ─────────────────────────
COMMENT ON ROLE atlas_api IS 'Atlas core API (business schema read/write)';
COMMENT ON ROLE atlas_worker IS 'Atlas background worker (dispatch, sweeps)';
COMMENT ON ROLE atlas_orchestrator IS 'Atlas orchestrator (owns orchestrator schema; outbox read-only)';
COMMENT ON ROLE atlas_readonly IS 'Atlas reporting / BI (read-only)';
