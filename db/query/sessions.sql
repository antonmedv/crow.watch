-- name: CreateSession :exec
INSERT INTO sessions (
    user_id,
    token_hash,
    user_agent,
    ip_address,
    expires_at,
    last_seen_at,
    created_at,
    updated_at
) VALUES (
    $1,
    $2,
    $3,
    $4,
    $5,
    now(),
    now(),
    now()
);

-- name: GetSessionUserByTokenHash :one
SELECT
    s.id AS session_id,
    s.expires_at,
    u.id,
    u.username,
    u.email,
    u.password_digest,
    u.is_moderator,
    u.banned_at,
    u.deleted_at,
    u.inviter_id,
    u.password_reset_token_hash,
    u.password_reset_token_created_at,
    u.email_confirmed_at,
    u.email_confirmation_token_created_at,
    u.unconfirmed_email,
    u.website,
    u.about,
    u.created_at,
    u.updated_at
FROM sessions AS s
JOIN users AS u ON u.id = s.user_id
WHERE s.token_hash = $1
  AND s.expires_at > now()
LIMIT 1;

-- name: TouchSession :exec
UPDATE sessions
SET updated_at = now(),
    last_seen_at = now()
WHERE id = $1;

-- name: DeleteSessionByTokenHash :exec
DELETE FROM sessions
WHERE token_hash = $1;

-- name: DeleteSessionsByUserID :exec
DELETE FROM sessions
WHERE user_id = @user_id;

-- name: DeleteExpiredSessions :exec
DELETE FROM sessions
WHERE expires_at <= now();
