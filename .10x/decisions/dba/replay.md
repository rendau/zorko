# DBA — replay

- PG `000003_project_replay` — три колонки `project` с умолчаниями (`off`, 0.1, 1000); down — `drop column … cascade`.
- CH `000002_replay`:
  - `replay_segment` — ReplacingMergeTree `(project_id, replay_id, segment_id)`, партиция и TTL по
    `received_at` (30 дней, `ttl_only_drop_parts`); повтор сегмента схлопывается. Сводка записи —
    агрегат по сегментам (`argMin`, `uniqExact`, `arrayDistinct(arrayFlatten(groupArray(error_ids)))`).
  - `replay_issue` — ReplacingMergeTree `(project_id, issue_id, replay_id, event_id)`, TTL 30 дней,
    bloom-индекс по `replay_id` (ошибки одной записи без номера проблемы).
- Фильтр по адресу/пользователю — в `having` по всем сегментам, постоянные поля — в `where`.
- Миграции — отдельными файлами (база применена на сервере).
