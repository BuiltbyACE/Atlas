defmodule AtlasOrchestrator.Contracts.HandlerTest do
  use ExUnit.Case, async: false

  alias AtlasOrchestrator.Contracts.Handler

  defp event(type, payload) do
    %{
      id: Ecto.UUID.generate(),
      type: type,
      payload: payload,
      occurred_at: ~U[2026-10-19 11:00:00Z]
    }
  end

  test "a created contract nudges the contracts board and the dashboard" do
    Phoenix.PubSub.subscribe(AtlasOrchestrator.PubSub, "contracts:board")
    Phoenix.PubSub.subscribe(AtlasOrchestrator.PubSub, "exec:dashboard")

    assert :ok =
             Handler.handle_event(
               event("contracts.contract.created.v1", %{
                 "contract_id" => "01999999-9999-7999-9999-999999999999",
                 "ref" => "CTR-2026-0001",
                 "status" => "draft"
               })
             )

    assert_receive %Phoenix.Socket.Broadcast{
      topic: "contracts:board",
      event: "contract.changed",
      payload: board
    }

    assert board["id"] == "01999999-9999-7999-9999-999999999999"
    assert board["occurred_at"] == "2026-10-19T11:00:00Z"

    assert_receive %Phoenix.Socket.Broadcast{
      topic: "exec:dashboard",
      event: "dashboard.changed",
      payload: dashboard
    }

    assert dashboard["reason"] == "contracts.contract.created.v1"
  end

  test "a signed contract reaches the board and the dashboard" do
    Phoenix.PubSub.subscribe(AtlasOrchestrator.PubSub, "contracts:board")
    Phoenix.PubSub.subscribe(AtlasOrchestrator.PubSub, "exec:dashboard")

    assert :ok =
             Handler.handle_event(
               event("contracts.contract.signed.v1", %{
                 "contract_id" => Ecto.UUID.generate(),
                 "ref" => "CTR-2026-0002",
                 "signed_by" => "Jane Client"
               })
             )

    assert_receive %Phoenix.Socket.Broadcast{topic: "contracts:board", event: "contract.changed"}
    assert_receive %Phoenix.Socket.Broadcast{topic: "exec:dashboard", event: "dashboard.changed"}
  end

  test "a change request approval reaches the board and the dashboard" do
    Phoenix.PubSub.subscribe(AtlasOrchestrator.PubSub, "contracts:board")
    Phoenix.PubSub.subscribe(AtlasOrchestrator.PubSub, "exec:dashboard")

    assert :ok =
             Handler.handle_event(
               event("contracts.change_request.approved.v1", %{
                 "change_request_id" => Ecto.UUID.generate(),
                 "contract_id" => Ecto.UUID.generate(),
                 "status" => "approved"
               })
             )

    assert_receive %Phoenix.Socket.Broadcast{
      topic: "contracts:board",
      event: "change_request.changed"
    }

    assert_receive %Phoenix.Socket.Broadcast{topic: "exec:dashboard", event: "dashboard.changed"}
  end

  test "an unknown contracts event type is ignored, not failed" do
    assert :ok = Handler.handle_event(event("contracts.contract.renewed.v1", %{}))
    assert :ok = Handler.handle_event(%{})
  end
end
