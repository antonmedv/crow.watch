-- name: CreateCommentFlag :one
WITH ins AS (
    INSERT INTO comment_flags (user_id, comment_id, reason)
    VALUES (@user_id, @comment_id, @reason)
    ON CONFLICT DO NOTHING
    RETURNING comment_id
)
UPDATE comments SET downvotes = downvotes + (SELECT count(*) FROM ins)::int
WHERE id = @comment_id
RETURNING upvotes - downvotes AS score;

-- name: DeleteCommentFlag :one
WITH del AS (
    DELETE FROM comment_flags
    WHERE comment_flags.user_id = @user_id AND comment_flags.comment_id = @comment_id
    RETURNING comment_id
)
UPDATE comments SET downvotes = downvotes - (SELECT count(*) FROM del)::int
WHERE id = @comment_id
RETURNING upvotes - downvotes AS score;

-- name: GetUserCommentFlags :many
SELECT comment_id
FROM comment_flags
WHERE user_id = @user_id AND comment_id = ANY(@comment_ids::bigint[]);

-- name: GetCommentFlagCounts :many
SELECT comment_id, reason, count(*)::int AS count
FROM comment_flags
WHERE comment_id = ANY(@comment_ids::bigint[])
GROUP BY comment_id, reason
ORDER BY comment_id, count DESC;
