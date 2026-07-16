-- +goose Up
SELECT 'up SQL query';
Create extension if not exists pg_trgm;
create index idx_posts_title_trgm on posts using gin (title gin_trgm_ops);
-- +goose Down
SELECT 'down SQL query';
drop index idx_posts_title_trgm;
drop extension if exists pg_trgm;