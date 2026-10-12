defmodule AtlasOrchestrator.DataCase do
  @moduledoc """
  Case template for tests that touch the database.

  Checks out an Ecto SQL sandbox owner so each test runs against a transaction
  that is rolled back afterwards.
  """
  use ExUnit.CaseTemplate

  alias AtlasOrchestrator.Repo
  alias Ecto.Adapters.SQL.Sandbox

  using do
    quote do
      alias AtlasOrchestrator.Repo

      import Ecto.Query
      import AtlasOrchestrator.DataCase
    end
  end

  setup tags do
    pid = Sandbox.start_owner!(Repo, shared: not tags[:async])
    on_exit(fn -> Sandbox.stop_owner(pid) end)
    :ok
  end
end
