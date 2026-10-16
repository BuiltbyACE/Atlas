defmodule AtlasOrchestrator.Dashboard.HandlerTest do
  use ExUnit.Case, async: false

  alias AtlasOrchestrator.Dashboard.Handler

  setup do
    Phoenix.PubSub.subscribe(AtlasOrchestrator.PubSub, "exec:dashboard")
    :ok
  end

  test "a file upload invalidates the dashboard's recent-highlights tile" do
    assert :ok =
             Handler.handle_event(%{
               type: "core.file.uploaded.v1",
               occurred_at: ~U[2026-10-16 10:00:00Z]
             })

    assert_receive %Phoenix.Socket.Broadcast{
      topic: "exec:dashboard",
      event: "dashboard.changed",
      payload: payload
    }

    assert payload["type"] == "dashboard.changed"
    assert payload["reason"] == "file.uploaded"
    assert payload["occurred_at"] == "2026-10-16T10:00:00Z"
  end

  test "a file deletion invalidates the dashboard" do
    assert :ok = Handler.handle_event(%{type: "core.file.deleted.v1"})

    assert_receive %Phoenix.Socket.Broadcast{
      topic: "exec:dashboard",
      event: "dashboard.changed",
      payload: payload
    }

    assert payload["reason"] == "file.deleted"
    # No occurred_at on the event → the signal stamps it now, JSON-safe.
    assert is_binary(payload["occurred_at"])
  end

  test "an unknown file event is ignored, not failed" do
    assert :ok = Handler.handle_event(%{type: "core.file.scanned.v1"})
    refute_receive %Phoenix.Socket.Broadcast{}, 50
  end

  test "an event that is not a file event is ignored" do
    assert :ok = Handler.handle_event(%{type: "core.approval.approved.v1"})
    refute_receive %Phoenix.Socket.Broadcast{}, 50
  end
end
