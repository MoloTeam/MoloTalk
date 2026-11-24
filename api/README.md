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
  gen/openapi/public.json       golden публичного OpenAPI
  gen/openapi/internal.json     golden внутреннего OpenAPI
  src/gen/                      TypeScript только публичного пакета
```

`proto/materials/` содержит снимки исходных материалов HLD, по которым
построен контракт: `openapi.json` (`17eedbb8…`) и `gateway.proto` (`f90e96ae…`).

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
| `connectrpc.com/validate` | v0.9.0 | Apache-2.0 |
| `@bufbuild/protoc-gen-es`, `@bufbuild/protobuf` | 2.17.0 | Apache-2.0 |
| `@bufbuild/protovalidate` | 1.3.0 | Apache-2.0 |
| `@connectrpc/connect`, `@connectrpc/connect-web` | 2.3.0 | Apache-2.0 |

Go-инструменты закреплены директивами `tool` в `backend/go.mod`, JS — точными
версиями и `package-lock.json`. Глобальная установка плагинов, `protoc` и Docker
не требуются.

## Команды

Из корня репозитория:

```bash
npm --prefix api ci
npm --prefix api run generate
npm --prefix api run check
```

Генерация ограничена каталогами контрактов: Go получает оба пакета, TypeScript —
только `proto/molotalk/v1`, OpenAPI собирается отдельно для публичного и
внутреннего пакетов.

Обновление golden входит в ревью изменения proto или base-файла и не выполняется
командой `check` автоматически.
