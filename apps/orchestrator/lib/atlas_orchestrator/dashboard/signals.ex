defmodule AtlasOrchestrator.Dashboard.Signals do
  @moduledoc """
  Builds wire shapes for executive-dashboard signals pushed over Realtime (ADR-009).

  A dashboard signal is a "something changed" nudge: clients refetch the tiles
  from Go on receipt. The shape is intentionally minimal and stable so the
  frontend can rely on it.
  """

  @doc """
  Builds the payload for `dashboard.changed`.

  `reason` names the producer whose change moved a tile (`file.uploaded`,
  `file.deleted`); it is for logging and debugging, not part of the data.
  """
  def dashboard_changed(reason, occurred_at \\ nil) do
    %{
      "type" => "dashboard.changed",
      "reason" => reason,
      "occurred_at" => iso8601(occurred_at)
    }
  end

  defp iso8601(%DateTime{} = dt), do: DateTime.to_iso8601(dt)
  defp iso8601(value) when is_binary(value), do: value
  defp iso8601(_), do: DateTime.utc_now() |> DateTime.to_iso8601()
end
