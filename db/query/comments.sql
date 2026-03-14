-- name: CreateComment :one
INSERT INTO comments (story_id, user_id, parent_id, body, depth, short_code)
VALUES (@story_id, @user_id, @parent_id, @body, @depth, @short_code)
RETURNING id, story_id, user_id, parent_id, body, depth, short_code, upvotes, downvotes, created_at, updated_at, deleted_at;

-- name: GetCommentByID :one
SELECT id, story_id, user_id, parent_id, body, depth, short_code, upvotes, downvotes, created_at, updated_at, deleted_at
FROM comments
WHERE id = @id;

-- name: ListCommentsByStory :many
SELECT
    c.id,
    c.story_id,
    c.user_id,
    c.parent_id,
    c.body,
    c.depth,
    c.short_code,
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

-- name: GetCommentByShortCode :one
SELECT id, story_id, user_id, parent_id, body, depth, short_code, upvotes, downvotes, created_at, updated_at, deleted_at
FROM comments
WHERE short_code = @short_code;

-- name: ListCommentsByStoryCode :many
SELECT
    c.short_code,
    c.body,
    c.depth,
    c.upvotes,
    c.downvotes,
    c.created_at,
    c.deleted_at,
    u.username,
    p.short_code AS parent_short_code
FROM comments AS c
JOIN users AS u ON u.id = c.user_id
LEFT JOIN comments AS p ON p.id = c.parent_id
JOIN stories AS s ON s.id = c.story_id
WHERE s.short_code = @story_short_code
ORDER BY c.created_at ASC;

-- name: StoryExistsByCode :one
SELECT EXISTS(SELECT 1 FROM stories WHERE short_code = @short_code AND deleted_at IS NULL) AS exists;

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
