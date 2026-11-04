# Handoff

**От:** QA + Security (`issue-ui`) → **к:** DevOps + SRE (Phase 6) или шагу 3

- Шаги 1–2 построены и проверены, не задеплоены.
- Что построено и отступления: `decisions/sde/error-ingest.md`, `decisions/sde/issue-ui.md`.
- Для деплоя: Postgres, ClickHouse, Redpanda; env — `internal/config/config.go`, `.env.example`
  (`JWT_SECRET` — секрет генерацией); три наружных адреса: приём (`INGEST_PORT`), API (`HTTP_PORT`,
  `HTTP_CORS=true`), интерфейс (`ADMIN_PORT`, `ADMIN_API_BASE_URL`); образ — `make build-admin` + `make`.
