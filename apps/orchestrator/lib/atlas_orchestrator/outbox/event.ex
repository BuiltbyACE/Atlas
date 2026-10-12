defmodule AtlasOrchestrator.Outbox.Event do
  @moduledoc """
  Read-only view of `outbox.events` (docs/ATLAS.md §7.1).

  The orchestrator reads these rows; it never writes them (ADR-002). `seq` is
  the global ordering used as the consumer cursor.
  """
  use Ecto.Schema

  @primary_key {:seq, :integer, autogenerate: false}
  @schema_prefix "outbox"

  schema "events" do
    field :id, :binary_id
    field :type, :string
    field :aggregate_type, :string
    field :aggregate_id, :binary_id
    field :occurred_at, :utc_datetime_usec
    field :actor_id, :binary_id
    field :correlation_id, :binary_id
    field :causation_id, :binary_id
    field :payload, :map
  end

  @type t :: %__MODULE__{}
end
