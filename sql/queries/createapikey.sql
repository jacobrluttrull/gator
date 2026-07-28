-- name: CreateAPIKey :one
INSERT INTO api_keys (id, created_at, updated_at, key_hash, user_id)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;
