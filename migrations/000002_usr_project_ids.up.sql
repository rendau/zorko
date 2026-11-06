-- проекты, которые видит участник; пусто — все проекты (у админа список не учитывается)
alter table usr
    add column if not exists project_ids bigint[] not null default '{}';
