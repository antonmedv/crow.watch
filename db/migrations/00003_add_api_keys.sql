-- +goose Up
CREATE TABLE api_keys (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL,
    name TEXT NOT NULL DEFAULT '',
    last_used_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX api_keys_token_hash_unique ON api_keys (token_hash);
CREATE INDEX api_keys_user_id_idx ON api_keys (user_id);

-- +goose Down
DROP TABLE api_keys;
