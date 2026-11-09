import http from "node:http";
import { createServer } from "vite";
// This synthetic ingress uses the production Origin/Host rule. Browser fixture
// routes mock application data; contract tests deliberately reach this server.
const upstream = http.createServer((req, res) => {
  const origin = req.headers.origin;
  if (origin && new URL(origin).host !== req.headers.host) {
    res.writeHead(403, { "content-type": "application/json" });
    res.end(JSON.stringify({ code: "ORIGIN_REJECTED" }));
    return;
  }
  if (req.url !== "/ui-api/contract") {
    res.writeHead(404);
    res.end();
    return;
  }
  let body = "";
  req.on("data", (chunk) => (body += chunk));
  req.on("end", () => {
    res.writeHead(200, { "content-type": "application/json" });
    res.end(
      JSON.stringify({
        method: req.method,
        host: req.headers.host,
        origin: origin || null,
        body: body ? JSON.parse(body) : null,
        authorization: req.headers.authorization || null,
      }),
    );
  });
});
await new Promise((resolve) => upstream.listen(4174, "127.0.0.1", resolve));
process.env.WORKBENCH_PROXY_TARGET = "http://127.0.0.1:4174";
const vite = await createServer({
  server: { host: "127.0.0.1", port: 4173, strictPort: true },
});
await vite.listen();
let closing = false;
async function close() {
  if (closing) return;
  closing = true;
  await vite.close();
  upstream.close(() => process.exit(0));
}
process.on("SIGTERM", close);
process.on("SIGINT", close);
