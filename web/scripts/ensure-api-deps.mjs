// Проверяет, что сгенерированные контракты api могут разрешить свои
// зависимости: файлы api/src/gen импортируют @bufbuild/protobuf, и Node/tsc
// ищут его в api/node_modules, а не в web/node_modules.
import fs from "node:fs";
import path from "node:path";

const marker = path.resolve(
  import.meta.dirname,
  "..",
  "..",
  "api",
  "node_modules",
  "@bufbuild",
  "protobuf",
  "package.json",
);

if (!fs.existsSync(marker)) {
  console.error(
    "api dependencies are missing: run `npm --prefix api ci --omit=dev` from the repository root",
  );
  process.exit(1);
}
