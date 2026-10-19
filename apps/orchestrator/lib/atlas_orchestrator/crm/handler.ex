defmodule AtlasOrchestrator.Crm.Handler do
  @moduledoc """
  Handles CRM events from the outbox (§7.4) and pushes realtime signals (§7.5,
  ADR-009).

  Go owns CRM state; this handler only nudges the views that changed:

    * `crm:sales` — the commercial board (leads, opportunities, proposals), for
      the team that holds `crm.lead.read`. Joining is authorised by Go's internal
      join check (`AtlasOrchestrator.Realtime.SalesChannel` →
      `AtlasOrchestrator.Authz`), so the orchestrator never resolves
      roles→permissions itself.
    * `user:{id}` — a *named* owner (a lead/opportunity/follow-up owner) on their
      own account channel, which is what a socket is addressed by (§7.5, §10.1).
    * `exec:dashboard` — a `dashboard.changed` tile invalidation on every event,
      because each one writes an audit row and can move the commercial tiles.

  Payloads are identifiers and change type only; clients refetch from Go, which
  enforces permissions.
  """

  require Logger

  alias AtlasOrchestrator.Crm.Signals
  alias AtlasOrchestrator.Dashboard.Signals, as: DashboardSignals
  alias AtlasOrchestrator.Realtime

  @sales_topic "crm:sales"
  @dashboard_topic "exec:dashboard"

  # Event type => {entity, id key, user key}. The keys mirror the Go payloads in
  # internal/outbox/crm.go; a nil user key means no individual is nudged. The
  # nudge target is the owner's *account* id (`owner_user_id`), which is what a
  # socket is addressed by (§7.5, §10.1).
  @events %{
    "crm.lead.created.v1" => {"lead", "lead_id", "owner_user_id"},
    "crm.lead.assigned.v1" => {"lead", "lead_id", "owner_user_id"},
    "crm.lead.status_changed.v1" => {"lead", "lead_id", nil},
    "crm.lead.converted.v1" => {"lead", "lead_id", nil},
    "crm.opportunity.created.v1" => {"opportunity", "opportunity_id", "owner_user_id"},
    "crm.opportunity.stage_changed.v1" => {"opportunity", "opportunity_id", nil},
    "crm.opportunity.won.v1" => {"opportunity", "opportunity_id", nil},
    "crm.opportunity.lost.v1" => {"opportunity", "opportunity_id", nil},
    "crm.interaction.recorded.v1" => {"interaction", "interaction_id", "owner_user_id"},
    "crm.followup.scheduled.v1" => {"followup", "followup_id", "owner_user_id"},
    "crm.followup.completed.v1" => {"followup", "followup_id", "owner_user_id"},
    "crm.followup.missed.v1" => {"followup", "followup_id", "owner_user_id"},
    "crm.proposal.created.v1" => {"proposal", "proposal_id", "owner_user_id"},
    "crm.proposal.sent.v1" => {"proposal", "proposal_id", "owner_user_id"}
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

        Enum.each(entity_topics(user_id), &push(&1, signal["type"], signal, type))

        dashboard = DashboardSignals.dashboard_changed(type, occurred_at)
        push(@dashboard_topic, dashboard["type"], dashboard, type)

      :error ->
        Logger.debug("crm event ignored", type: type)
    end

    :ok
  end

  def handle_event(_event), do: :ok

  defp push(topic, event, payload, type) do
    case Realtime.broadcast(topic, event, payload) do
      :ok ->
        :ok

      other ->
        Logger.warning("crm signal not delivered",
          topic: topic,
          type: type,
          result: inspect(other)
        )

        :ok
    end
  end

  defp entity_topics(user_id) do
    [@sales_topic, user_id && "user:#{user_id}"]
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
