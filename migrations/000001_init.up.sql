create table usr (
    id          bigserial primary key,
    created_at  timestamptz not null default now(),
    modified_at timestamptz not null default now(),
    active      boolean     not null default true,
    is_admin    boolean     not null default false,
    name        text        not null default '',
    username    text        not null unique,
    password    text        not null default ''
);

create table org (
    id          bigserial primary key,
    created_at  timestamptz not null default now(),
    modified_at timestamptz not null default now(),
    slug        text        not null unique,
    name        text        not null default ''
);

create table project (
    id          bigserial primary key,
    created_at  timestamptz not null default now(),
    modified_at timestamptz not null default now(),
    org_id      bigint      not null references org (id) on delete cascade,
    slug        text        not null,
    name        text        not null default '',
    platform    text        not null default '',
    -- карты с сайта (ADR-010): запасной путь, если из CI карты нет
    site_fetch_enabled      boolean not null default false,
    site_fetch_host         text[]  not null default '{}',
    site_fetch_header_name  text    not null default '',
    site_fetch_header_value text    not null default '',
    site_fetch_interval_min integer not null default 60,
    unique (org_id, slug)
);

create table project_key (
    id                 bigserial primary key,
    created_at         timestamptz not null default now(),
    modified_at        timestamptz not null default now(),
    project_id         bigint      not null references project (id) on delete cascade,
    public_key         text        not null unique,
    label              text        not null default '',
    active             boolean     not null default true,
    rate_limit_per_sec integer     not null default 0,
    allowed_origin     text[]      not null default '{}'
);
create index project_key_project_id_idx on project_key (project_id);

create table issue (
    id               bigserial primary key,
    created_at       timestamptz not null default now(),
    modified_at      timestamptz not null default now(),
    project_id       bigint      not null references project (id) on delete cascade,
    title            text        not null default '',
    culprit          text        not null default '',
    type             text        not null default '',
    value            text        not null default '',
    level            text        not null default '',
    platform         text        not null default '',
    status           text        not null default 'unresolved',
    first_seen       timestamptz not null,
    last_seen        timestamptz not null,
    times_seen       bigint      not null default 0,
    first_release    text        not null default '',
    last_release     text        not null default '',
    -- «хранить часть» (ADR-003): сколько полных записей сохранено в минуту stored_minute
    stored_minute    timestamptz not null default 'epoch',
    stored_in_minute integer     not null default 0
);
create index issue_project_id_last_seen_idx on issue (project_id, last_seen desc);
create index issue_project_id_status_last_seen_idx on issue (project_id, status, last_seen desc);

-- отпечаток → проблема (ADR-004)
create table issue_hash (
    project_id bigint not null references project (id) on delete cascade,
    hash       text   not null,
    version    text   not null,
    issue_id   bigint not null references issue (id) on delete cascade,
    primary key (project_id, hash)
);
create index issue_hash_issue_id_idx on issue_hash (issue_id);

-- токены CI организации (ADR-009): в базе только SHA-256
create table org_token (
    id           bigserial primary key,
    created_at   timestamptz not null default now(),
    org_id       bigint      not null references org (id) on delete cascade,
    created_by   bigint      references usr (id) on delete set null,
    name         text        not null default '',
    token_hash   bytea       not null unique,
    hint         text        not null default '',
    last_used_at timestamptz
);
create index org_token_org_id_idx on org_token (org_id);

-- залитый архив карт (artifact bundle) или скачанное с сайта
create table artifact_bundle (
    id           bigserial primary key,
    created_at   timestamptz not null default now(),
    org_id       bigint      not null references org (id) on delete cascade,
    checksum     text        not null,
    source       text        not null default 'upload',
    state        text        not null default 'assembling',
    detail       text        not null default '',
    release      text        not null default '',
    dist         text        not null default '',
    file_count   integer     not null default 0,
    size         bigint      not null default 0,
    assembled_at timestamptz,
    last_used_at timestamptz,
    unique (org_id, checksum)
);
create index artifact_bundle_org_id_created_at_idx on artifact_bundle (org_id, created_at desc);

create table artifact_bundle_project (
    bundle_id  bigint not null references artifact_bundle (id) on delete cascade,
    project_id bigint not null references project (id) on delete cascade,
    primary key (bundle_id, project_id)
);
create index artifact_bundle_project_project_id_idx on artifact_bundle_project (project_id);

-- файл в S3 (file/<sha1>): учёт для уборки
create table blob (
    sha1       text        primary key,
    size       bigint      not null default 0,
    created_at timestamptz not null default now()
);

create table artifact_file (
    id        bigserial primary key,
    bundle_id bigint not null references artifact_bundle (id) on delete cascade,
    url       text   not null,
    type      text   not null,
    debug_id  uuid,
    sourcemap text   not null default '',
    blob      text   not null references blob (sha1),
    size      bigint not null default 0
);
create index artifact_file_debug_id_type_idx on artifact_file (debug_id, type) where debug_id is not null;
create index artifact_file_bundle_id_url_idx on artifact_file (bundle_id, url);
create index artifact_file_blob_idx on artifact_file (blob);

-- релиз (минимум для плагина; статусы и деплои — шаг 4)
create table release (
    id            bigserial primary key,
    created_at    timestamptz not null default now(),
    org_id        bigint      not null references org (id) on delete cascade,
    version       text        not null,
    date_released timestamptz,
    unique (org_id, version)
);

create table release_project (
    release_id bigint not null references release (id) on delete cascade,
    project_id bigint not null references project (id) on delete cascade,
    primary key (release_id, project_id)
);
create index release_project_project_id_idx on release_project (project_id);

-- память скачивания с сайта: по адресу не ходим чаще интервала проекта
create table site_fetch (
    project_id bigint      not null references project (id) on delete cascade,
    url        text        not null,
    state      text        not null,
    detail     text        not null default '',
    bundle_id  bigint      references artifact_bundle (id) on delete set null,
    fetched_at timestamptz not null default now(),
    primary key (project_id, url)
);
create index site_fetch_project_id_fetched_at_idx on site_fetch (project_id, fetched_at desc);
