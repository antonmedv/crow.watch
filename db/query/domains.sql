-- name: GetOrCreateDomain :one
INSERT INTO domains (domain)
VALUES (@domain)
ON CONFLICT ((lower(domain))) DO UPDATE SET domain = domains.domain
RETURNING id, domain, banned, ban_reason, story_count, created_at, updated_at;

-- name: IncrementDomainStoryCount :exec
UPDATE domains
SET story_count = story_count + 1, updated_at = now()
WHERE id = @id;
