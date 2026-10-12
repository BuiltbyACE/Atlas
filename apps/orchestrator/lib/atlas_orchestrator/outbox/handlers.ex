defmodule AtlasOrchestrator.Outbox.Handlers do
  @moduledoc """
  Dispatches a consumed event to the code that reacts to it.

  Phase 0 has exactly one handler: `core.system.hello.v1` is turned into a
  realtime signal so the browser can see the event arrive (Appendix A). Real
  workflow handlers arrive with the modules in later phases.

  Reacting means *signalling*, never writing business data (ADR-009): Go remains
  the authority, the orchestrator nudges.
  """
  require Logger

  alias AtlasOrchestrator.Outbox.Event

  @doc "Handles one event. Unknown types are logged and ignored, not failed."
  @spec dispatch(Event.t()) :: :ok
  def dispatch(%Event{type: "core.system.hello.v1"} = event), do: handle_hello(event)

  def dispatch(%Event{type: type}) do
    Logger.debug("no handler registered", type: type)
    :ok
  end

  defp handle_hello(%Event{} = event) do
    message = get_in(event.payload, ["message"]) || "hello"

    signal = %{
      type: event.type,
      event_id: event.id,
      seq: event.seq,
      message: message,
      # Wire-safe payload: the channel JSON-encodes this and would crash on a raw
      # %DateTime{} (Jason has no encoder for it), so serialise here.
      occurred_at: DateTime.to_iso8601(event.occurred_at)
    }

    Enum.each(topics_for(event), fn topic ->
      AtlasOrchestrator.Realtime.broadcast(topic, "signal", signal)
      Logger.info("hello broadcast", topic: topic, seq: event.seq)
    end)

    :ok
  end

  # No actor (the Phase 0 dev event) → push to the demo topic the local demo
  # page joins, plus the executive dashboard topic (§7.5).
  defp topics_for(%Event{actor_id: nil}) do
    demo = Application.get_env(:atlas_orchestrator, :demo_topic, "user:demo")
    [demo, "exec:dashboard"]
  end

  defp topics_for(%Event{actor_id: actor_id}) do
    ["user:#{actor_id}", "exec:dashboard"]
  end
end
