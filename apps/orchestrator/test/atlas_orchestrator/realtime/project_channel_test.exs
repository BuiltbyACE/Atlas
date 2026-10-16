defmodule AtlasOrchestrator.Realtime.ProjectChannelTest do
  # Starting the shared Authz cache means these tests must not run concurrently
  # with each other.
  use ExUnit.Case, async: false

  alias AtlasOrchestrator.Authz
  alias AtlasOrchestrator.Realtime.DashboardChannel
  alias AtlasOrchestrator.Realtime.ProjectChannel
  alias AtlasOrchestrator.Realtime.UserChannel
  alias AtlasOrchestrator.Realtime.UserSocket
  alias Phoenix.Socket

  @project "01999999-9999-7999-9999-999999999991"
  @user "01999999-9999-7999-9999-999999999992"
  @permission "work.task.read"

  defp socket, do: %Socket{assigns: %{user_id: @user}}

  test "the socket routes project topics to this channel" do
    assert {ProjectChannel, _opts} = UserSocket.__channel__("work:project:#{@project}")
    assert {UserChannel, _opts} = UserSocket.__channel__("user:#{@user}")
    assert {DashboardChannel, _opts} = UserSocket.__channel__("exec:dashboard")
  end

  test "join succeeds when Go grants the read permission" do
    start_supervised!({Authz, client: fn uid, perm -> {:ok, uid == @user and perm == @permission} end})

    assert {:ok, %Socket{}} = ProjectChannel.join("work:project:#{@project}", %{}, socket())
  end

  test "join is refused when Go denies" do
    start_supervised!({Authz, client: fn _uid, _perm -> {:ok, false} end})

    assert {:error, %{reason: "forbidden"}} =
             ProjectChannel.join("work:project:#{@project}", %{}, socket())
  end

  test "join fails closed when the cache is not running" do
    assert {:error, %{reason: "forbidden"}} =
             ProjectChannel.join("work:project:#{@project}", %{}, socket())
  end

  test "an empty project id is refused" do
    start_supervised!({Authz, client: fn _uid, _perm -> {:ok, true} end})

    assert {:error, %{reason: "forbidden"}} = ProjectChannel.join("work:project:", %{}, socket())
  end

  test "ping is answered" do
    assert {:reply, {:ok, %{pong: true}}, %Socket{}} =
             ProjectChannel.handle_in("ping", %{}, socket())
  end
end
