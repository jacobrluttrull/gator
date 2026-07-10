-- +goose Up
SELECT 'up SQL query';
create table users (
    id UUID primary key,
    created_at timestamp not null,
    updated_at timestamp not null,
    name text unique not null
);
-- +goose Down
SELECT 'down SQL query';
drop table users;
