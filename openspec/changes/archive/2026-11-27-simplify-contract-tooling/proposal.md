# Proposal

## Why

Генерация контрактов обросла обходными путями: скрипты собирают `buf` и плагины
через `go build`, внешние proto закоммичены в `api/third_party`, а окружение
подменяется через `PATH`, `TMP`, `GOTMPDIR` и `LOCALAPPDATA`. Это 233 строки
самописного кода, 232 КБ вендоренных схем и побочный эффект: директивы `tool`
подтянули `docker/go-connections v0.8.2`, из-за чего интеграционные тесты #5
перестали собираться (`undefined: sockets.DialPipe`).

Реестр `buf.build` из рабочего окружения доступен (проверено), значит
зависимости и плагины можно закрепить штатно, а генерацию свести к
`buf generate`.

## What Changes

- **BREAKING (для разработчиков, не для контрактов):** `generate` = `buf generate`;
  `api/scripts/{generate,tools,check}.mjs` удаляются, логика переезжает в
  `api/buf*.yaml`.
- Внешние proto — из реестра: `deps` в `api/buf.yaml`
  (`buf.build/bufbuild/protovalidate`, `buf.build/googleapis/googleapis`),
  `buf.lock` коммитится. `api/third_party/` удаляется.
- Плагины — удалённые, с закреплёнными версиями в шаблонах:
  `buf.build/protocolbuffers/go`, `buf.build/connectrpc/go`,
  `buf.build/bufbuild/es`; OpenAPI остаётся локальным запуском (`go run`) из-за
  ограничения `base` у удалённого плагина (см. design, открытый вопрос).
- Директивы `tool` для плагинов удаляются из `backend/go.mod`; это же убирает
  конфликт версий, ломавший интеграционные тесты.
- `api/materials/` удаляется (снимки HLD больше не нужны как источник).
- Предопределённое правило ULID вместо 75 повторов регулярного выражения.
- README и `api/README.md` описывают новую установку и запасной путь при
  недоступном реестре.

## Capabilities

### New Capabilities

Нет.

### Modified Capabilities

- `contract-generation`: инструменты закрепляются удалёнными плагинами и
  `buf.lock`, а не локальной сборкой; внешние proto берутся из реестра;
  воспроизводимость и проверка совместимости описываются без вендоринга.

## Impact

- `api/`: удаляются `third_party/`, `materials/`, `scripts/`; меняются
  `buf.yaml`, `buf.gen.*.yaml`; добавляется `buf.lock`.
- `backend/go.mod`: удаляются `tool`-директивы плагинов; ожидается починка
  интеграционных тестов (проверяется в реализации).
- `backend/gen`, `api/src/gen`, `api/gen/openapi`: перегенерация; ожидаемо
  меняются только баннеры версий и комментарии `validate.proto`.
- `.github`, README, CI (карточка #8): команды проверки упрощаются.
- Сеть: первый `buf dep update`/`buf generate` требует доступа к `buf.build`;
  запасной путь документируется, но не обязателен.
