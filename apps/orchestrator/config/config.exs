import Config

# The orchestrator owns the `orchestrator` schema and reads `outbox`
# (docs/ATLAS.md §10.1). Every Ecto table it creates lives under that prefix,
# so it can never accidentally write into a business schema.
config :atlas_orchestrator,
  ecto_repos: [AtlasOrchestrator.Repo],
  env: config_env(),
  generator: [binary_id: true]

config :atlas_orchestrator, AtlasOrchestrator.Repo,
  migration_primary_key: [name: :id, type: :binary_id],
  migration_timestamps: [type: :utc_datetime_usec],
  migration_default_prefix: "orchestrator",
  priv: "priv/repo",
  pool_size: 10

# Oban runs entirely inside the orchestrator schema.
config :atlas_orchestrator, Oban,
  repo: AtlasOrchestrator.Repo,
  prefix: "orchestrator",
  plugins: [Oban.Plugins.Pruner],
  queues: [default: 10, reminders: 5, escalations: 5]

# Outbox consumption (§7.1): wake on NOTIFY, poll as a safety net.
config :atlas_orchestrator, AtlasOrchestrator.Outbox,
  notify_channel: "atlas_outbox",
  poll_interval_ms: 2_000,
  batch_size: 100

config :atlas_orchestrator, AtlasOrchestrator.Endpoint,
  url: [host: "localhost"],
  pubsub_server: AtlasOrchestrator.PubSub,
  adapter: Bandit.PhoenixAdapter,
  # A non-secret development key; runtime.exs overrides it in production.
  secret_key_base: "atlas-orchestrator-local-dev-secret-key-base-0000000000000000000000000000000000"

config :phoenix, :json_library, Jason

config :logger, :console,
  format: "$time $metadata[$level] $message\n",
  metadata: [
    :request_id,
    :event_id,
    :seq,
    :topic,
    :type,
    :reason,
    :error,
    :url,
    :result,
    :event_type,
    :payload_keys,
    :user_id,
    :permission,
    :allowed
  ]

import_config "#{config_env()}.exs"
