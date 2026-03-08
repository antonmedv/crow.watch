-- +goose Up
ALTER TABLE stories ADD COLUMN duplicate_of_id BIGINT REFERENCES stories(id);
CREATE INDEX stories_duplicate_of_id_idx ON stories (duplicate_of_id) WHERE duplicate_of_id IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS stories_duplicate_of_id_idx;
ALTER TABLE stories DROP COLUMN IF EXISTS duplicate_of_id;
