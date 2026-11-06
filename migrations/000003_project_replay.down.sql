alter table project
    drop column if exists replay_hour_limit cascade,
    drop column if exists replay_session_rate cascade,
    drop column if exists replay_mode cascade;
