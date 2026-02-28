-- name: ListActiveTags :many
SELECT id, tag, description, category_id, privileged, is_media, active, hotness_mod, created_at, updated_at
FROM tags
WHERE active = true
ORDER BY is_media DESC, tag ASC;

-- name: ListActiveTagsWithCategory :many
SELECT t.id, t.tag, t.description, t.category_id, t.privileged, t.is_media,
       COALESCE(c.name, '') AS category_name
FROM tags t
LEFT JOIN categories c ON c.id = t.category_id
WHERE t.active = true
ORDER BY category_name, t.tag;

-- name: GetTagsByIDs :many
SELECT id, tag, description, category_id, privileged, is_media, active, hotness_mod, created_at, updated_at
FROM tags
WHERE id = ANY(@ids::bigint[])
  AND active = true;

-- name: GetCategoryByName :one
SELECT id, name, created_at, updated_at
FROM categories
WHERE lower(name) = lower(@name)
LIMIT 1;

-- name: CreateCategory :one
INSERT INTO categories (name)
VALUES (@name)
RETURNING id, name, created_at, updated_at;

-- name: UpsertTag :exec
INSERT INTO tags (tag, description, category_id, privileged, is_media)
VALUES (@tag, @description, @category_id, @privileged, @is_media)
ON CONFLICT ((lower(tag)))
DO UPDATE SET
  description = EXCLUDED.description,
  category_id = EXCLUDED.category_id,
  privileged = EXCLUDED.privileged,
  is_media = EXCLUDED.is_media,
  active = true,
  updated_at = now();

-- name: GetTagByName :one
SELECT id, tag, description, category_id, privileged, is_media, active, hotness_mod, created_at, updated_at
FROM tags
WHERE lower(tag) = lower(@tag)
  AND active = true
LIMIT 1;
