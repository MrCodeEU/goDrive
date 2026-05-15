import { svelte } from "@sveltejs/vite-plugin-svelte";
import { defineConfig } from "vite";

export default defineConfig({
  plugins: [svelte()],
  build: {
    outDir: "../internal/server/static",
    emptyOutDir: true
  },
  server: {
    port: 5173,
    proxy: {
      "/api": "http://127.0.0.1:8121",
      "/health": "http://127.0.0.1:8121"
    }
  }
});
