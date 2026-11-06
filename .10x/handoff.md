# Handoff

**Сейчас:** Planning Complete `replay` → **к:** SDE (R1–R10, `decisions/engineering-manager/replay.md`)

- Спека: `specs/2026-11-06-replay-design.md`. Протокол сверен с `@sentry/replay` 11.11.0
  (пара `replay_event` + `replay_recording`, zlib, `replay_type` buffer/session, остановка записи
  по `X-Sentry-Rate-Limits` с категорией `replay`).
- Для дизайна: ADR на «сегменты сразу в S3 при приёме, описания — через отдельную тему Redpanda»,
  на «связь запись↔проблема отдельной таблицей, а не через сохранённые события» и на прирост
  единого бандла от проигрывателя (ADR-007).
- Миграции теперь новыми файлами: Postgres `000003_…`, ClickHouse `000002_…` (база уже применена).

---


**От:** QA + Security (`sourcemaps`) → **к:** DevOps + SRE (Phase 6, когда пользователь решит) или шагу 4

- Шаги 1–3 построены и проверены, не задеплоены.
- Что построено и отступления: `decisions/sde/sourcemaps.md`.
- Для деплоя добавилось: S3 (бакет закрытый; `S3_ENDPOINT`, `S3_BUCKET`, `S3_ACCESS_KEY`, `S3_SECRET_KEY`,
  `S3_REGION`, `S3_SECURE`), временный диск для сборки архивов (`ASSEMBLE_TMP_DIR`, до 1 ГБ на сборку,
  до 2 одновременно), исходящий доступ обработчика к сайтам проектов (карты с сайта).
- Для шага 4: таблицы `release`/`release_project` уже есть; коммиты и деплои плагина принимаются без хранения.
