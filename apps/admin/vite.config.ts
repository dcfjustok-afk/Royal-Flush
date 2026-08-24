import vue from "@vitejs/plugin-vue";
import { defineConfig } from "vitest/config";

export default defineConfig({
  plugins: [vue()],
  server: { proxy: { "/api": "http://127.0.0.1:8080" } },
  build: { sourcemap: true },
  test: { environment: "jsdom", clearMocks: true },
});
