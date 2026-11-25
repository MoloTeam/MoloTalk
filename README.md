# MoloTalk

Веб-мессенджер для личного и группового текстового общения.


## Возможности (первая версия)

| Модуль | Что умеет |
|---|---|
| Auth | Регистрация, вход, выход |
| Profile | Смена имени, аватара (JPG/PNG до 5 МБ) и пароля |
| Msg | Текстовые сообщения, статусы «Доставлено»/«Прочитано», редактирование и удаление |
| Group | Группы до 100 участников, роли владелец/администратор/участник, передача владения |
| History | История с постраничной подгрузкой, синхронизация между сеансами |
| Presence | Статус «в сети» / «не в сети» |
| Media | Вложения: JPG/PNG/GIF, PDF/DOCX, до 20 МБ, до 5 файлов в сообщении |


## Технологический стек

| Область | Технология |
|---|---|
| Клиент | React, TypeScript |
| Сервер | Go (сервисы Auth, Chat, Media, Presence, Gateway) |
| Вход | Caddy (HTTPS, маршрутизация) |
| API | ConnectRPC + JSON; события клиенту — WebSocket + Protobuf |
| Данные | PostgreSQL |
| Присутствие | Redis |
| Файлы | MinIO (S3-совместимое) |
| События между сервисами | NATS Core |
| Окружение | Docker Compose |


## Процесс разработки

- **Модель ветвления: GitHub Flow.** Единственная долгоживущая ветка — `main`, она защищена.
- Ветка на каждую карточку: `<тип>/<номер-issue>-<кратко>`, например `feat/12-send-message`, `fix/30-group-limit`.
  Типы: `feat`, `fix`, `refactor`, `test`, `docs`, `ci`, `chore`.
- Коммиты и заголовки PR — по [Conventional Commits](https://www.conventionalcommits.org/ru/v1.0.0/):
  `feat(chat): отправка текстового сообщения (F_Msg_1)`.
- **Прямые коммиты в `main` запрещены.** Все изменения идут через pull request.
- Каждый PR одобряет минимум один **другой** участник; каждый участник бывает и автором, и ревьюером.
- В описании PR обязательны идентификаторы требований FS и разделы HLD.
- Задачи ведутся на доске **GitHub Projects**: Backlog → Ready → In progress → In review → Done.
  Карточка = модуль + 2–5 связанных требований FS + раздел HLD.

## Локальный запуск

### Сборка backend в контейнере

Backend собирается многостадийным [`backend/Dockerfile`](backend/Dockerfile). Локальный Go для сборки не требуется: компиляция выполняется в образе builder.

Предпосылки:

- Docker с Compose v2;
- доступ к реестру образов для `golang:1.27-alpine` и `alpine:3.22`;
- доступ контейнера сборки к прокси Go-модулей (`proxy.golang.org`) при первой сборке.

Сборка отдельного процесса из корня репозитория:

```bash
docker build --target auth -t molotalk/auth:dev -f backend/Dockerfile .
```

Доступные цели: `auth`, `chat`, `media`, `presence`, `gateway`. Ошибка компиляции завершает сборку ненулевым кодом, и вызывающая команда видит сообщение компилятора; окружение при этом не запускается.

### Запуск окружения

Каноническая команда выполняется из корня репозитория:

```bash
docker compose --project-directory . --env-file .env.example -f deploy/docker-compose.yml up --build --wait --wait-timeout 180
```

Для личных значений используйте отдельный файл: `--env-file .env`. Переменные окружения оболочки имеют приоритет над файлом, поэтому значение из shell переопределяет значение из `.env`.

| Что | Адрес |
|---|---|
| Вход приложения (Caddy, HTTPS) | `https://localhost`, порт из `HTTPS_PORT` (по умолчанию `443`) |
| Прямой S3-доступ к MinIO | `http://127.0.0.1:{MINIO_S3_PORT}` (по умолчанию `9000`) |
| backend, PostgreSQL, Redis, NATS | только внутри сети Compose, на хост не публикуются |

Оба внешних адреса привязаны к loopback. Если порт `443` занят, например системным `wslrelay`, задайте другой: `HTTPS_PORT=8443`.

### Веб-клиент

Веб-клиент живёт в `web/` и собирается на Vite, React и TypeScript. Нужны Node ≥ 24.11 и npm ≥ 11.

```bash
cd web
npm ci
npm --prefix ../api ci --omit=dev   # зависимости сгенерированных контрактов
npm run dev       # http://127.0.0.1:5173, каркас работает без backend
npm run lint
npm run typecheck
npm run test
npm run build     # dist/ с готовой раздачей SPA
```

Файлы `api/src/gen` импортируют `@bufbuild/protobuf`, и TypeScript ищет его в
`api/node_modules`, поэтому вторая команда обязательна; без неё `typecheck`,
`test` и `build` завершатся с подсказкой.

В dev известные маршруты (`/`, `/login`, `/profile`, `/chats`, `/chats/:chatId`) открываются напрямую благодаря history fallback, неизвестный путь показывает экран «Страница не найдена». Сетевые вызовы в dev не требуются: API и WebSocket подставляются точкой сборки.

Собранный клиент попадает в образ Caddy (`deploy/caddy.Dockerfile`) и отдаётся по тому же HTTPS-адресу, что и RPC. Правила маршрутизации RPC и `/ws` применяются до выдачи SPA, поэтому HTML не подменяет ошибки API.

### Миграции схемы

Схема применяется автоматически: одноразовая служба `migrate` ждёт готовности PostgreSQL, выполняет версии из `backend/migrations/` и завершается, после чего стартуют пять backend-процессов. Фикстура автоматически не загружается.

```bash
# состояние службы миграций
docker compose --project-directory . --env-file .env.example -f deploy/docker-compose.yml ps --all migrate

# повторное применение вручную (например, после изменения встроенных миграций)
docker compose --project-directory . --env-file .env.example -f deploy/docker-compose.yml run --rm migrate
```

Версии хранятся в `goose_db_version`; повторный запуск пропускает выполненные миграции и не трогает данные. Ошибка миграции возвращает ненулевой код и блокирует запуск backend. Диагностика содержит этап и категорию ошибки без строки подключения и пароля.

### Демонстрационные данные (fixture)

Fixture загружается только явно и в отдельную демонстрационную БД с собственным volume. Обычный `deploy/docker-compose.yml` службу `fixtures` не содержит.

```bash
# отдельный проект: PostgreSQL и применённые миграции
docker compose -p molotalk-demo --env-file .env.example -f deploy/fixtures.compose.yml up -d postgres
docker compose -p molotalk-demo --env-file .env.example -f deploy/fixtures.compose.yml run --rm migrate

# явная загрузка набора basic
docker compose -p molotalk-demo --env-file .env.example -f deploy/fixtures.compose.yml run --rm fixtures
```

Набор `basic` создаёт только `users`, `channels` и `channel_members`: Alice, Bob, Carol, личный диалог и группу. Записи `user_auth`, сеансов и остальных таблиц не создаются. Повторный запуск возвращает ошибку `application tables are not empty` и не меняет данные; сброс выполняется удалением отдельного volume.

Состав и фиксированные значения набора описаны в [backend/cmd/fixtures/README.md](backend/cmd/fixtures/README.md).

### Интеграционные проверки

```bash
cd backend
go test -tags=integration -count=1 ./...
```

Для интеграционного прогона нужен работающий Docker: тесты поднимают PostgreSQL 17 в Testcontainers, применяют реальные миграции и выделяют отдельную БД каждому сценарию. При недоступном Docker прогон завершается явной ошибкой.

Обычный `go test ./...` контейнеры не поднимает: интеграционные файлы помечены тегом `integration`.

На Windows с Docker Desktop может потребоваться явный адрес демона, если он не совпадает с значением по умолчанию:

```powershell
$env:DOCKER_HOST = "npipe:////./pipe/dockerDesktopLinuxEngine"
```

### Доверие локальному удостоверяющему центру

Caddy обслуживает `localhost` сертификатом своего локального CA; закрытый ключ остаётся в volume `caddy-data`. Публичный сертификат CA извлекается из запущенного контейнера:

```powershell
$caddy = (docker compose --project-directory . --env-file .env.example -f deploy/docker-compose.yml ps -q caddy).Trim()
docker cp "${caddy}:/data/pki/authorities/local/root.crt" "$env:USERPROFILE\caddy-root.crt"
```

Команда разбита на два шага намеренно: PowerShell 5.1 неверно передаёт `docker cp` строку вида `"$(...)":/path`, и Docker сообщает `'docker cp' requires 2 arguments`. Сертификат сохраняется вне репозитория, чтобы не попасть в коммит.

Установка доверия на Windows (текущему пользователю, без прав администратора):

```powershell
certutil -addstore -User Root "$env:USERPROFILE\caddy-root.crt"
```

На других платформах сертификат добавляется в системное или браузерное хранилище доверенных корней. Доверие нужно установить один раз; сборка и запуск контейнеров от него не зависят.

Проверка TLS с явным CA и без отключения проверки сертификата:

```bash
curl.exe --ssl-no-revoke --cacert "$env:USERPROFILE\caddy-root.crt" https://localhost/caddy-healthz
```

Если `HTTPS_PORT` отличается от `443`, укажите порт в адресе: `https://localhost:8443/caddy-healthz`.

`--ssl-no-revoke` нужен для `curl.exe` в Windows: он использует Schannel, который требует проверить отзыв сертификата, а локальный CA Caddy не публикует CRL или OCSP, поэтому проверка отзыва невозможна (`CRYPT_E_NO_REVOCATION_CHECK`). Цепочка и имя хоста при этом по-прежнему проверяются. После установки CA в хранилище пользователя флаг `--cacert` можно не указывать.

### Диагностика

```bash
docker compose --project-directory . --env-file .env.example -f deploy/docker-compose.yml ps
docker compose --project-directory . --env-file .env.example -f deploy/docker-compose.yml logs --tail 50 minio-init
docker compose --project-directory . --env-file .env.example -f deploy/docker-compose.yml logs --tail 50 auth
```

Если `minio-init` завершился с ненулевым кодом, backend не запускается: проверьте `MINIO_ROOT_USER`, `MINIO_ROOT_PASSWORD` и `MINIO_BUCKET`. Если инфраструктурный контейнер остаётся `unhealthy`, команда `up --wait` завершается отказом, а причина видна в `ps` и `logs` этого контейнера.

### Проверка маршрутизации и отказов

Маршруты проверяются в изолированном контуре: вместо backend работают тестовые upstream из `deploy/tests/mock`, которые возвращают полученные путь, тело и `X-Request-Id`.

```bash
docker compose -p molotalk-routing -f deploy/tests/docker-compose.yml up --build --abort-on-container-exit --exit-code-from verify
```

Контур проверяет все разрешённые RPC, запрещённые и соседние пути, а также обмен контрольным кадром по `/ws`. Он поднимает собственный Caddy со своим CA и не требует запущенного основного окружения.

Отказы порядка запуска воспроизводятся отдельными override-файлами:

```bash
# инфраструктура не проходит проверку готовности
docker compose -p molotalk-neg --project-directory . --env-file .env.example -f deploy/docker-compose.yml -f deploy/tests/failing-postgres.override.yml up -d --wait

# подготовка bucket получает неверные credentials
docker compose -p molotalk-neg --project-directory . --env-file .env.example -f deploy/docker-compose.yml -f deploy/tests/failing-init.override.yml up -d --wait
```

Обе команды завершаются ненулевым кодом, а backend-процессы не запускаются.

### Остановка, повторный запуск и очистка

```bash
# остановка с сохранением данных (volumes остаются)
docker compose --project-directory . --env-file .env.example -f deploy/docker-compose.yml down

# повторный запуск без пересборки
docker compose --project-directory . --env-file .env.example -f deploy/docker-compose.yml up -d --wait --wait-timeout 180

# явная очистка данных выбранного окружения
docker compose --project-directory . --env-file .env.example -f deploy/docker-compose.yml down -v
```

`down -v` удаляет volumes PostgreSQL, MinIO и данных Caddy, включая выпущенный CA. Выполняйте его только когда данные действительно нужно удалить.

### Текущие ограничения этапа

- `GET /` и клиентские маршруты отдают сборку веб-клиента; отсутствующий ресурс возвращает `404`.
- Разрешённые RPC доходят до backend, но обработчиков ещё нет, поэтому возвращается `404`.
- `/ws` проксируется, но обработчик WebSocket появится в карточке Gateway.
- `/readyz` отвечает `503`: набор проверок пуст до T-32, `/healthz` уже возвращает `200`.
- Схема применяется миграциями при запуске; демонстрационная fixture загружается только явно.

## Definition of Done

Карточка переводится в «Done», когда выполнены **все** пункты:

1. Код влит в `main` через pull request с одобрением другого участника.
2. Реализованы все требования FS, указанные в карточке.
3. Проект собирается, приложение запускается.
4. Изменения не ломают ранее реализованную функциональность (CI зелёный).
5. Для логики добавлены модульные тесты.
