# Design

## Context

[Proposal](proposal.md) фиксирует границы issue #5 и сверку SQL с обновлённым отчётом. Единственный нормативный источник схемы — `docs/hld/schema.sql`. Каталог в этом design сгенерирован из SQL. Одобренные CHECK и индексы включены в целевую схему. U06 исключён; исходник сохраняется неизменённой копией, две целевые поправки выполняются в миграции.

В окружении #4 используется PostgreSQL 17, backend ещё не подключает SQL-адаптеры. Существующая дельта `environment-initialization` заменяет временный запуск без схемы на автоматические миграции.

## Goals / Non-Goals

**Goals**

- Использовать один механизм миграций в Compose и интеграционных тестах.
- Получать изолированную БД для каждого независимого сценария.
- Сохранять данные при повторном запуске и безопасно диагностировать ошибки.

**Non-Goals**

- Прикладные RPC, наборы для входа через Auth и настройка CI относятся к отдельным карточкам.

## Decisions

### Источник и структура миграций

Миграции формируются из `docs/hld/schema.sql` после сравнения SHA-256 с proposal.md. При несовпадении реализация останавливается. U06 не переносится в миграцию, а C12 заменяется на `char_length(text) <= 4096`. Начальная транзакционная миграция сохраняет порядок исходного SQL с отложенным FK владельца группы. Дальнейшие изменения оформляются новыми версиями.

Для сообщения используется `char_length(text) <= 4096`. Значение 4000 в буквальном снимке SQL и обновлённом отчёте заменено явным решением владельца. Тестируются 4096 и 4097 символов, включая многобайтовые символы.

### Инструменты и лицензии

| Модуль | Версия | Лицензия | Назначение |
|---|---|---|---|
| github.com/pressly/goose/v3 | v3.26.0 | MIT | Provider и PostgreSQL SessionLocker |
| github.com/jackc/pgx/v5 | v5.7.6 | MIT | stdlib-драйвер database/sql |
| github.com/testcontainers/testcontainers-go | v0.38.0 | MIT | Жизненный цикл контейнеров |
| github.com/testcontainers/testcontainers-go/modules/postgres | v0.38.0 | MIT основного репозитория | PostgreSQL и ожидание готовности |

Это фиксированные версии предложения с требуемыми API. Совместимость итогового графа зависимостей проверяется при реализации сборкой и тестами. В рамках ревью go.mod и go.sum не изменяются.

[Goose API](https://github.com/pressly/goose/blob/v3.26.0/provider_options.go), [Goose MIT](https://github.com/pressly/goose/blob/v3.26.0/LICENSE), [pgx MIT](https://github.com/jackc/pgx/blob/v5.7.6/LICENSE), [Testcontainers MIT](https://github.com/testcontainers/testcontainers-go/blob/v0.38.0/LICENSE).

### Один механизм выполнения

Команда `backend/cmd/migrate` читает `DATABASE_URL` и вызывает общий механизм `backend/internal/platform/migrations` с контекстом и явным подключением. PostgreSQL подключается через `database/sql` и `github.com/jackc/pgx/v5/stdlib`.

SQL находится в `backend/migrations/` и встраивается через `embed.FS`. `fs.Sub` задаёт корень файлов для `goose.NewProvider`. Provider получает диалект PostgreSQL и `goose.WithSessionLocker` с locker из `lock.NewPostgresSessionLocker`. Блокировка привязана к выделенной SQL-сессии. История ведётся в `goose_db_version`. Глобальные настройки пакета Goose не используются.

Каждая миграция начальной схемы выполняется транзакционно. При ошибке текущая версия откатывается, ранее успешные версии сохраняются. Конкурентные запуски сериализуются. Общий механизм вызывается той же командой в Compose и непосредственно интеграционными тестами, поэтому тесты проверяют путь рабочей подготовки.

### Диагностика без секретов

Ошибки разбора DSN, подключения, Ping и SQL классифицируются до вывода. Сообщение и журнал содержат безопасную категорию, этап и при наличии номер версии. Исходные ошибки драйвера могут содержать DSN, поэтому целиком в вывод не передаются.

Тест запускает migrate с уникальным контрольным секретом в ошибочном DATABASE_URL, затем с корректным DSN недоступной БД. Проверяются ненулевой код, ограниченное время ожидания и отсутствие контрольного пароля и полной строки подключения в stdout/stderr и журнале.

### Загрузчик и данные basic

Общий загрузчик и данные располагаются в `backend/internal/fixtures`, поскольку их импортирует команда `backend/cmd/fixtures`. Basic записывает только `users`, `channels`, `channel_members`. Идентификаторы и времена фиксированы. Alice владеет группой через `channels.owner_id` и имеет MEMBER в составе, Bob — ADMIN, Carol — MEMBER. Личный диалог содержит Alice и Bob.

Basic не создаёт строки `user_auth`, хеши паролей или сеансы. Набор для входа демо-аккаунтами добавит карточка Auth. Пароли в репозитории не хранятся.

Загрузчик получает явное подключение, проверяет пустоту всех таблиц прикладной схемы кроме `goose_db_version` и вставляет набор одной транзакцией. Проверка охватывает и таблицы, которые basic не заполняет. Непустое состояние вызывает ошибку до записи. Связь группы с владельцем проверяется при COMMIT.


### Детерминированные значения basic

Время created_at и joined_at — `2026-10-19T09:00:00Z`. Идентификаторы фиксированы.

| Объект | ID |
|---|---|
| Alice | `01ARZ3NDEKTSV4RRFFQ69G5FA1` |
| Bob | `01ARZ3NDEKTSV4RRFFQ69G5FA2` |
| Carol | `01ARZ3NDEKTSV4RRFFQ69G5FA3` |
| Личный диалог | `01ARZ3NDEKTSV4RRFFQ69G5FA4` |
| Группа | `01ARZ3NDEKTSV4RRFFQ69G5FA5` |

| Канал | created_by_id | client_request_id | create_request_hash |
|---|---|---|---|
| DIRECT Alice–Bob | `01ARZ3NDEKTSV4RRFFQ69G5FA1` | NULL | NULL |
| GROUP | `01ARZ3NDEKTSV4RRFFQ69G5FA1` | `01ARZ3NDEKTSV4RRFFQ69G5FA6` | `f001ef788b1ad462a54cb64d2f21d25277db6acf58c57f6e3930f4528f7f126c` |

Значение группы — SHA-256 UTF-8 строки `molotalk/basic/group/v1`, записанный lowercase hex. Это детерминированная метка fixture. Реальный формат `create_request_hash` и канонизацию рабочей команды определит карточка Chat; basic не претендует на совпадение с ним. Для DIRECT NULL в двух полях соответствует C09. creator задан явно и одинаков в независимых прогонах.

### Документация cmd/fixtures

Планируемый README команды должен перечислить три заполняемые таблицы, отсутствие строк user_auth и сеансов, роли, все фиксированные значения таблиц выше, явный выбор целевой БД и отказ на непустом состоянии. В README включается явное примечание: «Формат create_request_hash рабочей команды определит карточка Chat; значение basic является детерминированной меткой и не претендует на совпадение с ним». Создание README относится к реализации, текущий change содержит его требования. Авторизация не относится к basic. Отдельный будущий набор Auth описывается при его появлении.

Образ fixtures собирается отдельной стадией Dockerfile. Для явного запуска используется отдельный `deploy/fixtures.compose.yml` и команда `compose run --rm`. Обычный `deploy/docker-compose.yml` не содержит службы fixtures. README показывает работу с отдельным Compose-проектом и собственным volume.

### Testcontainers for Go

Интеграционные проверки помечаются `integration` и запускаются из backend командой `go test -tags=integration -count=1 ./...`. Обычный `go test ./...` доступен без запуска контейнеров. Для запрошенного integration-прогона отсутствие Docker является явной ошибкой.

Один PostgreSQL-контейнер обслуживает набор тестов. Версия образа согласуется с Compose, начальный ориентир — `postgres:17-alpine`. После ожидания SQL-подключения каждый независимый сценарий получает новую БД с уникальным именем. Тесты применяют реальные миграции общим Provider, затем нужную fixture. Два конкурентных мигратора намеренно используют одну выделенную БД.

Очистка регистрируется сразу после выделения ресурсов. Сначала закрываются соединения, затем удаляется БД сценария, после набора удаляется контейнер. Очистка выполняется и при ошибке подготовки или теста. DSN с динамическим портом выдаёт Testcontainers. Рабочая БД Compose не участвует в этих проверках.

[PostgreSQL-модуль Testcontainers](https://golang.testcontainers.org/modules/postgres/).

### Приёмка SQL-целостности

Каталог ниже получен программным разбором SQL. Одобрены все перечисленные объекты, кроме U06. C12 применяется в варианте `char_length(text) <= 4096`.

Проверяются согласованный состав схемы, повторный и конкурентный запуск, откат ошибочной версии, уникальность пользователей, пары DIRECT и ключа сообщения, ссылки на пользователей и каналы, отложенный FK владельца, роли, revision и связь read/delivered. Отрицательные операции выполняются в независимых транзакциях. Проверяются SQLSTATE и состояние БД, отложенный FK — при COMMIT.


### Golden-файл pg_catalog

При реализации добавляется `backend/internal/platform/migrations/testdata/schema.golden.json`. Эталон строится по `docs/hld/schema.sql` с двумя зафиксированными поправками: удалить U06, заменить C12. Первичная генерация выполняется в отдельной эталонной БД из источника, а не из проверяемых миграций. В тестах golden читается без автоматического обновления из результата текущего прогона.

Интроспекция использует pg_namespace, pg_class, pg_attribute, pg_type, pg_constraint, pg_index и pg_am, а определения — pg_get_constraintdef, pg_get_indexdef и pg_get_expr. Фиксируются прикладная схема, порядок и типы столбцов, NOT NULL и DEFAULT, домен и его CHECK, PK/UNIQUE/FK/CHECK, признак отложенного FK, ключи и методы индексов, частичные предикаты. Объекты сортируются по стабильным именам и порядку столбцов. OID, owner, статистика и прочие значения конкретного экземпляра исключаются. Служебная goose_db_version и её объекты отделяются от прикладной схемы.

Эталон содержит 10 таблиц, домен, 22 CHECK включая домен, 10 PK, 7 UNIQUE, 15 FK и 9 явных индексов. PK/UNIQUE имеют свои обслуживающие индексы, которые интроспекция связывает с ограничениями. Индекс U06 отсутствует. Фиксированная версия PostgreSQL сохраняет стабильный вид выражений.

### Проверка отсутствия зависимости от U06

Программный разбор всех REFERENCES исходника нашёл 15 FK. Единственная ссылка на messages — `message_receipts.message_id → messages(id)`. Ссылок на пару `(channel_id,id)` нет. Приёмка повторяет это на pg_constraint через confrelid/confkey и pg_attribute. Первичный ключ messages(id) сохраняет требуемую ссылочную целостность.

### Подготовка в Compose

Dockerfile получает стадии migrate и fixtures. Одноразовая служба migrate ждёт готовности PostgreSQL, пять backend-процессов — успешного завершения migrate. Зависимости bucket и инфраструктуры сохраняются. Автоматический запуск fixture отсутствует.

При изменении встроенных миграций служба migrate пересоздаётся документированным способом. В отдельном Compose-проекте проверяются первый запуск, повторное применение и блокировка первого запуска backend при ошибке задания. Проверка готовности backend остаётся через `/healthz` до T-32.

## Risks / Trade-offs

- Исходник схемы изменился → задача 1.1 сравнивает SHA-256 с proposal.md и останавливается при несовпадении.
- Случайно перенесён U06 → golden и отдельная проверка pg_constraint подтверждают его отсутствие.
- Тесты разделяют данные → отдельная БД на сценарий и заранее зарегистрированная очистка.
- Ошибка подключения раскрывает DSN → безопасная классификация и проверки с контрольным секретом.
- Повторный запуск fixture мог бы изменить ручные данные → отказ на любых непустых прикладных таблицах.

## Migration Plan

На новой БД применяется начальная версия. Повторный запуск сохраняет данные и историю версий. Для ранее созданной вручную прикладной схемы мигратор сообщает конфликт, автоматическая отметка миграций выполненными не производится. Исправления применённой схемы оформляются новой миграцией. Очистка отдельного тестового окружения выполняется явно.

## Open Questions

Отсутствуют.

## Каталог объектов исходного SQL

Каталог перенесён из спецификации. Выражения воспроизводят исходный файл. Решения владельца задают целевые отклонения U06 и C12.

### Таблицы и столбцы

| Таблица | Столбцы из файла |
|---|---|
| `users` | `id`, `identifier`, `display_name`, `avatar_attachment_id`, `created_at` |
| `user_auth` | `user_id`, `password_hash`, `password_changed_at` |
| `sessions` | `id`, `user_id`, `created_at`, `expires_at`, `revoked_at` |
| `refresh_tokens` | `token_hash`, `session_id`, `expires_at`, `consumed_at` |
| `login_attempt_windows` | `identifier_hash`, `failed_count`, `window_started_at` |
| `channels` | `id`, `created_by_id`, `client_request_id`, `create_request_hash`, `kind`, `name`, `owner_id`, `direct_pair_key`, `created_at`, `closed_at` |
| `channel_members` | `channel_id`, `user_id`, `role`, `joined_at` |
| `messages` | `id`, `channel_id`, `author_id`, `client_message_id`, `text`, `attachment_ids`, `revision`, `created_at`, `edited_at`, `deleted_at`, `request_hash` |
| `message_receipts` | `message_id`, `user_id`, `delivered_at`, `read_at` |
| `attachments` | `id`, `uploader_id`, `purpose`, `status`, `object_key`, `temp_object_key`, `thumbnail_key`, `filename`, `content_type`, `size_bytes`, `channel_id`, `linked_at`, `created_at`, `upload_expires_at`, `completed_at` |

### Домен

| ID | Объект | Выражение из файла | Решение |
|---|---|---|---|
| D01 | `ulid` | `CREATE DOMAIN ulid AS char(26) CHECK (VALUE ~ '^[0-7][0-9A-HJKMNP-TV-Z]{25}$')` | Сохранить |

### PRIMARY KEY

| ID | Объект | Выражение из файла | Решение |
|---|---|---|---|
| P01 | `users` | `PRIMARY KEY (id)` | Сохранить |
| P02 | `user_auth` | `PRIMARY KEY (user_id)` | Сохранить |
| P03 | `sessions` | `PRIMARY KEY (id)` | Сохранить |
| P04 | `refresh_tokens` | `PRIMARY KEY (token_hash)` | Сохранить |
| P05 | `login_attempt_windows` | `PRIMARY KEY (identifier_hash)` | Сохранить |
| P06 | `channels` | `PRIMARY KEY (id)` | Сохранить |
| P07 | `channel_members` | `PRIMARY KEY (channel_id,user_id)` | Сохранить |
| P08 | `messages` | `PRIMARY KEY (id)` | Сохранить |
| P09 | `message_receipts` | `PRIMARY KEY (message_id,user_id)` | Сохранить |
| P10 | `attachments` | `PRIMARY KEY (id)` | Сохранить |

### CHECK

| ID | Объект | Выражение из файла | Решение |
|---|---|---|---|
| C01 | `ulid` | `CHECK (VALUE ~ '^[0-7][0-9A-HJKMNP-TV-Z]{25}$')` | Одобрено владельцем |
| C02 | `users.identifier` | `CHECK (length(identifier) BETWEEN 1 AND 254 AND identifier=lower(identifier))` | Одобрено владельцем |
| C03 | `users.display_name` | `CHECK (length(display_name) BETWEEN 1 AND 64 AND display_name ~ '[^[:space:]]')` | Сохранить |
| C04 | `sessions` | `CHECK (expires_at > created_at)` | Одобрено владельцем |
| C05 | `login_attempt_windows.identifier_hash` | `CHECK (identifier_hash ~ '^[0-9a-f]{64}$')` | Сохранить |
| C06 | `login_attempt_windows.failed_count` | `CHECK (failed_count BETWEEN 1 AND 5)` | Сохранить |
| C07 | `channels.kind` | `CHECK (kind IN ('DIRECT','GROUP'))` | Сохранить |
| C08 | `channels.name` | `CHECK (length(name) BETWEEN 1 AND 100)` | Одобрено владельцем |
| C09 | `channels` | `CHECK ((kind='DIRECT' AND client_request_id IS NULL AND create_request_hash IS NULL) OR (kind='GROUP' AND client_request_id IS NOT NULL AND create_request_hash IS NOT NULL))` | Сохранить |
| C10 | `channels` | `CHECK ((kind='DIRECT' AND owner_id IS NULL AND direct_pair_key IS NOT NULL) OR (kind='GROUP' AND direct_pair_key IS NULL AND (closed_at IS NOT NULL OR (owner_id IS NOT NULL AND name IS NOT NULL AND name ~ '[^[:space:]]'))))` | Сохранить |
| C11 | `channel_members.role` | `CHECK (role IN ('ADMIN','MEMBER'))` | Сохранить |
| C12 | `messages.text` | `CHECK (length(text) <= 4000)` | Заменено решением владельца на char_length(text) <= 4096 |
| C13 | `messages.revision` | `CHECK (revision >= 1)` | Сохранить |
| C14 | `messages` | `CHECK (cardinality(attachment_ids) <= 5)` | Сохранить |
| C15 | `messages` | `CHECK (deleted_at IS NOT NULL OR text ~ '[^[:space:]]' OR cardinality(attachment_ids)>0)` | Одобрено владельцем |
| C16 | `message_receipts` | `CHECK (read_at IS NULL OR (delivered_at IS NOT NULL AND read_at >= delivered_at))` | Сохранить |
| C17 | `attachments.purpose` | `CHECK (purpose IN ('AVATAR','MESSAGE_ATTACHMENT'))` | Одобрено владельцем |
| C18 | `attachments.status` | `CHECK (status IN ('UPLOADING','VALIDATING','READY','REJECTED','DELETING'))` | Одобрено владельцем |
| C19 | `attachments.size_bytes` | `CHECK (size_bytes >= 0)` | Одобрено владельцем |
| C20 | `attachments` | `CHECK (purpose <> 'AVATAR' OR size_bytes <= 5242880)` | Сохранить |
| C21 | `attachments` | `CHECK (purpose <> 'MESSAGE_ATTACHMENT' OR size_bytes <= 20971520)` | Сохранить |
| C22 | `attachments` | `CHECK ((channel_id IS NULL) = (linked_at IS NULL))` | Сохранить |

### UNIQUE

| ID | Объект | Выражение из файла | Решение |
|---|---|---|---|
| U01 | `users` | `UNIQUE (identifier)` | Сохранить |
| U02 | `users` | `UNIQUE (avatar_attachment_id)` | Сохранить |
| U03 | `channels` | `UNIQUE (direct_pair_key)` | Сохранить |
| U04 | `channels` | `UNIQUE (created_by_id,client_request_id)` | Сохранить |
| U05 | `messages` | `UNIQUE (channel_id,author_id,client_message_id)` | Сохранить |
| U06 | `messages` | `UNIQUE (channel_id,id)` | Отклонено владельцем; исключить из миграции |
| U07 | `attachments` | `UNIQUE (object_key)` | Сохранить |
| U08 | `attachments` | `UNIQUE (temp_object_key)` | Сохранить |

### FOREIGN KEY

| ID | Объект | Выражение из файла | Решение |
|---|---|---|---|
| F01 | `user_auth.user_id` | `REFERENCES users(id)` | Сохранить |
| F02 | `sessions.user_id` | `REFERENCES users(id)` | Сохранить |
| F03 | `refresh_tokens.session_id` | `REFERENCES sessions(id)` | Сохранить |
| F04 | `channels.created_by_id` | `REFERENCES users(id)` | Сохранить |
| F05 | `channels.owner_id` | `REFERENCES users(id)` | Сохранить |
| F06 | `channel_members.channel_id` | `REFERENCES channels(id)` | Сохранить |
| F07 | `channel_members.user_id` | `REFERENCES users(id)` | Сохранить |
| F08 | `channels.group_owner_is_member` | `FOREIGN KEY (id,owner_id) REFERENCES channel_members(channel_id,user_id) DEFERRABLE INITIALLY DEFERRED` | Сохранить |
| F09 | `messages.channel_id` | `REFERENCES channels(id)` | Сохранить |
| F10 | `messages.author_id` | `REFERENCES users(id)` | Сохранить |
| F11 | `message_receipts.message_id` | `REFERENCES messages(id)` | Сохранить |
| F12 | `message_receipts.user_id` | `REFERENCES users(id)` | Сохранить |
| F13 | `attachments.uploader_id` | `REFERENCES users(id)` | Сохранить |
| F14 | `attachments.channel_id` | `REFERENCES channels(id)` | Сохранить |
| F15 | `users.user_avatar_fk` | `FOREIGN KEY (avatar_attachment_id) REFERENCES attachments(id)` | Сохранить |

### Явные индексы

| ID | Объект | Выражение из файла | Решение |
|---|---|---|---|
| I01 | `member_user_idx` | `CREATE INDEX member_user_idx ON channel_members(user_id,channel_id)` | Сохранить |
| I02 | `sessions_user_idx` | `CREATE INDEX sessions_user_idx ON sessions(user_id)` | Одобрено владельцем |
| I03 | `refresh_session_idx` | `CREATE INDEX refresh_session_idx ON refresh_tokens(session_id)` | Одобрено владельцем |
| I04 | `history_idx` | `CREATE INDEX history_idx ON messages(channel_id,id DESC)` | Сохранить |
| I05 | `attachment_reference_idx` | `CREATE INDEX attachment_reference_idx ON messages USING gin(attachment_ids)` | Сохранить |
| I06 | `receipt_user_idx` | `CREATE INDEX receipt_user_idx ON message_receipts(user_id,message_id)` | Сохранить |
| I07 | `attachment_cleanup_idx` | `CREATE INDEX attachment_cleanup_idx ON attachments(status,created_at) WHERE linked_at IS NULL` | Одобрено владельцем |
| I08 | `receipt_unread_idx` | `CREATE INDEX receipt_unread_idx ON message_receipts(user_id,message_id) WHERE read_at IS NULL` | Сохранить |
| I09 | `login_window_cleanup_idx` | `CREATE INDEX login_window_cleanup_idx ON login_attempt_windows(window_started_at)` | Одобрено владельцем |

Итого — 10 таблиц, 1 домен, 22 CHECK (включая домен), 10 PRIMARY KEY, 8 UNIQUE в исходнике (7 в миграции), 15 FK и 9 явных индексов. PK и UNIQUE создают собственные уникальные индексы PostgreSQL. Служебная таблица Goose не входит в исходный SQL.
