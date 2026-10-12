defmodule AtlasOrchestrator.Outbox.Consumer do
  @moduledoc """
  Reads Go's outbox in order and dispatches each event exactly once
  (docs/ATLAS.md §7.1).

  It is woken by `NOTIFY` (via `Outbox.Listener`) and also polls on a timer as a
  safety net, so a missed notification can never strand an event. The cursor
  (`orchestrator.consumer_cursors`) makes it resumable; the processed-events
  ledger makes it idempotent.
  """
  use GenServer

  require Logger

  alias AtlasOrchestrator.Outbox
  alias AtlasOrchestrator.Outbox.Handlers

  @default_poll_ms 2_000
  @default_batch 100

  def start_link(opts) do
    GenServer.start_link(__MODULE__, opts, name: __MODULE__)
  end

  @doc "Wakes the consumer; the payload is informational only."
  def wake(payload) do
    GenServer.cast(__MODULE__, {:wake, payload})
  end

  @impl true
  def init(opts) do
    conf = Application.get_env(:atlas_orchestrator, AtlasOrchestrator.Outbox, [])

    state = %{
      cursor: Outbox.current_cursor(),
      batch: Keyword.get(opts, :batch_size, conf[:batch_size] || @default_batch),
      poll: Keyword.get(opts, :poll_interval_ms, conf[:poll_interval_ms] || @default_poll_ms)
    }

    Logger.info("outbox consumer started", seq: state.cursor)
    {:ok, state, {:continue, :catch_up}}
  end

  @impl true
  def handle_continue(:catch_up, state) do
    {:noreply, state |> drain() |> schedule()}
  end

  @impl true
  def handle_cast({:wake, _payload}, state) do
    {:noreply, drain(state)}
  end

  @impl true
  def handle_info(:poll, state) do
    {:noreply, state |> drain() |> schedule()}
  end

  def handle_info(_message, state), do: {:noreply, state}

  defp schedule(state) do
    Process.send_after(self(), :poll, state.poll)
    state
  end

  defp drain(state) do
    case Outbox.fetch_after(state.cursor, state.batch) do
      [] ->
        state

      events ->
        Enum.each(events, &process/1)
        last = List.last(events)
        :ok = Outbox.advance_cursor(last.seq)
        next = %{state | cursor: last.seq}

        # A full batch may mean there is more waiting; keep going.
        if length(events) == state.batch, do: drain(next), else: next
    end
  rescue
    error ->
      Logger.error("outbox drain failed", error: Exception.message(error))
      state
  end

  defp process(event) do
    case Outbox.mark_processed(event) do
      :ok ->
        Logger.debug("outbox event handled", type: event.type, seq: event.seq, event_id: event.id)
        Handlers.dispatch(event)

      :duplicate ->
        Logger.debug("outbox event already handled", event_id: event.id)
        :ok
    end
  end
end
