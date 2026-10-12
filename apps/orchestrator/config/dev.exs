import Config

# Local development. The database URL comes from ORCHESTRATOR_DATABASE_URL
# (loaded from the repo-root .env by `task`), so it is not repeated here.
config :atlas_orchestrator, AtlasOrchestrator.Endpoint,
  http: [ip: {127, 0, 0, 1}, port: 4000],
  check_origin: false,
  server: true,
  debug_errors: true

config :atlas_orchestrator, AtlasOrchestrator.Repo, show_sensitive_data_on_connection_error: true

# Local realtime tokens are signed with a shared secret rather than verified
# against Go's JWKS, which arrives with authentication in Phase 1 (ADR-008).
config :atlas_orchestrator, :realtime, secret: "local-dev-realtime-secret-not-for-anywhere-else"

config :logger, level: :debug
