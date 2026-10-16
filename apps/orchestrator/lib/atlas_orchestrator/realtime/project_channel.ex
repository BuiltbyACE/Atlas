defmodule AtlasOrchestrator.Realtime.ProjectChannel do
  @moduledoc """
  A project's channel: task and board changes (docs/ATLAS.md §7.5).

  Joining `work:project:{id}` requires the topic's read permission
  (`work.task.read`). The orchestrator does not know roles→permissions or who
  belongs to a project, so it asks Go through `AtlasOrchestrator.Authz` — the
  same internal join check the dashboard uses (ADR-008, §7.5). Payloads are
  signals, not data; clients refetch from Go, which enforces permissions again.
  """

  use Phoenix.Channel

  alias AtlasOrchestrator.Authz

  # The permission a project signal requires. This is the REST read visibility
  # (§8.3), not literal project membership: a signal carries ids, not data
  # (ADR-009). Narrowing it to project members is a deferred refinement
  # (docs/PHASE2.md §9).
  @permission "work.task.read"

  @impl true
  def join("work:project:" <> project_id, _payload, socket) do
    if project_id != "" and Authz.permitted?(socket.assigns.user_id, @permission) do
      {:ok, socket}
    else
      {:error, %{reason: "forbidden"}}
    end
  end

  @impl true
  def handle_in("ping", _payload, socket) do
    {:reply, {:ok, %{pong: true}}, socket}
  end
end
