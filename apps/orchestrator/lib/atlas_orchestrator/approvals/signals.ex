defmodule AtlasOrchestrator.Approvals.Signals do
  @moduledoc """
  Builds wire shapes for approval signals pushed over Realtime (ADR-009).

  These are "signals, not data": clients refetch the approval from Go on
  receipt. The shape is intentionally minimal and stable so the frontend can
  rely on it.
  """

  @doc """
  Builds the payload for `approval.changed`.

  `status` is the approval's status after the event (`requested`, `approved`,
  `declined`, `escalated`, `cancelled`, `expired`).
  """
  def approval_changed(approval_id, ref, status, occurred_at \\ nil) do
    %{
      "type" => "approval.changed",
      "approval_id" => approval_id,
      "ref" => ref,
      "status" => status,
      "occurred_at" => iso8601(occurred_at)
    }
  end

  defp iso8601(%DateTime{} = dt), do: DateTime.to_iso8601(dt)
  defp iso8601(value) when is_binary(value), do: value
  defp iso8601(_), do: DateTime.utc_now() |> DateTime.to_iso8601()
end
