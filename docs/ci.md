# Непрерывная интеграция

Workflow [`.github/workflows/ci.yml`](../.github/workflows/ci.yml) проверяет
сборку, тесты, контракты и контейнерные сценарии на чистом Linux-раннере.
Каждая область получает отдельный статус, поэтому сбой указывает на компонент,
а не на общий прогон.

## Запуск

| Событие | Что проверяется |
| --- | --- |
| `pull_request` в `main` | Версия дерева из PR: Go, web, API и контейнеры. |
| `push` в `main` | Та же проверка после обновления `main`. |
| `workflow_dispatch` | Ручной запуск. Workflow должен присутствовать в ветке по умолчанию. |

База проверки совместимости для `pull_request` — `base.sha`. Для `push` в
`main` используется SHA до push, для ручного запуска на `main` — родитель
проверяемого коммита. Если база не передана, скрипт использует общий предок с
`origin/main`. Раннер получает только `contents: read`, а устаревшие запуски
одной ветки отменяются.

## Задачи и статусы

| Статус | Что выполняет | Артефакт |
| --- | --- | --- |
| `Go / build, vet, unit, coverage` | `go build`, `go vet`, модульные тесты, покрытие операторов и ветвлений. | `go-coverage` |
| `Web / lint, types, tests, build` | ESLint, `tsc`, Vitest с покрытием, `vite build`. | `web-coverage` |
| `API / buf, golden, compatibility` | Buf format/lint/build/breaking, генерация и сверка golden, типы и тесты API. | — |
| `Containers / integration and routing` | PostgreSQL-тесты с тегом `integration` и матрица маршрутизации Caddy. | — |

Сводка покрытия добавляется в страницу запуска Actions. Отсутствующий или
пустой отчёт считается ошибкой проверки: `scripts/ci/coverage-report.mjs`
завершается ненулевым кодом.

## Локальный повтор

Команды совпадают с шагами workflow. На Windows shell-скрипты запускаются из
Git Bash, а Go, Node и Buf должны быть в `PATH`.

### Go

```bash
cd backend
go build ./...
go vet ./...
go test -count=1 ./...
```

Покрытие операторов и ветвлений:

```bash
go install github.com/rillig/gobco@v1.3.4
export PATH="$(go env GOPATH)/bin:$PATH"
cd ..
bash scripts/ci/go-coverage.sh
node scripts/ci/coverage-report.mjs --kind go
```

Скрипт пишет `reports/go-coverage.out`, `reports/go-coverage.txt` и
`reports/go-branch.txt`. При другом каталоге отчётов задайте `REPORTS_DIR`.

### Web и API

```bash
npm ci
npm run lint
npm run typecheck
npm run test
npm run build
npm --workspace api run typecheck
npm --workspace web run test:coverage
node scripts/ci/coverage-report.mjs --kind web
```

`npm run test` запускает тесты обоих workspaces, включая контрактный тест API,
которому нужен `go` в `PATH`. Отчёт web лежит в `web/coverage/`.
`web/coverage/coverage-summary.json` даёт числитель и знаменатель ветвей,
`lcov.info` — построчные данные.

### Контракты

```bash
go install github.com/bufbuild/buf/cmd/buf@v1.75.0
export PATH="$(go env GOPATH)/bin:$PATH"

cd api
buf format --diff --exit-code
buf lint
buf build
buf generate --template buf.gen.go.yaml
buf generate --template buf.gen.ts.yaml --path proto/molotalk/v1
buf generate --template buf.gen.openapi.public.yaml --path proto/molotalk/v1
buf generate --template buf.gen.openapi.internal.yaml --path proto/molotalk/internal/v1
cd ..

git status --porcelain --untracked-files=all -- backend/gen api/gen api/src/gen
bash scripts/ci/buf-breaking.sh "$(git merge-base HEAD origin/main)"
```

Пустой вывод `git status` означает, что golden совпадает с генерацией. CI
завершает проверку с ошибкой при наличии изменённых или новых артефактов. Скрипт
`buf-breaking.sh` выгружает `api` из базового SHA во временный каталог и
сравнивает с ним текущий модуль.

### Контейнеры

```bash
cd backend
go test -tags=integration -count=1 ./...
cd ..

ROUTING_PROJECT=molotalk-routing-local bash deploy/tests/run-routing-checks.sh
```

Скрипт поднимает `deploy/tests/docker-compose.yml`, получает код контейнера
`verify` и в любом случае выполняет `down -v --remove-orphans` для своего
проекта. После очистки он проверяет, что контейнеры, сети и volumes этого
проекта удалены; остатки делают результат ненулевым. Для ручной очистки:

```bash
docker compose -p molotalk-routing-local -f deploy/tests/docker-compose.yml down -v --remove-orphans
```

## Покрытие

| Стек | Метрика | Область | Ограничения |
| --- | --- | --- | --- |
| Go | `go test -coverprofile` — операторы | Пакеты `backend`, кроме `backend/gen`, `internal/testenv`, `internal/archtest` и `internal/platform/rpc/testserver`. | Стандартная метрика операторов; исходы ветвлений не показывает. |
| Go | `gobco -branch` — исходы ветвлений | Те же пакеты; используется копия `backend` без `*_integration_test.go`. | `gobco` 1.3.4 (BSD-2-Clause) измеряет булевы условия, не покрывает `select`, запускается по одному пакету и не учитывает build tags. В пакете `internal/platform/rpc` пропускаются два теста, читающие `api/testdata` вне модуля; обычный `go test` их выполняет. |
| web | `@vitest/coverage-v8` (MIT) — ветви, операторы, строки, функции | Рукописные TS/TSX в `web/src`, включая файлы без тестов. | Исключены тесты, fixtures и `*.d.ts`; generated-файлов в `web/src` нет. |

Числители и знаменатели Go и web не суммируются: метрики и области разные.
Порог покрытия не задан: обязательное значение и список required checks
согласуются по карточке #8 и Q9 после появления наблюдаемого baseline.

## Ограничения

- Браузерный E2E приложения, аудит зависимостей и deployment в workflow не
  входят.
- Docker-проверки требуют доступного демона и реестра образов; недоступность
  Docker видна как ошибка задачи.
- Первый запуск Buf и Go-модулей требует сети; дальше работает кеш раннера.
