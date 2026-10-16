defmodule AtlasOrchestrator.Application do
  @moduledoc """
  The orchestrator's supervision tree (docs/ATLAS.md §5.2, ADR-006).

  Start order matters:

    1. `Repo`            — everything downstream needs the database.
    2. `Phoenix.PubSub`  — realtime fan-out to channels.
    3. `Oban`            — durable jobs (needs the Repo).
    4. `Endpoint`        — the `/socket` surface.
    5. `Outbox.Consumer` — reads Go's outbox and broadcasts signals.
    6. `Outbox.Listener` — wakes the consumer on `NOTIFY`.

  In `:test` only the Repo and PubSub start: the consumer and listener would
  otherwise fight the Ecto sandbox, and unit tests exercise their pieces
  directly. The live path is proven by running the service (see the README).
  """
  use Application

  @impl true
  def start(_type, _args) do
    children = base_children() ++ runtime_children()

    opts = [strategy: :one_for_one, name: AtlasOrchestrator.Supervisor]
    Supervisor.start_link(children, opts)
  end

  @impl true
  def config_change(changed, _new, removed) do
    AtlasOrchestrator.Endpoint.config_change(changed, removed)
    :ok
  end

  defp base_children do
    [
      AtlasOrchestrator.Repo,
      {Phoenix.PubSub, name: AtlasOrchestrator.PubSub}
    ]
  end

  defp runtime_children do
    if Application.get_env(:atlas_orchestrator, :env, :dev) == :test do
      []
    else
      jwks_children() ++
        authz_children() ++
        [
          {Oban, Application.fetch_env!(:atlas_orchestrator, Oban)},
          AtlasOrchestrator.Endpoint,
          AtlasOrchestrator.Outbox.Consumer,
          AtlasOrchestrator.Outbox.Listener
        ]
    end
  end

  # The JWKS cache is started only when Go's endpoint is configured; without it
  # realtime verification has no key source, so sockets would be rejected and
  # there is no point holding an empty cache.
  defp jwks_children do
    case Application.get_env(:atlas_orchestrator, :realtime, [])[:jwks_url] do
      url when is_binary(url) and url != "" ->
        [{AtlasOrchestrator.Auth.JWKS, url: url}]

      _ ->
        []
    end
  end

  # The join-check cache is started only when Go's internal API is configured.
  # Without it every join fails closed (which is safe, but silently so), so a
  # missing URL is the one case worth not starting a process for.
  defp authz_children do
    case Application.get_env(:atlas_orchestrator, :internal, [])[:base_url] do
      url when is_binary(url) and url != "" -> [AtlasOrchestrator.Authz]
      _ -> []
    end
  end
end
