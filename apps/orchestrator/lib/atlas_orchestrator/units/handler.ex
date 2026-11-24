defmodule AtlasOrchestrator.Units.Handler do
  @moduledoc """
  Handles units events from the outbox (§7.4) and pushes realtime signals
  (§7.5, ADR-009).

  Go owns the unit and integration lifecycle; this handler only nudges the
  views that changed:

    * `units:board` — the units board (units, integrations, indicators), for
      anyone who holds `units.unit.read`. Joining is authorised by Go's
      internal join check (`AtlasOrchestrator.Realtime.UnitsChannel` →
      `AtlasOrchestrator.Authz`).
    * `exec:dashboard` — a `dashboard.changed` tile invalidation on every event,
      because a new unit, integration or indicator moves the unit tiles.

  Only create/update events are emitted: a unit or integration that changes
  shape nudges the board; an event nobody reacts to is noise.

  Payloads are identifiers and change type only; clients refetch from Go, which
  enforces permissions. Units events name an actor (`actor_id`) — a person,
  not an account id — so no personal nudge is sent: a socket is addressed by an
  account id (§7.5, §10.1), and Go's notifications engine owns person-level
  delivery (§6.4).
  """

  require Logger

  alias AtlasOrchestrator.Dashboard.Signals, as: DashboardSignals
  alias AtlasOrchestrator.Realtime
  alias AtlasOrchestrator.Units.Signals

  @board_topic "units:board"
  @dashboard_topic "exec:dashboard"

  # Event type => {entity, id key}. The keys mirror the Go payloads in
  # internal/outbox/units.go; the entity name matches Go's aggregate constant
  # (unit, integration, indicator).
  @events %{
    "units.unit.created.v1" => {"unit", "unit_id"},
    "units.unit.updated.v1" => {"unit", "unit_id"},
    "units.integration.created.v1" => {"integration", "integration_id"},
    "units.integration.updated.v1" => {"integration", "integration_id"},
    "units.indicator.created.v1" => {"indicator", "indicator_id"},
    "units.indicator.updated.v1" => {"indicator", "indicator_id"}
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
        Logger.debug("units event ignored", type: type)
    end

    :ok
  end

  def handle_event(_event), do: :ok

  defp push(topic, event, payload, type) do
    case Realtime.broadcast(topic, event, payload) do
      :ok ->
        :ok

      other ->
        Logger.warning("units signal not delivered",
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
