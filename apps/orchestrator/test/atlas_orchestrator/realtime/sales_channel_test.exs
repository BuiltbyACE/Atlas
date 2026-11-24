defmodule AtlasOrchestrator.Realtime.SalesChannelTest do
  # Starting the shared Authz cache means these tests must not run concurrently
  # with each other.
  use ExUnit.Case, async: false

  alias AtlasOrchestrator.Authz
  alias AtlasOrchestrator.Realtime.SalesChannel
  alias AtlasOrchestrator.Realtime.UserSocket
  alias Phoenix.Socket

  @user "01999999-9999-7999-9999-999999999992"
  @permission "crm.lead.read"

  defp socket, do: %Socket{assigns: %{user_id: @user}}

  test "the socket routes the sales topic to this channel" do
    assert {SalesChannel, _opts} = UserSocket.__channel__("crm:sales")
  end

  test "join succeeds when Go grants the read permission" do
    start_supervised!({Authz, client: fn uid, perm -> {:ok, uid == @user and perm == @permission} end})

    assert {:ok, %Socket{}} = SalesChannel.join("crm:sales", %{}, socket())
  end

  test "join is refused when Go denies" do
    start_supervised!({Authz, client: fn _uid, _perm -> {:ok, false} end})

    assert {:error, %{reason: "forbidden"}} = SalesChannel.join("crm:sales", %{}, socket())
  end

  test "join fails closed when the cache is not running" do
    assert {:error, %{reason: "forbidden"}} = SalesChannel.join("crm:sales", %{}, socket())
  end

  test "ping is answered" do
    assert {:reply, {:ok, %{pong: true}}, %Socket{}} =
             SalesChannel.handle_in("ping", %{}, socket())
  end
end
