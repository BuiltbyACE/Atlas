defmodule AtlasOrchestrator.Work.HandlerTest do
  use ExUnit.Case, async: false

  alias AtlasOrchestrator.Work.Handler

  @project "01999999-9999-7999-9999-999999999991"
  @assignee "01999999-9999-7999-9999-999999999992"
  @nil_uuid "00000000-0000-0000-0000-000000000000"

  defp event(type, payload) do
    %{
      id: Ecto.UUID.generate(),
      type: type,
      payload: payload,
      occurred_at: ~U[2026-10-16 10:00:00Z]
    }
  end

  test "a created task nudges the project board, the assignee and the dashboard" do
    Phoenix.PubSub.subscribe(AtlasOrchestrator.PubSub, "work:project:#{@project}")
    Phoenix.PubSub.subscribe(AtlasOrchestrator.PubSub, "user:#{@assignee}")
    Phoenix.PubSub.subscribe(AtlasOrchestrator.PubSub, "exec:dashboard")

    assert :ok =
             Handler.handle_event(
               event("work.task.created.v1", %{
                 "task_id" => "01999999-9999-7999-9999-999999999999",
                 "project_id" => @project,
                 "assignee_user_id" => @assignee
               })
             )

    assert_receive %Phoenix.Socket.Broadcast{
      topic: "work:project:" <> _,
      event: "task.changed",
      payload: board
    }

    assert board["id"] == "01999999-9999-7999-9999-999999999999"
    assert board["project_id"] == @project
    assert board["occurred_at"] == "2026-10-16T10:00:00Z"

    assert_receive %Phoenix.Socket.Broadcast{topic: "user:#{@assignee}", event: "task.changed"}

    assert_receive %Phoenix.Socket.Broadcast{
      topic: "exec:dashboard",
      event: "dashboard.changed",
      payload: dashboard
    }

    assert dashboard["reason"] == "work.task.created.v1"
  end

  test "an overdue task with no assignee only reaches the board and the dashboard" do
    Phoenix.PubSub.subscribe(AtlasOrchestrator.PubSub, "work:project:#{@project}")
    Phoenix.PubSub.subscribe(AtlasOrchestrator.PubSub, "exec:dashboard")

    assert :ok =
             Handler.handle_event(
               event("work.task.overdue.v1", %{
                 "task_id" => Ecto.UUID.generate(),
                 "project_id" => @project,
                 # uuid.UUID is an array, so Go cannot omit it: unset arrives as
                 # the nil UUID, not a missing key.
                 "assignee_user_id" => @nil_uuid
               })
             )

    assert_receive %Phoenix.Socket.Broadcast{topic: "work:project:" <> _, event: "task.changed"}
    assert_receive %Phoenix.Socket.Broadcast{topic: "exec:dashboard", event: "dashboard.changed"}
    refute_receive %Phoenix.Socket.Broadcast{topic: "user:" <> _}, 50
  end

  test "an escalation reaches the project board, the owner and the dashboard" do
    Phoenix.PubSub.subscribe(AtlasOrchestrator.PubSub, "work:project:#{@project}")
    Phoenix.PubSub.subscribe(AtlasOrchestrator.PubSub, "user:#{@assignee}")
    Phoenix.PubSub.subscribe(AtlasOrchestrator.PubSub, "exec:dashboard")

    assert :ok =
             Handler.handle_event(
               event("core.escalation.raised.v1", %{
                 "escalation_id" => Ecto.UUID.generate(),
                 "project_id" => @project,
                 "assignee_user_id" => @assignee,
                 "level" => 1
               })
             )

    assert_receive %Phoenix.Socket.Broadcast{
      topic: "work:project:" <> _,
      event: "escalation.changed",
      payload: payload
    }

    assert payload["id"] != nil
    assert_receive %Phoenix.Socket.Broadcast{topic: "user:#{@assignee}", event: "escalation.changed"}
    assert_receive %Phoenix.Socket.Broadcast{topic: "exec:dashboard", event: "dashboard.changed"}
  end

  test "an unknown work event type is ignored, not failed" do
    assert :ok = Handler.handle_event(event("work.task.reopened.v1", %{}))
    assert :ok = Handler.handle_event(%{})
  end
end
