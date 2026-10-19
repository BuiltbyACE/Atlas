defmodule AtlasOrchestrator.Crm.Signals do
  @moduledoc """
  Builds wire shapes for CRM signals pushed over Realtime (§7.5, ADR-009).

  A CRM signal is "signal, not data": an entity type and its identifier, nothing
  more. The client refetches the record from Go, which enforces permissions:

      %{"type" => "lead.changed", "id" => "…"}

  Tile invalidation reuses the dashboard's `dashboard.changed` shape
  (`AtlasOrchestrator.Dashboard.Signals`), so the executive dashboard keeps one
  refetch path for every producer.
  """

  @doc """
  Builds `%{type: "<entity>.changed", id: id, occurred_at: iso}`.

  `entity` is the singular entity name (`lead`, `opportunity`, `followup`, …),
  matching ADR-009's `{type: task.changed, id}` example.
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
