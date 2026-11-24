defmodule AtlasOrchestrator.Contracts.Handler do
  @moduledoc """
  Handles contract events from the outbox (§7.4) and pushes realtime signals
  (§7.5, ADR-009).

  Go owns contract state; this handler only nudges the views that changed:

    * `contracts:board` — the contracts board, for anyone who holds
      `contracts.contract.read`. Joining is authorised by Go's internal join
      check (`AtlasOrchestrator.Realtime.ContractsChannel` →
      `AtlasOrchestrator.Authz`).
    * `exec:dashboard` — a `dashboard.changed` tile invalidation on every event,
      because each one writes an audit row and a signed contract moves the
      revenue tiles.

  Payloads are identifiers and change type only; clients refetch from Go, which
  enforces permissions. Contract payloads carry no owner account, so no personal
  nudge is sent.
  """

  require Logger

  alias AtlasOrchestrator.Contracts.Signals
  alias AtlasOrchestrator.Dashboard.Signals, as: DashboardSignals
  alias AtlasOrchestrator.Realtime

  @board_topic "contracts:board"
  @dashboard_topic "exec:dashboard"

  # Event type => {entity, id key}. The keys mirror the Go payloads in
  # internal/outbox/contracts.go.
  @events %{
    "contracts.contract.created.v1" => {"contract", "contract_id"},
    "contracts.contract.status_changed.v1" => {"contract", "contract_id"},
    "contracts.contract.signed.v1" => {"contract", "contract_id"},
    "contracts.change_request.created.v1" => {"change_request", "change_request_id"},
    "contracts.change_request.approved.v1" => {"change_request", "change_request_id"},
    "contracts.change_request.rejected.v1" => {"change_request", "change_request_id"}
  }

  @doc "Handles one event. Types it does not know are ignored, not failed."
  @spec handle_event(map()) :: :ok
  def handle_event(%{type: type, payload: payload} = event) do
    case Map.fetch(@events, type) do
      {:ok, {entity, id_key}} ->
        occurred_at = Map.get(event, :occurred_at)
        signal = Signals.entity_changed(entity, payload_value(payload, id_key), occurred_at)
        push(@board_topic, signal["type"], signal, type)

        dashboard = DashboardSignals.dashboard_changed(type, occurred_at)
        push(@dashboard_topic, dashboard["type"], dashboard, type)

      :error ->
        Logger.debug("contracts event ignored", type: type)
    end

    :ok
  end

  def handle_event(_event), do: :ok

  defp push(topic, event, payload, type) do
    case Realtime.broadcast(topic, event, payload) do
      :ok ->
        :ok

      other ->
        Logger.warning("contracts signal not delivered",
          topic: topic,
          type: type,
          result: inspect(other)
        )

        :ok
    end
  end

  defp payload_value(payload, key) when is_map(payload) do
    Map.get(payload, key) || Map.get(payload, String.to_atom(key))
  end

  defp payload_value(_payload, _key), do: nil
end
