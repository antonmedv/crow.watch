-- name: GetDomainByName :one
SELECT id, domain, banned, ban_reason, story_count, created_at, updated_at
FROM domains
WHERE lower(domain) = lower(@domain)
LIMIT 1;

-- name: CreateDomain :one
INSERT INTO domains (domain)
VALUES (@domain)
RETURNING id, domain, banned, ban_reason, story_count, created_at, updated_at;

-- name: IncrementDomainStoryCount :exec
UPDATE domains
SET story_count = story_count + 1, updated_at = now()
WHERE id = @id;
