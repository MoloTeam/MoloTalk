# Tasks

## 0. Подготовка

- [ ] 0.1 Убедиться, что ветка `chore/simplify-contract-tooling` создана от актуального `main` и содержит перенесённый коммит workspaces.
- [ ] 0.2 Зафиксировать исходные метрики: размер `api/third_party`, `api/materials`, строки `api/scripts/*`, время `npm run check:api`.

## 1. Удаление снимков материала и обходов окружения

- [ ] 1.1 Удалить `api/materials/` и ссылку на него в `api/README.md`.
- [ ] 1.2 Убрать из скриптов и README подмены `TMP`, `GOTMPDIR`, `PATH`, `LOCALAPPDATA` и Windows-специфичные пути.
- [ ] 1.3 Проверить, что `go build ./...`, `go vet ./...`, `go test ./...` проходят без этих переменных.

## 2. Зависимости из реестра и `buf.lock`

- [ ] 2.1 Добавить в `api/buf.yaml` `deps: buf.build/bufbuild/protovalidate`, `buf.build/googleapis/googleapis`; убрать модуль `third_party/proto`.
- [ ] 2.2 Выполнить `buf dep update`, закоммитить `buf.lock`.
- [ ] 2.3 Удалить `api/third_party/`; проверить `buf build`, `buf lint` и `buf generate` без него.
- [ ] 2.4 Убедиться, что повторный `buf dep update` не меняет `buf.lock`.

## 3. Удалённые плагины и упрощение скриптов

- [ ] 3.1 Перевести `buf.gen.go.yaml` и `buf.gen.ts.yaml` на `remote:` с версиями: `buf.build/protocolbuffers/go:v1.36.12`, `buf.build/connectrpc/go:v2.0.2`, `buf.build/bufbuild/es:v2.17.0`.
- [ ] 3.2 Для TS включить `include_imports: true` и убрать `--path third_party/...` из документированных команд.
- [ ] 3.3 OpenAPI оставить локальным запуском `go run connectrpc.com/protoc-gen-connect-openapi@v0.30.0` (см. открытый вопрос в design); `base` сохраняется.
- [ ] 3.4 Удалить `api/scripts/`; `check` заменить на `buf generate` + `git diff --exit-code` (при необходимости — тонкая обёртка без сборки инструментов).
- [ ] 3.5 Удалить `tool`-директивы плагинов из `backend/go.mod`; проверить, что интеграционные тесты снова собираются (`go test -tags=integration ./...`).
- [ ] 3.6 Перегенерировать артефакты и проверить diff: ожидаются только баннеры версий и комментарии `validate_pb.ts`.

## 4. Предопределённое правило ULID (условно, по решению владельца)

- [ ] 4.1 Добавить `api/proto/molotalk/v1/rules.proto` с расширением `StringRules` и CEL-правилом ULID.
- [ ] 4.2 Заменить 75 повторов `pattern` на `(buf.validate.field).string.(molotalk.v1.ulid) = true`.
- [ ] 4.3 Проверить, что Go/TS-валидация ведёт себя так же (тесты `validation_rules_test.go`), и зафиксировать изменение OpenAPI (потеря `pattern`) либо сохранить `pattern` дополнительно.

## 5. Документация

- [ ] 5.1 Обновить `api/README.md`: `buf generate`, `buf.lock`, версии плагинов, удаление `third_party`/`materials`.
- [ ] 5.2 Описать необязательный запасной путь при недоступном реестре.
- [ ] 5.3 Обновить корневой `README.md` и инструкции CI-проверок.

## 6. Приёмка

- [ ] 6.1 `go build ./...`, `go vet ./...`, `go test ./...` — успешно.
- [ ] 6.2 `go test -tags=integration ./...` — собирается и проходит (нужен Docker).
- [ ] 6.3 TypeScript typecheck для `api` и `web`, контрактные тесты Go и TS — успешно.
- [ ] 6.4 `buf breaking` против нормализованной базы `main` — успешно.
- [ ] 6.5 Повторная генерация в двух каталогах совпадает; `git diff --exit-code` после `buf generate` пуст.
- [ ] 6.6 `openspec validate simplify-contract-tooling --strict` — valid.
