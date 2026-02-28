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

-- name: ListRecentStories :many
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
    u.username,
    d.domain,
    o.origin
FROM stories AS s
JOIN users AS u ON u.id = s.user_id
LEFT JOIN domains AS d ON d.id = s.domain_id
LEFT JOIN origins AS o ON o.id = s.origin_id
WHERE s.deleted_at IS NULL
  AND s.id NOT IN (
    SELECT tg.story_id FROM taggings AS tg
    WHERE tg.tag_id = ANY(@hidden_tag_ids::bigint[])
  )
ORDER BY s.created_at DESC
LIMIT @story_limit;

-- name: ListRecentStoriesByTag :many
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
    u.username,
    d.domain,
    o.origin
FROM stories AS s
         JOIN users AS u ON u.id = s.user_id
         LEFT JOIN domains AS d ON d.id = s.domain_id
         LEFT JOIN origins AS o ON o.id = s.origin_id
         JOIN taggings AS tg ON tg.story_id = s.id
WHERE tg.tag_id = @tag_id
  AND s.deleted_at IS NULL
ORDER BY s.created_at DESC
LIMIT @story_limit;

-- name: GetStoryByID :one
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
    u.username,
    d.domain,
    o.origin
FROM stories AS s
JOIN users AS u ON u.id = s.user_id
LEFT JOIN domains AS d ON d.id = s.domain_id
LEFT JOIN origins AS o ON o.id = s.origin_id
WHERE s.id = @id AND s.deleted_at IS NULL;

-- name: GetStoryByShortCode :one
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
    u.username,
    d.domain,
    o.origin
FROM stories AS s
JOIN users AS u ON u.id = s.user_id
LEFT JOIN domains AS d ON d.id = s.domain_id
LEFT JOIN origins AS o ON o.id = s.origin_id
WHERE s.short_code = @short_code AND s.deleted_at IS NULL;

-- name: GetStoryTags :many
SELECT t.id, t.tag, t.is_media
FROM taggings AS tg
JOIN tags AS t ON t.id = tg.tag_id
WHERE tg.story_id = @story_id
ORDER BY t.is_media DESC, t.tag ASC;

-- name: GetStoryTagsWithMod :many
SELECT t.id, t.tag, t.is_media, t.hotness_mod
FROM taggings AS tg
JOIN tags AS t ON t.id = tg.tag_id
WHERE tg.story_id = @story_id
ORDER BY t.is_media DESC, t.tag ASC;

-- name: ListStoriesByUsername :many
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
    u.username,
    d.domain,
    o.origin
FROM stories AS s
JOIN users AS u ON u.id = s.user_id
LEFT JOIN domains AS d ON d.id = s.domain_id
LEFT JOIN origins AS o ON o.id = s.origin_id
WHERE lower(u.username) = lower(@username)
  AND s.deleted_at IS NULL
ORDER BY s.created_at DESC
LIMIT @story_limit;

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
