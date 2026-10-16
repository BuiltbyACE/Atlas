defmodule AtlasOrchestrator.Approvals.HandlerTest do
  use ExUnit.Case, async: false

  alias AtlasOrchestrator.Approvals.Handler

  @approver "01999999-9999-7999-9999-999999999991"
  @target "01999999-9999-7999-9999-999999999992"
  @nil_uuid "00000000-0000-0000-0000-000000000000"

  defp event(type, payload) do
    %{
      id: Ecto.UUID.generate(),
      type: type,
      payload: payload,
      occurred_at: ~U[2026-10-16 10:00:00Z]
    }
  end

  test "a requested approval nudges the named approver and the dashboard" do
    Phoenix.PubSub.subscribe(AtlasOrchestrator.PubSub, "user:#{@approver}")
    Phoenix.PubSub.subscribe(AtlasOrchestrator.PubSub, "exec:dashboard")

    assert :ok =
             Handler.handle_event(
               event("core.approval.requested.v1", %{
                 "approval_id" => "01999999-9999-7999-9999-999999999999",
                 "ref" => "APR-2026-000000000001",
                 "approver_id" => @approver
               })
             )

    assert_receive %Phoenix.Socket.Broadcast{
      topic: "user:" <> _,
      event: "approval.changed",
      payload: payload
    }

    assert payload["status"] == "requested"
    assert payload["ref"] == "APR-2026-000000000001"

    assert_receive %Phoenix.Socket.Broadcast{topic: "exec:dashboard", event: "approval.changed"}
  end

  test "a requested event with no named approver only nudges the dashboard" do
    Phoenix.PubSub.subscribe(AtlasOrchestrator.PubSub, "exec:dashboard")

    assert :ok =
             Handler.handle_event(
               event("core.approval.requested.v1", %{
                 "approval_id" => Ecto.UUID.generate(),
                 "ref" => "APR-1",
                 # uuid.UUID is an array, so Go cannot omit it: unset arrives as
                 # the nil UUID, not a missing key.
                 "approver_id" => @nil_uuid
               })
             )

    assert_receive %Phoenix.Socket.Broadcast{topic: "exec:dashboard", event: "approval.changed"}
  end

  test "an escalated event nudges the escalation target and the dashboard" do
    Phoenix.PubSub.subscribe(AtlasOrchestrator.PubSub, "user:#{@target}")

    assert :ok =
             Handler.handle_event(
               event("core.approval.escalated.v1", %{
                 "approval_id" => Ecto.UUID.generate(),
                 "ref" => "APR-2",
                 "to_person_id" => @target
               })
             )

    assert_receive %Phoenix.Socket.Broadcast{topic: "user:#{@target}", event: "approval.changed"}
  end

  test "terminal events land on the dashboard with the right status" do
    Phoenix.PubSub.subscribe(AtlasOrchestrator.PubSub, "exec:dashboard")

    for {type, status} <- [
          {"core.approval.approved.v1", "approved"},
          {"core.approval.declined.v1", "declined"},
          {"core.approval.cancelled.v1", "cancelled"},
          {"core.approval.expired.v1", "expired"}
        ] do
      assert :ok =
               Handler.handle_event(event(type, %{"approval_id" => Ecto.UUID.generate(), "ref" => "APR"}))

      assert_receive %Phoenix.Socket.Broadcast{
        topic: "exec:dashboard",
        event: "approval.changed",
        payload: payload
      }

      assert payload["status"] == status
    end
  end

  test "an unknown approval event type is ignored, not failed" do
    assert :ok = Handler.handle_event(event("core.approval.reopened.v1", %{}))
  end
end
