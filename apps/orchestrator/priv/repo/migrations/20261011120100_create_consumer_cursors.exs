defmodule AtlasOrchestrator.Repo.Migrations.CreateConsumerCursors do
  @moduledoc "Consumer cursor table (docs/ATLAS.md §7.1)."
  use Ecto.Migration

  @prefix "orchestrator"

  def change do
    create table(:consumer_cursors, primary_key: false, prefix: @prefix) do
      add :consumer, :text, primary_key: true
      add :last_seq, :bigint, null: false, default: 0
      add :updated_at, :utc_datetime_usec, null: false, default: fragment("now()")
    end
  end
end
