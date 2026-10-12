defmodule AtlasOrchestrator.Repo do
  @moduledoc """
  Ecto repository for the orchestrator.

  It owns the `orchestrator` schema and holds **read-only** access to `outbox`
  (docs/ATLAS.md §10.1): the orchestrator may read Go's events but must never
  write business data (ADR-002).
  """
  use Ecto.Repo,
    otp_app: :atlas_orchestrator,
    adapter: Ecto.Adapters.Postgres
end
