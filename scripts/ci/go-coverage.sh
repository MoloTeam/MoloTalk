#!/usr/bin/env bash
# Готовит два раздельных отчёта о покрытии Go:
#   - go test -coverprofile — покрытие операторов;
#   - gobco -branch — покрытые и измеренные исходы ветвлений.
#
# gobco принимает только один пакет за запуск, не объединяет файлы stats и не
# учитывает build tags. Поэтому пакеты обходятся по одному в копии backend без
# `*_integration_test.go`: интеграционные сценарии измеряются отдельной задачей,
# а gobco не должен пытаться типизировать их без тега.
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "${script_dir}/../.." && pwd)"
backend="${repo_root}/backend"
reports="${REPORTS_DIR:-${repo_root}/reports}"

if ! command -v gobco >/dev/null 2>&1; then
  echo "gobco не найден в PATH; установите github.com/rillig/gobco@v1.3.4" >&2
  exit 1
fi

# go и gobco — нативные процессы: в Git Bash передаём им Windows-пути.
to_native() {
  if command -v cygpath >/dev/null 2>&1; then
    cygpath -w "$1"
  else
    printf '%s\n' "$1"
  fi
}

mkdir -p "${reports}"
rm -rf "${reports}/gobco"
mkdir -p "${reports}/gobco"
cd "${backend}"

module_path="$(go list -m)"
mapfile -t packages < <(
  go list ./... |
    grep -vE '/gen($|/)' |
    grep -vE '/internal/testenv$' |
    grep -vE '/internal/archtest$' |
    grep -vE '/internal/platform/rpc/testserver$'
)
if [ "${#packages[@]}" -eq 0 ]; then
  echo "go list не вернул ни одного пакета для измерения" >&2
  exit 1
fi
printf '%s\n' "${packages[@]}" > "${reports}/go-packages.txt"

profile="$(to_native "${reports}/go-coverage.out")"
go test -count=1 -covermode=count -coverprofile="${profile}" "${packages[@]}"
go tool cover -func="${profile}" > "${reports}/go-coverage.txt"

work="${repo_root}/.gobco-src"
rm -rf "${work}"
mkdir -p "${work}"
trap 'rm -rf "${work}"' EXIT
(cd "${backend}" && tar -cf - .) | (cd "${work}" && tar -xf -)
find "${work}" -name '*_integration_test.go' -delete

cd "${work}"
: > "${reports}/go-branch.txt"
for pkg in "${packages[@]}"; do
  rel="${pkg#"${module_path}/"}"
  name="${rel//\//_}"
  stats="$(to_native "${reports}/gobco/${name}.json")"
  out="${reports}/gobco/${name}.txt"
  # Файлы stats не должны переживать повторный запуск: иначе gobco складывает
  # счётчики с прошлым прогоном и отчёт перестаёт быть воспроизводимым.
  rm -f "${reports}/gobco/${name}.json" "${out}"

  echo "# package: ${pkg}" >> "${reports}/go-branch.txt"
  # gobco копирует только модуль backend, поэтому тесты rpc, читающие
  # api/testdata по относительному пути, пропускаются. В обычном go test они
  # выполняются и проверяют кросс-языковые эталоны.
  gobco_args=()
  if [ "${rel}" = "internal/platform/rpc" ]; then
    gobco_args+=("-test=-skip=TestGo(DecodesTypeScriptFrame|ProducesReadyFixture)")
  fi
  if ! gobco -branch -stats "${stats}" ${gobco_args[@]+"${gobco_args[@]}"} "./${rel}" > "${out}" 2>&1; then
    echo "gobco завершился ошибкой для ${pkg}" >&2
    cat "${out}" >&2
    exit 1
  fi
  cat "${out}" >> "${reports}/go-branch.txt"
  echo >> "${reports}/go-branch.txt"
done

echo "Отчёты покрытия Go: ${reports}/go-coverage.out, ${reports}/go-branch.txt"
