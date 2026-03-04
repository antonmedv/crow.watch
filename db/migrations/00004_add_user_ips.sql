-- +goose Up
CREATE TABLE user_ips (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id),
    ip_address TEXT NOT NULL,
    action TEXT NOT NULL,
    first_seen_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    hit_count INT NOT NULL DEFAULT 1,
    UNIQUE(user_id, ip_address, action)
);
CREATE INDEX user_ips_ip_address_idx ON user_ips(ip_address);
CREATE INDEX user_ips_user_id_idx ON user_ips(user_id);

-- +goose Down
DROP TABLE user_ips;
