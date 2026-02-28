-- name: CreateInvitation :one
INSERT INTO invitations (inviter_id, email, token_hash)
VALUES (@inviter_id, @email, @token_hash)
RETURNING id, inviter_id, email, token_hash, used_by_id, created_at;

-- name: GetInvitationByTokenHash :one
SELECT
    i.id,
    i.inviter_id,
    i.email,
    i.used_by_id,
    i.created_at,
    u.username AS inviter_name
FROM invitations i
JOIN users u ON u.id = i.inviter_id
WHERE i.token_hash = @token_hash
  AND i.used_by_id IS NULL
  AND i.created_at > now() - INTERVAL '24 hours';

-- name: ClaimInvitation :one
UPDATE invitations
SET used_by_id = @used_by_id
WHERE id = @id AND used_by_id IS NULL
RETURNING id;

-- name: ListInvitationsByUser :many
SELECT
    i.id,
    i.email,
    i.used_by_id,
    i.created_at,
    ru.username AS registered_username
FROM invitations i
LEFT JOIN users ru ON ru.id = i.used_by_id
WHERE i.inviter_id = @inviter_id
ORDER BY i.created_at DESC
LIMIT 20;

-- name: GetUserByEmail :one
SELECT id, username, email
FROM users
WHERE lower(email) = lower(@email)
LIMIT 1;
