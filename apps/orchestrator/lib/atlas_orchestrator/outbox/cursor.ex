defmodule AtlasOrchestrator.Outbox.Cursor do
  @moduledoc """
  A consumer's position in the outbox (docs/ATLAS.md §7.1).

  One row per named consumer. Advancing the cursor is what makes processing
  resumable across restarts.
  """
  use Ecto.Schema

  @primary_key {:consumer, :string, autogenerate: false}
  @schema_prefix "orchestrator"

  schema "consumer_cursors" do
    field :last_seq, :integer, default: 0
    field :updated_at, :utc_datetime_usec
  end

  @type t :: %__MODULE__{}
end
