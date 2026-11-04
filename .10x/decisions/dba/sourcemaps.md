# DBA — sourcemaps

Всё в едином `migrations/000001_init` (проект не развёрнут).

| Таблица | Ключи и индексы | Зачем |
|---|---|---|
| `org_token` | `token_hash` unique, `(org_id)` | токены CI; поиск по SHA-256 |
| `artifact_bundle` | unique `(org_id, checksum)` — он же замок сборки; `(org_id, created_at desc)` | архивы |
| `artifact_bundle_project` | PK `(bundle_id, project_id)`, `(project_id)` | архив ↔ проекты |
| `artifact_file` | `(debug_id, type) where debug_id is not null`, `(bundle_id, url)`, `(blob)` | поиск по метке / по пути; уборка |
| `blob` | PK `sha1` | учёт файлов в S3 для уборки |
| `release`, `release_project` | unique `(org_id, version)` | релизы для плагина |
| `site_fetch` | PK `(project_id, url)`, `(project_id, fetched_at desc)` | память скачивания с сайта |
| `project` | + `site_fetch_*` | настройки сайта |

- Поиск по метке: `artifact_file` по `debug_id = any($)` + join готовых архивов проекта, свежие первыми.
- Поиск по релизу: `release`, `dist`, `url = any($)` (кандидаты пути).
- Уборка пачками по 1000/500: архивы `coalesce(last_used_at, created_at) < now() - 30 дней` (и не `ok`
  старше суток), затем `blob` без ссылок и не тронутые сутки — удаление строки с повторной проверкой,
  потом объект в S3.
- `last_used_at` архива — не чаще раза в час, пачкой раз в минуту; у токена — не чаще раза в минуту.
- Список архивов проекта: `count(*)` + страница; поиск — подстрока релиза, метка или отпечаток целиком.
