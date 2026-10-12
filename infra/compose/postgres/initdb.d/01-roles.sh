#!/bin/sh
# Atlas local development — database roles (see docs/ATLAS.md §5.6).
#
# One role per concern so local development exercises the same privilege
# boundaries as staging/production:
#
#   atlas_migrator      owns migrations; may create schemas
#   atlas_api           read/write on business schemas
#   atlas_worker        read/write on business schemas (health probes, jobs)
#   atlas_orchestrator  read/write on its own schema; READ-ONLY on outbox
#   atlas_readonly      reporting / BI
#
# Passwords come from the environment (see docker-compose.yml). These are
# LOCAL development credentials only and must never be reused anywhere else.

set -e

psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" <<-EOSQL
    CREATE ROLE atlas_migrator      LOGIN PASSWORD '${ATLAS_MIGRATOR_DB_PASSWORD:-atlas_migrator}';
    CREATE ROLE atlas_api           LOGIN PASSWORD '${ATLAS_API_DB_PASSWORD:-atlas_api}';
    CREATE ROLE atlas_worker        LOGIN PASSWORD '${ATLAS_WORKER_DB_PASSWORD:-atlas_worker}';
    CREATE ROLE atlas_orchestrator  LOGIN PASSWORD '${ATLAS_ORCHESTRATOR_DB_PASSWORD:-atlas_orchestrator}';
    CREATE ROLE atlas_readonly      LOGIN PASSWORD '${ATLAS_READONLY_DB_PASSWORD:-atlas_readonly}';

    -- The migrator owns schema creation; every other role connects only.
    GRANT CREATE, CONNECT ON DATABASE ${POSTGRES_DB} TO atlas_migrator;
    -- The orchestrator owns the `orchestrator` schema and may create it if the
    -- Go foundation migration has not been run yet.
    GRANT CREATE, CONNECT ON DATABASE ${POSTGRES_DB} TO atlas_orchestrator;
    GRANT CONNECT ON DATABASE ${POSTGRES_DB}
        TO atlas_api, atlas_worker, atlas_readonly;

    -- PostgreSQL 15+ removed CREATE on the public schema from PUBLIC, so
    -- goose cannot create its goose_db_version table there unless the migrator
    -- is granted it explicitly. Being the migrator, it owns that bookkeeping.
    GRANT CREATE, USAGE ON SCHEMA public TO atlas_migrator;

    -- Convenience for local discovery.
    COMMENT ON ROLE atlas_api IS 'Atlas core API (business schema read/write)';
    COMMENT ON ROLE atlas_orchestrator IS 'Atlas orchestrator (owns orchestrator schema; outbox read-only)';
EOSQL

echo "Atlas database roles created: migrator, api, worker, orchestrator, readonly"
