defmodule AtlasOrchestrator.Realtime.SystemsChannel do
  @moduledoc """
  The systems board channel: incident and register changes (docs/ATLAS.md §7.5,
  §8.6).

  Joining `systems:board` requires `systems.system.read`, the permission the
  systems page reads under. The orchestrator does not know roles→permissions, so
  it asks Go through `AtlasOrchestrator.Authz` (ADR-008, §7.5). Payloads are
  signals, not data; clients refetch from Go, which enforces permissions again.
  """

  use Phoenix.Channel

  alias AtlasOrchestrator.Authz

  @permission "systems.system.read"

  @impl true
  def join("systems:board", _payload, socket) do
    if Authz.permitted?(socket.assigns.user_id, @permission) do
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
