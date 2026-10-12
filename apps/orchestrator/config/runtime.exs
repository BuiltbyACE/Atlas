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

  # In production realtime tokens are verified against Go's JWKS (ADR-008).
  if jwks = System.get_env("ATLAS_REALTIME_JWKS") do
    config :atlas_orchestrator, :realtime, jwks: jwks, issuer: System.get_env("ATLAS_REALTIME_ISSUER")
  end

  config :logger, level: :info
end
