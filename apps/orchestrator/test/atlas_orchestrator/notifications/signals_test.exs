defmodule AtlasOrchestrator.Notifications.SignalsTest do
  use ExUnit.Case, async: true

  alias AtlasOrchestrator.Notifications.Signals

  test "builds notification.changed signal" do
    sig = Signals.notification_changed("01999999-9999-7999-9999-999999999999", ["a", "b"], true)

    assert sig["type"] == "notification.changed"
    assert sig["recipient_id"] == "01999999-9999-7999-9999-999999999999"
    assert sig["count_hint"] == 2
    assert sig["critical"] == true
    assert is_binary(sig["occurred_at"])
  end

  test "defaults to empty ids and false critical" do
    sig = Signals.notification_changed("01999999-9999-7999-9999-999999999999")

    assert sig["count_hint"] == 0
    assert sig["critical"] == false
  end
end
