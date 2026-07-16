-- name: DeleteBookmark :exec
DELETE FROM post_bookmarks
WHERE user_id = $1 AND post_id = $2;
