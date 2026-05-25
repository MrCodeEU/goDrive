import { svelte } from "@sveltejs/vite-plugin-svelte";
import { defineConfig } from "vite";

const backendURL = process.env.GODRIVE_BACKEND_URL || "http://127.0.0.1:8121";

export default defineConfig({
  plugins: [svelte()],
  build: {
    outDir: "../internal/server/static",
    emptyOutDir: true
  },
  server: {
    port: 5173,
    proxy: {
      "/api": backendURL,
      "/health": backendURL
    }
  }
});
