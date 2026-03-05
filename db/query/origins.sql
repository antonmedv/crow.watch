-- name: GetOrCreateOrigin :one
INSERT INTO origins (domain_id, origin)
VALUES (@domain_id, @origin)
ON CONFLICT ((lower(origin))) DO UPDATE SET origin = origins.origin
RETURNING id, domain_id, origin, banned, ban_reason, story_count, created_at, updated_at;

-- name: IncrementOriginStoryCount :exec
UPDATE origins
SET story_count = story_count + 1, updated_at = now()
WHERE id = @id;
