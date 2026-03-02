-- name: CreateAPIKey :one
INSERT INTO api_keys (user_id, token_hash, name)
VALUES (@user_id, @token_hash, @name)
RETURNING id, user_id, token_hash, name, last_used_at, created_at;

-- name: GetAPIKeyUserByTokenHash :one
SELECT
    ak.id AS api_key_id,
    u.id,
    u.username,
    u.email,
    u.password_digest,
    u.is_moderator,
    u.banned_at,
    u.deleted_at,
    u.inviter_id,
    u.campaign,
    u.password_reset_token_hash,
    u.password_reset_token_created_at,
    u.email_confirmed_at,
    u.email_confirmation_token_hash,
    u.email_confirmation_token_created_at,
    u.unconfirmed_email,
    u.website,
    u.about,
    u.created_at,
    u.updated_at
FROM api_keys ak
JOIN users u ON u.id = ak.user_id
WHERE ak.token_hash = @token_hash;

-- name: TouchAPIKey :exec
UPDATE api_keys SET last_used_at = now() WHERE id = @id;

-- name: DeleteAPIKey :exec
DELETE FROM api_keys WHERE id = @id AND user_id = @user_id;

-- name: ListAPIKeysByUserID :many
SELECT id, name, last_used_at, created_at
FROM api_keys
WHERE user_id = @user_id
ORDER BY created_at DESC;
