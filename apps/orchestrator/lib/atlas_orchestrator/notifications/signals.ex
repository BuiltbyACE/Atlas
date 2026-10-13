defmodule AtlasOrchestrator.Notifications.Signals do
  @moduledoc """
  Builds wire shapes for notification signals pushed over Realtime (ADR-009).

  These are "signals, not data": clients refetch the full notification from Go
  on receipt. The shapes are intentionally minimal and stable so the frontend
  can rely on them.
  """

  @doc """
  Builds the payload for `notification.changed` sent to `user:{recipient_id}`.
  """
  def notification_changed(recipient_id, notification_ids \\ [], critical \\ false, occurred_at \\ nil) do
    count = length(notification_ids)

    %{
      "type" => "notification.changed",
      "recipient_id" => to_string(recipient_id),
      "count_hint" => count,
      "critical" => critical,
      "occurred_at" => format_time(occurred_at || DateTime.utc_now())
    }
  end

  defp format_time(%DateTime{} = dt), do: DateTime.to_iso8601(dt)
  defp format_time(other) when is_binary(other), do: other
  defp format_time(_), do: DateTime.to_iso8601(DateTime.utc_now())
end
