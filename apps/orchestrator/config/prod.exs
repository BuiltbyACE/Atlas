import Config

# Staging and production are configured entirely from the environment in
# runtime.exs (docs/ATLAS.md §2.10). This file only sets non-secret defaults.
config :atlas_orchestrator, AtlasOrchestrator.Endpoint,
  server: true,
  cache_static_manifest: nil

config :logger, level: :info
