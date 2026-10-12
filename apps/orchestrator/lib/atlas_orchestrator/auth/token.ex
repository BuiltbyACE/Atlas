defmodule AtlasOrchestrator.Auth.Token do
  @moduledoc """
  Verifies realtime tokens presented when connecting to `/socket`
  (docs/ATLAS.md §7.5, ADR-008).

  Tokens carry the user id and a compact list of scopes. Go is the authority:
  in staging and production the token is signed by Go and verified against Go's
  JWKS. Local development uses a shared HS256 secret instead, because the auth
  service those JWKS come from does not exist until Phase 1.

  Two key sources are supported, selected by configuration:

    * `config :atlas_orchestrator, :realtime, secret: "..."` — HS256 (local dev)
    * `config :atlas_orchestrator, :realtime, jwks: "<json>"`  — JWKS document

  Fetching the JWKS over HTTP from Go is wired in Phase 1 alongside the token
  issuer; the verification logic below is the same either way.
  """
  @algs_hmac ["HS256"]
  @algs_asymmetric ["RS256", "RS384", "RS512", "ES256", "ES384", "EdDSA"]

  @doc """
  Verifies a token. Returns `{:ok, claims}` or `{:error, reason}`.
  """
  @spec verify(String.t()) :: {:ok, map()} | {:error, atom()}
  def verify(token) when is_binary(token) do
    with {:ok, keys} <- candidate_keys(),
         {:ok, claims} <- verify_with_keys(token, keys) do
      check_expiry(claims)
    end
  end

  @doc """
  Signs a token with the local HS256 secret. Local development only — it raises
  if no shared secret is configured, which is exactly the production case.
  """
  @spec sign(map()) :: String.t()
  def sign(claims) when is_map(claims) do
    secret =
      realtime_config()[:secret] ||
        raise "no :realtime secret configured; cannot sign tokens outside local development"

    jwk = JOSE.JWK.from_oct(secret)
    {_meta, token} = JOSE.JWT.sign(jwk, %{"alg" => "HS256"}, claims) |> JOSE.JWS.compact()
    token
  end

  defp verify_with_keys(_token, []), do: {:error, :no_key_available}

  defp verify_with_keys(token, [key | rest]) do
    case JOSE.JWT.verify_strict(key, @algs_hmac ++ @algs_asymmetric, token) do
      {true, %JOSE.JWT{fields: claims}, _jws} -> {:ok, claims}
      _ -> verify_with_keys(token, rest)
    end
  end

  defp check_expiry(claims) do
    now = System.system_time(:second)

    case claims["exp"] do
      nil -> {:ok, claims}
      exp when is_integer(exp) and exp > now -> {:ok, claims}
      _ -> {:error, :token_expired}
    end
  end

  defp candidate_keys do
    conf = realtime_config()

    cond do
      secret = conf[:secret] ->
        {:ok, [JOSE.JWK.from_oct(secret)]}

      jwks = conf[:jwks] ->
        {:ok, parse_jwks(jwks)}

      true ->
        {:error, :no_key_source_configured}
    end
  end

  defp parse_jwks(json) when is_binary(json) do
    case Jason.decode(json) do
      {:ok, %{"keys" => keys}} -> Enum.map(keys, &JOSE.JWK.from_map/1)
      _ -> []
    end
  end

  defp realtime_config, do: Application.get_env(:atlas_orchestrator, :realtime, [])
end
