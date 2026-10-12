defmodule AtlasOrchestrator.Realtime.DashboardChannel do
  @moduledoc """
  The executive dashboard channel: metric tile invalidations, new decisions,
  incidents (docs/ATLAS.md §7.5).

  Phase 0 joins it without an additional scope check because there are no roles
  yet; when roles land the scope list carried by the token is enforced here.
  """
  use Phoenix.Channel

  @impl true
  def join("exec:dashboard", _payload, socket) do
    {:ok, socket}
  end
end
