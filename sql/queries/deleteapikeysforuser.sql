-- name: DeleteAPIKeysForUser :exec
DELETE FROM api_keys
WHERE user_id = $1;
