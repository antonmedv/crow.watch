-- +goose Up
-- Recreate daily_referrers: replace destination path with source referrer_url.
-- Old aggregated data is lost but will be re-aggregated from page_views.
DROP TABLE daily_referrers;
CREATE TABLE daily_referrers (
    date            DATE NOT NULL,
    referrer_domain TEXT NOT NULL,
    referrer_url    TEXT NOT NULL DEFAULT '',
    hits            INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (date, referrer_domain, referrer_url)
);

-- +goose Down
DROP TABLE daily_referrers;
CREATE TABLE daily_referrers (
    date            DATE NOT NULL,
    referrer_domain TEXT NOT NULL,
    path            TEXT NOT NULL,
    hits            INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (date, referrer_domain, path)
);
