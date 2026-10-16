defmodule AtlasOrchestrator.AuthzTest do
  # The decision cache is a named process, so these tests must not run
  # concurrently with each other. `cache_ms` is overridden per test where a
  # cache is under test (the default is 0 in config/test.exs).
  use ExUnit.Case, async: false

  alias AtlasOrchestrator.Authz

  @user "01999999-9999-7999-9999-999999999991"
  @permission "work.task.read"

  test "permits when Go says yes" do
    start_supervised!({Authz, client: fn _uid, _perm -> {:ok, true} end})

    assert Authz.permitted?(@user, @permission) == true
  end

  test "denies when Go says no" do
    start_supervised!({Authz, client: fn _uid, _perm -> {:ok, false} end})

    assert Authz.permitted?(@user, @permission) == false
  end

  test "fails closed when the client errors, and does not cache the fault" do
    {:ok, counter} = Agent.start_link(fn -> 0 end)

    client = fn _uid, _perm ->
      Agent.update(counter, &(&1 + 1))
      {:error, :timeout}
    end

    start_supervised!({Authz, client: client, cache_ms: 60_000})

    refute Authz.permitted?(@user, @permission)
    refute Authz.permitted?(@user, @permission)

    # A fault is not a decision: both calls reached the client.
    assert Agent.get(counter, & &1) == 2
  end

  test "caches a decision for cache_ms" do
    {:ok, counter} = Agent.start_link(fn -> 0 end)

    client = fn _uid, _perm ->
      Agent.update(counter, &(&1 + 1))
      {:ok, true}
    end

    start_supervised!({Authz, client: client, cache_ms: 60_000})

    assert Authz.permitted?(@user, @permission)
    assert Authz.permitted?(@user, @permission)
    assert Agent.get(counter, & &1) == 1
  end

  test "fails closed when the cache is not running" do
    refute Authz.permitted?(@user, @permission)
  end

  test "rejects blank ids without calling the client" do
    {:ok, counter} = Agent.start_link(fn -> 0 end)

    client = fn _uid, _perm ->
      Agent.update(counter, &(&1 + 1))
      {:ok, true}
    end

    start_supervised!({Authz, client: client})

    refute Authz.permitted?("", @permission)
    refute Authz.permitted?(@user, "")

    assert Agent.get(counter, & &1) == 0
  end
end
