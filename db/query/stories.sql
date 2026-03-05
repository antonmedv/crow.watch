-- name: CreateStory :one
INSERT INTO stories (user_id, domain_id, origin_id, url, normalized_url, title, body, short_code)
VALUES (@user_id, @domain_id, @origin_id, @url, @normalized_url, @title, @body, @short_code)
RETURNING id, user_id, domain_id, origin_id, url, normalized_url, title, body, short_code, created_at, updated_at, deleted_at;

-- name: FindRecentByNormalizedURL :one
SELECT id, url, title, short_code, created_at
FROM stories
WHERE normalized_url = @normalized_url
  AND deleted_at IS NULL
  AND created_at > now() - INTERVAL '30 days'
ORDER BY created_at DESC
LIMIT 1;

-- name: CreateTagging :exec
INSERT INTO taggings (story_id, tag_id)
VALUES (@story_id, @tag_id);

-- name: ListStories :many
SELECT
    s.id,
    s.url,
    s.title,
    s.body,
    s.short_code,
    s.upvotes,
    s.downvotes,
    s.comment_count,
    s.created_at,
    s.deleted_at,
    u.username,
    d.domain,
    o.origin
FROM stories AS s
JOIN users AS u ON u.id = s.user_id
LEFT JOIN domains AS d ON d.id = s.domain_id
LEFT JOIN origins AS o ON o.id = s.origin_id
LEFT JOIN taggings AS tg ON tg.story_id = s.id AND tg.tag_id = sqlc.narg('tag_id')
WHERE
    (sqlc.narg('tag_id')::bigint IS NULL OR tg.tag_id IS NOT NULL)
    AND (sqlc.narg('username')::text IS NULL OR lower(u.username) = lower(sqlc.narg('username')))
    AND (NOT @hide_deleted::bool OR s.deleted_at IS NULL)
    AND s.id NOT IN (
        SELECT tg2.story_id FROM taggings AS tg2
        WHERE tg2.tag_id = ANY(@hidden_tag_ids::bigint[])
    )
ORDER BY s.created_at DESC
LIMIT @story_limit;

-- name: GetStory :one
SELECT
    s.id,
    s.user_id,
    s.url,
    s.title,
    s.body,
    s.short_code,
    s.upvotes,
    s.downvotes,
    s.comment_count,
    s.created_at,
    s.deleted_at,
    u.username,
    d.domain,
    o.origin
FROM stories AS s
JOIN users AS u ON u.id = s.user_id
LEFT JOIN domains AS d ON d.id = s.domain_id
LEFT JOIN origins AS o ON o.id = s.origin_id
WHERE (sqlc.narg('id')::bigint IS NULL OR s.id = sqlc.narg('id'))
  AND (sqlc.narg('short_code')::text IS NULL OR s.short_code = sqlc.narg('short_code'));

-- name: GetStoryTags :many
SELECT t.id, t.tag, t.is_media, t.hotness_mod
FROM taggings AS tg
JOIN tags AS t ON t.id = tg.tag_id
WHERE tg.story_id = @story_id
ORDER BY t.is_media DESC, t.tag ASC;

-- name: CountStories :one
SELECT count(*) FROM stories WHERE deleted_at IS NULL;

-- name: RecalculateStoryScores :execrows
UPDATE stories SET
  upvotes = coalesce(v.cnt, 0)::int,
  downvotes = coalesce(hf.cnt, 0)::int
FROM stories s2
LEFT JOIN (SELECT story_id, count(*) AS cnt FROM votes GROUP BY story_id) v ON v.story_id = s2.id
LEFT JOIN (
    SELECT hs.story_id, count(*) AS cnt
    FROM hidden_stories hs
    JOIN story_flags sf ON sf.user_id = hs.user_id AND sf.story_id = hs.story_id
    WHERE NOT EXISTS (
        SELECT 1 FROM comments c
        WHERE c.story_id = hs.story_id AND c.user_id = hs.user_id AND c.deleted_at IS NULL
    )
    GROUP BY hs.story_id
) hf ON hf.story_id = s2.id
WHERE stories.id = s2.id;

-- name: UpdateStoryTitle :exec
UPDATE stories SET title = @title, updated_at = now() WHERE id = @id;

-- name: UpdateStoryBody :exec
UPDATE stories SET body = @body, updated_at = now() WHERE id = @id;

-- name: UpdateStoryURL :exec
UPDATE stories SET url = @url, normalized_url = @normalized_url, domain_id = @domain_id, origin_id = @origin_id, updated_at = now() WHERE id = @id;

-- name: SetStoryUpvotes :exec
UPDATE stories SET upvotes = @upvotes WHERE id = @id;

-- name: DeleteTaggingsByStory :exec
DELETE FROM taggings WHERE story_id = @story_id;

-- name: SoftDeleteStory :exec
UPDATE stories SET deleted_at = now(), updated_at = now() WHERE id = @id;

-- name: GetTagsByNames :many
SELECT id, tag, description, category_id, privileged, is_media, active, hotness_mod, created_at, updated_at
FROM tags
WHERE lower(tag) = ANY(@names::text[])
  AND active = true;
