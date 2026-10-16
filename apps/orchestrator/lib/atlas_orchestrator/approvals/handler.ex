defmodule AtlasOrchestrator.Approvals.Handler do
  @moduledoc """
  Handles approval events from the outbox (§7.4) and pushes realtime signals.

  The Go engine owns approval state; this handler only nudges the views that
  changed (ADR-009):

    * the executive dashboard, on every approval event — the approvals-in-flight
      counts move with each one;
    * a *named* approver, when an approval is waiting on them;
    * a *named* escalation target, when an approval was escalated to them.

  Person-level nudges for the requester and for role-based approvers are not
  duplicated here: the Go engine already fans those out through the
  notifications engine (§6.4), which this same consumer turns into
  `notification.changed` on the recipient's channel. Resolving a role to people
  is Go's authority (§3.2), so the orchestrator never does it. Clients refetch
  the approval from Go on receipt.
  """

  require Logger

  alias AtlasOrchestrator.Approvals.Signals
  alias AtlasOrchestrator.Realtime

  @dashboard_topic "exec:dashboard"

  # The approval status implied by each event type, mirroring the Go constants
  # in internal/outbox/approval.go.
  @status_by_type %{
    "core.approval.requested.v1" => "requested",
    "core.approval.approved.v1" => "approved",
    "core.approval.declined.v1" => "declined",
    "core.approval.escalated.v1" => "escalated",
    "core.approval.cancelled.v1" => "cancelled",
    "core.approval.expired.v1" => "expired"
  }

  # A UUID that Go serialises for an absent optional column. uuid.UUID is an
  # array, so `omitempty` never drops it and unset fields arrive as the nil UUID.
  @nil_uuid "00000000-0000-0000-0000-000000000000"

  @doc """
  handle_event/1 is invoked by the outbox consumer's event router.
  """
  @spec handle_event(map()) :: :ok
  def handle_event(%{type: type, payload: payload} = event) do
    case Map.fetch(@status_by_type, type) do
      {:ok, status} ->
        signal =
          Signals.approval_changed(
            payload_value(payload, "approval_id"),
            payload_value(payload, "ref"),
            status,
            Map.get(event, :occurred_at)
          )

        type
        |> topics(payload)
        |> Enum.each(&push(&1, signal, type))

      :error ->
        Logger.debug("approval event ignored", type: type)
    end

    :ok
  end

  defp push(topic, signal, type) do
    case Realtime.broadcast(topic, signal["type"], signal) do
      :ok ->
        :ok

      other ->
        Logger.warning("approval signal not delivered",
          topic: topic,
          type: type,
          result: inspect(other)
        )

        :ok
    end
  end

  # The named approver cares that work is waiting; escalated approvals go to the
  # named target. Everything else is an aggregate concern (the dashboard) — the
  # requester is nudged via the notifications engine.
  defp topics("core.approval.requested.v1", payload),
    do: with_dashboard(uuid(payload, "approver_user_id"))

  defp topics("core.approval.escalated.v1", payload),
    do: with_dashboard(uuid(payload, "to_user_id"))

  defp topics(_type, _payload), do: [@dashboard_topic]

  defp with_dashboard(nil), do: [@dashboard_topic]
  defp with_dashboard(user_id), do: ["user:" <> user_id, @dashboard_topic]

  defp uuid(payload, key) do
    case payload_value(payload, key) do
      value when is_binary(value) and value != "" and value != @nil_uuid -> value
      _ -> nil
    end
  end

  defp payload_value(payload, key) when is_map(payload) do
    Map.get(payload, key) || Map.get(payload, String.to_atom(key))
  end

  defp payload_value(_payload, _key), do: nil
end
