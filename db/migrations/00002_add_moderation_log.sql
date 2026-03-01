-- +goose Up
CREATE TABLE moderation_log (
    id BIGSERIAL PRIMARY KEY,
    moderator_id BIGINT NOT NULL REFERENCES users(id),
    action TEXT NOT NULL,
    target_type TEXT NOT NULL,
    target_id BIGINT NOT NULL,
    reason TEXT NOT NULL DEFAULT '',
    metadata JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX moderation_log_created_at_idx ON moderation_log (created_at DESC);
CREATE INDEX moderation_log_target_idx ON moderation_log (target_type, target_id);

-- +goose Down
DROP TABLE IF EXISTS moderation_log;
