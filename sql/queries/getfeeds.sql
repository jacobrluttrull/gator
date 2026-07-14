-- name: GetFeeds :many
select feeds.name as feed_name, feeds.url as feed_url, users.name as username
from feeds
join users on feeds.user_id = users.id;