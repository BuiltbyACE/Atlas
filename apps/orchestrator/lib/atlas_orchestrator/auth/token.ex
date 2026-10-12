defmodule AtlasOrchestrator.Auth.Token do
  @moduledoc """
  Verifies realtime tokens presented when connecting to `/socket`
  (docs/ATLAS.md §6.1, §7.5, ADR-008).

  Go is the authority: it signs short-lived (5 minute) Ed25519 tokens and
  publishes the public half as JWKS at `/internal/v1/.well-known/jwks.json`.
  The orchestrator verifies against that set — it never signs a realtime token
  and never holds a private key.

  Two key sources are supported, selected by configuration:

    * `config :atlas_orchestrator, :realtime, jwks: "<json>"` — a static
      document, used by tests and by any environment that injects keys.
    * `config :atlas_orchestrator, :realtime, jwks_url: "..."` — fetched and
      cached by `AtlasOrchestrator.Auth.JWKS`.

  When `issuer` and `audience` are configured they are enforced, so a token
  minted for another purpose (for example an MFA challenge) is rejected here.
  """
  alias AtlasOrchestrator.Auth.JWKS

  @algs ["EdDSA"]

  @doc """
  Verifies a token. Returns `{:ok, claims}` or `{:error, reason}`.
  """
  @spec verify(String.t()) :: {:ok, map()} | {:error, atom()}
  def verify(token) when is_binary(token) do
    with {:ok, keys} <- candidate_keys(),
         {:ok, claims} <- verify_with_keys(token, keys),
         :ok <- check_issuer(claims),
         :ok <- check_audience(claims),
         :ok <- check_expiry(claims) do
      {:ok, claims}
    end
  end

  defp verify_with_keys(_token, []), do: {:error, :no_key_available}

  defp verify_with_keys(token, [key | rest]) do
    case JOSE.JWT.verify_strict(key, @algs, token) do
      {true, %JOSE.JWT{fields: claims}, _jws} -> {:ok, claims}
      _ -> verify_with_keys(token, rest)
    end
  end

  defp candidate_keys do
    conf = realtime_config()

    case conf[:jwks] do
      jwks when is_binary(jwks) -> {:ok, parse_jwks(jwks)}
      _ -> fetched_keys()
    end
  end

  defp fetched_keys do
    if Process.whereis(JWKS) do
      {:ok, JWKS.keys()}
    else
      {:error, :no_key_source_configured}
    end
  end

  defp parse_jwks(json) do
    case JWKS.parse_jwks(json) do
      {:ok, keys} -> keys
      _ -> []
    end
  end

  defp check_issuer(claims) do
    case realtime_config()[:issuer] do
      nil -> :ok
      expected -> if claims["iss"] == expected, do: :ok, else: {:error, :bad_issuer}
    end
  end

  defp check_audience(claims) do
    case realtime_config()[:audience] do
      nil -> :ok
      expected -> if audience_matches?(claims["aud"], expected), do: :ok, else: {:error, :bad_audience}
    end
  end

  defp audience_matches?(nil, _expected), do: false
  defp audience_matches?(aud, expected) when is_binary(aud), do: aud == expected
  defp audience_matches?(auds, expected) when is_list(auds), do: expected in auds

  defp check_expiry(claims) do
    case claims["exp"] do
      nil -> :ok
      exp when is_integer(exp) -> if exp > now(), do: :ok, else: {:error, :token_expired}
      _ -> {:error, :token_expired}
    end
  end

  defp now, do: System.system_time(:second)

  defp realtime_config, do: Application.get_env(:atlas_orchestrator, :realtime, [])
end
