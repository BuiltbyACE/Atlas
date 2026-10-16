defmodule AtlasOrchestrator.Work.SignalsTest do
  use ExUnit.Case, async: true

  alias AtlasOrchestrator.Work.Signals

  test "entity_changed is the ADR-009 signal shape: type and id only" do
    signal = Signals.entity_changed("task", "01999999-9999-7999-9999-999999999999")

    assert signal["type"] == "task.changed"
    assert signal["id"] == "01999999-9999-7999-9999-999999999999"
    refute Map.has_key?(signal, "project_id")
    assert is_binary(signal["occurred_at"])
  end

  test "entity_changed includes project_id when the record is project-scoped" do
    signal = Signals.entity_changed("task", "t1", "p1", ~U[2026-10-16 10:00:00Z])

    assert signal["project_id"] == "p1"
    assert signal["occurred_at"] == "2026-10-16T10:00:00Z"
  end

  test "entity_changed is JSON-safe (no structs)" do
    signal = Signals.entity_changed("escalation", "e1", nil, DateTime.utc_now())

    assert is_binary(Jason.encode!(signal))
  end
end
