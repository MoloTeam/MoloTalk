# Сверка с исходным OpenAPI HLD

Источник: `api/materials/openapi.json` (SHA-256 `17eedbb8627022554792728a4bb36afea4b8b535932ef6ef1a3c5b93d2a1bfeb`).
Результат: `api/gen/openapi/public.json` и `api/gen/openapi/internal.json`.

Сверка выполнена один раз вручную; дальше действует golden и обычный diff.

## Состав методов

| Сервис | Методов в HLD | В контракте | Пути |
|---|---:|---:|---|
| AuthService | 8 | 8 | сохранены |
| ChatService | 16 | 16 | сохранены |
| MediaService | 4 | 4 | сохранены |
| PresenceService | 1 | 1 | сохранён |
| **Итого внешних** | **29** | **29** | `/molotalk.v1.<Service>/<Method>` |
| AuthInternalService | 1 | 1 | новый `/molotalk.internal.v1.AuthInternalService/IntrospectSession` |
| ChatInternalService | 2 | 2 | новые `/molotalk.internal.v1.ChatInternalService/...` |
| PresenceInternalService | 2 | 2 | новые `/molotalk.internal.v1.PresenceInternalService/...` |

Пять внутренних методов (`AuthService/IntrospectSession`, `ChatService/CheckChatAccess`,
`ChatService/GetUserChatIds`, `PresenceService/Heartbeat`, `PresenceService/Disconnect`)
перенесены из публичного пакета; внешние 29 путей не изменились.

## Рассмотренные отличия

| Отличие | Где видно | Объяснение |
|---|---|---|
| Имена схем стали полными (`molotalk.v1.SendMessageRequest`) | `$ref` в public.json | Правило плагина; DTO не пропали |
| Enum-значения с префиксом (`CHAT_KIND_GROUP`, `MEMBER_ROLE_ADMIN`) | схемы enum | D9, настоящие proto-enum |
| `sizeBytes` — JSON-number int32 (было строкой) | `PrepareUploadRequest`, `Attachment` | D9, границы 0…20971520 |
| `Timestamp`/`Empty` — WKT | даты и пустые ответы | D9 |
| Обязательные массивы помечены `REQUIRED`, допускают `[]` | `required` в схемах | D3, документированное исключение |
| `optional`-поля могут допускать `null` в схеме | `UpdateProfileRequest.clearAvatar` и др. | Ограничение генератора; runtime protovalidate строже, ответы `null` не выдают |
| Произвольный CEL не исполняется схемой | `ReceiptUpdated` oneof | Ограничение генератора; исполнитель — protovalidate |
| Security: `bearerAuth` и `serviceBearer` из base-файлов | `components.securitySchemes` | Регистрация/вход/refresh имеют `security: []` |

## Проверки

```
public paths:   29
internal paths: 5
Login.security:   []            Register/RefreshToken: []
GetMe.security:   null (наследует bearerAuth)
internal:         serviceBearer
text.maxLength:   4096
attachmentIds:    maxItems 5, items.pattern ULID
```

Необъяснённых пропусков методов, полей и обязательности не найдено. Все отличия
перечислены в таблице отклонений design.md (D9).

## Обязательность скалярных полей

Generated OpenAPI попадает в `required` только для полей с явной аннотацией:
`google.api.field_behavior = REQUIRED` (обязательные массивы) и
`buf.validate.field.required` (сообщения и optional-поля). HLD размечал
`required` шире — на 33 поля в 10 DTO и на поля запросов (`LoginRequest`,
`SendMessageRequest` и другие).

Решение: обязательность скалярных полей обеспечивается правилами protovalidate
(`pattern`, `min_len`, `gte`), а не JSON Schema. Пустая строка или нулевой
идентификатор отклоняются на стороне сервера до вызова метода; тесты
`validation_rules_test.go` проверяют границы. Массовая расстановка
`field_behavior = REQUIRED` изменила бы generated-клиенты, поэтому выполняется
отдельным решением, а не в рамках первой сверки. Расхождение перечислено в
таблице отклонений design.md (D9).