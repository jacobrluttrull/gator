-- name: GetUserByAPIKey :one
select users.*
from users
inner join api_keys on api_keys.user_id = users.id
where api_keys.key_hash = $1;
