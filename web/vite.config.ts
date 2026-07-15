import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";

// Dev-server proxy - the Go control-plane has zero CORS middleware
// (deliberate; see docker-compose.yml's own `web` service comment), so
// `npm run dev` needs this to reach the API without cross-origin
// failures. VITE_PROXY_TARGET overrides the default for anyone running
// the dev server from inside the compose network instead of the bare
// host (defaults to the host-published control-plane port).
export default defineConfig({
  plugins: [react()],
  server: {
    proxy: {
      "/api": {
        target: process.env.VITE_PROXY_TARGET ?? "http://localhost:8443",
        changeOrigin: true,
      },
    },
  },
});
