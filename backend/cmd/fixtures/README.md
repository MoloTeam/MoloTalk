# Команда fixtures

Явная загрузка детерминированного набора данных в подготовленную схему. Обычный
запуск Compose команду не вызывает: набор загружается отдельной командой в
выбранную БД.

```bash
docker compose -p molotalk-demo --env-file .env.example -f deploy/fixtures.compose.yml run --rm fixtures
```

## Выбор набора и целевой БД

- `-set` — имя набора, обязателен. Сейчас доступен `basic`.
- `-database-url` или `DATABASE_URL` — строка подключения к БД с применёнными
  миграциями, обязательна.

Загрузчик проверяет пустоту **всех** прикладных таблиц схемы, кроме служебной
`goose_db_version`, и пишет набор одной транзакцией. Непустое состояние —
ошибка до записи; автоматического сброса данных нет. Сброс выполняется удалением
отдельной БД или её volume.

## Состав набора basic

Заполняются только `users`, `channels` и `channel_members`. Таблицы `user_auth`,
`sessions`, `refresh_tokens`, `login_attempt_windows`, `messages`, `attachments`
и `message_receipts` остаются пустыми: паролей, хешей и сеансов набор не
содержит.

| Объект | ID | Данные |
|---|---|---|
| Alice | `01ARZ3NDEKTSV4RRFFQ69G5FA1` | `alice@example.com`, `Alice` |
| Bob | `01ARZ3NDEKTSV4RRFFQ69G5FA2` | `bob@example.com`, `Bob` |
| Carol | `01ARZ3NDEKTSV4RRFFQ69G5FA3` | `carol@example.com`, `Carol` |
| Личный диалог Alice–Bob | `01ARZ3NDEKTSV4RRFFQ69G5FA4` | `direct_pair_key = direct:alice:bob` |
| Группа | `01ARZ3NDEKTSV4RRFFQ69G5FA5` | владелец Alice, имя `Team` |

Время `created_at` и `joined_at` — `2026-10-19T09:00:00Z`.

| Канал | `created_by_id` | `client_request_id` | `create_request_hash` |
|---|---|---|---|
| DIRECT Alice–Bob | `01ARZ3NDEKTSV4RRFFQ69G5FA1` | NULL | NULL |
| GROUP | `01ARZ3NDEKTSV4RRFFQ69G5FA1` | `01ARZ3NDEKTSV4RRFFQ69G5FA6` | `f001ef788b1ad462a54cb64d2f21d25277db6acf58c57f6e3930f4528f7f126c` |

Роли в группе: Alice — `MEMBER` и владелец через `channels.owner_id`, Bob —
`ADMIN`, Carol — `MEMBER`. В личном диалоге состоят Alice и Bob.

> Формат `create_request_hash` рабочей команды определит карточка Chat; значение
> в наборе basic — детерминированная метка fixture и не претендует на совпадение
> с рабочим форматом.

## Повторный запуск

Если прикладные таблицы уже содержат записи, команда завершается ошибкой
`fixtures: application tables are not empty: <таблица>` и ничего не меняет.
Для чистой демонстрации удалите отдельный volume проекта:

```bash
docker compose -p molotalk-demo --env-file .env.example -f deploy/fixtures.compose.yml down -v
```

Набор для входа демонстрационными аккаунтами появится отдельно в карточке Auth;
в этом наборе его нет.
