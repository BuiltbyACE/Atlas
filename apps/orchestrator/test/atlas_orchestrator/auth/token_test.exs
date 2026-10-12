defmodule AtlasOrchestrator.Auth.TokenTest do
  # The key source is injected through application config, so these tests must
  # not run concurrently with each other.
  use ExUnit.Case, async: false

  alias AtlasOrchestrator.Auth.TestKeys
  alias AtlasOrchestrator.Auth.Token

  setup do
    keys = TestKeys.generate()
    previous = Application.get_env(:atlas_orchestrator, :realtime)

    Application.put_env(:atlas_orchestrator, :realtime,
      jwks: keys.jwks,
      issuer: "atlas",
      audience: "atlas-realtime"
    )

    on_exit(fn -> Application.put_env(:atlas_orchestrator, :realtime, previous) end)

    {:ok, keys: keys}
  end

  defp claims(overrides) do
    Map.merge(
      %{
        "sub" => "user:0191f0a2-3b4c-7d5e-8f90-1234567890ab",
        "iss" => "atlas",
        "aud" => "atlas-realtime",
        "exp" => System.system_time(:second) + 60
      },
      overrides
    )
  end

  test "verifies an EdDSA token signed with Go's algorithm", %{keys: keys} do
    token = TestKeys.sign(keys.jwk, claims(%{}))

    assert {:ok, verified} = Token.verify(token)
    assert verified["sub"] == "user:0191f0a2-3b4c-7d5e-8f90-1234567890ab"
  end

  test "rejects an expired token", %{keys: keys} do
    token = TestKeys.sign(keys.jwk, claims(%{"exp" => System.system_time(:second) - 10}))

    assert {:error, :token_expired} = Token.verify(token)
  end

  test "rejects a token with the wrong issuer", %{keys: keys} do
    token = TestKeys.sign(keys.jwk, claims(%{"iss" => "somewhere-else"}))

    assert {:error, :bad_issuer} = Token.verify(token)
  end

  test "rejects a token minted for another purpose", %{keys: keys} do
    # An MFA challenge is signed by the same key but has a different audience.
    token = TestKeys.sign(keys.jwk, claims(%{"aud" => "atlas-mfa"}))

    assert {:error, :bad_audience} = Token.verify(token)
  end

  test "rejects a token signed with a different key" do
    other = TestKeys.generate()
    token = TestKeys.sign(other.jwk, claims(%{}))

    assert {:error, :no_key_available} = Token.verify(token)
  end
end
