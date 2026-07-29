-- +goose Up
create table api_keys (
    id UUID primary key,
    created_at timestamp not null,
    updated_at timestamp not null,
    key_hash text unique not null,
    user_id UUID not null references users(id) on delete cascade
);
-- +goose Down
drop table api_keys;
