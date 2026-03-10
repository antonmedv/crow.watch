-- +goose Up
CREATE TABLE daily_user_stats (
    date           DATE PRIMARY KEY,
    active_users   INTEGER NOT NULL DEFAULT 0,
    new_users      INTEGER NOT NULL DEFAULT 0,
    new_stories    INTEGER NOT NULL DEFAULT 0,
    new_comments   INTEGER NOT NULL DEFAULT 0
);

-- +goose Down
DROP TABLE IF EXISTS daily_user_stats;
