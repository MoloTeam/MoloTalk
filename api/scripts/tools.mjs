// Сборка локальных инструментов контрактов: buf и три protoc-плагина.
// Глобальные установки не требуются, protoc и Docker не нужны.

import { spawnSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";
import process from "node:process";

export const exe = process.platform === "win32" ? ".exe" : "";

const packages = [
  ["buf", "github.com/bufbuild/buf/cmd/buf"],
  ["protoc-gen-go", "google.golang.org/protobuf/cmd/protoc-gen-go"],
  ["protoc-gen-connect-go", "connectrpc.com/connect/v2/cmd/protoc-gen-connect-go"],
  ["protoc-gen-connect-openapi", "connectrpc.com/protoc-gen-connect-openapi"],
];

/**
 * Собирает инструменты в targetDir и возвращает путь к buf вместе с окружением,
 * в котором плагины находятся по PATH.
 */
export function buildTools({ backendDir, repoRoot, targetDir, env = process.env, log = () => {} }) {
  fs.mkdirSync(targetDir, { recursive: true });
  for (const [name, pkg] of packages) {
    const output = path.join(targetDir, name + exe);
    const result = spawnSync("go", ["build", "-o", output, pkg], { cwd: backendDir, env, stdio: "inherit" });
    if (result.status !== 0) {
      throw new Error(`go build ${pkg} failed with ${result.status}`);
    }
  }
  log("ok tools built");

  // npm workspaces держат исполняемые файлы плагинов в корне.
  const npmBin = repoRoot ? path.join(repoRoot, "node_modules", ".bin") : "";
  const toolsPath = [targetDir, npmBin].filter(Boolean).join(path.delimiter) + path.delimiter + (env.PATH ?? "");
  return {
    buf: path.join(targetDir, "buf" + exe),
    env: { ...env, PATH: toolsPath },
  };
}
