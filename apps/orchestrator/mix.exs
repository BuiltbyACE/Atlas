defmodule AtlasOrchestrator.MixProject do
  use Mix.Project

  @version "0.1.0"

  def project do
    [
      app: :atlas_orchestrator,
      version: @version,
      elixir: "~> 1.17",
      elixirc_paths: elixirc_paths(Mix.env()),
      start_permanent: Mix.env() == :prod,
      aliases: aliases(),
      deps: deps(),
      dialyzer: [plt_add_apps: [:mix]]
    ]
  end

  # Atlas's orchestrator is a headless service (docs/ATLAS.md §5.2, ADR-006):
  # it consumes Go's outbox, runs workflows and pushes realtime signals. It has
  # no HTML and no LiveView, so those dependencies are deliberately absent.
  def application do
    [
      mod: {AtlasOrchestrator.Application, []},
      # :inets/:ssl back the JWKS fetch from Go (ADR-008).
      extra_applications: [:logger, :runtime_tools, :inets, :ssl]
    ]
  end

  defp elixirc_paths(:test), do: ["lib", "test/support"]
  defp elixirc_paths(_), do: ["lib"]

  defp deps do
    [
      # HTTP + websockets (channels) for the /socket endpoint (§7.5).
      {:phoenix, "~> 1.7"},
      {:phoenix_pubsub, "~> 2.1"},
      {:bandit, "~> 1.5"},

      # Ecto against the `orchestrator` schema + read-only access to `outbox`.
      {:ecto_sql, "~> 3.12"},
      {:postgrex, "~> 0.19"},

      # Durable jobs (§5.4, ADR-006).
      {:oban, "~> 2.18"},

      # JSON and JWT verification for realtime tokens (ADR-008).
      {:jason, "~> 1.4"},
      {:jose, "~> 1.11"},

      # Telemetry/metrics scaffolding (§14).
      {:telemetry_metrics, "~> 1.0"},
      {:telemetry_poller, "~> 1.1"},

      # Dev/test tooling (Credo is run by `task orch:lint`).
      {:credo, "~> 1.7", only: [:dev, :test], runtime: false}
    ]
  end

  defp aliases do
    [
      # The Ecto bookkeeping table lives in the orchestrator schema too, so the
      # orchestrator role never needs write access to `public`.
      setup: ["deps.get", "ecto.migrate --prefix orchestrator"],
      test: ["ecto.migrate --quiet --prefix orchestrator", "test"]
    ]
  end
end
