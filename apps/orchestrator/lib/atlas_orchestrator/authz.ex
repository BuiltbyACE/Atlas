defmodule AtlasOrchestrator.Authz do
  @moduledoc """
  Answers "may this user join this topic?" by asking Go (docs/ATLAS.md §7.5).

  A socket joins a project topic only after Go — the authority on
  roles→permissions (ADR-008) — confirms the user holds the topic's read
  permission. The orchestrator never learns the mapping itself, so a permission
  change in Go takes effect here without a deploy.

  Decisions are cached briefly (`:cache_ms`, default 30 seconds) so a burst of
  joins does not become a burst of HTTP calls. The cache holds *decisions*, not
  faults: a transport error is never cached, so the next join retries
  immediately. Everything fails closed — if the check cannot be answered, the
  join is refused.

  The Go call itself lives in a `client` (default `AtlasOrchestrator.Authz.HTTP`,
  injected as a stub in tests); see `AtlasOrchestrator.Authz.Client`.
  """

  use GenServer

  require Logger

  alias AtlasOrchestrator.Authz.HTTP

  @default_cache_ms 30_000

  @doc "Starts the decision cache. `:client` and `:cache_ms` are optional."
  @spec start_link(keyword()) :: GenServer.on_start()
  def start_link(opts), do: GenServer.start_link(__MODULE__, opts, name: __MODULE__)

  @doc """
  True when `user_id` holds `permission`.

  Fails closed: if the cache process is not running (or any fault occurs) the
  answer is `false`, never a crash and never an accidental `true`. Callers treat
  the result as the join decision.
  """
  @spec permitted?(String.t(), String.t()) :: boolean()
  def permitted?(user_id, permission)
      when is_binary(user_id) and user_id != "" and is_binary(permission) and permission != "" do
    GenServer.call(__MODULE__, {:permitted?, user_id, permission})
  catch
    :exit, _ -> false
  end

  def permitted?(_user_id, _permission), do: false

  @impl true
  def init(opts) do
    state = %{
      client: Keyword.get(opts, :client, HTTP),
      cache_ms: Keyword.get(opts, :cache_ms, @default_cache_ms),
      cache: %{}
    }

    {:ok, state}
  end

  @impl true
  def handle_call({:permitted?, user_id, permission}, _from, state) do
    key = {user_id, permission}
    now = now_ms()

    case fresh(state.cache[key], now) do
      {:ok, decision} ->
        {:reply, decision, state}

      :stale ->
        {decision, cache} = decide(state, key, user_id, permission, now)
        {:reply, decision, %{state | cache: cache}}
    end
  end

  defp fresh({decision, expires_at}, now) when expires_at > now, do: {:ok, decision}
  defp fresh(_entry, _now), do: :stale

  defp decide(state, key, user_id, permission, now) do
    cache = prune(state.cache, now)

    case invoke(state.client, user_id, permission) do
      {:ok, decision} when is_boolean(decision) ->
        Logger.debug("authz decision",
          user_id: user_id,
          permission: permission,
          allowed: decision
        )

        {decision, Map.put(cache, key, {decision, now + state.cache_ms})}

      other ->
        Logger.warning("authz check failed; denying join",
          user_id: user_id,
          permission: permission,
          reason: inspect(other)
        )

        {false, cache}
    end
  end

  defp invoke(client, user_id, permission) when is_function(client, 2),
    do: client.(user_id, permission)

  defp invoke(client, user_id, permission), do: client.check(user_id, permission)

  # Drop expired decisions so an idle cache does not grow without bound.
  defp prune(cache, now) do
    Map.reject(cache, fn {_key, {_decision, expires_at}} -> expires_at <= now end)
  end

  defp now_ms, do: System.monotonic_time(:millisecond)
end
