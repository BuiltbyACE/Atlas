defmodule AtlasOrchestrator.Outbox.HandlersTest do
  use ExUnit.Case, async: false

  alias AtlasOrchestrator.Outbox.{Event, Handlers}

  setup do
    Phoenix.PubSub.subscribe(AtlasOrchestrator.PubSub, "user:demo")
    Phoenix.PubSub.subscribe(AtlasOrchestrator.PubSub, "exec:dashboard")
    :ok
  end

  test "a hello event is broadcast as a realtime signal" do
    event = %Event{
      id: Ecto.UUID.generate(),
      type: "core.system.hello.v1",
      seq: 1,
      payload: %{"message" => "hi"},
      actor_id: nil,
      occurred_at: ~U[2026-10-11 00:00:00.000000Z]
    }

    assert :ok = Handlers.dispatch(event)

    assert_receive %Phoenix.Socket.Broadcast{topic: "user:demo", event: "signal", payload: payload}
    assert payload.message == "hi"
    # The channel JSON-encodes the payload on push, so it must contain no structs.
    assert is_binary(payload.occurred_at)
    assert Jason.decode!(Jason.encode!(payload))["seq"] == 1

    assert_receive %Phoenix.Socket.Broadcast{topic: "exec:dashboard", event: "signal"}
  end

  test "an unknown event type is ignored, not failed" do
    assert :ok = Handlers.dispatch(%Event{id: Ecto.UUID.generate(), type: "x.y.z.v1", payload: %{}})
    refute_receive %Phoenix.Socket.Broadcast{}, 50
  end
end
