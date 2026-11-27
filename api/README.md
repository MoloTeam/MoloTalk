# Контракты API

Proto — единственный источник типов и правил. Go, TypeScript и OpenAPI —
производные файлы, они коммитятся как golden и проверяются повторной
генерацией.

## Структура

```text
api/
  proto/molotalk/v1/            публичные RPC и Gateway
  proto/molotalk/internal/v1/   внутренние RPC
  third_party/proto/            закоммиченные внешние proto (buf.validate, google.api)
  openapi/base.public.yaml      security и исключения без access token
  openapi/base.internal.yaml    служебный Bearer
  openapi/comparison.md         однократная сверка с материалом HLD
  gen/openapi/public.json       golden публичного OpenAPI
  gen/openapi/internal.json     golden внутреннего OpenAPI
  src/gen/                      TypeScript публичного пакета и зависимостей
```

## Внешние зависимости

Реестр Buf в окружении недоступен, поэтому внешние proto закоммичены в
[`third_party/`](third_party/README.md) с закреплённым происхождением, лицензиями
и SHA-256. `buf.lock` не используется: удалённых зависимостей нет.

## Инструменты

| Инструмент | Версия | Лицензия |
|---|---|---|
| `github.com/bufbuild/buf/cmd/buf` | v1.75.0 | Apache-2.0 |
| `google.golang.org/protobuf/cmd/protoc-gen-go` | v1.36.13-0.20261016172727-e9ee2a76ab8e | BSD-3-Clause |
| `connectrpc.com/connect/v2/cmd/protoc-gen-connect-go` | v2.0.2 | Apache-2.0 |
| `connectrpc.com/protoc-gen-connect-openapi` | v0.30.0 | MIT |
| `buf.build/go/protovalidate` | v1.4.1 | Apache-2.0 |
| `connectrpc.com/connect/v2` | v2.0.2 | Apache-2.0 |
| `@bufbuild/protoc-gen-es`, `@bufbuild/protobuf` | 2.17.0 | Apache-2.0 |
| `@bufbuild/protovalidate` | 1.3.0 | Apache-2.0 |
| `@connectrpc/connect`, `@connectrpc/connect-web` | 2.3.0 | Apache-2.0 |

Go-инструменты закреплены директивами `tool` в `backend/go.mod`, JS — точными
версиями и `package-lock.json`. Глобальная установка плагинов, `protoc` и Docker
не требуются.

## Команды

Из корня репозитория:

```bash
npm ci
npm run generate
npm run check
```

JS-зависимости общие с веб-клиентом и ставятся из корня через workspaces;
каталог `api` отдельного lock-файла не имеет.

Генерация ограничена каталогами контрактов: Go получает оба пакета, TypeScript —
только `proto/molotalk/v1` (и импортируемые им внешние схемы), OpenAPI собирается
отдельно для публичного и внутреннего пакетов.

`generate` собирает `buf` и плагины во временный каталог и прогоняет единую последовательность шаблонов; отдельной команды на каждый шаблон больше нет.

`check` собирает `buf` и плагины во временный каталог, проверяет
`buf format/lint/build`, генерирует артефакты в отдельный каталог и сравнивает их
с отслеживаемыми файлами, затем выполняет `go build/vet/test` и typecheck.
Отслеживаемые файлы не изменяются: устаревший артефакт даёт ненулевой код.
До первого слияния контрактов в `main` проверка `buf breaking` явно сообщает об
отсутствии baseline.

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
иначе `check` завершается ошибкой.
