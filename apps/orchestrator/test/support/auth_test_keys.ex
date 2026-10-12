defmodule AtlasOrchestrator.Auth.TestKeys do
  @moduledoc """
  Ed25519 key material for realtime-token tests.

  Go is the only real issuer; these helpers reproduce its wire format locally so
  the verifier can be exercised without an HTTP fetch or a running Go process.
  """

  @doc """
  Generates an Ed25519 key pair. Returns:

    * `:jwk`  — the private JWK, for signing test tokens
    * `:jwks` — a JWKS JSON document containing only the public half
  """
  @spec generate() :: %{jwk: JOSE.JWK.t(), jwks: String.t()}
  def generate do
    jwk = JOSE.JWK.generate_key({:okp, :Ed25519})
    {_meta, map} = JOSE.JWK.to_map(jwk)

    public = Map.take(map, ["kty", "crv", "x", "kid"])

    %{jwk: jwk, jwks: Jason.encode!(%{"keys" => [public]})}
  end

  @doc "Signs claims with EdDSA, mirroring what Go emits."
  @spec sign(JOSE.JWK.t(), map()) :: String.t()
  def sign(jwk, claims) do
    {_meta, token} =
      JOSE.JWT.sign(jwk, %{"alg" => "EdDSA"}, claims)
      |> JOSE.JWS.compact()

    token
  end
end
