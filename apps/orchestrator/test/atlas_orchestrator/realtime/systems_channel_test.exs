defmodule AtlasOrchestrator.Realtime.SystemsChannelTest do
  # Starting the shared Authz cache means these tests must not run concurrently
  # with each other.
  use ExUnit.Case, async: false

  alias AtlasOrchestrator.Authz
  alias AtlasOrchestrator.Realtime.SystemsChannel
  alias AtlasOrchestrator.Realtime.UserSocket
  alias Phoenix.Socket

  @user "01999999-9999-7999-9999-999999999993"
  @permission "systems.system.read"

  defp socket, do: %Socket{assigns: %{user_id: @user}}

  test "the socket routes the systems topic to this channel" do
    assert {SystemsChannel, _opts} = UserSocket.__channel__("systems:board")
  end

  test "join succeeds when Go grants the read permission" do
    start_supervised!({Authz, client: fn uid, perm -> {:ok, uid == @user and perm == @permission} end})

    assert {:ok, %Socket{}} = SystemsChannel.join("systems:board", %{}, socket())
  end

  test "join is refused when Go denies" do
    start_supervised!({Authz, client: fn _uid, _perm -> {:ok, false} end})

    assert {:error, %{reason: "forbidden"}} = SystemsChannel.join("systems:board", %{}, socket())
  end

  test "join fails closed when the cache is not running" do
    assert {:error, %{reason: "forbidden"}} = SystemsChannel.join("systems:board", %{}, socket())
  end

  test "ping is answered" do
    assert {:reply, {:ok, %{pong: true}}, %Socket{}} =
             SystemsChannel.handle_in("ping", %{}, socket())
  end
end
