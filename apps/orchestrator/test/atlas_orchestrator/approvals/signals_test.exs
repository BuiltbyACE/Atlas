defmodule AtlasOrchestrator.Approvals.SignalsTest do
  use ExUnit.Case, async: true

  alias AtlasOrchestrator.Approvals.Signals

  test "builds a minimal approval.changed signal" do
    signal =
      Signals.approval_changed(
        "01999999-9999-7999-9999-999999999999",
        "APR-2026-000000000001",
        "approved",
        ~U[2026-10-16 10:00:00Z]
      )

    assert signal["type"] == "approval.changed"
    assert signal["approval_id"] == "01999999-9999-7999-9999-999999999999"
    assert signal["ref"] == "APR-2026-000000000001"
    assert signal["status"] == "approved"
    assert signal["occurred_at"] == "2026-10-16T10:00:00Z"
  end

  test "is JSON-encodable (no structs on the wire)" do
    signal = Signals.approval_changed("01999999-9999-7999-9999-999999999999", "APR-1", "requested")
    assert Jason.decode!(Jason.encode!(signal))["status"] == "requested"
  end
end
