-- вход MCP-клиентов по OAuth (ADR-020). oauth_grant — подключение клиента (например, Claude),
-- которое человек разрешил на странице согласия интерфейса: его токены дают клиенту права этого
-- человека на чтение. Подключение без connected_at ждёт обмена кода на токены.
create table if not exists oauth_grant (
    id             text        not null default gen_random_uuid()::text,
    usr_id         bigint      not null,
    client_id      text        not null,
    client_name    text        not null default '',
    redirect_uri   text        not null default '',
    code_challenge text        not null default '',
    created_at     timestamptz not null default now(),
    connected_at   timestamptz,
    last_used_at   timestamptz,
    primary key (id),
    foreign key (usr_id) references usr (id) on delete cascade
);

create index if not exists oauth_grant_usr_id_idx on oauth_grant (usr_id);

-- коды авторизации, access- и refresh-токены подключений; хранится только sha256 токена
create table if not exists oauth_token (
    token_hash text        not null,
    grant_id   text        not null,
    kind       text        not null,
    expires_at timestamptz not null,
    created_at timestamptz not null default now(),
    primary key (token_hash),
    foreign key (grant_id) references oauth_grant (id) on delete cascade
);

create index if not exists oauth_token_grant_id_idx on oauth_token (grant_id);
create index if not exists oauth_token_expires_at_idx on oauth_token (expires_at);
