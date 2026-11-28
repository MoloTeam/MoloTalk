#!/usr/bin/env bash
# Запускает матрицу маршрутизации Caddy в отдельном Compose-проекте.
# Cleanup выполняется и при отказе: удаляются только ресурсы этого проекта,
# после чего проверяется, что контейнеры, сети и volumes не остались.
set -uo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "${script_dir}/../.." && pwd)"
cd "${repo_root}"

project="${ROUTING_PROJECT:-molotalk-routing}"
compose=(docker compose -p "${project}" -f deploy/tests/docker-compose.yml)
if [ -n "${ROUTING_COMPOSE_OVERRIDE:-}" ]; then
  compose+=(-f "${ROUTING_COMPOSE_OVERRIDE}")
fi

project_resources() {
  docker ps -aq --filter "label=com.docker.compose.project=${project}"
  docker network ls -q --filter "label=com.docker.compose.project=${project}"
  docker volume ls -q --filter "label=com.docker.compose.project=${project}"
}

cleanup() {
  local status=$?

  echo "::group::Compose cleanup (${project})"
  "${compose[@]}" down -v --remove-orphans || true

  if [ -n "$(project_resources)" ]; then
    echo "после cleanup остались ресурсы проекта ${project}:" >&2
    docker ps -a --filter "label=com.docker.compose.project=${project}" >&2
    docker network ls --filter "label=com.docker.compose.project=${project}" >&2
    docker volume ls --filter "label=com.docker.compose.project=${project}" >&2
    if [ "${status}" -eq 0 ]; then
      status=1
    fi
  else
    echo "контейнеры, сети и volumes проекта ${project} удалены"
  fi
  echo "::endgroup::"

  exit "${status}"
}
trap cleanup EXIT

echo "::group::Routing matrix (${project})"
"${compose[@]}" up --build --abort-on-container-exit --exit-code-from verify
status=$?
echo "маршрутная матрица завершилась с кодом ${status}"
echo "::endgroup::"
exit "${status}"
