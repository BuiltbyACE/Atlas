defmodule AtlasOrchestrator.Realtime.SalesChannel do
  @moduledoc """
  The commercial board channel: CRM lead, opportunity and proposal changes
  (docs/ATLAS.md §7.5, §8.1).

  Joining `crm:sales` requires `crm.lead.read`. The orchestrator does not know
  roles→permissions, so it asks Go through `AtlasOrchestrator.Authz` — the same
  internal join check the project and dashboard channels use (ADR-008, §7.5).
  Payloads are signals, not data; clients refetch from Go, which enforces
  permissions again.
  """

  use Phoenix.Channel

  alias AtlasOrchestrator.Authz

  @permission "crm.lead.read"

  @impl true
  def join("crm:sales", _payload, socket) do
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
