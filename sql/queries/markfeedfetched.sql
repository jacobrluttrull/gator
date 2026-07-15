-- name: MarkFeedFetched :one
update feeds
set updated_at = $2, last_fetched_at = $2
where id = $1
returning *;