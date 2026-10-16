defmodule AtlasOrchestrator.Work.Signals do
  @moduledoc """
  Builds wire shapes for work signals pushed over Realtime (§7.5, ADR-009).

  A work signal is "signal, not data": an entity type and its identifier, nothing
  more. The client refetches the record from Go, which enforces permissions:

      %{"type" => "task.changed", "id" => "…", "project_id" => "…"}

  Tile invalidation reuses the dashboard's `dashboard.changed` shape
  (`AtlasOrchestrator.Dashboard.Signals`), so the executive dashboard keeps one
  refetch path for every producer.
  """

  @doc """
  Builds `%{type: "<entity>.changed", id: id, project_id: project_id, occurred_at: iso}`.

  `entity` is the singular entity name (`task`, `project`, `escalation`, …),
  matching ADR-009's `{type: task.changed, id}` example. `project_id` is omitted
  when the record is not project-scoped.
  """
  def entity_changed(entity, id, project_id \\ nil, occurred_at \\ nil) do
    signal = %{
      "type" => "#{entity}.changed",
      "id" => id,
      "occurred_at" => iso8601(occurred_at)
    }

    if project_id, do: Map.put(signal, "project_id", project_id), else: signal
  end

  defp iso8601(%DateTime{} = dt), do: DateTime.to_iso8601(dt)
  defp iso8601(value) when is_binary(value), do: value
  defp iso8601(_), do: DateTime.utc_now() |> DateTime.to_iso8601()
end
