defmodule AtlasOrchestrator.Finance.Signals do
  @moduledoc """
  Builds wire shapes for finance signals pushed over Realtime (§7.5, ADR-009).

  A finance signal is "signal, not data": an entity type and its identifier,
  nothing more. The client refetches the record from Go, which enforces
  permissions:

      %{"type" => "expense.changed", "id" => "…"}

  Tile invalidation reuses the dashboard's `dashboard.changed` shape
  (`AtlasOrchestrator.Dashboard.Signals`).
  """

  @doc "Builds `%{type: \"<entity>.changed\", id: id, occurred_at: iso}`."
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
