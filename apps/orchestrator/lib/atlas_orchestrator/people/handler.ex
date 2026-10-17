defmodule AtlasOrchestrator.People.Handler do
  @moduledoc """
  Handles people events from the outbox (§7.4) and pushes realtime signals
  (§7.5, ADR-009).

  Go owns people state; this handler only nudges the views that changed:

    * `user:{id}` — a *named* person the event is about (the requester whose
      leave was approved, declined or cancelled).
    * `exec:dashboard` — a `dashboard.changed` tile invalidation on every event,
      because each one writes an audit row and can move the daily-report
      compliance tile. Reusing the dashboard's own signal keeps the tile refetch
      path uniform.

  Payloads are identifiers and change type only; clients refetch from Go, which
  enforces permissions. Nudges for people the event does not name (e.g. who must
  decide a leave request) are deliberately absent: resolving a role to people is
  Go's authority (§3.2), so the orchestrator never does it.
  """

  require Logger

  alias AtlasOrchestrator.Dashboard.Signals, as: DashboardSignals
  alias AtlasOrchestrator.People.Signals
  alias AtlasOrchestrator.Realtime

  @dashboard_topic "exec:dashboard"

  # Event type => {entity, id key, user key}. The keys mirror the Go payloads in
  # internal/outbox/people.go; a nil user key means no individual is nudged. The
  # nudge target is the person's account id (`*_user_id`), which is what a
  # socket is addressed by (§7.5, §10.1).
  @events %{
    "people.daily_report.due.v1" => {"daily_report", "report_id", nil},
    "people.daily_report.submitted.v1" => {"daily_report", "report_id", nil},
    "people.daily_report.missed.v1" => {"daily_report", "report_id", nil},
    "people.leave.requested.v1" => {"leave_request", "leave_request_id", nil},
    "people.leave.approved.v1" => {"leave_request", "leave_request_id", "person_user_id"},
    "people.leave.declined.v1" => {"leave_request", "leave_request_id", "person_user_id"},
    "people.leave.cancelled.v1" => {"leave_request", "leave_request_id", "person_user_id"},
    "people.person.offboarded.v1" => {"person", "person_id", nil}
  }

  # uuid.UUID is an array, so Go cannot omit an unset one: it arrives as the nil
  # UUID rather than a missing key.
  @nil_uuid "00000000-0000-0000-0000-000000000000"

  @doc "Handles one event. Types it does not know are ignored, not failed."
  @spec handle_event(map()) :: :ok
  def handle_event(%{type: type, payload: payload} = event) do
    case Map.fetch(@events, type) do
      {:ok, {entity, id_key, user_key}} ->
        occurred_at = Map.get(event, :occurred_at)
        user_id = user_key && uuid(payload, user_key)

        signal = Signals.entity_changed(entity, payload_value(payload, id_key), occurred_at)

        Enum.each(user_topics(user_id), &push(&1, signal["type"], signal, type))

        dashboard = DashboardSignals.dashboard_changed(type, occurred_at)
        push(@dashboard_topic, dashboard["type"], dashboard, type)

      :error ->
        :ok
    end

    :ok
  end

  def handle_event(_event), do: :ok

  defp push(topic, event, payload, type) do
    case Realtime.broadcast(topic, event, payload) do
      :ok ->
        :ok

      other ->
        Logger.warning("people signal not delivered",
          topic: topic,
          type: type,
          result: inspect(other)
        )

        :ok
    end
  end

  # A named person is nudged on their own account channel (§10.1).
  defp user_topics(nil), do: []
  defp user_topics(user_id), do: ["user:#{user_id}"]

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
