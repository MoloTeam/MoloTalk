import path from "node:path";
import { fileURLToPath } from "node:url";

import react from "@vitejs/plugin-react";
import { defineConfig } from "vitest/config";

const projectRoot = path.dirname(fileURLToPath(import.meta.url));

// Клиентские маршруты отдаёт dev-сервер через history fallback, как и Caddy.
export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      "@api": path.resolve(projectRoot, "..", "api", "src", "gen"),
    },
  },
  server: {
    port: 5173,
    strictPort: true,
  },
  test: {
    environment: "jsdom",
    include: ["src/**/*.test.{ts,tsx}"],
    restoreMocks: true,
  },
});
