# DBA — issue-ui

- Postgres: `usr` (уникальный `username`, bcrypt в `password`), в `000001_init`.
- Чтение проблем: `issue` по `id = any($1)`, поиск слов — `ilike` по `title`/`culprit` в проектах
  фильтра (до 5000, по `last_seen`), индекс `(project_id, last_seen desc)` уже есть.
- ClickHouse: все запросы фильтруют `project_id in ?` (первичный ключ — «project_id in N-element set»,
  проверено `explain indexes`), время — `fromUnixTimestamp64Milli`, события — `final`
  (схлопывание повторов на чтении, на одной проблеме дёшево).
