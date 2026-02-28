-- name: CreateCampaign :one
INSERT INTO campaigns (slug, welcome_message, sponsor_id, created_by_id)
VALUES (@slug, @welcome_message, @sponsor_id, @created_by_id)
RETURNING *;

-- name: GetActiveCampaignBySlug :one
SELECT
    c.*,
    s.username AS sponsor_name
FROM campaigns c
JOIN users s ON s.id = c.sponsor_id
WHERE lower(c.slug) = lower(@slug)
  AND c.active = true
LIMIT 1;

-- name: ListCampaigns :many
SELECT
    c.*,
    s.username AS sponsor_name,
    cb.username AS created_by_name,
    (SELECT count(*) FROM users WHERE campaign = c.slug)::bigint AS registered_count
FROM campaigns c
JOIN users s ON s.id = c.sponsor_id
JOIN users cb ON cb.id = c.created_by_id
ORDER BY c.created_at DESC;

-- name: SetCampaignActive :exec
UPDATE campaigns
SET active = @active, updated_at = now()
WHERE id = @id;
