#!/usr/bin/env bash
# Сравнивает модуль api с переданным коммитом или общим предком HEAD и origin/main.
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "${script_dir}/../.." && pwd)"
cd "${repo_root}"

base="${1:-${BASE_SHA:-}}"

if [ -z "${base}" ] || [[ "${base}" =~ ^0+$ ]]; then
  if git rev-parse --verify --quiet origin/main >/dev/null; then
    base="$(git merge-base HEAD origin/main)"
  else
    echo "не удалось определить базу: BASE_SHA пуст, origin/main не найден" >&2
    exit 1
  fi
fi

if ! git cat-file -e "${base}^{commit}" 2>/dev/null; then
  git fetch --no-tags origin "${base}" >/dev/null 2>&1 || true
fi
if ! git cat-file -e "${base}^{commit}" 2>/dev/null; then
  echo "базовый коммит ${base} недоступен в checkout" >&2
  exit 1
fi

tmp="$(mktemp -d)"
trap 'rm -rf "${tmp}"' EXIT

git archive "${base}" api | tar -x -C "${tmp}"
if [ ! -f "${tmp}/api/buf.yaml" ]; then
  echo "в базе ${base} нет api/buf.yaml — модуль api не нормализован" >&2
  exit 1
fi

base_api="$(cd "${tmp}/api" && pwd)"
if command -v cygpath >/dev/null 2>&1; then
  base_api="$(cygpath -w "${base_api}")"
fi

echo "buf breaking: база ${base}, каталог ${base_api}"
cd "${repo_root}/api"
buf breaking --against "${base_api}"
