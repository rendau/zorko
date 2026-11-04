# Architect — error-ingest

ADR: 001 (протокол Sentry, отдельный порт), 002 (Redpanda), 003 (считать всё / хранить часть),
004 (отпечаток v1, issue_hash). Спека: `specs/2026-11-04-error-ingest-design.md`.

## Компоненты (один бинарь, части включаются env)
| Часть | Где | Зависит от |
|---|---|---|
| HTTP-приём Sentry | `internal/handler/ingest` (свой http.Server) | usecase ingest |
| usecase приёма | `internal/usecase/ingest` | keycache, ratelimit, queue |
| кэш ключей (фон) | `internal/service/keycache` | domain project_key |
| лимиты | `internal/service/ratelimit` | — |
| Redpanda-клиент | `internal/service/queue` | franz-go |
| нормализация + вычистка | `internal/service/normalize` | — |
| отпечаток, заголовок | `internal/service/grouping` | — |
| обработчик (фон) | `internal/service/processor` | queue, normalize, grouping, domain issue, domain event |
| проблемы | `internal/domain/issue` (repo/db, кастомные запросы) | Postgres |
| события | `internal/domain/event` (repo/ch) | ClickHouse |
| админ API | `internal/domain/{org,project,project_key}` + usecase + handler/grpc | Postgres |

## Потоки
1. SDK → handler ingest (разбор auth, распаковка, envelope) → usecase (ключ, origin, лимит) →
   queue.Produce → `200`.
2. queue.Consume (пачка) → normalize → grouping → issue.Resolve/Create/Bump (PG) →
   event.InsertStats/InsertEvents (CH, повтор) → commit.

## Отказы
- Redpanda: приём 503; обработка ждёт переподключения.
- Postgres: обработка ждёт и повторяет пачку целиком (до шага 5 ничего не записано).
- ClickHouse: повтор только шага вставки.
- Битое сообщение: пропуск с метрикой.
- Остановка: приём перестаёт принимать → дожидается отправок → обработка дописывает текущую
  пачку и подтверждает смещения → закрытие клиентов.
