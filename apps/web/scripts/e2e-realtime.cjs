#!/usr/bin/env node
/**
 * Phase 0 end-to-end probe (Appendix A acceptance path):
 *
 *   Go writes core.system.hello.v1 ──→ outbox ──→ Elixir consumes ──→ browser (websocket)
 *
 * This script plays the browser role with the *real* Phoenix JS client
 * (apps/web depends on `phoenix`, the same code shipped to users):
 *
 *   1. fetch /dev/hello from the orchestrator to obtain a signed realtime token
 *   2. open a WebSocket to /socket, join `user:demo`
 *   3. POST /api/v1/dev/hello to the Go API
 *   4. wait for the `signal` broadcast and print it
 *
 * Usage:
 *   node scripts/e2e-realtime.cjs
 *   ATLAS_API_URL / ATLAS_SOCKET_URL override the endpoints.
 *
 * Prereqs: `task up`, Go API running (:8080), orchestrator running (:4000).
 */
const { Socket } = require("phoenix");

const API_URL = process.env.ATLAS_API_URL || "http://localhost:8080";
const SOCKET_URL = process.env.ATLAS_SOCKET_URL || "ws://localhost:4000/socket";
const DEVPAGE_URL = process.env.ATLAS_DEVPAGE_URL || "http://localhost:4000/dev/hello";

async function main() {
  const page = await (await fetch(DEVPAGE_URL)).text();
  const match = page.match(/token: "([^"]+)"/);
  if (!match) {
    throw new Error(`no embedded token found at ${DEVPAGE_URL} (is the orchestator running?)`);
  }
  const token = match[1];

  const socket = new Socket(SOCKET_URL, { params: { token }, timeout: 5_000 });
  const channel = socket.channel("user:demo", {});

  // Log every inbound message so failures are visible.
  channel.onMessage = (event, payload, ref) => {
    console.log(`  [wire] event=${event} ref=${ref} payload=${JSON.stringify(payload).slice(0, 220)}`);
    return payload;
  };

  const gotSignal = new Promise((resolve, reject) => {
    const timer = setTimeout(
      () => reject(new Error("timed out waiting for the realtime signal (20s)")),
      20_000
    );
    channel.on("signal", (payload) => {
      clearTimeout(timer);
      resolve(payload);
    });
  });

  socket.connect();
  console.log(`connecting to ${SOCKET_URL} with token sub=${JSON.parse(Buffer.from(token.split(".")[1], "base64").toString()).sub || "?"}`);

  await new Promise((resolve, reject) => {
    channel
      .join()
      .receive("ok", resolve)
      .receive("error", (e) => reject(new Error(`join user:demo failed: ${JSON.stringify(e)}`)));
  });
  console.log("joined user:demo ✓");

  const response = await fetch(`${API_URL}/api/v1/dev/hello`, {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify({ message: "e2e browser-transport probe" }),
  });
  const body = await response.text();
  console.log(`POST ${API_URL}/api/v1/dev/hello -> ${response.status} ${body}`);

  const payload = await gotSignal;
  console.log("SIGNAL on user:demo ✓", JSON.stringify(payload));

  socket.disconnect();
  console.log("OK — Go → outbox → Elixir → browser signal proven end to end.");
}

main().catch((error) => {
  console.error(`PROBE FAILED: ${error.message}`);
  process.exit(1);
});