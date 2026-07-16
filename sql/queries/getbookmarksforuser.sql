-- name: GetBookmarksForUser :many
SELECT posts.*
FROM post_bookmarks
INNER JOIN posts ON post_bookmarks.post_id = posts.id
WHERE post_bookmarks.user_id = $1
ORDER BY post_bookmarks.created_at DESC;
