import Config

config :atlas_orchestrator, AtlasOrchestrator.Endpoint, server: false

config :atlas_orchestrator, :realtime, secret: "test-realtime-secret"

config :logger, level: :warning

# Tests exercise the real outbox against the locally running PostgreSQL
# (started by `task up`). They are integration tests by design: the outbox
# contract is a database contract.
config :atlas_orchestrator, AtlasOrchestrator.Repo, pool: Ecto.Adapters.SQL.Sandbox
