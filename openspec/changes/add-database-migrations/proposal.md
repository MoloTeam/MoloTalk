# Proposal

## Why

[Issue #5](https://github.com/MoloTeam/MoloTalk/issues/5) подготавливает PostgreSQL для следующих модулей MoloTalk. Миграции и фиксированные данные обеспечат воспроизводимые интеграционные проверки.

## What Changes

- Оформить `docs/hld/schema.sql` как миграции Goose с embed.FS и PostgreSQL SessionLocker.
- Создать все объекты исходника с двумя принятыми изменениями: `char_length(text) <= 4096` и исключение U06 `UNIQUE messages(channel_id,id)`.
- Проверять схему через golden-файл нормализованной интроспекции pg_catalog.
- Добавить basic только для users, channels и channel_members с фиксированными ID, временем и полями создания чатов.
- Применять миграции до backend в Compose. Fixture запускается явно в отдельной БД.
- Проверять миграции, ограничения и изоляцию через Testcontainers for Go.

## Capabilities

### New Capabilities

- `database-migrations` — версии схемы, целостность, golden-проверка и безопасная диагностика.
- `test-fixtures` — фиксированные данные и изоляция проверок.

### Modified Capabilities

- `environment-initialization` — ожидание миграций до запуска backend.

## Impact

Единственный источник — `docs/hld/schema.sql`. SHA-256 точных байтов файла, по которому построена сверка: `CD4201662F63B9D76BA4CEBA4C418F8B4274C03D57DD9D7968DC031ABA71ADCE`. Репозиторный файл добавляется как неизменённая копия этого исходника. Задача 1.1 сравнивает его хеш с указанным значением и останавливает реализацию при расхождении.

В исходнике 10 таблиц, 1 домен, 22 CHECK включая домен, 10 PK, 8 UNIQUE, 15 FK и 9 явных индексов. В миграции остаются 7 UNIQUE; C12 заменяется на `char_length(text) <= 4096`. Каталог и нормализация golden находятся в [design.md](design.md).

Будущая реализация затронет миграции, команды migrate/fixtures, `backend/internal/fixtures`, Dockerfile, Compose, README и интеграционные тесты. Прикладные RPC и CI относятся к своим карточкам.

## Решения владельца

- Одобрены C01, C02, C04, C08, C15, C17, C18, C19 и индексы sessions_user_idx, refresh_session_idx, attachment_cleanup_idx, login_window_cleanup_idx.
- U06 отклонён как избыточный. Программный разбор исходника обнаружил 15 FK и единственную ссылку на messages — `message_receipts.message_id → messages(id)`. Ссылок на `messages(channel_id,id)` нет.
- Basic не содержит user_auth, хешей паролей или сеансов. Набор для входа добавит Auth. Пароли в репозитории не хранятся.
- Значение create_request_hash группы в basic — детерминированная метка fixture. Формат отпечатка рабочей команды определит Chat.
- Прикладные таблицы — все таблицы схемы кроме goose_db_version. Загрузчик расположен в backend/internal/fixtures и вызывается явно.

## Открытые вопросы

Отсутствуют.
