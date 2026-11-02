defmodule AtlasOrchestrator.Release do
  @moduledoc """
  Operational tasks that need the compiled release rather than a Mix shell.

  The runtime image ships a release only — no `mix`, no source tree — so Ecto
  migrations cannot be run the usual way. They are invoked through `eval`:

      /app/bin/atlas_orchestrator eval "AtlasOrchestrator.Release.migrate()"

  Railway runs exactly that as this service's pre-deploy command, so the
  `orchestrator` schema is brought up to date on every deploy before the new
  release starts. `Ecto.Migrator` only applies versions that are not yet
  recorded, so running it on every deploy is safe.

  Migrations live under the `orchestrator` prefix because that is the Repo's
  `migration_default_prefix`; this module deliberately does not pass a prefix of
  its own, so the schema cannot drift from the app's configuration.
  """

  @app :atlas_orchestrator

  @doc """
  Apply all pending migrations to every configured repo.
  """
  def migrate do
    load_app()

    for repo <- repos() do
      {:ok, _, _} =
        Ecto.Migrator.with_repo(repo, fn repo ->
          Ecto.Migrator.run(repo, :up, all: true)
        end)
    end

    :ok
  end

  @doc """
  Roll a repo back to `version`. Kept for operators; not used by deploys.
  """
  def rollback(repo, version) when is_atom(repo) and is_integer(version) do
    load_app()

    {:ok, _, _} =
      Ecto.Migrator.with_repo(repo, fn repo ->
        Ecto.Migrator.run(repo, :down, to: version)
      end)

    :ok
  end

  defp repos, do: Application.fetch_env!(@app, :ecto_repos)

  # The release boots without starting the application, so its config has to be
  # loaded explicitly before anything reads :ecto_repos.
  defp load_app, do: Application.load(@app)
end
