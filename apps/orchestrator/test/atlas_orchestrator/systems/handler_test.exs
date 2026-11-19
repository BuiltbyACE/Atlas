defmodule AtlasOrchestrator.Systems.HandlerTest do
  use ExUnit.Case, async: false

  alias AtlasOrchestrator.Systems.Handler

  defp event(type, payload) do
    %{
      id: Ecto.UUID.generate(),
      type: type,
      payload: payload,
      occurred_at: ~U[2026-10-20 11:00:00Z]
    }
  end

  test "a SEV2 incident opening nudges the systems board and the dashboard" do
    Phoenix.PubSub.subscribe(AtlasOrchestrator.PubSub, "systems:board")
    Phoenix.PubSub.subscribe(AtlasOrchestrator.PubSub, "exec:dashboard")

    assert :ok =
             Handler.handle_event(
               event("systems.incident.opened.v1", %{
                 "incident_id" => "01999999-9999-7999-9999-999999999999",
                 "ref" => "INC-2026-0001",
                 "severity" => "sev2",
                 "status" => "open",
                 "system_ids" => [Ecto.UUID.generate()],
                 "detected_at" => "2026-10-20T10:55:00Z",
                 "review_required" => true
               })
             )

    assert_receive %Phoenix.Socket.Broadcast{
      topic: "systems:board",
      event: "incident.changed",
      payload: board
    }

    assert board["id"] == "01999999-9999-7999-9999-999999999999"
    assert board["occurred_at"] == "2026-10-20T11:00:00Z"

    assert_receive %Phoenix.Socket.Broadcast{
      topic: "exec:dashboard",
      event: "dashboard.changed",
      payload: dashboard
    }

    assert dashboard["reason"] == "systems.incident.opened.v1"
  end

  test "resolving and reviewing both reach the board" do
    Phoenix.PubSub.subscribe(AtlasOrchestrator.PubSub, "systems:board")
    Phoenix.PubSub.subscribe(AtlasOrchestrator.PubSub, "exec:dashboard")

    for type <- ["systems.incident.resolved.v1", "systems.incident.review_recorded.v1"] do
      assert :ok =
               Handler.handle_event(
                 event(type, %{
                   "incident_id" => Ecto.UUID.generate(),
                   "ref" => "INC-2026-0002",
                   "severity" => "sev1",
                   "status" => "resolved",
                   "review_required" => true
                 })
               )

      assert_receive %Phoenix.Socket.Broadcast{topic: "systems:board", event: "incident.changed"}
      assert_receive %Phoenix.Socket.Broadcast{topic: "exec:dashboard", event: "dashboard.changed"}
    end
  end

  test "an unknown systems event is ignored, not failed" do
    assert :ok = Handler.handle_event(event("systems.check.paused.v1", %{"check_id" => Ecto.UUID.generate()}))
  end

  test "a malformed event is ignored, not failed" do
    assert :ok = Handler.handle_event(%{})
  end
end
