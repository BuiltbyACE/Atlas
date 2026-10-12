defmodule AtlasOrchestrator.Realtime.UserSocket do
  @moduledoc """
  The realtime socket (docs/ATLAS.md §7.5).

  A connection is accepted only with a valid token; the token's `sub` becomes
  the socket's user id, and channels may only join topics that identity is
  allowed to see (ADR-008).
  """
  use Phoenix.Socket

  alias AtlasOrchestrator.Auth.Token

  # Realtime is signals-only (ADR-009): these channels carry small "something
  # changed" payloads, never business data.
  channel "user:*", AtlasOrchestrator.Realtime.UserChannel
  channel "exec:dashboard", AtlasOrchestrator.Realtime.DashboardChannel

  @impl true
  def connect(%{"token" => token}, socket, _connect_info) do
    case Token.verify(token) do
      {:ok, claims} ->
        case user_id_from(claims) do
          nil -> :error
          user_id -> {:ok, assign(socket, :user_id, user_id) |> assign(:claims, claims)}
        end

      {:error, reason} ->
        require Logger
        Logger.debug("socket rejected", reason: reason)
        :error
    end
  end

  def connect(_params, _socket, _connect_info), do: :error

  @impl true
  def id(socket), do: "user_socket:#{socket.assigns.user_id}"

  # Go sets `sub` to "user:<uuid>"; the socket and channels key on the bare id.
  defp user_id_from(claims) do
    case claims["sub"] || claims["user_id"] do
      "user:" <> id -> id
      id -> id
    end
  end
end
