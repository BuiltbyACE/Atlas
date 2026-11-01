defmodule AtlasOrchestrator.Intel.Handler do
  @moduledoc """
  Handles intel events from the outbox (§7.4) and pushes realtime signals
  (§7.5, ADR-009).

  Go owns the decision, risk, brief and metric snapshot lifecycle; this handler
  only nudges the views that changed:

    * `intel:board` — the intel board (decisions, risks, briefs), for anyone
      who holds `intel.decision.read`. Joining is authorised by Go's internal
      join check (`AtlasOrchestrator.Realtime.IntelChannel` →
      `AtlasOrchestrator.Authz`).
    * `exec:dashboard` — a `dashboard.changed` tile invalidation on every event,
      because a new decision, risk, delivered brief or metric snapshot moves
      the intel tiles.

  Only create/update/decide/deliver events are emitted: a decision that changes
  state nudges the board; an event nobody reacts to is noise.

  Payloads are identifiers and change type only; clients refetch from Go, which
  enforces permissions. Intel events name an actor (`actor_id`) — a person,
  not an account id — so no personal nudge is sent: a socket is addressed by an
  account id (§7.5, §10.1), and Go's notifications engine owns person-level
  delivery (§6.4).
  """

  require Logger

  alias AtlasOrchestrator.Dashboard.Signals, as: DashboardSignals
  alias AtlasOrchestrator.Realtime
  alias AtlasOrchestrator.Intel.Signals

  @board_topic "intel:board"
  @dashboard_topic "exec:dashboard"

  # Event type => {entity, id key}. The keys mirror the Go payloads in
  # internal/outbox/intel.go; the entity name matches Go's aggregate constant
  # (decision, risk, brief, metric).
  @events %{
    "intel.decision.created.v1" => {"decision", "decision_id"},
    "intel.decision.updated.v1" => {"decision", "decision_id"},
    "intel.decision.decided.v1" => {"decision", "decision_id"},
    "intel.risk.created.v1" => {"risk", "risk_id"},
    "intel.risk.updated.v1" => {"risk", "risk_id"},
    "intel.brief.created.v1" => {"brief", "brief_id"},
    "intel.brief.updated.v1" => {"brief", "brief_id"},
    "intel.brief.delivered.v1" => {"brief", "brief_id"},
    "intel.metric_snapshot.created.v1" => {"metric", "snapshot_id"}
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
        Logger.debug("intel event ignored", type: type)
    end

    :ok
  end

  def handle_event(_event), do: :ok

  defp push(topic, event, payload, type) do
    case Realtime.broadcast(topic, event, payload) do
      :ok ->
        :ok

      other ->
        Logger.warning("intel signal not delivered",
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
