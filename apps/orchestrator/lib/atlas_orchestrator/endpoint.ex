defmodule AtlasOrchestrator.Endpoint do
  @moduledoc """
  The orchestrator's HTTP/websocket endpoint (docs/ATLAS.md §7.5).

  It serves no HTML application — only the realtime `/socket` and a couple of
  health routes. The public edge routes `/socket` here through Caddy.
  """
  use Phoenix.Endpoint, otp_app: :atlas_orchestrator

  socket "/socket", AtlasOrchestrator.Realtime.UserSocket,
    websocket: true,
    longpoll: false

  plug Plug.RequestId
  plug Plug.Telemetry, event_prefix: [:phoenix, :endpoint]

  plug Plug.Parsers,
    parsers: [:urlencoded, :json],
    pass: ["*/*"],
    json_decoder: Jason

  plug Plug.MethodOverride
  plug Plug.Head

  # Serve the Phoenix JS client locally so the local demo page needs no CDN.
  plug Plug.Static, at: "/js", from: {:phoenix, "priv/static"}, gzip: false

  plug AtlasOrchestrator.Router
end
