defmodule AtlasOrchestrator.Repo.Migrations.EnsureOrchestratorSchema do
  @moduledoc """
  Ensures the `orchestrator` schema exists (docs/ATLAS.md §10.1).

  The Go foundation migration normally creates it first; this makes the Elixir
  side self-sufficient when run on its own (the `atlas_orchestrator` role is
  granted CREATE on the database for exactly this).
  """
  use Ecto.Migration

  def up do
    execute("CREATE SCHEMA IF NOT EXISTS orchestrator")
  end

  def down do
    # Intentionally left in place: dropping the schema would cascade to tables
    # this migration does not own.
    :ok
  end
end
