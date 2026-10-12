defmodule AtlasOrchestrator.Outbox.Listener do
  @moduledoc """
  Holds a dedicated PostgreSQL connection that `LISTEN`s on `atlas_outbox` and
  wakes the consumer when Go inserts an event (docs/ATLAS.md §7.1).

  The notification is only a hint — the consumer still reads the row — so it is
  safe for this to be lossy. The poll loop covers anything missed.
  """
  use GenServer

  require Logger

  alias AtlasOrchestrator.Outbox.Consumer

  def start_link(opts) do
    GenServer.start_link(__MODULE__, opts, name: __MODULE__)
  end

  @impl true
  def init(opts) do
    channel = Keyword.get(opts, :channel, notify_channel())

    case Postgrex.Notifications.start_link(connection_opts()) do
      {:ok, notifier} ->
        case Postgrex.Notifications.listen(notifier, channel) do
          {:ok, _ref} ->
            Logger.info("outbox listener subscribed", topic: channel)
            {:ok, %{notifier: notifier, channel: channel}}

          {:error, reason} ->
            {:stop, reason}
        end

      {:error, reason} ->
        {:stop, reason}
    end
  end

  @impl true
  def handle_info({:notification, _conn, _ref, _channel, payload}, state) do
    Consumer.wake(payload)
    {:noreply, state}
  end

  def handle_info(_message, state), do: {:noreply, state}

  defp notify_channel do
    conf = Application.get_env(:atlas_orchestrator, AtlasOrchestrator.Outbox, [])
    conf[:notify_channel] || "atlas_outbox"
  end

  # LISTEN needs its own long-lived connection, separate from the pool, so we
  # derive connection details from the same URL the Repo uses.
  defp connection_opts do
    url = System.get_env("ORCHESTRATOR_DATABASE_URL") || System.get_env("DATABASE_URL")
    uri = URI.parse(url)
    {user, password} = split_userinfo(uri.userinfo)

    [
      hostname: uri.host,
      port: uri.port || 5432,
      username: user,
      password: password,
      database: String.trim_leading(uri.path || "", "/")
    ]
  end

  # %URI{} has :userinfo ("user:pass"), not :user/:password.
  defp split_userinfo(nil), do: {nil, nil}

  defp split_userinfo(userinfo) do
    case String.split(userinfo, ":", parts: 2) do
      [user, password] -> {URI.decode(user), URI.decode(password)}
      [user] -> {URI.decode(user), nil}
    end
  end
end
