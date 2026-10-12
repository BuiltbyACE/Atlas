defmodule AtlasOrchestrator.Auth.JWKS do
  @moduledoc """
  Fetches and caches the public key set Go publishes for realtime tokens
  (docs/ATLAS.md §6.1, §7.5, ADR-008).

  Go is the only issuer. It exposes the public half of its Ed25519 signing key
  at `/internal/v1/.well-known/jwks.json`; the orchestrator verifies tokens
  against that set and never holds a signing key itself.

  The keys are fetched once at boot and refreshed on a timer. A failed refresh
  keeps the previous set, so a brief Go restart does not break live sockets.
  Verification calls never block on the network after the first successful
  fetch; only an empty cache triggers a synchronous fetch.
  """

  use GenServer

  require Logger

  @default_refresh_ms 300_000
  @request_timeout_ms 5_000

  @doc "Starts the key cache. `:url` is the JWKS endpoint."
  @spec start_link(keyword()) :: GenServer.on_start()
  def start_link(opts), do: GenServer.start_link(__MODULE__, opts, name: __MODULE__)

  @doc """
  Returns the current keys as `JOSE.JWK` structs.

  Fetches synchronously when the cache is empty, so a socket connecting before
  the first timer tick still gets a chance to verify.
  """
  @spec keys() :: [JOSE.JWK.t()]
  def keys, do: GenServer.call(__MODULE__, :keys)

  @doc "Forces a refresh; useful after a token arrives with an unknown `kid`."
  @spec refresh() :: :ok
  def refresh, do: GenServer.cast(__MODULE__, :refresh)

  @impl true
  def init(opts) do
    state = %{
      url: Keyword.fetch!(opts, :url),
      refresh_ms: Keyword.get(opts, :refresh_ms, @default_refresh_ms),
      keys: []
    }

    {:ok, state, {:continue, :fetch}}
  end

  @impl true
  def handle_continue(:fetch, state) do
    state = refresh_state(state)
    Process.send_after(self(), :refresh, state.refresh_ms)
    {:noreply, state}
  end

  @impl true
  def handle_call(:keys, _from, %{keys: []} = state) do
    state = refresh_state(state)
    {:reply, state.keys, state}
  end

  def handle_call(:keys, _from, state), do: {:reply, state.keys, state}

  @impl true
  def handle_cast(:refresh, state), do: {:noreply, refresh_state(state)}

  @impl true
  def handle_info(:refresh, state) do
    state = refresh_state(state)
    Process.send_after(self(), :refresh, state.refresh_ms)
    {:noreply, state}
  end

  defp refresh_state(state) do
    case fetch(state.url) do
      {:ok, keys} when keys != [] ->
        %{state | keys: keys}

      {:ok, []} ->
        Logger.warning("JWKS document contained no keys", url: state.url)
        state

      {:error, reason} ->
        Logger.warning("JWKS refresh failed", url: state.url, reason: inspect(reason))
        state
    end
  end

  defp fetch(url) do
    request = {String.to_charlist(url), []}
    http_options = [timeout: @request_timeout_ms, connect_timeout: @request_timeout_ms]

    case :httpc.request(:get, request, http_options, body_format: :binary) do
      {:ok, {{_version, 200, _reason}, _headers, body}} ->
        parse_jwks(body)

      {:ok, {{_version, status, _reason}, _headers, _body}} ->
        {:error, {:status, status}}

      {:error, reason} ->
        {:error, reason}
    end
  end

  @doc """
  Parses a JWKS document into `JOSE.JWK` structs. Exposed so the verifier can
  share one parser between fetched and statically configured keys.
  """
  @spec parse_jwks(String.t()) :: {:ok, [JOSE.JWK.t()]} | {:error, atom()}
  def parse_jwks(json) when is_binary(json) do
    case Jason.decode(json) do
      {:ok, %{"keys" => keys}} when is_list(keys) ->
        {:ok, Enum.map(keys, &JOSE.JWK.from_map/1)}

      _ ->
        {:error, :invalid_jwks}
    end
  end
end
