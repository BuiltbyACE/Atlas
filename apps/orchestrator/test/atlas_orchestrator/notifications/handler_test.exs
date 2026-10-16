defmodule AtlasOrchestrator.Notifications.HandlerTest do
  use ExUnit.Case, async: true

  alias AtlasOrchestrator.Notifications.Handler

  @event %{
    type: "core.notification.requested.v1",
    payload: %{
      "recipient_user_id" => "01999999-9999-7999-9999-999999999999",
      "notification_ids" => ["01999999-9999-8999-9999-999999999998"],
      "template" => "work.task.assigned",
      "critical" => false,
      "requested_at" => "2026-10-13T10:00:00Z"
    }
  }

  test "handles requested event without crashing" do
    assert :ok = Handler.handle_event(@event, %{})
  end

  test "skips events with missing recipient" do
    bad = put_in(@event.payload["recipient_user_id"], "")
    assert :ok = Handler.handle_event(bad, %{})
  end

  test "ignores unknown event types" do
    other = %{@event | type: "core.unknown.v1"}
    assert :ok = Handler.handle_event(other, %{})
  end
end
