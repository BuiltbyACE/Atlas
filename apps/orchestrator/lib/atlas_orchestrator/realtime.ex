defmodule AtlasOrchestrator.Realtime do
  @moduledoc """
  Publishing realtime signals to channel topics (docs/ATLAS.md §7.5, ADR-009).

  Signals are small "something changed" nudges; clients respond by re-fetching
  from Go. This module is the single place the wire shape is built, so producers
  never depend on a running Endpoint process.
  """
  alias Phoenix.Channel.Server, as: ChannelServer

  @pubsub AtlasOrchestrator.PubSub

  @doc """
  Broadcasts `payload` as `event` on `topic` to every subscribed channel.

  This is exactly what `Endpoint.broadcast/3` calls (`Phoenix.Channel.Server.broadcast/4`),
  which passes `Phoenix.Channel.Server` as the PubSub **dispatcher**. We call it
  directly instead of `Endpoint.broadcast/3` so tests work without a running
  Endpoint supervisor.

  Do NOT use a bare `Phoenix.PubSub.broadcast/3` with a `%Phoenix.Socket.Broadcast{}`
  here: the default dispatcher (`Phoenix.PubSub`) delivers the message to the
  channel *process*, which then calls `handle_out/3` and crashes. The channel
  dispatcher instead uses each subscriber's `:fastlane` transport pid, pushing
  the encoded message straight to the client connection.
  """
  def broadcast(topic, event, payload) when is_binary(topic) and is_binary(event) do
    ChannelServer.broadcast(@pubsub, topic, event, payload)
  end
end
