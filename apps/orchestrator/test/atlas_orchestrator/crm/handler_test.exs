defmodule AtlasOrchestrator.Crm.HandlerTest do
  use ExUnit.Case, async: false

  alias AtlasOrchestrator.Crm.Handler

  @owner "01999999-9999-7999-9999-999999999992"
  @nil_uuid "00000000-0000-0000-0000-000000000000"

  defp event(type, payload) do
    %{
      id: Ecto.UUID.generate(),
      type: type,
      payload: payload,
      occurred_at: ~U[2026-10-19 10:00:00Z]
    }
  end

  test "a created lead nudges the sales board, the owner and the dashboard" do
    Phoenix.PubSub.subscribe(AtlasOrchestrator.PubSub, "crm:sales")
    Phoenix.PubSub.subscribe(AtlasOrchestrator.PubSub, "user:#{@owner}")
    Phoenix.PubSub.subscribe(AtlasOrchestrator.PubSub, "exec:dashboard")

    assert :ok =
             Handler.handle_event(
               event("crm.lead.created.v1", %{
                 "lead_id" => "01999999-9999-7999-9999-999999999999",
                 "ref" => "LEAD-2026-0001",
                 "owner_user_id" => @owner
               })
             )

    assert_receive %Phoenix.Socket.Broadcast{
      topic: "crm:sales",
      event: "lead.changed",
      payload: board
    }

    assert board["id"] == "01999999-9999-7999-9999-999999999999"
    assert board["occurred_at"] == "2026-10-19T10:00:00Z"

    assert_receive %Phoenix.Socket.Broadcast{topic: "user:#{@owner}", event: "lead.changed"}

    assert_receive %Phoenix.Socket.Broadcast{
      topic: "exec:dashboard",
      event: "dashboard.changed",
      payload: dashboard
    }

    assert dashboard["reason"] == "crm.lead.created.v1"
  end

  test "a lead change with no owner only reaches the board and the dashboard" do
    Phoenix.PubSub.subscribe(AtlasOrchestrator.PubSub, "crm:sales")
    Phoenix.PubSub.subscribe(AtlasOrchestrator.PubSub, "exec:dashboard")

    assert :ok =
             Handler.handle_event(
               event("crm.lead.status_changed.v1", %{
                 "lead_id" => Ecto.UUID.generate(),
                 # uuid.UUID is an array, so Go cannot omit it: unset arrives as
                 # the nil UUID, not a missing key.
                 "owner_user_id" => @nil_uuid,
                 "from" => "new",
                 "to" => "qualified"
               })
             )

    assert_receive %Phoenix.Socket.Broadcast{topic: "crm:sales", event: "lead.changed"}
    assert_receive %Phoenix.Socket.Broadcast{topic: "exec:dashboard", event: "dashboard.changed"}
    refute_receive %Phoenix.Socket.Broadcast{topic: "user:" <> _}, 50
  end

  test "an opportunity stage change reaches the board and the dashboard" do
    Phoenix.PubSub.subscribe(AtlasOrchestrator.PubSub, "crm:sales")
    Phoenix.PubSub.subscribe(AtlasOrchestrator.PubSub, "exec:dashboard")

    assert :ok =
             Handler.handle_event(
               event("crm.opportunity.stage_changed.v1", %{
                 "opportunity_id" => Ecto.UUID.generate(),
                 "from" => "discovery",
                 "to" => "proposal_sent"
               })
             )

    assert_receive %Phoenix.Socket.Broadcast{
      topic: "crm:sales",
      event: "opportunity.changed"
    }

    assert_receive %Phoenix.Socket.Broadcast{topic: "exec:dashboard", event: "dashboard.changed"}
  end

  test "a scheduled follow-up nudges the owner" do
    Phoenix.PubSub.subscribe(AtlasOrchestrator.PubSub, "crm:sales")
    Phoenix.PubSub.subscribe(AtlasOrchestrator.PubSub, "user:#{@owner}")
    Phoenix.PubSub.subscribe(AtlasOrchestrator.PubSub, "exec:dashboard")

    assert :ok =
             Handler.handle_event(
               event("crm.followup.scheduled.v1", %{
                 "followup_id" => Ecto.UUID.generate(),
                 "owner_user_id" => @owner,
                 "due_at" => "2026-10-20T09:00:00Z"
               })
             )

    assert_receive %Phoenix.Socket.Broadcast{topic: "crm:sales", event: "followup.changed"}
    assert_receive %Phoenix.Socket.Broadcast{topic: "user:#{@owner}", event: "followup.changed"}
    assert_receive %Phoenix.Socket.Broadcast{topic: "exec:dashboard", event: "dashboard.changed"}
  end

  test "an unknown CRM event type is ignored, not failed" do
    assert :ok = Handler.handle_event(event("crm.lead.reopened.v1", %{}))
    assert :ok = Handler.handle_event(%{})
  end
end
