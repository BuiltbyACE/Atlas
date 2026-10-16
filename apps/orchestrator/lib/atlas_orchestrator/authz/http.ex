defmodule AtlasOrchestrator.Authz.HTTP do
  @moduledoc """
  Calls Go's internal join check over HTTP (docs/ATLAS.md §7.5).

  The orchestrator calls `GET /internal/v1/authz/check` as a service, presenting
  the static `ATLAS_SERVICE_TOKEN` minted by `atlas-cli service token`. Go
  resolves the user's roles to effective permissions and answers yes or no; an
  unknown user is a 404, which is a "no" rather than a fault (a socket for a
  user who no longer exists simply cannot join).

  Only a 200 with a boolean `allowed` is a decision. Anything else — a
  transport error, an unexpected status, a malformed body — is `{:error, …}`,
  which `AtlasOrchestrator.Authz` turns into a denial.
  """

  @behaviour AtlasOrchestrator.Authz.Client

  @timeout_ms 5_000

  @impl true
  def check(user_id, permission) do
    with {:ok, base_url} <- configured(:base_url),
         {:ok, token} <- configured(:service_token) do
      url =
        base_url <>
          "/internal/v1/authz/check?" <>
          URI.encode_query(%{"user_id" => user_id, "permission" => permission})

      request = {String.to_charlist(url), [{~c"authorization", String.to_charlist("Bearer " <> token)}]}
      http_options = [timeout: @timeout_ms, connect_timeout: @timeout_ms]

      case :httpc.request(:get, request, http_options, body_format: :binary) do
        {:ok, {{_, 200, _}, _headers, body}} -> decode(body)
        # An unknown user is a decision, not a fault: deny.
        {:ok, {{_, 404, _}, _headers, _body}} -> {:ok, false}
        {:ok, {{_, status, _}, _headers, _body}} -> {:error, {:http_status, status}}
        {:error, reason} -> {:error, reason}
      end
    end
  end

  defp decode(body) do
    case Jason.decode(body) do
      {:ok, %{"allowed" => allowed}} when is_boolean(allowed) -> {:ok, allowed}
      other -> {:error, {:unexpected_body, other}}
    end
  end

  defp configured(key) do
    case internal_config()[key] do
      value when is_binary(value) and value != "" -> {:ok, value}
      _ -> {:error, {:not_configured, key}}
    end
  end

  defp internal_config, do: Application.get_env(:atlas_orchestrator, :internal, [])
end
