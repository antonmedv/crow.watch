-- name: CreateStoryFlag :exec
INSERT INTO story_flags (user_id, story_id, reason)
VALUES (@user_id, @story_id, @reason)
ON CONFLICT DO NOTHING;

-- name: DeleteStoryFlag :exec
DELETE FROM story_flags
WHERE story_flags.user_id = @user_id AND story_flags.story_id = @story_id;

-- name: GetUserStoryFlags :many
SELECT story_id
FROM story_flags
WHERE user_id = @user_id AND story_id = ANY(@story_ids::bigint[]);

-- name: GetStoryFlagCounts :many
SELECT reason, count(*)::int AS count
FROM story_flags
WHERE story_id = @story_id
GROUP BY reason
ORDER BY count DESC;

-- name: RecalculateStoryDownvotes :exec
-- Count users who hid AND flagged this story AND have no comments on it
UPDATE stories SET downvotes = (
    SELECT count(*)
    FROM hidden_stories hs
    JOIN story_flags sf ON sf.user_id = hs.user_id AND sf.story_id = hs.story_id
    WHERE hs.story_id = @story_id
      AND NOT EXISTS (
          SELECT 1 FROM comments c
          WHERE c.story_id = @story_id AND c.user_id = hs.user_id AND c.deleted_at IS NULL
      )
)
WHERE id = @story_id;
