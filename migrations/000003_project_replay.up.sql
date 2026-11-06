-- запись сессий (ADR-013): режим приёма, доля сессий для примера подключения, предел новых записей в час
alter table project
    add column if not exists replay_mode         text    not null default 'off',
    add column if not exists replay_session_rate real    not null default 0.1,
    add column if not exists replay_hour_limit   integer not null default 1000;
