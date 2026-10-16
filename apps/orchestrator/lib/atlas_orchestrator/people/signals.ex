defmodule AtlasOrchestrator.People.Signals do
  @moduledoc """
  Builds wire shapes for people signals pushed over Realtime (§7.5, ADR-009).

  A people signal is "signal, not data": an entity type and its identifier,
  nothing more. The client refetches the record from Go, which enforces
  permissions:

      %{"type" => "leave_request.changed", "id" => "…"}

  Tile invalidation reuses the dashboard's `dashboard.changed` shape
  (`AtlasOrchestrator.Dashboard.Signals`), so the executive dashboard keeps one
  refetch path for every producer.
  """

  @doc """
  Builds `%{type: "<entity>.changed", id: id, occurred_at: iso}`.

  `entity` is the singular entity name (`daily_report`, `leave_request`),
  matching ADR-009's `{type: task.changed, id}` example. People records are not
  project-scoped, so no `project_id` is ever added.
  """
  def entity_changed(entity, id, occurred_at \\ nil) do
    %{
      "type" => "#{entity}.changed",
      "id" => id,
      "occurred_at" => iso8601(occurred_at)
    }
  end

  defp iso8601(%DateTime{} = dt), do: DateTime.to_iso8601(dt)
  defp iso8601(value) when is_binary(value), do: value
  defp iso8601(_), do: DateTime.utc_now() |> DateTime.to_iso8601()
end
