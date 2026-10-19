defmodule AtlasOrchestrator.Finance.HandlerTest do
  use ExUnit.Case, async: false

  alias AtlasOrchestrator.Finance.Handler

  defp event(type, payload) do
    %{
      id: Ecto.UUID.generate(),
      type: type,
      payload: payload,
      occurred_at: ~U[2026-10-19 11:00:00Z]
    }
  end

  test "a submitted expense nudges the finance board and the dashboard" do
    Phoenix.PubSub.subscribe(AtlasOrchestrator.PubSub, "finance:board")
    Phoenix.PubSub.subscribe(AtlasOrchestrator.PubSub, "exec:dashboard")

    assert :ok =
             Handler.handle_event(
               event("finance.expense.submitted.v1", %{
                 "expense_id" => "01999999-9999-7999-9999-999999999999",
                 "ref" => "EXP-2026-0001",
                 "claimant_person_id" => Ecto.UUID.generate(),
                 "category_key" => "transport",
                 "amount_minor" => 42_000,
                 "currency" => "KES",
                 "status" => "submitted"
               })
             )

    assert_receive %Phoenix.Socket.Broadcast{
      topic: "finance:board",
      event: "expense.changed",
      payload: board
    }

    assert board["id"] == "01999999-9999-7999-9999-999999999999"
    assert board["occurred_at"] == "2026-10-19T11:00:00Z"

    assert_receive %Phoenix.Socket.Broadcast{
      topic: "exec:dashboard",
      event: "dashboard.changed",
      payload: dashboard
    }

    assert dashboard["reason"] == "finance.expense.submitted.v1"
  end

  test "an overdue invoice reaches the board and the dashboard" do
    Phoenix.PubSub.subscribe(AtlasOrchestrator.PubSub, "finance:board")
    Phoenix.PubSub.subscribe(AtlasOrchestrator.PubSub, "exec:dashboard")

    assert :ok =
             Handler.handle_event(
               event("finance.invoice.overdue.v1", %{
                 "invoice_id" => Ecto.UUID.generate(),
                 "ref" => "INV-2026-0002",
                 "organization_id" => Ecto.UUID.generate(),
                 "total_minor" => 450_000,
                 "currency" => "KES",
                 "status" => "overdue",
                 "due_date" => "2026-10-15"
               })
             )

    assert_receive %Phoenix.Socket.Broadcast{topic: "finance:board", event: "invoice.changed"}
    assert_receive %Phoenix.Socket.Broadcast{topic: "exec:dashboard", event: "dashboard.changed"}
  end

  test "a received payment reaches the board and the dashboard" do
    Phoenix.PubSub.subscribe(AtlasOrchestrator.PubSub, "finance:board")
    Phoenix.PubSub.subscribe(AtlasOrchestrator.PubSub, "exec:dashboard")

    assert :ok =
             Handler.handle_event(
               event("finance.payment.received.v1", %{
                 "payment_id" => Ecto.UUID.generate(),
                 "ref" => "PAY-2026-0003",
                 "invoice_id" => Ecto.UUID.generate(),
                 "amount_minor" => 450_000,
                 "currency" => "KES",
                 "method" => "bank"
               })
             )

    assert_receive %Phoenix.Socket.Broadcast{topic: "finance:board", event: "payment.changed"}
    assert_receive %Phoenix.Socket.Broadcast{topic: "exec:dashboard", event: "dashboard.changed"}
  end

  test "a budget threshold crossing reaches the board and the dashboard" do
    Phoenix.PubSub.subscribe(AtlasOrchestrator.PubSub, "finance:board")
    Phoenix.PubSub.subscribe(AtlasOrchestrator.PubSub, "exec:dashboard")

    assert :ok =
             Handler.handle_event(
               event("finance.budget.threshold_crossed.v1", %{
                 "budget_id" => Ecto.UUID.generate(),
                 "scope" => "project",
                 "scope_ref" => Ecto.UUID.generate(),
                 "amount_minor" => 90_000,
                 "actual_minor" => 90_000,
                 "currency" => "KES"
               })
             )

    assert_receive %Phoenix.Socket.Broadcast{topic: "finance:board", event: "budget.changed"}
    assert_receive %Phoenix.Socket.Broadcast{topic: "exec:dashboard", event: "dashboard.changed"}
  end

  test "an event whose optional keys are absent does not crash" do
    Phoenix.PubSub.subscribe(AtlasOrchestrator.PubSub, "finance:board")
    Phoenix.PubSub.subscribe(AtlasOrchestrator.PubSub, "exec:dashboard")

    assert :ok =
             Handler.handle_event(
               event("finance.invoice.issued.v1", %{
                 "invoice_id" => Ecto.UUID.generate(),
                 "ref" => "INV-2026-0004",
                 "organization_id" => Ecto.UUID.generate(),
                 "currency" => "KES",
                 "status" => "issued"
               })
             )

    assert_receive %Phoenix.Socket.Broadcast{topic: "finance:board", event: "invoice.changed"}
    assert_receive %Phoenix.Socket.Broadcast{topic: "exec:dashboard", event: "dashboard.changed"}
  end

  test "an unknown finance event type is ignored, not failed" do
    assert :ok = Handler.handle_event(event("finance.account.opened.v1", %{}))
    assert :ok = Handler.handle_event(%{})
  end
end
