# Статус

**Фаза:** Verification Complete (`error-ingest`) → Phase 6 (Delivery) — ждёт решения о деплое
**Обновлено:** 2026-11-04

## Задачи error-ingest
- [x] T1 стенд в docker
- [x] T2 конфиг, ClickHouse, миграции CH
- [x] T3 миграции PG и CH
- [x] T4 CRUD org/project/project_key
- [x] T5 service/queue
- [x] T6 keycache, ratelimit
- [x] T7 handler/usecase ingest
- [x] T8 normalize
- [x] T9 grouping
- [x] T10 domain issue/event
- [x] T11 processor
- [x] T12 метрики
- [x] T13 проверка по разделу 8 спеки — все пункты ✅ (`reviews/2026-11-04-qa-report.md`)

## Безопасность
- `reviews/2026-11-04-security-review.md`: админский API не публиковать до шага 5; долг — общий
  предел памяти приёма.

## Дальше
- Phase 6: чарт и kusec (какой кластер — решает пользователь), включить CI.
- Шаг 2 дорожной карты: интерфейс (Nuxt UI) и API проблем.

## Блокеры
- Нет.
