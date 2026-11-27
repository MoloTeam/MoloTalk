# Контракты API

Proto — единственный источник типов и правил. Go, TypeScript и OpenAPI —
производные файлы, они коммитятся как golden и проверяются повторной
генерацией.

## Структура

```text
api/
  proto/molotalk/v1/            публичные RPC и Gateway
  proto/molotalk/internal/v1/   внутренние RPC
  buf.yaml                      модуль и зависимости (deps)
  buf.lock                      закреплённые коммиты зависимостей
  buf.gen.go.yaml               Go и Connect
  buf.gen.ts.yaml               TypeScript публичного пакета
  buf.gen.openapi.*.yaml        OpenAPI public/internal с base-файлами
  openapi/base.public.yaml      security и исключения без access token
  openapi/base.internal.yaml    служебный Bearer
  openapi/comparison.md         однократная сверка и отклонения
  gen/openapi/public.json       golden публичного OpenAPI
  gen/openapi/internal.json     golden внутреннего OpenAPI
  src/gen/                      TypeScript публичного пакета и зависимостей
```

## Внешние зависимости

Импортируемые схемы (`buf/validate/validate.proto`,
`google/api/field_behavior.proto`) берутся из реестра: они перечислены в `deps`
файла [`buf.yaml`](buf.yaml), а точные коммиты и digest — в [`buf.lock`](buf.lock).
Каталога `third_party/` нет. Обновление зависимости: `buf dep update` меняет
только `buf.lock`.

## Инструменты

| Инструмент | Версия | Лицензия |
|---|---|---|
| `buf` CLI (устанавливается разработчиком) | v1.75.0 | Apache-2.0 |
| удалённый `buf.build/protocolbuffers/go` | v1.36.12 | BSD-3-Clause |
| удалённый `buf.build/connectrpc/go` | v2.0.2 | Apache-2.0 |
| удалённый `buf.build/bufbuild/es` | 2.17.0 | Apache-2.0 |
| локальный `go run connectrpc.com/protoc-gen-connect-openapi` | v0.30.0 | MIT |
| `buf.build/go/protovalidate` (runtime Go) | v1.4.1 | Apache-2.0 |
| `connectrpc.com/connect/v2` (runtime Go) | v2.0.2 | Apache-2.0 |
| `@bufbuild/protobuf`, `@connectrpc/connect*` | 2.17.0 / 2.3.0 | Apache-2.0 |

Плагины Go, Connect и TypeScript — удалённые, их версии закреплены в шаблонах.
OpenAPI собирается локальным запуском закреплённой версии плагина: удалённый
плагин не поддерживает `base`. `protoc` и Docker не требуются.

## Команды

`buf` ставится один раз:

```bash
go install github.com/bufbuild/buf/cmd/buf@v1.75.0
```

Генерация из каталога `api/`:

```bash
buf generate --template buf.gen.go.yaml
buf generate --template buf.gen.ts.yaml --path proto/molotalk/v1
buf generate --template buf.gen.openapi.public.yaml --path proto/molotalk/v1
buf generate --template buf.gen.openapi.internal.yaml --path proto/molotalk/internal/v1
```

Первый запуск требует доступа к `buf.build`; дальше работает кеш. Запасной путь
при недоступном реестре — заранее заполненный кеш `buf` (необязательно).

Проверка после генерации: те же команды плюс `git diff --exit-code` (golden не
переписывается автоматически), затем `go build/vet/test` в `backend` и
`npm --workspace api test`.

## Проверка совместимости

`buf breaking` сравнивает модули, поэтому база должна иметь ту же модульную
структуру, что и рабочая версия. После слияния этого change:

```bash
buf breaking --against '.git#branch=main,subdir=api'
```

Во время перехода (в `main` ещё два модуля: `proto` и `third_party/proto`) база
нормализуется: выгружается `api/proto` из `main`, рядом кладутся текущие
`buf.yaml` и `buf.lock`, сравнение идёт с этим каталогом. Несовпадение числа
модулей считается ошибкой конфигурации, а не «нет baseline».


## Примеры использования

Go-клиент:

```go
client := connect.NewClient(connecthttp.NewTransport(http.DefaultClient, "https://localhost"))
auth := molotalkv1connect.NewAuthServiceClient(client)
tokens, err := auth.Login(ctx, &molotalkv1.LoginRequest{Identifier: "alice@example.com", Password: "secret1"})
```

Сервер с общей валидацией:

```go
server := rpc.NewServer()
molotalkv1connect.RegisterAuthServiceHandler(server, handler)
connecthttp.Mount(mux, server)
```

TypeScript-клиент (только публичный пакет):

```ts
import { createClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import { AuthService } from "./gen/molotalk/v1/auth_pb.js";

const transport = createConnectTransport({ baseUrl: "https://localhost" });
const auth = createClient(AuthService, transport);
const tokens = await auth.login({ identifier: "alice@example.com", password: "secret1" });
```

## Wire-формат Gateway (срез C)

Один protobuf-кадр `GatewayEnvelope` на кадр WebSocket, без JSON-обёртки и поля
`sequence`.

- Heartbeat отправляется клиентом каждые 25 секунд, сервер отвечает
  `HeartbeatAck`; при отсутствии кадров 60 секунд соединение закрывается.
- Повторный `Identify` на том же соединении заменяет подписку и возвращает новый
  `Ready`.
- После `Ready` клиент перечитывает историю и состояния прочтения: replay не
  гарантируется.
- Коды закрытия: `4001` — неавторизован, `4003` — нет доступа, `4008` —
  превышено время ожидания кадров.
- `ServerEvent` содержит `event_id`, `occurred_at` и ровно одно событие;
  `ReceiptUpdated` содержит сводку для автора либо собственное подтверждение
  читателя.

Контрольные кадры с фиксированным временем описаны в тестах
`backend/internal/platform/rpc/gateway_contract_test.go`.

## Сверка и golden

Однократная сверка с материалом HLD зафиксирована в
[`openapi/comparison.md`](openapi/comparison.md). Дальше действует golden:
изменение proto или base-файла требует обновить generated-файлы в том же PR,
иначе `git diff` после `buf generate` не пуст.

Повторяющийся ULID-шаблон заменён предопределённым правилом
`(buf.validate.field).string.(molotalk.v1.ulid)`. Protovalidate исполняет его в
рантайме, но генератор OpenAPI не выводит CEL в схему, поэтому `pattern` для
ULID-полей в `gen/openapi/*.json` отсутствует. Это документированное отклонение
(см. `comparison.md`); остальные `pattern` сохранены.
