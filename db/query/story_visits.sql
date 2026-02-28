-- name: UpsertStoryVisit :exec
INSERT INTO story_visits (user_id, story_id, last_seen_at)
VALUES (@user_id, @story_id, now())
ON CONFLICT (user_id, story_id) DO UPDATE SET last_seen_at = now();

-- name: GetStoryVisit :one
SELECT last_seen_at
FROM story_visits
WHERE user_id = @user_id AND story_id = @story_id;

-- name: ListReplies :many
SELECT
    c.id AS comment_id,
    c.body,
    c.created_at,
    c.deleted_at,
    u.username AS comment_author,
    s.title AS story_title,
    s.short_code AS story_short_code,
    (sv.last_seen_at IS NULL OR c.created_at > sv.last_seen_at)::bool AS is_unread
FROM comments AS c
JOIN users AS u ON u.id = c.user_id
JOIN comments AS parent ON parent.id = c.parent_id
JOIN stories AS s ON s.id = c.story_id
LEFT JOIN story_visits AS sv ON sv.user_id = @user_id AND sv.story_id = c.story_id
WHERE parent.user_id = @user_id
  AND c.user_id != @user_id
  AND c.deleted_at IS NULL
ORDER BY c.created_at DESC
LIMIT 50;

-- name: CountUnreadReplies :one
SELECT count(*)
FROM comments AS c
JOIN comments AS parent ON parent.id = c.parent_id
LEFT JOIN story_visits AS sv ON sv.user_id = @user_id AND sv.story_id = c.story_id
WHERE parent.user_id = @user_id
  AND c.user_id != @user_id
  AND c.deleted_at IS NULL
  AND (sv.last_seen_at IS NULL OR c.created_at > sv.last_seen_at);
