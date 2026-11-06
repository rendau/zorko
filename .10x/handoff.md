# Handoff

**Сейчас:** Verification Complete `replay` → **к:** DevOps + SRE (Phase 6, когда пользователь решит) или шагу 4

- Построено и проверено вживую, не задеплоено: `decisions/sde/replay.md`, отчёты в `reviews/2026-11-06-replay-*`.
- Для деплоя: миграции PG `000003`, CH `000002` применятся при старте; новая тема Redpanda
  `zorko.replays` создаётся при старте (`KAFKA_TOPIC_REPLAYS`); S3 — тот же бакет, префикс `replay/`;
  новые env (по умолчанию годятся): `REPLAY_MAX_SEGMENT_BYTES`, `REPLAY_RETENTION`.
- Метрики: `replay_segment_total{result}`, `replay_bytes_total`, `replay_new_total`, `replay_store_seconds`,
  `replay_processor_*`, `replay_gc_deleted_total`; `queue_consumer_lag_messages` теперь с меткой `topic`.
- seller: запись «при ошибке» уже включена на сайте — после деплоя достаточно режима в проекте.

---


**От:** QA + Security (`sourcemaps`) → **к:** DevOps + SRE (Phase 6, когда пользователь решит) или шагу 4

- Шаги 1–3 построены и проверены, не задеплоены.
- Что построено и отступления: `decisions/sde/sourcemaps.md`.
- Для деплоя добавилось: S3 (бакет закрытый; `S3_ENDPOINT`, `S3_BUCKET`, `S3_ACCESS_KEY`, `S3_SECRET_KEY`,
  `S3_REGION`, `S3_SECURE`), временный диск для сборки архивов (`ASSEMBLE_TMP_DIR`, до 1 ГБ на сборку,
  до 2 одновременно), исходящий доступ обработчика к сайтам проектов (карты с сайта).
- Для шага 4: таблицы `release`/`release_project` уже есть; коммиты и деплои плагина принимаются без хранения.
