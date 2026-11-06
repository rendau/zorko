-- сегменты записей сессий (ADR-012): описание сегмента, тело — в S3 в папке дня приёма (received_at).
-- Партиция и TTL — по времени приёма (часы сервера), как папки S3: часы устройства бывают неточны
create table if not exists replay_segment
(
    project_id   UInt64,
    replay_id    UUID,
    segment_id   UInt16,
    replay_type  LowCardinality(String),
    replay_start DateTime64(3, 'UTC'),
    timestamp    DateTime64(3, 'UTC'),
    received_at  DateTime64(3, 'UTC'),
    urls         Array(String),
    error_ids    Array(UUID),
    environment  LowCardinality(String),
    release      LowCardinality(String),
    user_id      String,
    user_email   String,
    user_ip      String,
    browser      LowCardinality(String),
    os           LowCardinality(String),
    device       LowCardinality(String),
    sdk_name     LowCardinality(String),
    sdk_version  LowCardinality(String),
    size         UInt32
)
engine = ReplacingMergeTree
partition by toYYYYMMDD(received_at)
order by (project_id, replay_id, segment_id)
ttl toDateTime(received_at) + interval 30 day
settings ttl_only_drop_parts = 1;

-- связь записи с проблемой (ADR-014): по тегу replayId каждой ошибки, а не только сохранённой полностью
create table if not exists replay_issue
(
    project_id UInt64,
    issue_id   UInt64,
    replay_id  UUID,
    event_id   UUID,
    timestamp  DateTime64(3, 'UTC'),
    -- ошибки одной записи (отметки на шкале проигрывателя) — без номера проблемы
    index replay_id_idx replay_id type bloom_filter granularity 4
)
engine = ReplacingMergeTree
partition by toYYYYMMDD(timestamp)
order by (project_id, issue_id, replay_id, event_id)
ttl toDateTime(timestamp) + interval 30 day
settings ttl_only_drop_parts = 1;
