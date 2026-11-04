-- каждое событие коротко: графики, уникальные люди, теги (ADR-003)
create table if not exists event_stat
(
    project_id  UInt64,
    issue_id    UInt64,
    event_id    UUID,
    timestamp   DateTime64(3, 'UTC'),
    received_at DateTime64(3, 'UTC'),
    level       LowCardinality(String),
    environment LowCardinality(String),
    release     LowCardinality(String),
    user_key    String,
    tags        Map(LowCardinality(String), String)
)
engine = ReplacingMergeTree
partition by toYYYYMMDD(received_at)
order by (project_id, issue_id, timestamp, event_id)
ttl toDateTime(received_at) + interval 30 day
settings ttl_only_drop_parts = 1;

-- полные записи: не больше ISSUE_EVENT_STORE_PER_MINUTE в минуту на проблему (ADR-003)
create table if not exists event
(
    project_id      UInt64,
    issue_id        UInt64,
    event_id        UUID,
    timestamp       DateTime64(3, 'UTC'),
    received_at     DateTime64(3, 'UTC'),
    level           LowCardinality(String),
    environment     LowCardinality(String),
    release         LowCardinality(String),
    user_key        String,
    tags            Map(LowCardinality(String), String),
    platform        LowCardinality(String),
    title           String,
    culprit         String,
    message         String,
    exception_type  String,
    exception_value String,
    transaction     String,
    user_id         String,
    user_email      String,
    user_ip         String,
    browser         LowCardinality(String),
    os              LowCardinality(String),
    device          LowCardinality(String),
    sdk_name        LowCardinality(String),
    sdk_version     LowCardinality(String),
    data            String CODEC(ZSTD(3)),
    index event_id_idx event_id type bloom_filter granularity 4
)
engine = ReplacingMergeTree
partition by toYYYYMMDD(received_at)
order by (project_id, issue_id, timestamp, event_id)
ttl toDateTime(received_at) + interval 30 day
settings ttl_only_drop_parts = 1;
