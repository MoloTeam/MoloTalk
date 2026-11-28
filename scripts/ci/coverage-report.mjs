#!/usr/bin/env node
// Публикует отдельные отчёты о покрытии Go и web: проверяет, что отчёт
// существует и не пуст, печатает сводку и добавляет её в GITHUB_STEP_SUMMARY.
import fs from "node:fs";
import path from "node:path";
import process from "node:process";

const repoRoot = path.resolve(import.meta.dirname, "..", "..");

function fail(message) {
  console.error(`::error::${message}`);
  process.exit(1);
}

function parseArgs(argv) {
  const options = {};
  for (let index = 0; index < argv.length; index++) {
    const arg = argv[index];
    if (!arg.startsWith("--")) {
      fail(`unexpected argument: ${arg}`);
    }
    const value = argv[index + 1];
    if (value === undefined || value.startsWith("--")) {
      fail(`missing value for ${arg}`);
    }
    options[arg.slice(2)] = value;
    index++;
  }
  return options;
}

function relative(file) {
  return path.relative(repoRoot, file).split(path.sep).join("/");
}

function readRequired(file, label) {
  if (!fs.existsSync(file)) {
    fail(`${label} отсутствует: ${relative(file)}`);
  }
  const stats = fs.statSync(file);
  if (!stats.isFile() || stats.size === 0) {
    fail(`${label}: пустой файл ${relative(file)}`);
  }
  return fs.readFileSync(file, "utf8");
}

function percent(covered, total) {
  if (total === 0) {
    return "0.00";
  }
  return ((covered / total) * 100).toFixed(2);
}

function formatRow(metric, covered, total) {
  return `| ${metric} | ${covered} | ${total} | ${percent(covered, total)}% |`;
}

function summarizeWeb(input) {
  const summaryFile = path.join(input, "coverage-summary.json");
  const lcovFile = path.join(input, "lcov.info");
  const summary = JSON.parse(readRequired(summaryFile, "Сводка покрытия web"));
  readRequired(lcovFile, "LCOV-отчёт web");

  const total = summary.total ?? {};
  const branches = total.branches ?? { covered: 0, total: 0 };
  const statements = total.statements ?? { covered: 0, total: 0 };
  const lines = total.lines ?? { covered: 0, total: 0 };
  const functions = total.functions ?? { covered: 0, total: 0 };

  if (!branches.total) {
    fail(`в отчёте web нет измеренных ветвлений: ${relative(summaryFile)}`);
  }
  const lines_ = [
    "## Покрытие web",
    "",
    "| Измерение | Покрыто | Измерено | Доля |",
    "| --- | --- | --- | --- |",
    formatRow("Ветви (`branches`)", branches.covered, branches.total),
    formatRow("Операторы (`statements`)", statements.covered, statements.total),
    formatRow("Строки (`lines`)", lines.covered, lines.total),
    formatRow("Функции (`functions`)", functions.covered, functions.total),
    "",
    "Область: рукописные TypeScript/TSX в `web/src`, включая файлы без тестов.",
    "Исключены тесты, fixtures и декларации `*.d.ts`; generated-файлов в `web/src` нет.",
    "Источник: `@vitest/coverage-v8`, отчёты `coverage-summary.json` и `lcov.info`.",
    "",
  ];
  return lines_.join("\n");
}

function summarizeGo(input) {
  const profile = readRequired(path.join(input, "go-coverage.out"), "Профиль покрытия Go");
  readRequired(path.join(input, "go-coverage.txt"), "Текстовый отчёт покрытия Go");
  const branchReport = readRequired(path.join(input, "go-branch.txt"), "Отчёт о покрытии ветвлений Go");

  let coveredStatements = 0;
  let totalStatements = 0;
  for (const line of profile.split(/\r?\n/)) {
    if (!line || line.startsWith("mode:")) {
      continue;
    }
    const match = line.match(/^\S+:\d+\.\d+,\d+\.\d+ (\d+) (\d+)$/);
    if (!match) {
      fail(`не удалось разобрать строку профиля покрытия Go: ${line}`);
    }
    const statements = Number(match[1]);
    const count = Number(match[2]);
    totalStatements += statements;
    if (count > 0) {
      coveredStatements += statements;
    }
  }
  if (!totalStatements) {
    fail(`профиль покрытия Go не содержит операторов: ${relative(path.join(input, "go-coverage.out"))}`);
  }

  let coveredBranches = 0;
  let totalBranches = 0;
  let packages = 0;
  for (const line of branchReport.split(/\r?\n/)) {
    if (line.startsWith("# package: ")) {
      packages++;
    }
    const match = line.match(/^Branch coverage: (\d+)\/(\d+)$/);
    if (match) {
      coveredBranches += Number(match[1]);
      totalBranches += Number(match[2]);
    }
  }
  if (!packages || !totalBranches) {
    fail(`отчёт о покрытии ветвлений Go пуст: ${relative(path.join(input, "go-branch.txt"))}`);
  }
  const lines_ = [
    "## Покрытие Go",
    "",
    "| Измерение | Покрыто | Измерено | Доля |",
    "| --- | --- | --- | --- |",
    formatRow("Операторы (`go test -coverprofile`)", coveredStatements, totalStatements),
    formatRow("Исходы ветвлений (`gobco -branch`)", coveredBranches, totalBranches),
    "",
    `Область ветвлений: ${packages} пакетов \`backend\` без \`backend/gen\` и тестовых помощников (\`internal/testenv\`, \`internal/archtest\`, \`internal/platform/rpc/testserver\`).`,
    "Ограничения: `gobco` 1.3.4 (BSD-2-Clause) измеряет исходы булевых условий и не покрывает `select`; запуск выполняется по одному пакету.",
    "Показатель операторов — стандартный `go test -coverprofile`, он публикуется отдельно и не суммируется с покрытием ветвлений.",
    "",
  ];
  return lines_.join("\n");
}

const options = parseArgs(process.argv.slice(2));
if (options.kind !== "go" && options.kind !== "web") {
  fail("--kind должен быть go или web");
}

const input = path.resolve(
  repoRoot,
  options.input ?? (options.kind === "web" ? path.join("web", "coverage") : "reports"),
);
const summary = options.kind === "web" ? summarizeWeb(input) : summarizeGo(input);

console.log(summary);
if (process.env.GITHUB_STEP_SUMMARY) {
  fs.appendFileSync(process.env.GITHUB_STEP_SUMMARY, `${summary}\n`);
}
