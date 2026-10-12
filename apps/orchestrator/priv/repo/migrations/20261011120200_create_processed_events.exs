defmodule AtlasOrchestrator.Repo.Migrations.CreateProcessedEvents do
  @moduledoc "Idempotency ledger for consumed events (docs/ATLAS.md §7.1)."
  use Ecto.Migration

  @prefix "orchestrator"

  def change do
    create table(:processed_events, primary_key: false, prefix: @prefix) do
      add :event_id, :binary_id, primary_key: true
      add :type, :text, null: false
      add :seq, :bigint, null: false
      add :handled_at, :utc_datetime_usec, null: false, default: fragment("now()")
    end

    create index(:processed_events, [:seq], prefix: @prefix)
  end
end
