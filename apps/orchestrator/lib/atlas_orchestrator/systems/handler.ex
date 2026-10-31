defmodule AtlasOrchestrator.Systems.Handler do
  @moduledoc """
  Handles systems events from the outbox (§7.4) and pushes realtime signals
  (§7.5, ADR-009).

  Go owns the register and the incident lifecycle; this handler only nudges the
  views that changed:

    * `systems:board` — the systems board (incidents and the register), for
      anyone who holds `systems.system.read`. Joining is authorised by Go's
      internal join check (`AtlasOrchestrator.Realtime.SystemsChannel` →
      `AtlasOrchestrator.Authz`).
    * `exec:dashboard` — a `dashboard.changed` tile invalidation on every event,
      because an incident moves the open-incident, MTTA/MTTR and availability
      tiles.

  Only incident events are emitted: registering a system or editing a check has
  no consumer outside the module, and an event nobody reacts to is noise.

  Payloads are identifiers and change type only; clients refetch from Go, which
  enforces permissions. Systems events name a commander (`commander_person_id`)
  and an actor (`actor_id`) — persons, not account ids — so no personal nudge is
  sent: a socket is addressed by an account id (§7.5, §10.1), and Go's
  notifications engine owns person-level delivery (§6.4).
  """

  require Logger

  alias AtlasOrchestrator.Dashboard.Signals, as: DashboardSignals
  alias AtlasOrchestrator.Realtime
  alias AtlasOrchestrator.Systems.Signals

  @board_topic "systems:board"
  @dashboard_topic "exec:dashboard"

  # Event type => {entity, id key}. The keys mirror the Go payloads in
  # internal/outbox/systems.go; the entity name matches Go's aggregate constant
  # (incident). The other keys those payloads carry (`severity`, `status`,
  # `system_ids`, `review_required`, …) are deliberately not read: a signal is an
  # id and a change type, never data.
  @events %{
    "systems.incident.opened.v1" => {"incident", "incident_id"},
    "systems.incident.acknowledged.v1" => {"incident", "incident_id"},
    "systems.incident.mitigated.v1" => {"incident", "incident_id"},
    "systems.incident.resolved.v1" => {"incident", "incident_id"},
    "systems.incident.closed.v1" => {"incident", "incident_id"},
    "systems.incident.review_recorded.v1" => {"incident", "incident_id"}
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
        Logger.debug("systems event ignored", type: type)
    end

    :ok
  end

  def handle_event(_event), do: :ok

  defp push(topic, event, payload, type) do
    case Realtime.broadcast(topic, event, payload) do
      :ok ->
        :ok

      other ->
        Logger.warning("systems signal not delivered",
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
