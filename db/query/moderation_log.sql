-- name: CreateModerationLog :one
INSERT INTO moderation_log (moderator_id, action, target_type, target_id, reason, metadata)
VALUES (@moderator_id, @action, @target_type, @target_id, @reason, @metadata)
RETURNING *;

-- name: ListModerationLog :many
SELECT ml.*, u.username AS moderator_username
FROM moderation_log ml
JOIN users u ON u.id = ml.moderator_id
ORDER BY ml.created_at DESC
LIMIT @log_limit OFFSET @log_offset;

-- name: CountModerationLog :one
SELECT count(*) FROM moderation_log;
