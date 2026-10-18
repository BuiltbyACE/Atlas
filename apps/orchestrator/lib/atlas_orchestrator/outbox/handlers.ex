defmodule AtlasOrchestrator.Outbox.Handlers do
  @moduledoc """
  Dispatches a consumed event to the code that reacts to it.

  The router is a thin map from event type to handler:

    * `core.system.hello.v1` — the Phase 0 smoke event (Appendix A);
    * `core.notification.requested.v1` — push `notification.changed` to the
      recipient's channel;
    * `core.approval.*.v1` — push `approval.changed` to the dashboard and any
      named approver/escalation target;
    * `core.file.*.v1` — push `dashboard.changed` to the dashboard, because a
      file upload or archive moves the recent-highlights tile;
    * `core.notification.failed.v1` — push `dashboard.changed` to the dashboard,
      so an exhausted delivery attempt is visible rather than only in the audit
      trail;
    * `work.*.v1` and `core.escalation.*.v1` — push `<entity>.changed` to the
      project board and any named person, plus `dashboard.changed` to the
      dashboard (task/board and overdue-work tiles);
    * `people.*.v1` — push `<entity>.changed` to any named person plus
      `dashboard.changed` to the dashboard (daily-report and leave tiles);
    * `integrations.git.pull_request.merged.v1` — a merged pull request the
      integration linked to a task (`work` entity): the same project board and
      dashboard nudges as `work.evidence.added.v1`.

  Reacting means *signalling*, never writing business data (ADR-009): Go remains
  the authority, the orchestrator nudges.
  """
  require Logger

  alias AtlasOrchestrator.Approvals.Handler, as: ApprovalsHandler
  alias AtlasOrchestrator.Dashboard.Handler, as: DashboardHandler
  alias AtlasOrchestrator.Notifications.Handler, as: NotificationsHandler
  alias AtlasOrchestrator.Outbox.Event
  alias AtlasOrchestrator.People.Handler, as: PeopleHandler
  alias AtlasOrchestrator.Work.Handler, as: WorkHandler

  @doc "Handles one event. Unknown types are logged and ignored, not failed."
  @spec dispatch(Event.t()) :: :ok
  def dispatch(%Event{type: "core.system.hello.v1"} = event), do: handle_hello(event)

  def dispatch(%Event{type: "core.notification.requested.v1"} = event) do
    NotificationsHandler.handle_event(event)
  end

  # A failed send is written to the audit trail and is an operational signal, so
  # it nudges the dashboard's tile invalidation like a file event (D6-adjacent,
  # §6.4). The recipient's account id is not on the event, so there is no
  # per-user push.
  def dispatch(%Event{type: "core.notification.failed.v1"} = event) do
    DashboardHandler.handle_event(event)
  end

  def dispatch(%Event{type: "core.approval." <> _} = event) do
    ApprovalsHandler.handle_event(event)
  end

  def dispatch(%Event{type: "core.file." <> _} = event) do
    DashboardHandler.handle_event(event)
  end

  def dispatch(%Event{type: "work." <> _} = event) do
    WorkHandler.handle_event(event)
  end

  # The Go integration emits this when it links a merged PR to a task (D6,
  # ADR-018). Go's payload mirrors work.go (`evidence_id`, `project_id`), so the
  # work handler nudges the same project board and dashboard topics.
  def dispatch(%Event{type: "integrations.git.pull_request.merged.v1"} = event) do
    WorkHandler.handle_event(event)
  end

  def dispatch(%Event{type: "people." <> _} = event) do
    PeopleHandler.handle_event(event)
  end

  def dispatch(%Event{type: "core.escalation." <> _} = event) do
    WorkHandler.handle_event(event)
  end

  def dispatch(%Event{type: type}) do
    Logger.debug("no handler registered", type: type)
    :ok
  end

  defp handle_hello(%Event{} = event) do
    message = get_in(event.payload, ["message"]) || "hello"

    signal = %{
      type: event.type,
      event_id: event.id,
      seq: event.seq,
      message: message,
      # Wire-safe payload: the channel JSON-encodes this and would crash on a raw
      # %DateTime{} (Jason has no encoder for it), so serialise here.
      occurred_at: DateTime.to_iso8601(event.occurred_at)
    }

    Enum.each(topics_for(event), fn topic ->
      AtlasOrchestrator.Realtime.broadcast(topic, "signal", signal)
      Logger.info("hello broadcast", topic: topic, seq: event.seq)
    end)

    :ok
  end

  # No actor (the Phase 0 dev event) → push to the demo topic the local demo
  # page joins, plus the executive dashboard topic (§7.5).
  defp topics_for(%Event{actor_id: nil}) do
    demo = Application.get_env(:atlas_orchestrator, :demo_topic, "user:demo")
    [demo, "exec:dashboard"]
  end

  defp topics_for(%Event{actor_id: actor_id}) do
    ["user:#{actor_id}", "exec:dashboard"]
  end
end
