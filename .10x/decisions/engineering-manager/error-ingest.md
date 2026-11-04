# Engineering Manager — error-ingest

Задачи ≤ полдня, по порядку зависимостей. Ветка — master (решение пользователя).

| # | Задача | Зависит от | Готово, когда |
|---|---|---|---|
| T1 | Стенд в docker (Postgres 17, ClickHouse, Redpanda), раздел «Тестовый стенд» в CLAUDE.md, `.env.example` | — | контейнеры поднимаются одной командой |
| T2 | Конфиг (env спеки), подключение ClickHouse, миграции CH (`migrations-ch/`), Dockerfile | T1 | `go run` стартует и применяет обе миграции |
| T3 | Миграции PG и CH `000001_init` | T2 | таблицы созданы на стенде |
| T4 | CRUD `org`, `project`, `project_key` (proto → handler), генерация ключа, DSN | T3 | curl через HTTP-шлюз создаёт ключ и отдаёт DSN |
| T5 | `service/queue`: продюсер и потребитель franz-go | T1 | тест на стенде: записал → прочитал → подтвердил |
| T6 | `service/keycache`, `service/ratelimit` | T4 | юнит-тесты, ключ находится по `project_id` + `public_key` |
| T7 | `handler/ingest` + `usecase/ingest`: envelope, store, auth, распаковка, пределы, CORS, origin | T5, T6 | юнит-тесты на образцах; события в теме |
| T8 | `service/normalize`: разбор, нормализация, вычистка | — | табличные тесты |
| T9 | `service/grouping`: отпечаток v1, заголовок, culprit | T8 | табличные тесты из спеки |
| T10 | `domain/issue` (resolve/create/bump) и `domain/event` (вставки CH) | T3 | интеграционные тесты на стенде, включая гонку двух копий |
| T11 | `service/processor`: пачки, повторы CH, подтверждение, остановка; wiring в app.go | T5, T8–T10 | событие из темы доходит до обеих баз |
| T12 | Метрики спеки | T7, T11 | видны на `/metrics` |
| T13 | Проверка: `@sentry/node`, `@sentry/browser` в Chrome, 1000 одинаковых, наплыв 100 тыс., падение CH, мусор/лимиты, нагрузка | всё | раздел 8 спеки выполнен |

**Риски:** образцы `sentry_flutter`/`@sentry/react-native` без устройств — берём тестовые
пачки из репозиториев SDK; нагрузочный замер на ноутбуке — ориентир, не гарантия для кластера.
