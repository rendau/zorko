# Architect — общие решения

## Каркас из gotemplate
- Один Go-бинарь `svc`: gRPC (5050) + HTTP-шлюз grpc-gateway (`HTTP_PORT`) + системный HTTP
  (3003: `/healthcheck`, `/docs/*`, `/metrics`).
- Слои Clean Architecture + DDD: handler → usecase → domain service → repo; фоновое и
  интеграции — `internal/service/*`. Композиция — только `internal/app/app.go`.
- Postgres через pgx + mobone/squirrel, миграции golang-migrate при старте.

## Решено с пользователем
- Хранилища: Postgres (проекты, ключи, проблемы, настройки), ClickHouse (события, логи,
  замеры), S3 (карты исходников, записи сессий).
- Интерфейс — SPA на Nuxt UI, статика раздаётся Go-сервером (как в planora).

## Активные
- [error-ingest](error-ingest.md) — компоненты, потоки, отказы шага 1
