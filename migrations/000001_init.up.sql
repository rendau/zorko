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
