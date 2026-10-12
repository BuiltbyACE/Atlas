defmodule AtlasOrchestrator.Router do
  @moduledoc """
  The orchestrator's small HTTP surface.

  Deliberately minimal: health for operators. Nothing here is business API —
  that is Go's job (ADR-002). The Phase 0 hello demo page was removed once the
  event spine was proven; the orchestrator now exposes only operational probes.
  """
  use Plug.Router

  alias AtlasOrchestrator.Repo
  alias Ecto.Adapters.SQL

  plug :match
  plug :dispatch

  get "/health" do
    send_json(conn, 200, %{status: "up", service: "atlas-orchestrator"})
  end

  get "/health/ready" do
    case readiness() do
      :ok -> send_json(conn, 200, %{status: "up"})
      {:error, reason} -> send_json(conn, 503, %{status: "down", detail: inspect(reason)})
    end
  end

  match _ do
    send_resp(conn, 404, "not found")
  end

  defp readiness do
    SQL.query(Repo, "SELECT 1", [])
    :ok
  rescue
    e -> {:error, e}
  end

  defp send_json(conn, status, body) do
    conn
    |> put_resp_content_type("application/json")
    |> send_resp(status, Jason.encode!(body))
  end
end
