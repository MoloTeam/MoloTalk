# Gateway Contracts

## Purpose

Определяет типизированные protobuf-кадры и аудитории событий WebSocket по утверждённому владельцем варианту B. Продуктовая семантика HLD §5.8 сохраняется, изменения wire-формата перечислены в design.

## ADDED Requirements

### Requirement: Типизированная оболочка Gateway

GatewayEnvelope SHALL содержать обязательный oneof payload из Identify, Ready, Heartbeat, HeartbeatAck и ServerEvent. Один бинарный WebSocket-кадр SHALL содержать одну оболочку. Payload SHALL быть типизированным protobuf-сообщением без вторичного JSON-кодирования. Identify SHALL содержать token, Ready — userId, chatIds, readStates. Токен SHALL NOT передаваться в URL.

#### Scenario: Типизированное кодирование служебных кадров

- **GIVEN** созданы фиксированные сообщения пяти типов
- **WHEN** Go кодирует их для TS, а TS кодирует их для Go
- **THEN** case и значения сохраняются, Ready допускает пустые chatIds/readStates и использует общую модель ReadState

#### Scenario: Пустая или повреждённая оболочка

- **GIVEN** подготовлены Envelope без payload и обрезанные protobuf-байты
- **WHEN** выполняются разбор и штатная валидация
- **THEN** отсутствие payload отвергается protovalidate, повреждённые байты дают ошибку разбора, событие не выдаётся

### Requirement: Типизированные события ServerEvent

ServerEvent SHALL содержать eventId, occurredAt и обязательный oneof события. MessageCreated/Updated SHALL использовать Message, ChatChanged — Chat, PresenceChanged — Presence. MessageDeleted SHALL содержать chatId/messageId/int32 revision, MembershipChanged — chatId/userId. ProfileChanged SHALL использовать отдельный публичный профиль без identifier.

#### Scenario: Полный каталог событий

- **GIVEN** созданы MessageCreated, MessageUpdated, MessageDeleted, ReceiptUpdated, ChatChanged, MembershipChanged, ProfileChanged, PresenceChanged
- **WHEN** выполняются двунаправленное декодирование и protovalidate
- **THEN** все восемь case известны, значения и Timestamp сохраняются, отсутствующий обязательный ULID и revision 0 отвергаются

#### Scenario: Публичный профиль не раскрывает identifier

- **GIVEN** сгенерирован дескриптор ProfileChanged
- **WHEN** проверяется его модель публичного профиля
- **THEN** она содержит id/displayName/optional avatarAttachmentId и не имеет поля identifier, при этом DTO GetMe сохраняет собственный identifier

### Requirement: Содержание ReceiptUpdated

ReceiptUpdated SHALL содержать обязательные chatId/messageId и optional receiptSummary/receipt. Ограничение protovalidate SHALL разрешать ровно одно содержательное поле. Автор SHALL получать receiptSummary, сеансы читателя — только его собственный receipt. Полный групповой список SHALL NOT передаваться; персональные отметки доступны через GetMessageReceipts.

#### Scenario: Взаимоисключающие поля

- **GIVEN** четыре сообщения со сводкой, подтверждением, обоими полями и без обоих
- **WHEN** выполняется protovalidate
- **THEN** первые два допустимы, последние два отвергаются штатным правилом message.oneof required

#### Scenario: Персональное подтверждение

- **GIVEN** примеры события для автора и сеансов читателя
- **WHEN** проверяются их контрактные payload и описание аудитории
- **THEN** пример автора содержит только receiptSummary, пример читателя — только receipt с его userId, ни один не содержит группового списка; применение аудитории в работающей системе остаётся задачей Gateway

### Requirement: Семантика соединения сохраняется

Контракт SHALL сохранять повторный Identify для того же сеанса, heartbeat по умолчанию раз в 25 секунд, тайм-аут дольше 60 секунд, коды закрытия 4001/4003/4008 и повторное чтение API после Ready. EventId SHALL служить отбрасыванию дубликатов, revision — защите от устаревшего обновления. Replay и гарантированная доставка SHALL NOT обещаться.

#### Scenario: Закрытие и последующее действие

- **GIVEN** истёкший access token, отозванный сеанс и отсутствие heartbeat дольше 60 секунд
- **WHEN** проверяются примеры протокола
- **THEN** причины сопоставлены 4001/4003/4008, действия — refresh и переподключение, переход ко входу, переподключение и чтение текущего состояния соответственно

#### Scenario: Дубликат и устаревшая revision

- **GIVEN** повторный eventId и MessageUpdated со старой revision
- **WHEN** проверяется описанный контракт обработки
- **THEN** они не заменяют новое состояние, а документация не использует их для обещания replay
