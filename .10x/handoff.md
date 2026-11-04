# Handoff

**От:** QA + Security (Phase 5) → **к:** DevOps + SRE (Phase 6) или шагу 2

- Шаг 1 построен и проверен: `decisions/sde/error-ingest.md` (что и отступления),
  `reviews/2026-11-04-qa-report.md`, `reviews/2026-11-04-security-review.md`.
- Для деплоя нужны: Postgres, ClickHouse, Redpanda; env — `internal/config/config.go` и
  `.env.example`; приём — отдельный домен на `INGEST_PORT`; админский API наружу не открывать.
- CI `deploy.yml` выключен (ветка `disabled`), тесты в нём закомментированы.
