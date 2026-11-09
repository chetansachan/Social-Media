import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// The Go API gateway serves GraphQL on :8080. In development the frontend runs
// on :5173, so /query is proxied to keep the browser on a single origin and the
// cookies/subprotocol handling simple.
export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    proxy: {
      "/query": {
        target: process.env.VITE_GRAPHQL_HTTP ?? "http://localhost:8080",
        changeOrigin: true,
        // The gateway also serves the WebSocket transport on the same path.
        ws: true,
      },
    },
  },
  build: {
    outDir: "dist",
    sourcemap: true,
  },
});
