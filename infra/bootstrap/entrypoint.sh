#!/bin/sh
# Atlas one-shot bootstrap (docs/ATLAS.md §5.6, §10.1, §16.1).
#
# This service exists because two jobs need capabilities the runtime images
# deliberately do not have:
#
#   migrate  creating login roles and extensions (psql), then the Go schema
#            migrations and the role/permission seed (atlas-cli)
#   account  creating the first human account (Atlas has no self-service signup)
#   token    creating the orchestrator's service identity and minting its token
#
# The service has restart policy "Never" in Railway; each mode is one deliberate
# run, not a supervisor. Every mode is idempotent and safe to re-run.
#
# Usage: atlas-bootstrap [migrate|account|token|shell]

set -eu

log() { printf '[atlas-bootstrap] %s\n' "$*"; }
die() { printf '[atlas-bootstrap] ERROR: %s\n' "$*" >&2; exit 1; }

# require VAR_NAME — fail with a readable message rather than an empty value.
require() {
    eval "value=\${$1:-}"
    [ -n "$value" ] || die "$1 is required in mode '$mode'"
}

wait_for_db() {
    url=$1
    attempt=0
    until pg_isready -q -d "$url"; do
        attempt=$((attempt + 1))
        [ "$attempt" -lt 60 ] || die "database not ready after 60s"
        sleep 1
    done
    log "database is accepting connections"
}

mode="${1:-migrate}"

case "$mode" in
migrate)
    require ATLAS_ADMIN_DATABASE_URL
    require ATLAS_MIGRATOR_DB_PASSWORD
    require ATLAS_API_DB_PASSWORD
    require ATLAS_WORKER_DB_PASSWORD
    require ATLAS_ORCHESTRATOR_DB_PASSWORD
    require ATLAS_READONLY_DB_PASSWORD

    wait_for_db "$ATLAS_ADMIN_DATABASE_URL"

    log "applying login roles, grants and extensions"
    # `-o /dev/null` keeps the generated CREATE/ALTER ROLE statements (which
    # contain the passwords) out of the service logs; \gexec still executes
    # them and errors still reach stderr.
    psql "$ATLAS_ADMIN_DATABASE_URL" \
        -v ON_ERROR_STOP=1 \
        -o /dev/null \
        -v migrator_pw="$ATLAS_MIGRATOR_DB_PASSWORD" \
        -v api_pw="$ATLAS_API_DB_PASSWORD" \
        -v worker_pw="$ATLAS_WORKER_DB_PASSWORD" \
        -v orchestrator_pw="$ATLAS_ORCHESTRATOR_DB_PASSWORD" \
        -v readonly_pw="$ATLAS_READONLY_DB_PASSWORD" \
        -f /usr/local/share/atlas/roles.sql

    log "applying Go schema migrations"
    atlas-cli migrate up

    log "seeding the role and permission catalogue"
    atlas-cli seed

    log "schema status"
    atlas-cli migrate status
    log "migrate: done"
    ;;

account)
    require ATLAS_BOOTSTRAP_EMAIL
    require ATLAS_BOOTSTRAP_NAME
    require ATLAS_BOOTSTRAP_PASSWORD

    set -- bootstrap \
        --email "$ATLAS_BOOTSTRAP_EMAIL" \
        --name "$ATLAS_BOOTSTRAP_NAME" \
        --password "$ATLAS_BOOTSTRAP_PASSWORD"

    if [ -n "${ATLAS_BOOTSTRAP_ROLE:-}" ]; then
        set -- "$@" --role "$ATLAS_BOOTSTRAP_ROLE"
    fi
    if [ -n "${ATLAS_BOOTSTRAP_PHONE:-}" ]; then
        set -- "$@" --phone "$ATLAS_BOOTSTRAP_PHONE"
    fi

    log "creating the first account ($ATLAS_BOOTSTRAP_EMAIL)"
    atlas-cli "$@"
    log "account: done — remove ATLAS_BOOTSTRAP_PASSWORD from this service now"
    ;;

token)
    service_id="${ATLAS_SERVICE_ID:-svc:orchestrator}"

    log "ensuring the service identity $service_id"
    atlas-cli service create \
        --id "$service_id" \
        --permission core.authz.check \
        --permission core.notification.manage \
        --description "Elixir orchestrator: realtime authorisation checks and notifications"

    log "minting a token for $service_id"
    log "---- copy the line below into the orchestrator service's ATLAS_SERVICE_TOKEN ----"
    atlas-cli service token --id "$service_id"
    log "---- end of token ----"
    ;;

shell)
    log "interactive-ish shell; the container will exit when this returns"
    exec sh
    ;;

*)
    die "unknown mode '$mode' (expected migrate|account|token|shell)"
    ;;
esac
