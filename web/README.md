# Веб-клиент MoloTalk

Каркас SPA по issue #7: Vite, React, TypeScript. Клиент не требует запущенных
backend-сервисов: API и WebSocket подставляются на границе портов.

## Требования

- Node.js ≥ 24.11.1
- npm ≥ 11

## Команды

```bash
npm ci                              # зависимости веб-клиента
npm --prefix ../api ci --omit=dev   # зависимости сгенерированных контрактов
npm run dev       # http://127.0.0.1:5173
npm run lint
npm run typecheck
npm run test
npm run build     # dist/
```

Сгенерированные дескрипторы лежат в `api/src/gen` и импортируют
`@bufbuild/protobuf`; TypeScript и Vite ищут его в `api/node_modules`, поэтому
зависимости api ставятся отдельно. Если их нет, `typecheck`, `test` и `build`
завершаются с подсказкой о команде установки.

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
- `src/sync/gatewaySocket.ts` — бинарный `GatewayEnvelope` по `/ws`, схема
  `ws`/`wss` из origin приложения, повреждённый кадр не выдаётся как событие.
- `src/app/createApp.ts` — принимает независимые порты и создаёт экземпляр
  приложения со своим store.

Внутренние RPC (`molotalk.internal.v1`) в клиент не попадают: TypeScript
генерируется только для публичного пакета.

## Проверки

`npm run test` — Vitest (jsdom): изоляция store, действие чтения, обработка
события и снятие подписки, выбор диалога, маршруты с одним `chatId`, адаптеры
Connect и Gateway на тестовых транспортах.
