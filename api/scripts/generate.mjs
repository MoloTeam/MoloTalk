// Единственная последовательность генерации контрактов.
// Используется и командой generate, и проверкой check.

import { spawnSync } from "node:child_process";
import path from "node:path";
import process from "node:process";
import { fileURLToPath } from "node:url";

export const apiDir = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
export const repoRoot = path.resolve(apiDir, "..");
export const backendDir = path.resolve(repoRoot, "backend");

export const steps = [
  {
    name: "go",
    template: "buf.gen.go.yaml",
    paths: ["proto/molotalk/v1", "proto/molotalk/internal/v1"],
  },
  {
    name: "ts",
    template: "buf.gen.ts.yaml",
    paths: ["proto/molotalk/v1", "third_party/proto/buf/validate", "third_party/proto/google/api"],
  },
  {
    name: "openapi-public",
    template: "buf.gen.openapi.public.yaml",
    paths: ["proto/molotalk/v1"],
  },
  {
    name: "openapi-internal",
    template: "buf.gen.openapi.internal.yaml",
    paths: ["proto/molotalk/internal/v1"],
  },
];

/**
 * Генерирует контракты в каталоге api. При root, отличном от apiDir,
 * предполагается, что исходники уже скопированы туда.
 */
export function generate({ buf, root = apiDir, env = process.env, log = () => {} }) {
  for (const step of steps) {
    const args = ["generate", "--template", step.template];
    for (const p of step.paths) {
      args.push("--path", p);
    }
    const result = spawnSync(buf, args, { cwd: root, env, stdio: "inherit" });
    if (result.status !== 0) {
      throw new Error(`buf ${args.join(" ")} failed with ${result.status}`);
    }
    log(`ok generate ${step.name}`);
  }
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const { buildTools } = await import("./tools.mjs");
  const tools = buildTools({
    backendDir,
    repoRoot,
    targetDir: path.resolve(apiDir, "..", ".tmp", "tools"),
    env: process.env,
    log: console.log,
  });
  generate({ buf: tools.buf, env: tools.env, log: console.log });
}
