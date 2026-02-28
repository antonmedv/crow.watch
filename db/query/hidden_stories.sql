-- name: HideStory :exec
INSERT INTO hidden_stories (user_id, story_id)
VALUES (@user_id, @story_id)
ON CONFLICT DO NOTHING;

-- name: UnhideStory :exec
DELETE FROM hidden_stories
WHERE user_id = @user_id AND story_id = @story_id;

-- name: GetUserHiddenStories :many
SELECT story_id
FROM hidden_stories
WHERE user_id = @user_id AND story_id = ANY(@story_ids::bigint[]);