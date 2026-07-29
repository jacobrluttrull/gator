-- name: SetUserPassword :exec
-- Setting a password also revokes every API key the user holds: a key
-- issued against the old password must not outlive it. Both happen in one
-- data-modifying CTE, so there is no window where the password has changed
-- but the old keys still authenticate.
WITH updated_user AS (
    UPDATE users
    SET password_hash = $2,
        updated_at    = $3
    WHERE users.id = $1
    RETURNING users.id
)
DELETE FROM api_keys
USING updated_user
WHERE api_keys.user_id = updated_user.id;
