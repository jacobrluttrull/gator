-- name: SetUserPassword :exec
UPDATE users
SET password_hash = $2,
    updated_at    = $3
WHERE id = $1;
