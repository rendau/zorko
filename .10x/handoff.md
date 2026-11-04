# Handoff

**От:** брейншторм `issue-ui` → **к:** пользователю на просмотр, затем CTO/PM

- Спека шага 2: `.10x/specs/2026-11-04-issue-ui-design.md`.
- Образцы: вход — kusec (`internal/domain/session/service/service.go`, `internal/usecase/usr`);
  раздача статики — kusec/ruto (`internal/app/spa.go`, `apps/admin/scripts/compress.mjs`);
  раскладка фронта — planora/admin.
- Нужны ADR: вход (stateless JWT + refresh как в kusec); список проблем через ClickHouse + сортировка
  в Go; один бандл + предсжатая статика с кэшем.
