-- вход через внешний провайдер (ADR-019): ext_sub — постоянный id человека у провайдера (claim sub),
-- пусто — локальная учётка. all_projects — видит все проекты; пустой project_ids теперь значит
-- «ни одного», поэтому тем, у кого список пуст (раньше это было «все»), галочка ставится.
alter table usr
    add column if not exists ext_sub      text    not null default '',
    add column if not exists all_projects boolean not null default false;

update usr
set all_projects = true
where cardinality(project_ids) = 0;

create unique index if not exists usr_ext_sub_key on usr (ext_sub) where ext_sub <> '';
