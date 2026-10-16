defmodule AtlasOrchestrator.Work.Handler do
  @moduledoc """
  Handles work and escalation events from the outbox (§7.4) and pushes realtime
  signals (§7.5, ADR-009).

  Go owns work state; this handler only nudges the views that changed:

    * `work:project:{id}` — task/board changes, for project members and CEOs
      (§7.5). Joining the channel is authorised by Go's internal join check
      (`AtlasOrchestrator.Realtime.ProjectChannel` → `AtlasOrchestrator.Authz`),
      so the orchestrator never resolves roles→permissions itself.
    * `user:{id}` — a *named* person the event is about (an assignee who just got
      work, or an owner an escalation landed on).
    * `exec:dashboard` — a `dashboard.changed` tile invalidation on every event,
      because each one writes an audit row and can move the overdue-work tile.
      Reusing the dashboard's own signal keeps the tile refetch path uniform.

  Payloads are identifiers and change type only; clients refetch from Go, which
  enforces permissions. Person-level nudges for people the event does not name
  (e.g. who must approve an absence) are deliberately absent: resolving a role to
  people is Go's authority (§3.2), so the orchestrator never does it.
  """

  require Logger

  alias AtlasOrchestrator.Dashboard.Signals, as: DashboardSignals
  alias AtlasOrchestrator.Realtime
  alias AtlasOrchestrator.Work.Signals

  @dashboard_topic "exec:dashboard"

  # Event type => {entity, id key, project key, person key}. The keys mirror the
  # Go payloads in internal/outbox/work.go and escalation.go; a nil project key
  # means the record is not project-scoped, a nil person key means no individual
  # is nudged. `core.escalation.*` is produced by the same work engine.
  @events %{
    "work.project.created.v1" => {"project", "project_id", nil, "owner_person_id"},
    "work.project.status_changed.v1" => {"project", "project_id", nil, nil},
    "work.task.created.v1" => {"task", "task_id", "project_id", "assignee_person_id"},
    "work.task.status_changed.v1" => {"task", "task_id", "project_id", nil},
    "work.task.assigned.v1" => {"task", "task_id", "project_id", "assignee_person_id"},
    "work.task.blocked.v1" => {"task", "task_id", "project_id", nil},
    "work.task.unblocked.v1" => {"task", "task_id", "project_id", nil},
    "work.task.overdue.v1" => {"task", "task_id", "project_id", "assignee_person_id"},
    "work.evidence.added.v1" => {"evidence", "evidence_id", "project_id", nil},
    "core.escalation.raised.v1" => {"escalation", "escalation_id", "project_id", "assignee_person_id"},
    "core.escalation.resolved.v1" => {"escalation", "escalation_id", "project_id", "assignee_person_id"}
  }

  # uuid.UUID is an array, so Go cannot omit an unset one: it arrives as the nil
  # UUID rather than a missing key.
  @nil_uuid "00000000-0000-0000-0000-000000000000"

  @doc "Handles one event. Types it does not know are ignored, not failed."
  @spec handle_event(map()) :: :ok
  def handle_event(%{type: type, payload: payload} = event) do
    case Map.fetch(@events, type) do
      {:ok, {entity, id_key, project_key, person_key}} ->
        occurred_at = Map.get(event, :occurred_at)
        project_id = project_key && uuid(payload, project_key)
        person_id = person_key && uuid(payload, person_key)

        signal =
          Signals.entity_changed(entity, payload_value(payload, id_key), project_id, occurred_at)

        Enum.each(entity_topics(project_id, person_id), &push(&1, signal["type"], signal, type))

        dashboard = DashboardSignals.dashboard_changed(type, occurred_at)
        push(@dashboard_topic, dashboard["type"], dashboard, type)

      :error ->
        Logger.debug("work event ignored", type: type)
    end

    :ok
  end

  def handle_event(_event), do: :ok

  defp push(topic, event, payload, type) do
    case Realtime.broadcast(topic, event, payload) do
      :ok ->
        :ok

      other ->
        Logger.warning("work signal not delivered",
          topic: topic,
          type: type,
          result: inspect(other)
        )

        :ok
    end
  end

  # Project members watch the board; a named person is nudged directly.
  defp entity_topics(project_id, person_id) do
    [
      project_id && "work:project:#{project_id}",
      person_id && "user:#{person_id}"
    ]
    |> Enum.reject(&is_nil/1)
  end

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
