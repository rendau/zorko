# Staff Engineer — error-ingest

## Библиотеки
- `github.com/twmb/franz-go/pkg/kgo` (+ `kadm` для создания темы на стенде).
- `github.com/ClickHouse/clickhouse-go/v2` — нативный протокол, батч-вставки.
- ~~golang-migrate для CH~~ → свой раннер `internal/infra/chmigrate` (см. sde/error-ingest.md, п. 1).
- `github.com/andybalholm/brotli` — распаковка `br`.
- `github.com/rendau/mobone/v2` — CRUD org/project/project_key.

## Раскладка и соглашения
- CRUD-сущности — по skill `crud` (proto → model → domain → repo → usecase → handler), PK
  `bigserial`, gRPC + HTTP.
- Сервисные модули — по skill `golang-service`: фасад `interface.go`, `service/service.go` с
  `New` → `*Service`, порты зависимостей в `service/interface.go`; `queue` — клиент внешней
  системы (`model/` + `service/model/`).
- Приём — свой транспорт `internal/handler/ingest`: разбор протокола Sentry (envelope) живёт
  здесь, в usecase уходят уже доменные структуры (сырые байты событий + метаданные).
- Доменная модель события — без тегов; JSON Sentry разбирается в `service/normalize` через
  локальные структуры с json-тегами (`service/model`).
- ClickHouse-соединение и его миграции — `internal/app/chconn.go`, `migration.go`
  (`migrations-ch/`). Dockerfile копирует `migrations-ch/`.
- Метрики — через `metrics.Factory` (нулевой Factory без регистрации, когда метрики выключены).
- Фоновые части (keycache, processor) — `Start(ctx)` / `Wait()`, горутины через `wg.Go`;
  пары Start/Stop/WaitJobs в app.go в одном порядке.
- Ошибки — `fmt.Errorf("…: %w")`, `errors.Is/AsType`; отказы приёма — свои коды HTTP, не errs.
- JSON — только `encoding/json`. Указатели — `new(expr)`. Тесты — testify.
