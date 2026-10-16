import Config

# Runtime configuration (docs/ATLAS.md §2.10). Everything that differs between
# environments and anything secret is read here, at boot, from the environment.

database_url =
  System.get_env("ORCHESTRATOR_DATABASE_URL") ||
    System.get_env("DATABASE_URL") ||
    raise """
    ORCHESTRATOR_DATABASE_URL is not set.

    Run the orchestrator through `task orch:serve`, which loads the repo-root
    .env, or set the variable explicitly.
    """

config :atlas_orchestrator, AtlasOrchestrator.Repo,
  url: database_url,
  pool_size: String.to_integer(System.get_env("POOL_SIZE") || "10")

if config_env() == :prod do
  secret_key_base =
    System.get_env("SECRET_KEY_BASE") ||
      raise "SECRET_KEY_BASE is not set (required to sign session/websocket state)."

  config :atlas_orchestrator, AtlasOrchestrator.Endpoint,
    secret_key_base: secret_key_base,
    http: [
      ip: {0, 0, 0, 0},
      port: String.to_integer(System.get_env("PORT") || "4000")
    ]

  # In production realtime tokens are verified against Go's JWKS (ADR-008). A
  # static document may be injected instead of a URL, for example when keys are
  # shipped by the secret manager rather than fetched.
  jwks_url =
    System.get_env("ATLAS_JWKS_URL") ||
      raise "ATLAS_JWKS_URL is not set (the Go JWKS endpoint for realtime tokens)."

  config :atlas_orchestrator, :realtime,
    jwks_url: jwks_url,
    issuer: System.get_env("ATLAS_REALTIME_ISSUER", "atlas"),
    audience: System.get_env("ATLAS_REALTIME_AUDIENCE", "atlas-realtime")

  # The orchestrator calls Go's internal API as a service (§7.5), for example
  # the realtime join check. The static token is minted by
  # `atlas-cli service token` and signed with the API's key.
  internal_url =
    System.get_env("ATLAS_API_INTERNAL_URL") ||
      raise "ATLAS_API_INTERNAL_URL is not set (the Go internal API for join checks)."

  service_token =
    System.get_env("ATLAS_SERVICE_TOKEN") ||
      raise "ATLAS_SERVICE_TOKEN is not set (mint one with `atlas-cli service token`)."

  config :atlas_orchestrator, :internal,
    base_url: internal_url,
    service_token: service_token

  config :logger, level: :info
end
