drop index if exists usr_ext_sub_key cascade;

alter table usr
    drop column if exists all_projects cascade,
    drop column if exists ext_sub cascade;
