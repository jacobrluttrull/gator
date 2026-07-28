-- +goose Up
alter table users add column password_hash text;
-- +goose Down
alter table users drop column password_hash;
