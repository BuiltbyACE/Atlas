defmodule AtlasOrchestrator.Realtime.UserChannel do
  @moduledoc """
  A user's private channel: notifications, approvals waiting, escalations
  (docs/ATLAS.md §7.5). Only the user whose id matches the topic may join.
  """
  use Phoenix.Channel

  @impl true
  def join("user:" <> user_id, _payload, socket) do
    if user_id == socket.assigns.user_id do
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
