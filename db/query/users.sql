-- name: GetUserByLogin :one
SELECT id, username, email, password_digest, is_moderator, banned_at, deleted_at, inviter_id, campaign, password_reset_token_hash, password_reset_token_created_at, email_confirmed_at, email_confirmation_token_hash, email_confirmation_token_created_at, unconfirmed_email, website, about, created_at, updated_at
FROM users
WHERE (lower(email) = lower(sqlc.arg(login)) AND email_confirmed_at IS NOT NULL)
   OR lower(username) = lower(sqlc.arg(login))
ORDER BY (lower(username) = lower(sqlc.arg(login))) DESC
LIMIT 1;

-- name: CreateUser :one
INSERT INTO users (username, email, password_digest, inviter_id, campaign)
VALUES (@username, @email, @password_digest, @inviter_id, @campaign)
RETURNING id, username, email;

-- name: SetPasswordResetTokenHash :exec
UPDATE users
SET password_reset_token_hash = @password_reset_token_hash,
    password_reset_token_created_at = now(),
    updated_at = now()
WHERE id = @id;

-- name: GetUserByPasswordResetTokenHash :one
SELECT id, username, email, password_digest, is_moderator, banned_at, deleted_at, inviter_id, campaign, password_reset_token_hash, password_reset_token_created_at, email_confirmed_at, email_confirmation_token_hash, email_confirmation_token_created_at, unconfirmed_email, website, about, created_at, updated_at
FROM users
WHERE password_reset_token_hash = @password_reset_token_hash
  AND password_reset_token_created_at > now() - INTERVAL '24 hours'
LIMIT 1;

-- name: GetUserByID :one
SELECT id, username, email, password_digest, is_moderator, banned_at, deleted_at, inviter_id, campaign, password_reset_token_hash, password_reset_token_created_at, email_confirmed_at, email_confirmation_token_hash, email_confirmation_token_created_at, unconfirmed_email, website, about, created_at, updated_at
FROM users
WHERE id = @id
LIMIT 1;

-- name: UpdateUserEmail :exec
UPDATE users
SET email = @email, updated_at = now()
WHERE id = @id;

-- name: UpdateUserPasswordByID :exec
UPDATE users
SET password_digest = @password_digest, updated_at = now()
WHERE id = @id;

-- name: ClearPasswordResetTokenHash :exec
UPDATE users
SET password_reset_token_hash = NULL,
    password_reset_token_created_at = NULL,
    updated_at = now()
WHERE id = @id;

-- name: GetPublicProfile :one
SELECT
    u.username,
    u.about,
    u.website,
    u.is_moderator,
    u.created_at,
    (SELECT count(*) FROM stories s WHERE s.user_id = u.id AND s.deleted_at IS NULL)::bigint AS story_count,
    inviter.username AS inviter_name
FROM users u
LEFT JOIN users inviter ON inviter.id = u.inviter_id
WHERE lower(u.username) = lower(@username)
  AND u.banned_at IS NULL
  AND u.deleted_at IS NULL
LIMIT 1;

-- name: UpdateUserProfile :exec
UPDATE users
SET website = @website, about = @about, updated_at = now()
WHERE id = @id;

-- name: SetEmailConfirmationToken :exec
UPDATE users
SET email_confirmation_token_hash = @email_confirmation_token_hash,
    email_confirmation_token_created_at = now(),
    updated_at = now()
WHERE id = @id;

-- name: SetEmailChangeConfirmationToken :exec
UPDATE users
SET email_confirmation_token_hash = @email_confirmation_token_hash,
    email_confirmation_token_created_at = now(),
    unconfirmed_email = @unconfirmed_email,
    updated_at = now()
WHERE id = @id;

-- name: GetUserByEmailConfirmationTokenHash :one
SELECT id, username, email, password_digest, is_moderator, banned_at, deleted_at, inviter_id, campaign, password_reset_token_hash, password_reset_token_created_at, email_confirmed_at, email_confirmation_token_hash, email_confirmation_token_created_at, unconfirmed_email, website, about, created_at, updated_at
FROM users
WHERE email_confirmation_token_hash = @email_confirmation_token_hash
  AND email_confirmation_token_created_at > now() - INTERVAL '24 hours'
LIMIT 1;

-- name: ConfirmUserEmail :exec
UPDATE users
SET email = COALESCE(unconfirmed_email, email),
    email_confirmed_at = now(),
    unconfirmed_email = NULL,
    email_confirmation_token_hash = NULL,
    email_confirmation_token_created_at = NULL,
    updated_at = now()
WHERE id = @id;

-- name: CheckEmailExists :one
SELECT EXISTS(SELECT 1 FROM users WHERE lower(email) = lower(@email) AND id != @id) AS exists;
