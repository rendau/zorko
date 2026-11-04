# DBA — общие решения

- Postgres, миграции golang-migrate в `migrations/`, пока пусто.
- Пока проект не задеплоен — вся схема в одном `000001_init.up.sql` / `.down.sql`.
- В `down` — `DROP ... CASCADE`, порядок обратный `up`.
- ClickHouse — решено использовать для событий; способ миграций для него ещё не выбран.

## Активные
- [error-ingest](error-ingest.md)
- [issue-ui](issue-ui.md)
