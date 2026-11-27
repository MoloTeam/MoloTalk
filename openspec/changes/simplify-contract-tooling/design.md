# Design

## Context

[ФАКТ] Все проверки ниже выполнены в отдельном worktree на `main`
(`a0b6675`), buf 1.75.0 собран из `backend/go.mod`; реестр `buf.build` доступен:
`curl -I https://buf.build` → `HTTP/1.1 200 OK`, DNS `34.95.71.106`.

[ФАКТ] Песочница не даёт писать в `%LOCALAPPDATA%`, поэтому buf запускался с
`LOCALAPPDATA=D:\projects\molotalk\.tmp\localappdata`. Это ограничение среды
исследования, а не проекта; в обычной машине переменная не нужна.

## Goals / Non-Goals

**Goals.** `generate` = один `buf generate`; версии зависимостей и плагинов
закреплены; в репозитории нет вендоренных proto, самописной сборки инструментов
и подмен окружения; поведение контрактов не меняется.

**Non-Goals.** Изменение proto-схем, DTO, путей RPC и поведения сервисов;
переход на другой генератор OpenAPI; CI (карточка #8).

## Decisions

### D1. Зависимости — из реестра, `buf.lock` коммитится

[ФАКТ] В чистом каталоге без `third_party` создан `api/buf.yaml`:

```yaml
version: v2
modules:
  - path: proto
deps:
  - buf.build/bufbuild/protovalidate
  - buf.build/googleapis/googleapis
```

`buf dep update` → exit 0, создан `buf.lock`:

```
deps:
  - name: buf.build/bufbuild/protovalidate
    commit: 511051f7f4374c3ca873b53ae68a9288
    digest: b5:a4a2d4d8…
  - name: buf.build/googleapis/googleapis
    commit: 8ac61b9e502343289c8c42b17aa5b245
    digest: b5:0318e73c…
```

Повторный `buf dep update` дважды дал одинаковый SHA-256 (`967ab452…fec`).
`buf build` (exit 0) и `buf lint` (exit 0) прошли без `third_party`.

**Альтернативы.** Оставить вендоринг (отвергнуто: 232 КБ, ручные SHA-256,
лицензии, рассинхронизация). Закрепить `buf.lock` без `deps` нельзя — lock
пустой.

**Трудно менять.** Коммиты зависимостей в lock меняются только `buf dep update`;
ручное редактирование сломает digest.

### D2. Go-плагины — удалённые, версии закреплены

[ФАКТ] `buf.build/protocolbuffers/go` — последняя версия `v1.36.12`; наша
`google.golang.org/protobuf` — `v1.36.13-0.20261016172727-…`, локальный плагин
печатает `v1.36.12-devel`.

[ФАКТ] Генерация удалённым `v1.36.12` в отдельный каталог: набор файлов совпал
с `backend/gen` (12 файлов), diff — только баннер в 7 `*.pb.go`:

```diff
-// 	protoc-gen-go v1.36.12-devel
+// 	protoc-gen-go v1.36.12
```

`buf.build/connectrpc/go:v2.0.2` совпадает с `connectrpc.com/connect/v2 v2.0.2`;
connect-файлы побайтно идентичны.

**Альтернативы.** Оставить `tool` + `go build` (отвергнуто: 10.5 с на сборку в
тёплом кеше, конфликт зависимостей). `protoc_builtin` (нет, нужен protoc).

**Трудно менять.** Баннер версии войдёт в golden; при обновлении remote-плагина
нужна перегенерация и коммит diff.

### D3. TypeScript — удалённый `bufbuild/es` и `include_imports`

[ФАКТ] `buf.build/bufbuild/es:v2.17.0` совпадает с `@bufbuild/protobuf 2.17.0`.
Шаблон с `include_imports: true` и `--path proto/molotalk/v1` даёт 8 файлов —
ровно как в `api/src/gen`, включая `buf/validate/validate_pb.ts` (235 939 Б) и
`google/api/field_behavior_pb.ts`. Отличие только в комментариях
`validate_pb.ts` (7 строк): в реестре более новая редакция protovalidate
(`protovalidate-py` вместо `protovalidate-python`).

[ФАКТ] `@bufbuild/protovalidate@1.3.0` экспортирует
`./gen/buf/validate/validate_pb.js`, но сгенерированные файлы импортируют
дескриптор относительным путём `../../buf/validate/validate_pb.ts`; заменить
его импортом из пакета без переписывания сгенерированного кода нельзя.

[ФАКТ] Вклад дескриптора в бандл (esbuild, minify+gzip):

| Что собираем | min | gzip |
|---|---|---|
| пустой entry | 0 | 20 Б |
| `validate_pb.ts` | 132 879 Б | 29 593 Б |
| `types_pb.ts` + validate | 138 662 Б | 32 014 Б |

Текущий бандл web — 475.59 КБ / 136.72 КБ gzip, то есть дескриптор ~22% gzip.
Убрать его из бандла можно только отказавшись от импорта сгенерированных типов
или переписав импорты, поэтому файл остаётся сгенерированным и закоммиченным.

**Альтернативы.** Не коммитить `validate_pb.ts` и генерировать при установке
(отвергнуто: сборка web перестаёт быть воспроизводимой без buf). Ссылаться на
`@bufbuild/protovalidate` (отвергнуто: относительные импорты).

### D4. OpenAPI — блокер удалённого плагина

[ФАКТ] `buf.build/community/sudorandom-connect-openapi` — последняя версия
`v0.28.0`, локальный модуль — `v0.30.0` (несовпадение). Попытка генерации с
`base=` даёт:

```
The 'base' option is disabled when ran with a remote plugin.
If you need this option, please use the local plugin instead or consider
using gnostic OpenAPIv3 annotations in your protobuf.
```

Значит security-схемы из `api/openapi/base.*.yaml` нельзя применить удалённым
плагином. Варианты:

1. **Рекомендуется:** оставить OpenAPI локальным, но без самописной сборки —
   `local: ["go", "run", "connectrpc.com/protoc-gen-connect-openapi@v0.30.0"]`
   в шаблоне. Логика остаётся в buf-файлах, `base` работает, версия закреплена.
2. Перевести security в gnostic-аннотации proto и включить remote-плагин —
   меняет proto и требует новой зависимости; вне этого change.
3. Пост-обработка JSON — противоречит цели «вся логика в buf-файлах».

**Открытый вопрос человеку:** принять вариант 1 (рекомендация) или планировать
gnostic-аннотации отдельным change.

### D5. Managed mode — не включаем в этом change

[ФАКТ] Проверено на копии proto без `option go_package` с
`managed.enabled: true`, `disable` для обеих зависимостей и `override`
`go_package_prefix = github.com/MoloTeam/MoloTalk/backend/gen`:

- пути файлов не изменились;
- в дескрипторы добавились языковые опции (`java_package`, `csharp_namespace`,
  `php_namespace`, `ruby_package`, `objc_class_prefix`), diff — 35 вставок /
  26 удалений в 8 файлах;
- пакет внутреннего модуля стал `internalv1` вместо `molotalkinternalv1`
  (алиас из `go_package` теряется) — это ломает Go-импорты.

**Решение:** оставить `option go_package` сейчас. Включение managed mode — это
отдельное изменение с переименованием пакета и перегенерацией golden.

**Открытый вопрос:** нужен ли managed mode вообще, если `go_package` — семь
строк, а цена — несовместимость имён пакетов и лишние опции в дескрипторах.

### D6. ULID — предопределённое правило

[ФАКТ] Полный шаблон ULID встречается в proto **75 раз** (не 62).
protovalidate поддерживает предопределённые правила
(`buf.validate.predefined`, `validate.proto:86-103`): можно объявить расширение
`StringRules` с CEL и применять его как
`(buf.validate.field).string.(molotalk.v1.ulid) = true`.

**Риск:** генератор OpenAPI не знает пользовательского правила и перестанет
выводить `pattern` в схемах — документация станет слабее. Варианты: оставить
`pattern` и не вводить правило (дублирование остаётся) либо принять потерю
`pattern` в OpenAPI.

**Открытый вопрос человеку:** внедрять правило с потерей `pattern` в OpenAPI
или отложить.

### D7. Что удаляется и упрощается

- `api/third_party/` (5 файлов, 232 412 Б) — заменяется `buf.lock`.
- `api/materials/` (150 156 Б) — снимки HLD; ссылка только в `api/README.md:22`.
- `api/scripts/` (`generate.mjs` 58 строк, `tools.mjs` 34, `check.mjs` 141) —
  заменяются `buf generate`; проверка в CI — `buf generate` + `git diff
  --exit-code` (или `check` как тонкая обёртка над `buf`).
- `tool`-директивы плагинов в `backend/go.mod` (5 строк) — удаляются; это же
  убирает источник `go-connections v0.8.2`, ломавший интеграционные тесты
  (проверяется в реализации).
- Подмены `TMP`/`GOTMPDIR`/`PATH`/`LOCALAPPDATA` в скриптах и документации.

[ФАКТ] `buf breaking --against` выгрузки `main` падает на несовпадении числа
модулей (`input contained 1 images, whereas against contained 2 images`).
Нормализованная база (proto из `main` + новый `buf.yaml`/`buf.lock`) проходит:
exit 0. Это войдёт в документацию и, если нужен автоматический check, в его
реализацию.

### D8. Поведение при недоступности реестра

[ФАКТ] Первый `buf dep update`/`generate` требует сети; после заполнения
кеша `buf` работает офлайн (в тестах повторная генерация прошла без обращений).

**Решение:** основной путь — сеть при первом запуске. Запасной путь
документируется как необязательный: `buf generate` с локальными плагинами из
кеша модулей или заранее заполненный `buf` кеш; отдельного скрипта в репозитории
не будет.

### D9. Лицензии

| Компонент | Версия | Лицензия |
|---|---|---|
| `buf.build/bufbuild/protovalidate` | commit `511051f7f437` | Apache-2.0 |
| `buf.build/googleapis/googleapis` | commit `8ac61b9e5023` | Apache-2.0 |
| `protocolbuffers/go` (remote) | v1.36.12 | BSD-3-Clause |
| `connectrpc/go` (remote) | v2.0.2 | Apache-2.0 |
| `bufbuild/es` (remote) | 2.17.0 | Apache-2.0 |
| `sudorandom-connect-openapi` | v0.28.0 remote / v0.30.0 local | MIT |

### D10. Разбиение на коммиты

1. Удаление `api/materials` и обходов окружения в скриптах/README.
2. `buf.yaml` с `deps`, `buf.lock`, удаление `third_party`.
3. Удалённые плагины, шаблоны, удаление `scripts/`, `tool`-директив.
4. Предопределённое правило ULID (если принято).
5. README/`api/README.md`, документирование запасного пути.

## Критерии приёмки

1. `go build ./...`, `go vet ./...`, `go test ./...` — проходят.
2. `npm run typecheck` (web и api) и контрактные тесты Go/TS — проходят.
3. `buf breaking` против `main` включён и проходит после нормализации базы.
4. Повторный `buf generate` в двух каталогах даёт одинаковые файлы.
5. `git diff --exit-code` после `buf generate` пуст (golden совпадает).

## Risks / Trade-offs

- **Недоступность реестра** в CI/у коллеги — блокирует первый запуск; лечится
  кешем и документированным запасным путём.
- **Версии remote-плагинов отстают** от Go-модулей (protobuf 1.36.12 vs
  1.36.13, OpenAPI 0.28 vs 0.30) — баннеры и, для OpenAPI, отказ от `base`.
- **Managed mode и ULID-правило** меняют golden и документацию — вынесены в
  открытые вопросы, чтобы не смешивать с упрощением.

## Migration Plan

Ветка `chore/simplify-contract-tooling` от `main`, включая ранее подготовленный
коммит workspaces. Обратный путь: вернуть `third_party`, локальные плагины и
`tool`-директивы; контракты при этом не меняются, поэтому откат затрагивает
только инструментарий.
