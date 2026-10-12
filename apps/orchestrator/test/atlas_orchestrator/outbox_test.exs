defmodule AtlasOrchestrator.OutboxTest do
  use AtlasOrchestrator.DataCase, async: false

  alias AtlasOrchestrator.Outbox
  alias AtlasOrchestrator.Outbox.Event

  test "the cursor advances and persists" do
    # The live outbox consumer owns this table too (tests share the local DB),
    # so never assume a starting value; always work from the current cursor.
    start = Outbox.current_cursor()

    assert :ok = Outbox.advance_cursor(start + 1000)
    assert Outbox.current_cursor() == start + 1000

    assert :ok = Outbox.advance_cursor(start + 7)
    assert Outbox.current_cursor() == start + 7
  end

  test "fetch_after returns an ordered list of outbox events" do
    events = Outbox.fetch_after(0, 50)

    assert is_list(events)
    assert events == Enum.sort_by(events, & &1.seq)
    assert Enum.all?(events, &match?(%Event{}, &1))
  end

  test "mark_processed is idempotent by event id" do
    event = %Event{id: Ecto.UUID.generate(), type: "core.system.hello.v1", seq: 999_999}

    assert :ok = Outbox.mark_processed(event)
    assert :duplicate = Outbox.mark_processed(event)
  end
end
