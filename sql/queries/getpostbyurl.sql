-- name: GetPostByUrl :one
SELECT * FROM posts
WHERE url = $1;
