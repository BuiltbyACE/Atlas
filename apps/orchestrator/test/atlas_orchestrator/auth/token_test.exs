defmodule AtlasOrchestrator.Auth.TokenTest do
  use ExUnit.Case, async: true

  alias AtlasOrchestrator.Auth.Token

  test "verifies a token it signed" do
    token = Token.sign(%{"sub" => "u1", "exp" => System.system_time(:second) + 60})

    assert {:ok, claims} = Token.verify(token)
    assert claims["sub"] == "u1"
  end

  test "rejects an expired token" do
    token = Token.sign(%{"sub" => "u1", "exp" => System.system_time(:second) - 10})

    assert {:error, :token_expired} = Token.verify(token)
  end

  test "rejects a token signed with a different secret" do
    other = JOSE.JWK.from_oct("a-secret-that-is-not-atlas")

    {_meta, token} =
      JOSE.JWT.sign(other, %{"alg" => "HS256"}, %{"sub" => "x"}) |> JOSE.JWS.compact()

    assert {:error, :no_key_available} = Token.verify(token)
  end
end
