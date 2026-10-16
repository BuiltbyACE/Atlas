defmodule AtlasOrchestrator.Dashboard.SignalsTest do
  use ExUnit.Case, async: true

  alias AtlasOrchestrator.Dashboard.Signals

  test "builds a minimal dashboard.changed signal" do
    signal = Signals.dashboard_changed("file.uploaded", ~U[2026-10-16 10:00:00Z])

    assert signal["type"] == "dashboard.changed"
    assert signal["reason"] == "file.uploaded"
    assert signal["occurred_at"] == "2026-10-16T10:00:00Z"
  end

  test "stamps the time when the caller has none" do
    signal = Signals.dashboard_changed("file.deleted")
    assert {:ok, _, _} = DateTime.from_iso8601(signal["occurred_at"])
  end

  test "is JSON-encodable (no structs on the wire)" do
    signal = Signals.dashboard_changed("file.uploaded", ~U[2026-10-16 10:00:00Z])
    assert Jason.decode!(Jason.encode!(signal))["type"] == "dashboard.changed"
  end
end
