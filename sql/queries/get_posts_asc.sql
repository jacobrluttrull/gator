-- name: GetPostsForUserAsc :many
select posts.*
from posts
inner join feed_follows on posts.feed_id = feed_follows.feed_id
where feed_follows.user_id = $1
order by posts.published_at asc
limit $2;