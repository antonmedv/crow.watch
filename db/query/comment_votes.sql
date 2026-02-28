-- name: CreateCommentVote :one
WITH ins AS (
    INSERT INTO comment_votes (user_id, comment_id)
    VALUES (@user_id, @comment_id)
    ON CONFLICT DO NOTHING
    RETURNING comment_id
)
UPDATE comments SET upvotes = upvotes + (SELECT count(*) FROM ins)::int
WHERE id = @comment_id
RETURNING upvotes - downvotes AS score;

-- name: DeleteCommentVote :one
WITH del AS (
    DELETE FROM comment_votes
    WHERE comment_votes.user_id = @user_id AND comment_votes.comment_id = @comment_id
    RETURNING comment_id
)
UPDATE comments SET upvotes = upvotes - (SELECT count(*) FROM del)::int
WHERE id = @comment_id
RETURNING upvotes - downvotes AS score;

-- name: GetUserCommentVotes :many
SELECT comment_id
FROM comment_votes
WHERE user_id = @user_id AND comment_id = ANY(@comment_ids::bigint[]);
