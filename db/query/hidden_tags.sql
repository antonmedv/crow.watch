-- name: HideTag :exec
INSERT INTO hidden_tags (user_id, tag_id)
VALUES (@user_id, @tag_id)
ON CONFLICT DO NOTHING;

-- name: UnhideTag :exec
DELETE FROM hidden_tags
WHERE user_id = @user_id AND tag_id = @tag_id;

-- name: ListUserHiddenTagIDs :many
SELECT tag_id FROM hidden_tags WHERE user_id = @user_id;
