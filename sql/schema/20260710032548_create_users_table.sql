-- +goose Up
SELECT 'up SQL query';
create table users (
    id UUID primary key,
    created_at timestamp not null,
    updated_at timestamp not null,
    name text unique not null
);
create table feeds (
    id UUID primary key,
    created_at timestamp not null,
    updated_at timestamp not null,
    name text not null,
    url text not null,
    user_id UUID not null references users(id) on delete cascade
);
-- +goose Down
SELECT 'down SQL query';
drop table feeds;
drop table users;

