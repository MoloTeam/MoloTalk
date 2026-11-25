# ingress-routing Specification

## Purpose
Определяет единый локальный HTTPS-вход и маршруты Caddy. Основание — HLD §3.3, правила внешнего доступа §5.1, каталоги API §5.3–5.6 и транспорт WebSocket §5.8.

## Requirements

### Requirement: Локальный HTTPS-origin

Caddy SHALL предоставлять HTTPS-origin приложения на `localhost` с портом из окружения и сертификатом локального CA. README SHALL описывать извлечение и установку доверия CA на хосте как отдельный шаг. Сборка и запуск контейнеров SHALL выполняться независимо от установки доверия браузера.

#### Scenario: Проверка HTTPS с явным доверием CA

- **GIVEN** окружение запущено, сертификат CA получен по инструкции README
- **WHEN** проверочный клиент обращается к Caddy с этим CA без отключения проверки TLS
- **THEN** TLS-проверка имени `localhost` и цепочки сертификата проходит

### Requirement: Разрешённые внешние RPC

Caddy SHALL направлять `POST` по точным путям `/molotalk.v1.<Service>/<Method>` из списка ниже в соответствующий backend, сохраняя путь и тело запроса. Имена методов SHALL соответствовать HLD §5.3–5.6.

#### Scenario: Все разрешённые методы достигают своего upstream

- **GIVEN** для Auth, Chat, Media и Presence подключены изолированные тестовые upstream, записывающие полученные запросы
- **WHEN** по HTTPS отправляют `POST` для каждого метода из следующего списка с контрольным телом
- **THEN** запрос достигает только указанного сервиса, а путь и тело совпадают с исходными

| Service | Method |
|---|---|
| AuthService | Register, Login, RefreshToken, Logout, GetMe, UpdateProfile, ChangePassword, SearchUsers |
| ChatService | ListChats, GetChat, CreateDirectChat, CreateGroupChat, AddGroupMember, RemoveGroupMember, LeaveGroup, SetGroupMemberRole, SendMessage, EditMessage, DeleteMessage, GetMessages, ConfirmDelivery, MarkRead, GetMessageReceipts, GetReadStates |
| MediaService | PrepareUpload, CompleteUpload, GetAttachment, GetDownloadURL |
| PresenceService | GetPresence |

### Requirement: Внутренние и неизвестные методы закрыты

По HLD §5.1 Caddy SHALL отклонять внешние обращения к `AuthService/IntrospectSession`, `ChatService/CheckChatAccess`, `ChatService/GetUserChatIds`, `PresenceService/Heartbeat` и `PresenceService/Disconnect`. Неизвестные RPC-пути SHALL отклоняться. Отклонённый запрос SHALL получать `404` без передачи backend.

#### Scenario: Запрещённый метод не достигает backend

- **GIVEN** тестовые upstream записывают обращения
- **WHEN** извне отправляют `POST` по каждому запрещённому пути с префиксом `/molotalk.v1.`, а затем по `/molotalk.v1.AuthService/UnknownMethod`
- **THEN** каждый ответ имеет код `404`, а ни один upstream не получает эти запросы

#### Scenario: Соседний путь не совпадает с разрешённым

- **GIVEN** маршрут `/molotalk.v1.AuthService/Login` разрешён
- **WHEN** отправляют `POST` по путям `/molotalk.v1.AuthService/LoginExtra` и `/molotalk.v1.AuthService/Login/extra`
- **THEN** оба запроса получают `404` без передачи backend

### Requirement: Проксирование WebSocket по точному пути

Caddy SHALL направлять WebSocket Upgrade по `/ws` в Gateway и поддерживать двусторонний обмен после успешного Upgrade upstream. Префиксные совпадения вроде `/ws/extra` SHALL отклоняться. Контракт соответствует HLD §5.8.

#### Scenario: Двусторонний обмен с тестовым upstream

- **GIVEN** в изолированной проверке Gateway заменён тестовым WebSocket upstream
- **WHEN** клиент устанавливает `wss://localhost:<порт>/ws` и отправляет контрольный кадр
- **THEN** Upgrade проходит, upstream получает кадр, а его ответ достигает клиента

#### Scenario: Непредусмотренный WebSocket-путь закрыт

- **GIVEN** тестовый Gateway регистрирует входящие обращения
- **WHEN** клиент запрашивает Upgrade по `/ws/extra`
- **THEN** Caddy возвращает `404`, Gateway не получает обращение

### Requirement: Корреляция проходит через входной прокси

Caddy SHALL сохранять допустимый `X-Request-Id` запроса и итоговый `X-Request-Id` ответа upstream. Backend продолжает выбирать итоговый ID по своему контракту HTTP-корреляции из HLD §5.1 и §6.5.

#### Scenario: Идентификатор сохраняется в обоих направлениях

- **GIVEN** тестовый upstream для разрешённого RPC возвращает полученный `X-Request-Id`
- **WHEN** клиент обращается к Caddy с `X-Request-Id: abc-123`
- **THEN** upstream получает `abc-123`, а клиент видит это же значение в ответе

### Requirement: Выдача веб-клиента через единый вход

Caddy SHALL отдавать сборку SPA по GET /, известным клиентским маршрутам и путям её статических ресурсов. RPC и WebSocket SHALL сохранять прежние правила маршрутизации. Отсутствующий статический ресурс SHALL возвращать 404.

#### Scenario: Загрузка и обновление клиентского маршрута

- **GIVEN** Caddy запущен с собранным веб-клиентом и локальному CA доверяют
- **WHEN** браузер запрашивает GET /, открывает известный маршрут диалога напрямую и запрашивает JS из страницы
- **THEN** корень и маршрут диалога возвращают HTML с HTTP 200, ресурс отдаётся со своим типом содержимого

#### Scenario: HTML SPA не подменяет API, WebSocket и отсутствующий ресурс

- **GIVEN** клиентские файлы и тестовые upstream доступны
- **WHEN** проверяются разрешённый RPC, Upgrade по /ws, неизвестный или внутренний RPC, /ws/extra и отсутствующий JS
- **THEN** разрешённый RPC и Upgrade доходят до своих upstream, остальные запросы получают 404 без HTML SPA
