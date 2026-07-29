-- +goose Up
-- Feed URLs identify feeds in the shared pool (follow-by-URL assumes at
-- most one feed per URL), so enforce that at the schema level.
alter table feeds add constraint feeds_url_key unique (url);

-- +goose Down
alter table feeds drop constraint feeds_url_key;
