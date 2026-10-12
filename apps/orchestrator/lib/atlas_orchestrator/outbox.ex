defmodule AtlasOrchestrator.Outbox do
  @moduledoc """
  The outbox context: read events, track the cursor, record handled events.

  This is the Elixir half of the Go → Elixir contract (docs/ATLAS.md §7.1).
  Everything here is read-mostly: the only writes go to the `orchestrator`
  schema's own bookkeeping tables, never to `outbox` (ADR-002).
  """
  import Ecto.Query

  alias AtlasOrchestrator.Outbox.{Cursor, Event, ProcessedEvent}
  alias AtlasOrchestrator.Repo

  @consumer "orchestrator-outbox"

  @doc "The name of the consumer whose cursor this context manages."
  def consumer, do: @consumer

  @doc "Reads up to `limit` events with `seq > cursor`, oldest first."
  @spec fetch_after(integer(), pos_integer()) :: [Event.t()]
  def fetch_after(cursor, limit) do
    from(e in Event,
      where: e.seq > ^cursor,
      order_by: [asc: e.seq],
      limit: ^limit
    )
    |> Repo.all()
  end

  @doc "Returns the current cursor (0 when the consumer has never run)."
  @spec current_cursor() :: integer()
  def current_cursor do
    case Repo.get(Cursor, @consumer) do
      nil -> 0
      %Cursor{last_seq: seq} -> seq
    end
  end

  @doc "Advances the cursor to `seq`, inserting the row on first use."
  @spec advance_cursor(integer()) :: :ok
  def advance_cursor(seq) when is_integer(seq) do
    now = DateTime.utc_now()

    %Cursor{consumer: @consumer, last_seq: seq, updated_at: now}
    |> Repo.insert(
      on_conflict: [set: [last_seq: seq, updated_at: now]],
      conflict_target: :consumer
    )

    :ok
  end

  @doc """
  Records an event as handled. Returns `:ok` the first time and `:duplicate`
  if it has already been handled (docs/ATLAS.md §7.1).
  """
  @spec mark_processed(Event.t()) :: :ok | :duplicate
  def mark_processed(%Event{id: id, type: type, seq: seq}) do
    {count, _} =
      Repo.insert_all(
        ProcessedEvent,
        [%{event_id: id, type: type, seq: seq, handled_at: DateTime.utc_now()}],
        on_conflict: :nothing
      )

    if count > 0, do: :ok, else: :duplicate
  end
end
