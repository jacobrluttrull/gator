-- +goose Up
-- Collapse duplicate-URL feeds so 20260729041444_add_feed_url_unique can
-- add its constraint.
--
-- Nothing enforced one-feed-per-URL before that migration and addFeed
-- never checked, so a database written earlier can hold two feeds on the
-- same URL added by different users. The constraint would abort on such a
-- database, and goose stops at the first failing migration — so this runs
-- one version *before* it rather than as a follow-up, which would never
-- be reached. That is also why it is not timestamped with `goose create`:
-- ordering ahead of an existing migration is the whole point.
--
-- Per URL the oldest row wins (ties broken by id); every Following and
-- post is repointed onto it, and the duplicates' names are dropped along
-- with their rows.

-- A user whose follows all land on the same canonical feed would collide
-- on feed_follows' unique(user_id, feed_id) once repointed — whether they
-- followed the canonical row plus a duplicate, or several duplicates of
-- each other with no follow on the canonical at all. Checking only
-- against the canonical row misses the second shape, so rank every
-- follow by the canonical feed it will end up on and keep one
-- deterministic winner per (user_id, canonical feed): a follow already
-- on the canonical row first, then the oldest, ties broken by id.
with canonical as (
    select id,
           first_value(id) over (partition by url order by created_at, id) as canonical_id
    from feeds
),
ranked as (
    select ff.id,
           row_number() over (
               partition by ff.user_id, c.canonical_id
               order by (ff.feed_id = c.canonical_id) desc, ff.created_at, ff.id
           ) as keep_rank
    from feed_follows ff
    join canonical c on c.id = ff.feed_id
)
delete from feed_follows ff
using ranked r
where ff.id = r.id
  and r.keep_rank > 1;

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

-- +goose Down
-- Nothing to undo: the collapsed duplicates are gone for good, and
-- re-splitting them would invent rows nobody asked for.
select 'no down migration for feed-url dedup';
