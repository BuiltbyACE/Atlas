import type { NextConfig } from "next";

/**
 * Atlas web configuration.
 *
 * Routing model (blueprint section 5.1 / 5.3):
 *   - In production, Caddy owns same-origin routing: `/api/*` goes to the Go
 *     service (`atlas-api`) and everything else goes to this Next.js app. That
 *     keeps the session cookie first-party and avoids CORS. So Next.js itself
 *     does not proxy `/api/*` in production.
 *   - In development there is no Caddy, so we add a `rewrites` rule that
 *     forwards `/api/*` to the Go service on localhost:8080. This is a dev-only
 *     convenience; it must never contain business logic (see section 5.2).
 *
 * `/socket` (Phoenix Channels WebSocket) is intentionally NOT rewritten here:
 * Next.js rewrites do not proxy WebSockets. In development the browser connects
 * to the orchestrator directly via NEXT_PUBLIC_REALTIME_URL; in production
 * Caddy proxies `/socket` to the Elixir orchestrator.
 */
const apiInternalUrl =
  process.env.ATLAS_API_INTERNAL_URL ?? "http://localhost:8080";

const nextConfig: NextConfig = {
  reactStrictMode: true,
  async rewrites() {
    return [
      {
        source: "/api/:path*",
        destination: `${apiInternalUrl}/api/:path*`,
      },
    ];
  },
};

export default nextConfig;
