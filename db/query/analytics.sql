-- name: InsertPageView :exec
INSERT INTO page_views (path, visitor_id, referrer, device, browser, os, is_bot)
VALUES ($1, $2, $3, $4, $5, $6, $7);

-- name: GetLiveStats :one
SELECT
    COUNT(*)::int AS views,
    COUNT(DISTINCT visitor_id)::int AS visitors
FROM page_views
WHERE created_at >= @since::timestamptz
  AND NOT is_bot;

-- name: GetLiveTopPages :many
SELECT
    path,
    COUNT(*)::int AS views,
    COUNT(DISTINCT visitor_id)::int AS visitors
FROM page_views
WHERE created_at >= @since::timestamptz
  AND NOT is_bot
GROUP BY path
ORDER BY views DESC
LIMIT @max_results::int;

-- name: GetLiveTopReferrers :many
SELECT
    split_part(referrer, '/', 1) AS referrer_domain,
    COUNT(*)::int AS hits
FROM page_views
WHERE created_at >= @since::timestamptz
  AND NOT is_bot
  AND referrer != ''
GROUP BY split_part(referrer, '/', 1)
ORDER BY hits DESC
LIMIT @max_results::int;

-- name: GetLiveDeviceBreakdown :many
SELECT
    device,
    COUNT(*)::int AS count
FROM page_views
WHERE created_at >= @since::timestamptz
  AND NOT is_bot
  AND device != ''
GROUP BY device
ORDER BY count DESC;

-- name: GetLiveBrowserBreakdown :many
SELECT
    browser,
    COUNT(*)::int AS count
FROM page_views
WHERE created_at >= @since::timestamptz
  AND NOT is_bot
  AND browser != ''
GROUP BY browser
ORDER BY count DESC;

-- name: GetDailyStatsRange :many
SELECT
    date,
    SUM(views)::int AS views,
    SUM(visitors)::int AS visitors
FROM daily_stats
WHERE date >= @start_date::date AND date <= @end_date::date
GROUP BY date
ORDER BY date;

-- name: GetDailyStatsTotals :one
SELECT
    COALESCE(SUM(views), 0)::int AS views,
    COALESCE(SUM(visitors), 0)::int AS visitors
FROM daily_stats
WHERE date >= @start_date::date AND date <= @end_date::date;

-- name: GetTopPagesRange :many
SELECT
    path,
    SUM(views)::int AS views,
    SUM(visitors)::int AS visitors
FROM daily_stats
WHERE date >= @start_date::date AND date <= @end_date::date
GROUP BY path
ORDER BY views DESC
LIMIT @max_results::int;

-- name: GetTopReferrersRange :many
SELECT
    referrer_domain,
    SUM(hits)::int AS hits
FROM daily_referrers
WHERE date >= @start_date::date AND date <= @end_date::date
GROUP BY referrer_domain
ORDER BY hits DESC
LIMIT @max_results::int;

-- name: GetLiveReferrerURLs :many
SELECT
    split_part(referrer, '/', 1) AS referrer_domain,
    referrer AS referrer_url,
    COUNT(*)::int AS hits
FROM page_views
WHERE created_at >= @since::timestamptz
  AND NOT is_bot
  AND referrer LIKE '%/%'
GROUP BY referrer
ORDER BY split_part(referrer, '/', 1), hits DESC;

-- name: GetReferrerURLsRange :many
SELECT
    referrer_domain,
    referrer_url,
    SUM(hits)::int AS hits
FROM daily_referrers
WHERE date >= @start_date::date AND date <= @end_date::date
  AND referrer_url != referrer_domain
GROUP BY referrer_domain, referrer_url
ORDER BY referrer_domain, hits DESC;

-- name: AggregatePageViews :exec
INSERT INTO daily_stats (date, path, views, visitors)
SELECT
    @target_date::date,
    path,
    COUNT(*)::int,
    COUNT(DISTINCT visitor_id)::int
FROM page_views
WHERE created_at >= @day_start::timestamptz AND created_at < @day_end::timestamptz
  AND NOT is_bot
GROUP BY path
ON CONFLICT (date, path) DO UPDATE
SET views = EXCLUDED.views, visitors = EXCLUDED.visitors;

-- name: AggregateReferrers :exec
INSERT INTO daily_referrers (date, referrer_domain, referrer_url, hits)
SELECT
    @target_date::date,
    split_part(referrer, '/', 1),
    referrer,
    COUNT(*)::int
FROM page_views
WHERE created_at >= @day_start::timestamptz AND created_at < @day_end::timestamptz
  AND NOT is_bot
  AND referrer != ''
GROUP BY referrer
ON CONFLICT (date, referrer_domain, referrer_url) DO UPDATE
SET hits = EXCLUDED.hits;

-- name: PurgePageViews :execrows
DELETE FROM page_views
WHERE created_at < @before::timestamptz;

-- name: AggregateUserStats :exec
INSERT INTO daily_user_stats (date, active_users, new_users, new_stories, new_comments)
VALUES (
    @target_date::date,
    (SELECT COUNT(DISTINCT user_id)::int FROM sessions WHERE last_seen_at >= @day_start::timestamptz AND last_seen_at < @day_end::timestamptz),
    (SELECT COUNT(*)::int FROM users WHERE created_at >= @day_start::timestamptz AND created_at < @day_end::timestamptz AND deleted_at IS NULL),
    (SELECT COUNT(*)::int FROM stories WHERE created_at >= @day_start::timestamptz AND created_at < @day_end::timestamptz AND deleted_at IS NULL),
    (SELECT COUNT(*)::int FROM comments WHERE created_at >= @day_start::timestamptz AND created_at < @day_end::timestamptz AND deleted_at IS NULL)
)
ON CONFLICT (date) DO UPDATE
SET active_users = EXCLUDED.active_users,
    new_users = EXCLUDED.new_users,
    new_stories = EXCLUDED.new_stories,
    new_comments = EXCLUDED.new_comments;

-- name: GetDailyUserStatsRange :many
SELECT date, active_users, new_users, new_stories, new_comments
FROM daily_user_stats
WHERE date >= @start_date::date AND date <= @end_date::date
ORDER BY date;

-- name: GetUserActivityStats :one
SELECT
    (SELECT COUNT(DISTINCT user_id)::int FROM sessions WHERE last_seen_at >= @since::timestamptz) AS active_users,
    (SELECT COUNT(*)::int FROM users WHERE created_at >= @since::timestamptz AND deleted_at IS NULL) AS new_users,
    (SELECT COUNT(*)::int FROM stories WHERE created_at >= @since::timestamptz AND deleted_at IS NULL) AS new_stories,
    (SELECT COUNT(*)::int FROM comments WHERE created_at >= @since::timestamptz AND deleted_at IS NULL) AS new_comments;

-- name: GetTopContributors :many
SELECT u.username, COUNT(*)::int AS stories
FROM stories s
JOIN users u ON u.id = s.user_id
WHERE s.created_at >= @since::timestamptz AND s.deleted_at IS NULL
GROUP BY u.username
ORDER BY stories DESC
LIMIT @max_results::int;

-- name: GetTopCommenters :many
SELECT u.username, COUNT(*)::int AS comments
FROM comments c
JOIN users u ON u.id = c.user_id
WHERE c.created_at >= @since::timestamptz AND c.deleted_at IS NULL
GROUP BY u.username
ORDER BY comments DESC
LIMIT @max_results::int;
