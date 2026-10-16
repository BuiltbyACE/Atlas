defmodule AtlasOrchestrator.People.HandlerTest do
  use ExUnit.Case, async: false

  alias AtlasOrchestrator.People.Handler

  @person "01999999-9999-7999-9999-999999999992"

  defp event(type, payload) do
    %{
      id: Ecto.UUID.generate(),
      type: type,
      payload: payload,
      occurred_at: ~U[2026-10-16 10:00:00Z]
    }
  end

  test "an approved leave nudges the requester and the dashboard" do
    Phoenix.PubSub.subscribe(AtlasOrchestrator.PubSub, "user:#{@person}")
    Phoenix.PubSub.subscribe(AtlasOrchestrator.PubSub, "exec:dashboard")

    assert :ok =
             Handler.handle_event(
               event("people.leave.approved.v1", %{
                 "leave_request_id" => Ecto.UUID.generate(),
                 "person_id" => @person
               })
             )

    assert_receive %Phoenix.Socket.Broadcast{
      topic: "user:#{@person}",
      event: "leave_request.changed",
      payload: signal
    }

    assert signal["id"] != nil

    assert_receive %Phoenix.Socket.Broadcast{
      topic: "exec:dashboard",
      event: "dashboard.changed",
      payload: dashboard
    }

    assert dashboard["reason"] == "people.leave.approved.v1"
  end

  test "a leave request only invalidates the dashboard" do
    Phoenix.PubSub.subscribe(AtlasOrchestrator.PubSub, "exec:dashboard")

    assert :ok =
             Handler.handle_event(
               event("people.leave.requested.v1", %{
                 "leave_request_id" => Ecto.UUID.generate(),
                 "person_id" => @person
               })
             )

    assert_receive %Phoenix.Socket.Broadcast{topic: "exec:dashboard", event: "dashboard.changed"}
    refute_receive %Phoenix.Socket.Broadcast{topic: "user:" <> _}, 50
  end

  test "a daily report only invalidates the dashboard" do
    Phoenix.PubSub.subscribe(AtlasOrchestrator.PubSub, "exec:dashboard")

    assert :ok =
             Handler.handle_event(
               event("people.daily_report.submitted.v1", %{"report_id" => Ecto.UUID.generate()})
             )

    assert_receive %Phoenix.Socket.Broadcast{topic: "exec:dashboard", event: "dashboard.changed"}
    refute_receive %Phoenix.Socket.Broadcast{topic: "user:" <> _}, 50
  end

  test "an unknown people event type is ignored, not failed" do
    assert :ok = Handler.handle_event(event("people.something.happened.v1", %{}))
    assert :ok = Handler.handle_event(%{})
  end
end
