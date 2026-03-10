-- +goose Up
CREATE TABLE page_views (
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    path       TEXT NOT NULL,
    visitor_id TEXT NOT NULL,
    referrer   TEXT NOT NULL DEFAULT '',
    device     TEXT NOT NULL DEFAULT '',
    browser    TEXT NOT NULL DEFAULT '',
    os         TEXT NOT NULL DEFAULT '',
    is_bot     BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_page_views_created_at ON page_views (created_at);
CREATE INDEX idx_page_views_path_created ON page_views (path, created_at);

CREATE TABLE daily_stats (
    date     DATE NOT NULL,
    path     TEXT NOT NULL,
    views    INTEGER NOT NULL DEFAULT 0,
    visitors INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (date, path)
);

CREATE TABLE daily_referrers (
    date            DATE NOT NULL,
    referrer_domain TEXT NOT NULL,
    path            TEXT NOT NULL,
    hits            INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (date, referrer_domain, path)
);

-- +goose Down
DROP TABLE IF EXISTS daily_referrers;
DROP TABLE IF EXISTS daily_stats;
DROP TABLE IF EXISTS page_views;
