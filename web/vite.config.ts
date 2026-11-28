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
    // Покрытие измеряется по рукописным TS/TSX в src, включая файлы без тестов.
    // Тесты, fixtures и декларации исключены; generated-файлов в web/src нет.
    coverage: {
      provider: "v8",
      reporter: ["text", "json-summary", "lcov"],
      reportsDirectory: "coverage",
      include: ["src/**/*.{ts,tsx}"],
      exclude: [
        "src/**/*.test.{ts,tsx}",
        "src/**/test/**",
        "src/**/__fixtures__/**",
        "src/**/*.d.ts",
      ],
      all: true,
    },
  },
});
