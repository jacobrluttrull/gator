-- name: GetFeedFollowsForUser :many

select feed_follows.*, users.name as user_name, feeds.name as feed_name
from feed_follows
inner join users on feed_follows.user_id = users.id
inner join feeds on feed_follows.feed_id = feeds.id
where feed_follows.user_id = $1;