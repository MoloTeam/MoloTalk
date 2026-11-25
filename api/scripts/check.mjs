#!/usr/bin/env node
// Проверка контрактов: buf format/lint/build, повторная генерация без
// расхождений, сборка и тесты Go, typecheck и контрактные тесты TypeScript.
// Скрипт не требует глобальных плагинов, protoc и Docker.

import { spawnSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";
import process from "node:process";

import { apiDir, backendDir, generate } from "./generate.mjs";
import { buildTools } from "./tools.mjs";

const root = path.resolve(apiDir, "..");
const workRoot = path.join(root, ".tmp");
fs.mkdirSync(workRoot, { recursive: true });
const workDir = fs.mkdtempSync(path.join(workRoot, "check-"));

// buf format использует системный diff; Git для Windows поставляет его сам.
const gitUsrBin = "C:\\Program Files\\Git\\usr\\bin";
const extraPath = fs.existsSync(path.join(gitUsrBin, "diff.exe")) ? `${gitUsrBin}${path.delimiter}` : "";

// Системный TMP может быть недоступен: buf и go используют рабочий каталог.
const env = {
  ...process.env,
  TMP: workDir,
  TEMP: workDir,
  GOTMPDIR: workDir,
  PATH: `${extraPath}${process.env.PATH ?? ""}`,
};

function run(command, args, options = {}) {
  const result = spawnSync(command, args, { cwd: options.cwd ?? root, env, encoding: "utf8" });
  if (result.status !== 0) {
    process.stderr.write(result.stdout ?? "");
    process.stderr.write(result.stderr ?? "");
    throw new Error(`${command} ${args.join(" ")} failed with ${result.status}`);
  }
  return result.stdout ?? "";
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
    const actual = path.join(generated, file);
    if (!fs.existsSync(actual)) {
      problems.push(`${file}: missing in generated output`);
      continue;
    }
    if (!fs.readFileSync(path.join(tracked, file)).equals(fs.readFileSync(actual))) {
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

function copySources(target) {
  const entries = [
    "proto",
    "third_party",
    "openapi",
    "buf.yaml",
    "buf.gen.go.yaml",
    "buf.gen.ts.yaml",
    "buf.gen.openapi.public.yaml",
    "buf.gen.openapi.internal.yaml",
  ];
  for (const entry of entries) {
    fs.cpSync(path.join(apiDir, entry), path.join(target, entry), { recursive: true });
  }
}

function main() {
  const tools = buildTools({
    backendDir,
    apiDir,
    targetDir: path.join(workDir, "tools"),
    env,
    log: console.log,
  });

  run(tools.buf, ["format", "-d"], { cwd: apiDir });
  run(tools.buf, ["lint"], { cwd: apiDir });
  run(tools.buf, ["build", "-o", path.join(workDir, "schema.binpb")], { cwd: apiDir });
  console.log("ok buf format/lint/build");

  const generatedRoot = path.join(workDir, "generated");
  copySources(path.join(generatedRoot, "api"));
  generate({ buf: tools.buf, root: path.join(generatedRoot, "api"), env: tools.env, log: console.log });

  const problems = [
    ...compareTrees(path.join(generatedRoot, "backend", "gen"), path.join(backendDir, "gen")),
    ...compareTrees(path.join(generatedRoot, "api", "src", "gen"), path.join(apiDir, "src", "gen")),
    ...compareTrees(path.join(generatedRoot, "api", "gen", "openapi"), path.join(apiDir, "gen", "openapi")),
  ];
  if (problems.length > 0) {
    throw new Error(`generated artifacts are stale:\n${problems.join("\n")}`);
  }
  console.log("ok generated artifacts match committed files");

  run("go", ["build", "./..."], { cwd: backendDir });
  run("go", ["vet", "./..."], { cwd: backendDir });
  run("go", ["test", "-count=1", "./..."], { cwd: backendDir });
  console.log("ok go build/vet/test");

  run("node", [path.join(apiDir, "node_modules", "typescript", "bin", "tsc"), "--noEmit"], { cwd: apiDir });
  console.log("ok typescript typecheck");

  run(
    "node",
    ["--experimental-transform-types", "--test", "--test-timeout=120000", "src/test/gateway.test.mjs", "src/test/contract.test.mjs"],
    { cwd: apiDir },
  );
  console.log("ok typescript contract tests");

  const protoInMain = spawnSync("git", ["cat-file", "-e", "main:api/proto/molotalk/v1/types.proto"], { cwd: root, env });
  if (protoInMain.status === 0) {
    // .git лежит в корне репозитория, а buf.yaml — в api/, поэтому нужен subdir.
    run(tools.buf, ["breaking", "--against", "../.git#branch=main,subdir=api"], { cwd: apiDir });
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
  fs.rmSync(workDir, { recursive: true, force: true });
}
