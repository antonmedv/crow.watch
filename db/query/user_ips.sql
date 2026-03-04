-- name: UpsertUserIP :exec
INSERT INTO user_ips (user_id, ip_address, action)
VALUES ($1, $2, $3)
ON CONFLICT (user_id, ip_address, action)
DO UPDATE SET last_seen_at = now(), hit_count = user_ips.hit_count + 1;

-- name: GetIPsByUserID :many
SELECT * FROM user_ips WHERE user_id = $1 ORDER BY last_seen_at DESC;

-- name: GetUsersByIP :many
SELECT * FROM user_ips WHERE ip_address = $1 ORDER BY last_seen_at DESC;
