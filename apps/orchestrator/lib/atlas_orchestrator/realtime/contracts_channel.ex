defmodule AtlasOrchestrator.Realtime.ContractsChannel do
  @moduledoc """
  The contracts board channel: contract and change-request changes
  (docs/ATLAS.md §7.5, §8.3).

  Joining `contracts:board` requires `contracts.contract.read`. The orchestrator
  does not know roles→permissions, so it asks Go through
  `AtlasOrchestrator.Authz` (ADR-008, §7.5). Payloads are signals, not data;
  clients refetch from Go, which enforces permissions again.
  """

  use Phoenix.Channel

  alias AtlasOrchestrator.Authz

  @permission "contracts.contract.read"

  @impl true
  def join("contracts:board", _payload, socket) do
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
