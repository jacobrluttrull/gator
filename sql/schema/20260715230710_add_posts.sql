-- +goose Up
create table posts (
    id UUID primary key,
    created_at timestamp not null,
    updated_at timestamp not null,
    title text not null,
    url text unique not null,
    description text not null,
    published_at timestamp not null,
    feed_id UUID not null references feeds(id) on delete cascade
);

-- +goose Down
drop table posts;
