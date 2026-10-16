defmodule AtlasOrchestrator.Notifications.Handler do
  @moduledoc """
  Handles notification events from the outbox (§7.1) and pushes realtime signals.

  Consumes `core.notification.requested.v1` (emitted by Go in the same
  transaction as the notification rows). On receipt it publishes a
  `notification.changed` signal to `user:{recipient_person_id}` (ADR-009).
  """

  require Logger

  alias AtlasOrchestrator.Notifications.Signals
  alias AtlasOrchestrator.Realtime

  @event_requested "core.notification.requested.v1"

  @doc """
  handle_event/2 is invoked by the outbox consumer's event router.
  """
  def handle_event(event, meta \\ %{})

  def handle_event(%{type: @event_requested, payload: payload} = _event, _meta) do
    with {:ok, recipient_id} <- extract_recipient(payload),
         {:ok, ids} <- extract_notification_ids(payload),
         {:ok, critical} <- extract_critical(payload),
         {:ok, occurred_at} <- extract_occurred_at(payload) do
      topic = "user:#{recipient_id}"
      signal = Signals.notification_changed(recipient_id, ids, critical, occurred_at)

      case Realtime.broadcast(topic, signal["type"], signal) do
        :ok ->
          :ok

        {:error, reason} ->
          Logger.warning("notifications.handler.broadcast_failed",
            topic: topic,
            event_type: @event_requested,
            reason: inspect(reason)
          )

          :ok
      end
    else
      {:error, reason} ->
        Logger.warning("notifications.handler.skipping_event",
          event_type: @event_requested,
          reason: reason,
          payload_keys: payload_keys(payload)
        )

        :ok
    end
  end

  def handle_event(_event, _meta), do: :ok

  defp extract_recipient(%{"recipient_person_id" => id}) when is_binary(id) and id != "" do
    {:ok, id}
  end

  defp extract_recipient(%{recipient_person_id: id}) when is_binary(id) and id != "" do
    {:ok, id}
  end

  defp extract_recipient(_), do: {:error, "missing recipient_person_id"}

  defp extract_notification_ids(%{"notification_ids" => ids}) when is_list(ids) do
    {:ok, ids}
  end

  defp extract_notification_ids(%{notification_ids: ids}) when is_list(ids) do
    {:ok, ids}
  end

  defp extract_notification_ids(_), do: {:ok, []}

  defp extract_critical(%{"critical" => c}) when is_boolean(c), do: {:ok, c}
  defp extract_critical(%{critical: c}) when is_boolean(c), do: {:ok, c}
  defp extract_critical(_), do: {:ok, false}

  defp extract_occurred_at(%{"requested_at" => at}) when is_binary(at), do: {:ok, at}
  defp extract_occurred_at(%{requested_at: at}) when is_binary(at), do: {:ok, at}
  defp extract_occurred_at(%{"occurred_at" => at}) when is_binary(at), do: {:ok, at}
  defp extract_occurred_at(%{occurred_at: at}) when is_binary(at), do: {:ok, at}
  defp extract_occurred_at(_), do: {:ok, nil}

  defp payload_keys(%{} = p) when is_map(p), do: Map.keys(p) |> Enum.map(&to_string/1)
  defp payload_keys(_), do: []
end
