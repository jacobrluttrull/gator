-- name: CreateBookmark :one
WITH inserted_bookmark AS (
    INSERT INTO post_bookmarks (id, created_at, updated_at, user_id, post_id)
    VALUES ($1, $2, $3, $4, $5)
    RETURNING *
)
SELECT inserted_bookmark.*, posts.title AS post_title, posts.url AS post_url
FROM inserted_bookmark
INNER JOIN posts ON inserted_bookmark.post_id = posts.id;
