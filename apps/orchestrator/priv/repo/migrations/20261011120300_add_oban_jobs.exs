defmodule AtlasOrchestrator.Repo.Migrations.AddObanJobs do
  @moduledoc "Oban's job tables, inside the orchestrator schema (§5.4, ADR-006)."
  use Ecto.Migration

  @prefix "orchestrator"

  def up do
    Oban.Migrations.up(prefix: @prefix)
  end

  def down do
    Oban.Migrations.down(prefix: @prefix)
  end
end
