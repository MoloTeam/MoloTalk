# Веб-клиент MoloTalk

Каркас SPA по issue #7: Vite, React, TypeScript. Клиент не требует запущенных
backend-сервисов: API и WebSocket подставляются на границе портов.

## Требования

- Node.js ≥ 24.11.1
- npm ≥ 11

## Команды

Репозиторий использует npm workspaces `api` и `web`: зависимости ставятся один раз из корня, общий lock-файл лежит там же.

```bash
npm ci            # из корня репозитория
npm run dev       # http://127.0.0.1:5173
npm run lint
npm run typecheck
npm run test
npm run build     # dist/
```

Те же команды работают из каталога `web`, установка всё равно выполняется из корня.

## Структура

| Каталог | Ответственность |
|---|---|
| `src/app` | точка сборки, провайдер экземпляра, маршруты |
| `src/features/access` | вход, профиль, экран «Страница не найдена» |
| `src/features/dialogs` | список диалогов и выбор |
| `src/features/composer` | основа редактора |
| `src/features/timeline` | история и статусы |
| `src/sync` | порты API/WebSocket, действия, производственные адаптеры |
| `src/state` | фабрика store и начальное состояние |

## Маршруты

`/`, `/login`, `/profile`, `/chats`, `/chats/:chatId`. Неизвестный путь в dev
показывает 404-экран. Прямой вход по маршруту работает за счёт history fallback
Vite; в собранном виде его обеспечивает Caddy.

## Транспорт

- `src/sync/ports.ts` — интерфейсы `ApiPort` и `SocketPort`; UI о них не знает.
- `src/sync/connectApi.ts` — Connect-клиент поверх публичных дескрипторов
  `api/src/gen` (алиас `@api`), метод `GetMe`.
- `src/sync/gatewaySocket.ts` — бинарный `GatewayEnvelope` по `/ws`: доставляет `Ready`, `HeartbeatAck` и `ServerEvent`, сообщает об открытии и закрытии (код и причина без интерпретации), а кадры `Identify`/`Heartbeat` от сервера, пустую оболочку и повреждённый кадр отдаёт как ошибку протокола. Схема
  `ws`/`wss` из origin приложения, повреждённый кадр не выдаётся как событие.
- `src/app/createApp.ts` — принимает независимые порты и создаёт экземпляр
  приложения со своим store.

Внутренние RPC (`molotalk.internal.v1`) в клиент не попадают: TypeScript
генерируется только для публичного пакета.

## Проверки

`npm run test` — Vitest (jsdom): изоляция store, действие чтения, обработка
события и снятие подписки, выбор диалога, маршруты с одним `chatId`, адаптеры
Connect и Gateway на тестовых транспортах.
