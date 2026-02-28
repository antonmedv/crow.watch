-- name: CreateVote :one
WITH ins AS (
    INSERT INTO votes (user_id, story_id)
    VALUES (@user_id, @story_id)
    ON CONFLICT DO NOTHING
    RETURNING story_id
)
UPDATE stories SET upvotes = upvotes + (SELECT count(*) FROM ins)::int
WHERE id = @story_id
RETURNING upvotes;

-- name: DeleteVote :one
WITH del AS (
    DELETE FROM votes
    WHERE votes.user_id = @user_id AND votes.story_id = @story_id
    RETURNING story_id
)
UPDATE stories SET upvotes = upvotes - (SELECT count(*) FROM del)::int
WHERE id = @story_id
RETURNING upvotes;

-- name: GetUserVotes :many
SELECT story_id
FROM votes
WHERE user_id = @user_id AND story_id = ANY(@story_ids::bigint[]);
