-- name: SearchPosts :many
select posts.*, similarity(lower(posts.title), lower($2)) as sim
from posts
inner join feed_follows on posts.feed_id = feed_follows.feed_id
where feed_follows.user_id = $1 and lower(posts.title) % lower($2)
order by sim desc
limit $3;