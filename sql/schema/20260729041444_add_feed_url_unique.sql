-- +goose Up
-- Feed URLs identify feeds in the shared pool (follow-by-URL assumes at
-- most one feed per URL), so enforce that at the schema level.
--
-- Nothing enforced this before and addFeed never checked, so an existing
-- database can already hold two feeds with the same URL added by different
-- users. Adding the constraint on such a database would abort the whole
-- migration, so collapse duplicates first: per URL the oldest row wins
-- (ties broken by id), and every Following and post is repointed onto it.

-- A user who followed both duplicates would collide on feed_follows'
-- unique(user_id, feed_id) once both rows point at the canonical feed.
-- Drop the losing row: the Following it represents survives as the one
-- already on the canonical feed.
with canonical as (
    select id,
           first_value(id) over (partition by url order by created_at, id) as canonical_id
    from feeds
)
delete from feed_follows ff
using canonical c
where ff.feed_id = c.id
  and c.id <> c.canonical_id
  and exists (
      select 1 from feed_follows kept
      where kept.user_id = ff.user_id and kept.feed_id = c.canonical_id
  );

with canonical as (
    select id,
           first_value(id) over (partition by url order by created_at, id) as canonical_id
    from feeds
)
update feed_follows ff
set feed_id = c.canonical_id
from canonical c
where ff.feed_id = c.id and c.id <> c.canonical_id;

-- posts.url is globally unique, so two duplicate feeds can never hold the
-- same post: repointing these can't collide.
with canonical as (
    select id,
           first_value(id) over (partition by url order by created_at, id) as canonical_id
    from feeds
)
update posts p
set feed_id = c.canonical_id
from canonical c
where p.feed_id = c.id and c.id <> c.canonical_id;

-- Last: everything referencing the duplicates has been repointed, so the
-- on-delete cascade takes nothing with them.
with canonical as (
    select id,
           first_value(id) over (partition by url order by created_at, id) as canonical_id
    from feeds
)
delete from feeds f
using canonical c
where f.id = c.id and c.id <> c.canonical_id;

alter table feeds add constraint feeds_url_key unique (url);

-- +goose Down
-- Only the constraint comes off: the collapsed duplicates are gone for
-- good, and re-splitting them would invent rows nobody asked for.
alter table feeds drop constraint feeds_url_key;
