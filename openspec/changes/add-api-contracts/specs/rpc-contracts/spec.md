# RPC Contracts

## Purpose

Определяет состав RPC, типы и проверяемые форматные ограничения публичных и внутренних вызовов. Основание — HLD §5.1–5.7 с явно перечисленными в design решениями владельца об изменении контракта.

## ADDED Requirements

### Requirement: Разделение публичных и внутренних сервисов

Контракт SHALL содержать 29 внешних методов в molotalk.v1 и 5 внутренних в molotalk.internal.v1. TypeScript-клиенты SHALL генерироваться только из публичного пакета. Внешние пути SHALL сохраняться. Внутренний пакет SHALL NOT импортироваться публичным.

#### Scenario: Структурная граница

- **GIVEN** определения сервисов в двух пакетах
- **WHEN** строятся дескрипторы Go и публичные дескрипторы TypeScript
- **THEN** публичный пакет содержит 29 методов, внутренний — 5, TS не содержит внутренних сервисов и публичные определения не зависят от внутренних

#### Scenario: Каталог методов

- **GIVEN** определения RPC
- **WHEN** выполняется одноразовая сверка с каталогом HLD
- **THEN** методы сохранены с приведённым распределением, а внешние пути имеют прежний вид `/molotalk.v1.<Service>/<Method>`

| Пакет | Сервис | Методы |
|---|---|---|
| molotalk.v1 | AuthService | Register, Login, RefreshToken, Logout, GetMe, UpdateProfile, ChangePassword, SearchUsers |
| molotalk.v1 | ChatService | ListChats, GetChat, CreateDirectChat, CreateGroupChat, AddGroupMember, RemoveGroupMember, LeaveGroup, SetGroupMemberRole, SendMessage, EditMessage, DeleteMessage, GetMessages, ConfirmDelivery, MarkRead, GetMessageReceipts, GetReadStates |
| molotalk.v1 | MediaService | PrepareUpload, CompleteUpload, GetAttachment, GetDownloadURL |
| molotalk.v1 | PresenceService | GetPresence |
| molotalk.internal.v1 | AuthInternalService | IntrospectSession |
| molotalk.internal.v1 | ChatInternalService | CheckChatAccess, GetUserChatIds |
| molotalk.internal.v1 | PresenceInternalService | Heartbeat, Disconnect |

### Requirement: Единые proto-типы

Enum SHALL иметь стандартные имена Buf, нулевое UNSPECIFIED и проверку допустимых значений. Обязательный enum SHALL отвергать ноль и неизвестное значение. Даты SHALL использовать google.protobuf.Timestamp, пустые сообщения — google.protobuf.Empty, ограниченные числа — int32 согласно аудиту design. Id SHALL быть строкой с ULID-шаблоном схемы #5.

#### Scenario: Enum принимает только содержательное определённое значение

- **GIVEN** обязательный Chat.kind
- **WHEN** protovalidate проверяет CHAT_KIND_GROUP, CHAT_KIND_UNSPECIFIED и неизвестный номер enum
- **THEN** первый допустим, остальные отвергаются, JSON успешного DTO содержит CHAT_KIND_GROUP

#### Scenario: Стандартные дата и пустой результат

- **GIVEN** Timestamp с известным временем и пустой результат RPC
- **WHEN** Go и TS выполняют сериализацию
- **THEN** дата выдаётся UTC RFC3339, эквивалентный допустимый UTC offset читается как то же время, Empty выдаётся как `{}`

#### Scenario: Размер файла не требует строкового числа

- **GIVEN** PrepareUpload с размером 20971520 и отдельный запрос с 20971521
- **WHEN** сообщения проходят protovalidate и JSON-сериализацию
- **THEN** первый размер укладывается в общий предел вложения и передаётся JSON-числом int32, второй отвергается; ограничение аватара 5242880 проверяется отдельно по purpose

### Requirement: Presence и значения по умолчанию

Контракт SHALL сохранять отсутствие optional-поля и отличать его от явного false/0. Ответы SHALL явно выдавать обязательные false, 0 и пустые массивы, не подставляя null для отсутствующих optional-полей. Допускающий [] repeated SHALL NOT получать правило required, запрещающее пустой массив.

#### Scenario: Частичное обновление

- **GIVEN** UpdateProfile без clearAvatar и отдельный запрос с clearAvatar false
- **WHEN** Go и TS сериализуют и разбирают запросы
- **THEN** первый сохраняет отсутствие поля, второй сохраняет явное false

#### Scenario: Пустой успешный набор

- **GIVEN** ответ с пустым attachments, attachmentsLinked false и нулевыми счётчиками
- **WHEN** сгенерированный тестовый handler возвращает ответ
- **THEN** JSON содержит обязательные [], false и 0, проходит контрактную проверку, а отсутствующие optional-поля не превращаются в null

### Requirement: Декларативные форматные ограничения

Ограничения SHALL задаваться в proto через buf.validate и исполняться protovalidate. Identifier SHALL иметь длину 1–254 и нижний регистр, display_name — 1–64, название группы — 1–100, пароль — 6–64, текст — до 4096 Unicode-символов. Id SHALL соответствовать `^[0-7][0-9A-HJKMNP-TV-Z]{25}$`. Состав пароля SHALL оставаться доменным правилом Auth.

#### Scenario: Границы текста и длины полей

- **GIVEN** тексты из 4096/4097 символов вне BMP, поля displayName длиной 0/1/64/65, group name 0/1/100/101, password 5/6/64/65 и identifier 0/1/254/255
- **WHEN** выполняется protovalidate
- **THEN** граничные значения внутри указанных диапазонов допустимы, остальные отвергаются с путём поля, а длина Unicode не считается по числу UTF-16 code units

#### Scenario: Регистр identifier и формат Id

- **GIVEN** identifier с нижним регистром ASCII/кириллицы, identifier с прописной буквой, корректный ULID и строка из 26 символов с недопустимой первой цифрой или буквой I
- **WHEN** выполняется protovalidate
- **THEN** принимаются нижний регистр и корректный ULID, остальные варианты отвергаются

#### Scenario: История и вложения

- **GIVEN** GetMessages с limit 1/100/0/101 и без limit, SearchUsers с 50/51, запросы с пятью и шестью attachmentIds
- **WHEN** выполняется protovalidate и проверяется описание контракта
- **THEN** допустимы 1–100 и отсутствие limit, предел поиска равен 50, вложений не более пяти, каждый attachmentId проходит ULID-правило; в описании отсутствие limit означает 50 для истории и 20 для поиска

### Requirement: Авторизация в описании API

Generated OpenAPI SHALL описывать bearerAuth для публичных пользовательских RPC и serviceBearer для внутренних. Register, Login и RefreshToken SHALL иметь security без access token. Определение security SHALL NOT считаться реализацией авторизации.

#### Scenario: Security по пакетам и исключения

- **GIVEN** публичный и внутренний OpenAPI, сгенерированные со штатными base-файлами
- **WHEN** проверяется итоговый golden
- **THEN** публичные пользовательские операции наследуют bearerAuth, три метода без access token имеют security [], а внутренний документ использует serviceBearer

### Requirement: Стандартная ошибка и ранняя валидация

Нарушение формата SHALL давать Connect invalid_argument со штатными типизированными protovalidate violations до вызова прикладного метода. Непредвиденная ошибка SHALL давать безопасный internal. Коды доменных ошибок SHALL сохраняться. Секреты SHALL отсутствовать в ответах и журнале.

#### Scenario: Валидатор останавливает некорректный запрос

- **GIVEN** тестовый handler с готовым validation-interceptor и счётчиком вызовов
- **WHEN** запрос содержит text длиной 4097
- **THEN** оба клиента получают invalid_argument с указанием text, а прикладной тестовый метод не вызван

#### Scenario: Неизвестная ошибка зависимости

- **GIVEN** тестовая зависимость возвращает ошибку с контрольным секретом
- **WHEN** общий адаптер формирует ответ
- **THEN** code равен internal, X-Request-Id сохранён, секрет и исходная ошибка не попали в message, details и журнал

#### Scenario: Доменные коды не теряются

- **GIVEN** тестовые реализации возвращают failed_precondition, unauthenticated, permission_denied, not_found, already_exists и resource_exhausted
- **WHEN** ответы читают Go- и TS-клиенты
- **THEN** коды сохранены, ошибки не заменены успешным пустым набором

### Requirement: Unary Connect и HTTP-корреляция

RPC SHALL использовать POST, JSON, Connect-Protocol-Version 1, успешный HTTP 200 и тело без дополнительной RPC-оболочки. Транспорт SHALL сохранять действующий контракт X-Request-Id на успехе и ошибке. Бизнес-обработчики не требуются для контрактного стенда.

#### Scenario: Успех и повреждённое тело

- **GIVEN** локальный сгенерированный handler с тестовой реализацией и действующей HTTP-корреляцией
- **WHEN** клиенты отправляют допустимый запрос и отдельный запрос с повреждённым JSON, используя X-Request-Id abc-123
- **THEN** первый получает типизированный 200 без оболочки, второй — ошибку Connect до вызова метода, оба ответа содержат abc-123
