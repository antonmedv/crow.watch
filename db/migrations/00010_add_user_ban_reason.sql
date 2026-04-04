-- +goose Up
ALTER TABLE users ADD COLUMN ban_reason TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE users DROP COLUMN ban_reason;
