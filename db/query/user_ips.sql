-- name: UpsertUserIP :exec
INSERT INTO user_ips (user_id, ip_address, action)
VALUES ($1, $2, $3)
ON CONFLICT (user_id, ip_address, action)
DO UPDATE SET last_seen_at = now(), hit_count = user_ips.hit_count + 1;

-- name: GetIPsByUserID :many
SELECT * FROM user_ips WHERE user_id = $1 ORDER BY last_seen_at DESC;

-- name: GetUsersByIP :many
SELECT * FROM user_ips WHERE ip_address = $1 ORDER BY last_seen_at DESC;

-- name: GetUsersSharingIPsWith :many
SELECT
    ui.ip_address,
    ui.action,
    ui.hit_count,
    ui.first_seen_at,
    ui.last_seen_at,
    u.id AS user_id,
    u.username,
    u.created_at AS user_created_at,
    u.banned_at,
    u.campaign,
    inviter.username AS inviter_name
FROM user_ips ui
JOIN users u ON u.id = ui.user_id
LEFT JOIN users inviter ON inviter.id = u.inviter_id
WHERE ui.ip_address IN (SELECT uii.ip_address FROM user_ips uii WHERE uii.user_id = $1)
  AND ui.user_id != $1
ORDER BY ui.ip_address, u.username, ui.action;
