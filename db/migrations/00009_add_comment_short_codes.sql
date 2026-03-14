-- +goose Up
ALTER TABLE comments ADD COLUMN short_code CHAR(6);

-- Backfill existing comments with random short codes
UPDATE comments SET short_code = substr(md5(random()::text || id::text), 1, 6)
WHERE short_code IS NULL;

ALTER TABLE comments ALTER COLUMN short_code SET NOT NULL;
CREATE UNIQUE INDEX comments_short_code_unique ON comments (short_code);

-- +goose Down
DROP INDEX IF EXISTS comments_short_code_unique;
ALTER TABLE comments DROP COLUMN short_code;
