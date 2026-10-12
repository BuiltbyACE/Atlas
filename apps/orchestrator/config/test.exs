import Config

config :atlas_orchestrator, AtlasOrchestrator.Endpoint, server: false

# Realtime tokens are verified against a static key set injected by each test
# (see AtlasOrchestrator.Auth.TokenTest); no fetch happens in the suite.
config :atlas_orchestrator, :realtime, jwks: nil

config :logger, level: :warning

# Tests exercise the real outbox against the locally running PostgreSQL
# (started by `task up`). They are integration tests by design: the outbox
# contract is a database contract.
config :atlas_orchestrator, AtlasOrchestrator.Repo, pool: Ecto.Adapters.SQL.Sandbox
