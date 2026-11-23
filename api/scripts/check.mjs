#!/usr/bin/env node
// Проверка контрактов: buf format/lint/build, повторная генерация без
// расхождений, сборка и тесты Go, typecheck TypeScript.
// Скрипт не требует глобальных плагинов, protoc и Docker.

import { execFileSync, spawnSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";
import process from "node:process";

const root = path.resolve(import.meta.dirname, "..", "..");
const apiDir = path.join(root, "api");
const backendDir = path.join(root, "backend");
const exe = process.platform === "win32" ? ".exe" : "";

// Рабочий каталог внутри репозитория: системный TMP может быть недоступен.
const workDir = path.join(root, ".tmp");
fs.mkdirSync(workDir, { recursive: true });
const tools = fs.mkdtempSync(path.join(workDir, "check-"));

// buf format использует системный diff; Git для Windows поставляет его сам.
const gitUsrBin = "C:\\Program Files\\Git\\usr\\bin";
const extraPath = fs.existsSync(path.join(gitUsrBin, "diff.exe")) ? `${gitUsrBin}${path.delimiter}` : "";

function environment() {
  return {
    ...process.env,
    PATH: `${tools}${path.delimiter}${extraPath}${path.join(apiDir, "node_modules", ".bin")}${path.delimiter}${process.env.PATH}`,
    TMP: tools,
    TEMP: tools,
    GOTMPDIR: tools,
  };
}

function run(command, args, options = {}) {
  const result = spawnSync(command, args, {
    cwd: options.cwd ?? root,
    env: environment(),
    encoding: "utf8",
  });
  if (result.status !== 0) {
    process.stderr.write(result.stdout ?? "");
    process.stderr.write(result.stderr ?? "");
    throw new Error(`${command} ${args.join(" ")} failed with ${result.status}`);
  }
  return result.stdout ?? "";
}

function buildTools() {
  const toolsToBuild = [
    ["buf", "github.com/bufbuild/buf/cmd/buf"],
    ["protoc-gen-go", "google.golang.org/protobuf/cmd/protoc-gen-go"],
    ["protoc-gen-connect-go", "connectrpc.com/connect/v2/cmd/protoc-gen-connect-go"],
    ["protoc-gen-connect-openapi", "connectrpc.com/protoc-gen-connect-openapi"],
  ];
  for (const [name, pkg] of toolsToBuild) {
    run("go", ["build", "-o", path.join(tools, name + exe), pkg], { cwd: backendDir });
  }
}

function buf(...args) {
  return run(path.join(tools, "buf" + exe), args, { cwd: apiDir });
}

function listFiles(directory) {
  const result = [];
  const walk = (current) => {
    for (const entry of fs.readdirSync(current, { withFileTypes: true })) {
      const full = path.join(current, entry.name);
      if (entry.isDirectory()) {
        walk(full);
      } else {
        result.push(path.relative(directory, full).replaceAll("\\", "/"));
      }
    }
  };
  if (fs.existsSync(directory)) {
    walk(directory);
  }
  return result.sort();
}

function compareTrees(generated, tracked) {
  const problems = [];
  const generatedFiles = listFiles(generated);
  const trackedFiles = listFiles(tracked);
  for (const file of trackedFiles) {
    const expectedPath = path.join(tracked, file);
    const actualPath = path.join(generated, file);
    if (!fs.existsSync(actualPath)) {
      problems.push(`${file}: missing in generated output`);
      continue;
    }
    if (!fs.readFileSync(expectedPath).equals(fs.readFileSync(actualPath))) {
      problems.push(`${file}: content differs`);
    }
  }
  for (const file of generatedFiles) {
    if (!trackedFiles.includes(file)) {
      problems.push(`${file}: not tracked, regenerate and commit`);
    }
  }
  return problems;
}

function generatedDrift() {
  const result = spawnSync("git", ["status", "--porcelain", "--", "backend/gen", "api/src/gen", "api/gen"], {
    cwd: root,
    encoding: "utf8",
  });
  return (result.stdout ?? "").trim();
}

function main() {
  buildTools();

  buf("format", "-d");
  buf("lint");
  buf("build", "-o", path.join(tools, "schema.binpb"));
  console.log("ok buf format/lint/build");

  const output = path.join(tools, "out");
  buf("generate", "--template", "buf.gen.go.yaml", "--output", path.join(output, "go"), "--path", "proto/molotalk/v1", "--path", "proto/molotalk/internal/v1");
  buf("generate", "--template", "buf.gen.ts.yaml", "--output", path.join(output, "ts"), "--path", "proto/molotalk/v1", "--path", "third_party/proto/buf/validate", "--path", "third_party/proto/google/api");
  buf("generate", "--template", "buf.gen.openapi.public.yaml", "--output", path.join(output, "openapi"), "--path", "proto/molotalk/v1");
  buf("generate", "--template", "buf.gen.openapi.internal.yaml", "--output", path.join(output, "openapi"), "--path", "proto/molotalk/internal/v1");

  const differences = [
    ...compareTrees(path.join(output, "backend", "gen"), path.join(backendDir, "gen")),
    ...compareTrees(path.join(output, "ts", "src", "gen"), path.join(apiDir, "src", "gen")),
    ...compareTrees(path.join(output, "openapi", "gen", "openapi"), path.join(apiDir, "gen", "openapi")),
  ];
  if (differences.length > 0) {
    throw new Error(`generated artifacts are stale:\n${differences.join("\n")}`);
  }
  console.log("ok generated artifacts match committed files");

  run("go", ["build", "./..."], { cwd: backendDir });
  run("go", ["vet", "./..."], { cwd: backendDir });
  run("go", ["test", "-count=1", "./..."], { cwd: backendDir });
  console.log("ok go build/vet/test");

  run("node", [path.join(apiDir, "node_modules", "typescript", "bin", "tsc"), "--noEmit"], { cwd: apiDir });
  console.log("ok typescript typecheck");

  const protoInMain = spawnSync("git", ["cat-file", "-e", "main:api/proto/molotalk/v1/types.proto"], { cwd: root });
  if (protoInMain.status === 0) {
    buf("breaking", "--against", ".git#branch=main");
    console.log("ok buf breaking against main");
  } else {
    console.log("skip buf breaking: main has no proto baseline yet (bootstrap)");
  }

  console.log("check passed");
}

try {
  main();
} catch (error) {
  console.error(error.message);
  process.exit(1);
} finally {
  fs.rmSync(tools, { recursive: true, force: true });
}
