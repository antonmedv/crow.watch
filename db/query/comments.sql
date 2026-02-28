-- name: CreateComment :one
INSERT INTO comments (story_id, user_id, parent_id, body, depth)
VALUES (@story_id, @user_id, @parent_id, @body, @depth)
RETURNING id, story_id, user_id, parent_id, body, depth, upvotes, downvotes, created_at, updated_at, deleted_at;

-- name: GetCommentByID :one
SELECT id, story_id, user_id, parent_id, body, depth, upvotes, downvotes, created_at, updated_at, deleted_at
FROM comments
WHERE id = @id;

-- name: GetCommentDepth :one
SELECT depth FROM comments WHERE id = @id;

-- name: ListCommentsByStory :many
SELECT
    c.id,
    c.story_id,
    c.user_id,
    c.parent_id,
    c.body,
    c.depth,
    c.upvotes,
    c.downvotes,
    c.created_at,
    c.updated_at,
    c.deleted_at,
    u.username
FROM comments AS c
JOIN users AS u ON u.id = c.user_id
WHERE c.story_id = @story_id
ORDER BY c.created_at ASC;

-- name: UpdateCommentBody :exec
UPDATE comments SET body = @body, updated_at = now()
WHERE id = @id;

-- name: SoftDeleteComment :exec
UPDATE comments SET deleted_at = now(), body = ''
WHERE id = @id;

-- name: IncrementStoryCommentCount :exec
UPDATE stories SET comment_count = comment_count + 1 WHERE id = @id;

-- name: DecrementStoryCommentCount :exec
UPDATE stories SET comment_count = comment_count - 1 WHERE id = @id AND comment_count > 0;

-- name: GetCommentRankingDataByStories :many
SELECT
    c.story_id,
    count(*)::int AS total,
    count(*) FILTER (WHERE c.user_id = s.user_id)::int AS by_submitter
FROM comments c
JOIN stories s ON s.id = c.story_id
WHERE c.story_id = ANY(@story_ids::bigint[])
  AND c.deleted_at IS NULL
GROUP BY c.story_id;
