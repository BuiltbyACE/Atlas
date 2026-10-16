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

  test "a notification.requested event is routed to the notifications handler" do
    recipient = "01999999-9999-7999-9999-999999999997"
    Phoenix.PubSub.subscribe(AtlasOrchestrator.PubSub, "user:#{recipient}")

    event = %Event{
      id: Ecto.UUID.generate(),
      type: "core.notification.requested.v1",
      seq: 2,
      payload: %{
        "recipient_person_id" => recipient,
        "notification_ids" => [Ecto.UUID.generate()],
        "template" => "core.approval.requested",
        "critical" => false,
        "requested_at" => "2026-10-16T10:00:00Z"
      },
      occurred_at: ~U[2026-10-16 10:00:00.000000Z]
    }

    assert :ok = Handlers.dispatch(event)

    assert_receive %Phoenix.Socket.Broadcast{topic: topic, event: "notification.changed"}
    assert topic == "user:#{recipient}"
  end

  test "an approval event is routed to the approvals handler" do
    event = %Event{
      id: Ecto.UUID.generate(),
      type: "core.approval.approved.v1",
      seq: 3,
      payload: %{"approval_id" => Ecto.UUID.generate(), "ref" => "APR-2026-000000000001"},
      occurred_at: ~U[2026-10-16 10:00:00.000000Z]
    }

    assert :ok = Handlers.dispatch(event)

    assert_receive %Phoenix.Socket.Broadcast{topic: "exec:dashboard", event: "approval.changed"}
  end

  test "a file event is routed to the dashboard handler" do
    event = %Event{
      id: Ecto.UUID.generate(),
      type: "core.file.uploaded.v1",
      seq: 4,
      payload: %{"file_id" => Ecto.UUID.generate()},
      occurred_at: ~U[2026-10-16 10:00:00.000000Z]
    }

    assert :ok = Handlers.dispatch(event)

    assert_receive %Phoenix.Socket.Broadcast{topic: "exec:dashboard", event: "dashboard.changed"}
  end

  test "a work event is routed to the work handler" do
    project = Ecto.UUID.generate()
    Phoenix.PubSub.subscribe(AtlasOrchestrator.PubSub, "work:project:#{project}")

    event = %Event{
      id: Ecto.UUID.generate(),
      type: "work.task.status_changed.v1",
      seq: 5,
      payload: %{"task_id" => Ecto.UUID.generate(), "project_id" => project, "to" => "done"},
      occurred_at: ~U[2026-10-16 10:00:00.000000Z]
    }

    assert :ok = Handlers.dispatch(event)

    assert_receive %Phoenix.Socket.Broadcast{topic: "work:project:" <> _, event: "task.changed"}
    assert_receive %Phoenix.Socket.Broadcast{topic: "exec:dashboard", event: "dashboard.changed"}
  end

  test "a people event is routed to the people handler" do
    person = Ecto.UUID.generate()
    Phoenix.PubSub.subscribe(AtlasOrchestrator.PubSub, "user:#{person}")

    event = %Event{
      id: Ecto.UUID.generate(),
      type: "people.leave.approved.v1",
      seq: 6,
      payload: %{"leave_request_id" => Ecto.UUID.generate(), "person_id" => person},
      occurred_at: ~U[2026-10-16 10:00:00.000000Z]
    }

    assert :ok = Handlers.dispatch(event)

    assert_receive %Phoenix.Socket.Broadcast{topic: topic, event: "leave_request.changed"}
    assert topic == "user:#{person}"
    assert_receive %Phoenix.Socket.Broadcast{topic: "exec:dashboard", event: "dashboard.changed"}
  end

  test "an escalation event is routed to the work handler" do
    event = %Event{
      id: Ecto.UUID.generate(),
      type: "core.escalation.resolved.v1",
      seq: 6,
      payload: %{"escalation_id" => Ecto.UUID.generate()},
      occurred_at: ~U[2026-10-16 10:00:00.000000Z]
    }

    assert :ok = Handlers.dispatch(event)

    assert_receive %Phoenix.Socket.Broadcast{
      topic: "exec:dashboard",
      event: "dashboard.changed"
    }
  end

  test "an unknown event type is ignored, not failed" do
    assert :ok = Handlers.dispatch(%Event{id: Ecto.UUID.generate(), type: "x.y.z.v1", payload: %{}})
    refute_receive %Phoenix.Socket.Broadcast{}, 50
  end
end
