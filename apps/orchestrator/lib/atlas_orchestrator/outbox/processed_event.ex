defmodule AtlasOrchestrator.Outbox.ProcessedEvent do
  @moduledoc """
  Idempotency ledger (docs/ATLAS.md §7.1).

  A row per handled event id. Processing is `INSERT ... ON CONFLICT DO NOTHING`,
  so an event redelivered after a crash (or seen by both the NOTIFY path and the
  poll path) is applied exactly once.
  """
  use Ecto.Schema

  @primary_key {:event_id, :binary_id, autogenerate: false}
  @schema_prefix "orchestrator"

  schema "processed_events" do
    field :type, :string
    field :seq, :integer
    field :handled_at, :utc_datetime_usec
  end

  @type t :: %__MODULE__{}
end
