# Обновить pgx и подтвердить результат аудита Go

Issue MoloTeam/MoloTalk#24.

## Why

В `backend/go.mod` выбран `github.com/jackc/pgx/v5 v5.7.6`. Ревью зафиксировало GO-2026-5004 в графе проекта. Исправление для v5 доступно начиная с v5.9.2. Миграции и fixtures используют этот драйвер, поэтому обновление требует проверки реального PostgreSQL.

## What Changes

- Выбрать совместимый стабильный релиз pgx v5 с исправлением и обновить Go dependency graph.
- Сопоставить свежие отчёты `govulncheck` до и после по идентификатору advisory и зафиксировать результаты Go и PostgreSQL проверок.
- Проверить миграции и fixtures на пустой и уже подготовленной базе. Если к моменту реализации появятся PostgreSQL-адаптеры Auth/Chat, прогнать их тесты на целевом main.

## Capabilities

### New Capabilities

- `go-postgres-dependency` — проверяемая версия драйвера и совместимость при обновлении.

### Modified Capabilities

—

## Impact

`backend/go.mod` и `backend/go.sum`. Дополнительный regression test затрагивает существующий адаптер только при обнаруженной проблеме. Публичный API, proto и SQL-схема сохраняются. Общий CI остаётся в #8.
