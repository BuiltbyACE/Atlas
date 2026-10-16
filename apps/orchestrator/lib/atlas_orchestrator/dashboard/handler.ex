defmodule AtlasOrchestrator.Dashboard.Handler do
  @moduledoc """
  Invalidates the executive dashboard (§8.8) when a tile's data changes.

  The approvals handler already nudges the dashboard on every approval event,
  because the approvals-in-flight tiles move with each one. This handler covers
  the other Phase 1 producer: a file upload or deletion is written to the audit
  trail and so moves the recent-highlights tile, so on `core.file.uploaded.v1`
  and `core.file.deleted.v1` it pushes a generic `dashboard.changed` signal to
  `exec:dashboard` (ADR-009). Clients refetch the tiles from Go.
  """

  require Logger

  alias AtlasOrchestrator.Dashboard.Signals
  alias AtlasOrchestrator.Realtime

  @dashboard_topic "exec:dashboard"

  @reasons %{
    "core.file.uploaded.v1" => "file.uploaded",
    "core.file.deleted.v1" => "file.deleted"
  }

  @doc "Handles one event. Types it does not know are ignored, not failed."
  @spec handle_event(map()) :: :ok
  def handle_event(%{type: type} = event) do
    case Map.fetch(@reasons, type) do
      {:ok, reason} ->
        signal = Signals.dashboard_changed(reason, Map.get(event, :occurred_at))

        case Realtime.broadcast(@dashboard_topic, signal["type"], signal) do
          :ok ->
            :ok

          other ->
            Logger.warning("dashboard.signal_not_delivered", type: type, result: inspect(other))
            :ok
        end

      :error ->
        :ok
    end
  end

  def handle_event(_event), do: :ok
end
