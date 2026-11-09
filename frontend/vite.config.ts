import { defineConfig } from "vite";
import tailwindcss from "@tailwindcss/vite";
import react from "@vitejs/plugin-react";

const proxyTarget =
  process.env.WORKBENCH_PROXY_TARGET || "http://127.0.0.1:8080";

export default defineConfig({
  base: "/app/",
  resolve: { alias: { "@": new URL("./src", import.meta.url).pathname } },
  plugins: [react(), tailwindcss()],
  build: { outDir: "dist", sourcemap: false },
  server: {
    // Preserve the browser Host so the trusted ingress can enforce matching Origin.
    proxy: {
      "/ui-api": { target: proxyTarget, changeOrigin: false },
      "/dashboard-api": { target: proxyTarget, changeOrigin: false },
    },
  },
});
