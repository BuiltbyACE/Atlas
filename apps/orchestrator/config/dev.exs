import Config

# Local development. The database URL comes from ORCHESTRATOR_DATABASE_URL
# (loaded from the repo-root .env by `task`), so it is not repeated here.
config :atlas_orchestrator, AtlasOrchestrator.Endpoint,
  http: [ip: {127, 0, 0, 1}, port: 4000],
  check_origin: false,
  server: true,
  debug_errors: true

config :atlas_orchestrator, AtlasOrchestrator.Repo, show_sensitive_data_on_connection_error: true

# Realtime tokens are signed by Go and verified against its JWKS (ADR-008).
# The URL is where atlas-api publishes the public key set; it is reachable
# inside the local network but 404s on the public edge.
config :atlas_orchestrator, :realtime,
  jwks_url: System.get_env("ATLAS_JWKS_URL", "http://localhost:8080/internal/v1/.well-known/jwks.json"),
  issuer: "atlas",
  audience: "atlas-realtime"

config :logger, level: :debug
