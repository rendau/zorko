# DBA — error-ingest

## Postgres (`migrations/000001_init`)
- `org`, `project` (unique `org_id, slug`), `project_key` (unique `public_key`, индекс `project_id`),
  `issue` (индексы `(project_id, last_seen desc)`, `(project_id, status, last_seen desc)` — под
  список проблем шага 2), `issue_hash` (PK `project_id, hash`, индекс `issue_id`).
- Каскады: org → project → project_key/issue → issue_hash.
- Горячий запрос — `Bump`: один `update … from (values …)` на пачку, блокировка строк `for update`
  по возрастанию id (без взаимных блокировок копий). На 100 тыс. событий одной проблемы —
  ~200 обновлений одной строки, без конкуренции на событие.

## ClickHouse (`migrations-ch/000001_init`)
- `event_stat`, `event`: `ReplacingMergeTree`, партиция по дню `received_at`, TTL 30 дней с
  `ttl_only_drop_parts` (дни удаляются целыми частями), ключ `(project_id, issue_id, timestamp,
  event_id)`, у `event` — `bloom_filter` по `event_id`, `data` с `ZSTD(3)`.
- Повторы пачек схлопываются при слияниях; запросы, которым нужна точность до слияния, —
  `uniqExact(event_id)` или `FINAL`.
- Миграции — свой раннер, таблица версий `schema_migrations` (MergeTree).

## Проверено на стенде
- 101 000 событий одной проблемы: `times_seen` = 101 000, `event_stat` = 101 000,
  `event` = 100 в каждую минуту.
