defmodule AtlasOrchestrator.DevPage do
  @moduledoc """
  A local-only page that proves the Phase 0 acceptance path end to end
  (Appendix A): Go writes `core.system.hello.v1` → the orchestrator consumes it
  → the browser receives a live signal.

  Open <http://localhost:4000/dev/hello>, then in another terminal:

      curl -X POST http://localhost:8080/api/v1/dev/hello -d '{"message":"hi"}'

  The page joins `user:demo`, which is where the hello handler broadcasts events
  that have no actor. Nothing here is reachable outside local development.
  """

  alias AtlasOrchestrator.Auth.Token

  @doc "Returns the demo page HTML with a freshly-signed local token embedded."
  def html do
    token = Token.sign(%{"sub" => "demo", "exp" => System.system_time(:second) + 3_600})

    """
    <!doctype html>
    <html lang="en">
      <head>
        <meta charset="utf-8" />
        <meta name="viewport" content="width=device-width, initial-scale=1" />
        <title>Atlas · Phase 0 hello demo</title>
        <style>
          body { font: 14px/1.5 ui-monospace, SFMono-Regular, Menlo, monospace; margin: 2rem; max-width: 52rem; }
          h1 { font-size: 1.1rem; }
          #status { padding: .25rem .5rem; border-radius: 4px; background: #eee; display: inline-block; }
          #log { margin-top: 1rem; border: 1px solid #ddd; border-radius: 6px; padding: .75rem; min-height: 8rem; white-space: pre-wrap; }
          .muted { color: #666; }
        </style>
      </head>
      <body>
        <h1>Atlas · Phase 0 hello demo</h1>
        <p class="muted">
          Emit an event: <code>curl -X POST http://localhost:8080/api/v1/dev/hello -d '{"message":"hi"}'</code>
        </p>
        <p>Socket: <span id="status">connecting…</span></p>
        <div id="log"></div>

        <script src="/js/phoenix.min.js"></script>
        <script>
          const status = document.getElementById("status");
          const log = document.getElementById("log");
          const write = (m) => { log.textContent += m + "\\n"; };

          const socket = new Phoenix.Socket("/socket", { params: { token: "#{token}" } });
          socket.onOpen(() => { status.textContent = "connected"; });
          socket.onError(() => { status.textContent = "error"; });
          socket.onClose(() => { status.textContent = "disconnected"; });
          socket.connect();

          const channel = socket.channel("user:demo", {});
          channel.on("signal", (payload) => {
            write("← signal " + JSON.stringify(payload));
          });
          channel.join()
            .receive("ok", () => write("joined user:demo"))
            .receive("error", (e) => write("join failed: " + JSON.stringify(e)));
        </script>
      </body>
    </html>
    """
  end
end
